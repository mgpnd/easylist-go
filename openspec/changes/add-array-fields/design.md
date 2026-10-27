# Design

## Context

See proposal.md (Why) for the motivation, and specs/ for the behaviour. Today:

- **easylist-core** has scalar types only:
  - `Type.goType()` maps each type to one Go type.
  - `Runtime.Stream` scans each selected field into a `sql.Null*` holder chosen by type (`newHolder`, `holderValue`).
  - `checkEnumValue` checks scalar enum values.
  - `Calc`/`CalcNullable` record the function's Go result type, and `compileList` compares it with `Type.goType()`.
- **easylist-sql-goqu** selects `fieldExpression(...) AS "name"`. `buildPredicate` dispatches on `field.Type` and converts literals with `predicateArgument`. `BETWEEN` already splits a string literal on a comma and converts each bound.
- **easylist-gqlgen** maps types in two places: `graphQLType`/`scalarFor` (SDL) and `goFields` (Go type plus `Row` accessor). `<P>SortField` holds every non-calculated field.
- **easylist-export** formats cells by Go value type in `csvValue` and `xlsxEncoder.cell`.
- **null-filters-and-excludes** (applied first) adds `= NULL`/`!= NULL`, compiled to `IS NULL`/`IS NOT NULL` for every field. Arrays reuse that path unchanged.

## Goals / Non-Goals

**Goals:**
- One orthogonal `Array` flag rather than nine new types, so every type-based switch gains one branch, not nine cases.
- Keep `easylist-core` free of database-driver dependencies. Arrays reach it as JSON text that it decodes with `encoding/json`.
- Array predicates bind every value and let PostgreSQL infer each argument's type from the column, the same way scalar comparisons work today.

**Non-Goals:**
- Sorting on arrays, whole-array equality, element ordering comparisons, array length filters, and nested arrays.
- Filtering on calculated array fields; calculated fields remain unfilterable.
- A way to filter on an element that contains a comma. Commas always separate items.
- Index-assisted array filtering (GIN). See the trade-off below.

## Decisions

### `Field.Array` and `PlanField.Array`
- `Field` gains `Array bool` next to `Nullable`, and `planField` copies it into `PlanField`, so drivers learn which fields are arrays.
- `compileList` rejects an array primary key. `Field.Values` stays the declared values for enum arrays.
- `CompiledList.typeNames` treats an enum array like an enum, so it still reserves the `<P><Field>` GraphQL enum name.

**Calculated arrays.** `Calc[T]` and `CalcNullable[T]` set `Array` when `T` is a slice type. The expected Go type for an array field becomes `reflect.SliceOf(f.Type.goType())`. A mismatch fails with the existing error naming both Go types. Users write `easylist.Calc("labels", easylist.Text, func(...) ([]string, error) {...})` and need no extra option.

*Alternative:* a `WithArray()` option, as for titles. Rejected because it can disagree with `T` and would need its own error, while the slice type already says it.

### Driver-to-core transport: JSON text
- Drivers select arrays as JSON array text, or NULL.
- Core scans array fields into `sql.NullString` and decodes the text with a `json.Decoder` using `UseNumber`. Elements are converted per type:

| Element type | JSON element | Go element |
|---|---|---|
| text, uuid, enum | string | `string` |
| integer | number | `int64` via `strconv.ParseInt` |
| float | number, or the string `NaN`, `Infinity`, or `-Infinity` | `float64` |
| numeric | number, or the string `NaN` | `string`, with exact digits kept by `json.Number` |
| boolean | `true` or `false` | `bool` |
| date | `"YYYY-MM-DD"` | `time.Time` at midnight UTC |
| timestamp | ISO 8601 with offset, or without one meaning UTC | `time.Time` |

- A JSON `null` element, a wrong JSON kind, or an unparsable string fails the row with an error naming the field.
- `[]` decodes to a non-nil empty slice, so "empty" and "NULL" (absent) stay distinct.
- Enum elements go through `checkEnumValue`, which is extended to check every element of an enum slice. The same function covers calculated results.

*Alternative:* scanning PostgreSQL arrays natively (`pq.Array`, pgx types). Rejected: core may depend only on the standard library, and each `database/sql` driver represents arrays differently.

### PostgreSQL selection: `to_json(expr)::text`
- The goqu driver wraps an array field's expression in `to_json(...)::text`. `to_json` keeps NULL arrays NULL and produces the element encodings above. For example, it gives numerics as JSON numbers with every digit, and dates and timestamps as ISO strings using the session time zone for `timestamptz`.
- The `::text` cast guarantees every `database/sql` driver returns a string rather than a driver-specific JSON value.
- `ORDER BY` never sees an array field, because the planner rejects sorting on one.

### List literal conversion
For an array field, `buildPredicate` produces the list of values first:
- **String literal:** split on `,`, trim each item, and reject an empty item at the literal's position. Each item's literal kind follows the element type, as `BETWEEN` bounds already do: number for integer, float, and numeric; boolean for boolean, with `true`/`false` case-insensitive; string otherwise. The item is then converted by the existing `predicateArgument`.
- **Unquoted number or boolean literal:** a single value converted by `predicateArgument`. On a text array, it fails the same way a scalar text field does.

Splitting applies only to array fields. Scalar text `CONTAINS`/`EXCLUDES` keeps today's substring search.

### Membership SQL: one `= ANY` per value
With `col` the parenthesized field expression and one bound argument per value:

| Filter | SQL |
|---|---|
| `x CONTAINS 'a,b'` | `(? = ANY(col) AND ? = ANY(col))` |
| `x EXCLUDES 'a,b'` | `COALESCE(NOT (? = ANY(col) OR ? = ANY(col)), TRUE)` |
| `x = NULL` / `x != NULL` | `col IS NULL` / `col IS NOT NULL`, from null-filters-and-excludes |

- `? = ANY(col)` makes PostgreSQL infer the argument's type from the element type of `col`, so the same Go arguments work for `int4[]` and `int8[]`, `varchar[]` and `text[]`, `timestamp[]` and `timestamptz[]`, and PostgreSQL enum arrays. This matches how scalar `col = ?` works today.
- `COALESCE(..., TRUE)` makes a NULL array, and a comparison against an array that holds NULL elements but none of the values, count as "excludes".
- Numeric arguments keep their `?::numeric` cast.

*Alternative (agreed during exploration, now rejected):* `col @> ARRAY[?, ?]::<type>[]`, with `&&` for `EXCLUDES`.
- PostgreSQL's `@>` and `&&` need both arrays to have the same type, and the driver cannot know the column's exact array type. An enum array's PostgreSQL type name is not in the list definition, and an `integer` field may be `int4[]`, which `bigint[]` does not match.
- Casting the column instead (`col::text[]`, `col::bigint[]`) needs a per-type cast table, changes comparison semantics for some types, and loses the index anyway.
- Binding one array-valued argument needs driver-specific array encoding.
- `= ANY` is the form that is correct for every element type.

### easylist-gqlgen
- **SDL.** `graphQLType` wraps array fields as `[T!]`, and the existing nullability marker adds the outer `!`. `usedScalars` already keys on `Type`, so array element scalars are declared. `<P>SortField` and the `SortField` enum skip array fields as well as calculated ones.
- **Go code.**
  - `goFields` maps arrays to `[]T` with new `Row` accessors: `Strings`, `Int64s`, `Float64s`, `Bools`, and `Times`.
  - Enum arrays convert `[]string` to `[]<P><Field>` with a loop.
  - Nullable arrays are a plain `[]T`, not `*[]T`: a nil slice resolves to `null` for a nullable list, and core never leaves a non-nullable array absent.
  - Calculated array methods return `([]T, error)` whether or not the field is nullable.

### easylist-export
`csvValue` and `xlsxEncoder.cell` gain an array branch that builds the cell text:
- Each element is formatted with the scalar CSV rules. Dates are `YYYY-MM-DD`, timestamps RFC 3339 in UTC, floats use `formatFloat`, and numerics keep their exact text.
- Text, enum, uuid, date, and timestamp elements are wrapped in `"` with any `"` inside doubled.
- Elements are joined with `,`.
- The result goes into CSV as a normal field, which `encoding/csv` quotes again, and into XLSX as a text cell.
- An empty array yields `""`, which is the same empty cell as NULL.

### Row accessors return shared slices
`Row.Strings` and the other array accessors return the slice stored in the row, without copying, to keep streaming exports allocation-light. The doc comment says callers must not modify it.

## Risks / Trade-offs

- **[No GIN index use]** `= ANY` and `NOT (... OR ...)` are not served by GIN array indexes, so membership filters scan the rows that the other conditions leave. Correctness across element types was chosen over index use. A later change could add an optional per-field PostgreSQL array type name that enables `@>`/`&&`.
- **[Author SQL repeated per value]** A list of N values repeats the field expression N times, which is costly if the expression is a subquery. Lists are short in practice, and list authors who need it can move expensive expressions into a join.
- **[`timestamp` without a time zone]** `to_json` emits no offset for these, and core reads them as UTC, matching how drivers return scalar `timestamp` values.
- **[Non-finite and special values]** `NaN`/`Infinity` floats and `NaN` numerics arrive as JSON strings and are decoded explicitly. `infinity` dates and timestamps fail decoding with an error naming the field.
- **[Third-party drivers]** Drivers must implement JSON transport for arrays. A driver that ignores `PlanField.Array` makes core fail decoding with an error naming the field rather than return wrong values.

## Migration Plan

- Apply `null-filters-and-excludes` first; this change relies on its NULL compilation path.
- Existing lists are unaffected: `Array` defaults to false, and no generated schema changes unless a list declares array fields.
- Rollback means reverting the change. Lists that declare `Array` fields then stop compiling, which is intended.
