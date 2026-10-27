# Tasks

`easylist-filter` needs no changes: array value lists are ordinary string literals. Apply `null-filters-and-excludes` before starting.

## 1. easylist-core

- [ ] 1.1 Add `Array bool` to `Field` and `PlanField`, copy it in `planField`, and make `Calc`/`CalcNullable` set it when `T` is a slice. In `compileList`, compare calculated results with `reflect.SliceOf(Type.goType())` for array fields and reject an array primary key. Verify with registry tests for every `core-list-definition` array scenario: each type accepted, array enum without values, calculated array accepted, mismatched calculated result, and array primary key
- [ ] 1.2 Reject sort entries on array fields in `CompiledList.order`, with an error naming the field; verify with planning tests for both `core-query-planning` array scenarios, including that `Plan.Lookup` reports `Array` for a filtered array field
- [ ] 1.3 Scan array fields into `sql.NullString` in `Runtime.Stream` and decode the JSON per element type as in design.md: `UseNumber`, NaN/Infinity strings, dates at midnight UTC, timestamps with or without an offset, a non-nil empty slice for `[]`, and an error naming the field for a NULL element or bad element. Verify with fake-`Rows` tests covering every `core-list-execution` array scenario plus each element type's decoding
- [ ] 1.4 Extend `checkEnumValue` to check every element of an enum array, for scanned rows and calculated results; verify with tests for the undeclared-element scenarios on an SQL field and a calculated field
- [ ] 1.5 Add `Row` accessors `Strings`, `Int64s`, `Float64s`, `Bools`, and `Times`, documented as returning shared slices that must not be modified; verify with accessor tests for present, absent, and wrong-type values
- [ ] 1.6 Document array fields in `easylist-core/README.md`: the `Array` flag, calculated arrays, slice representations, NULL versus empty, no NULL elements, no sorting, array filter operators and list literals, and JSON transport in the driver contract. Verify `go test ./...` passes in `easylist-core`, including README example tests, with an example declaring an array field

## 2. easylist-sql-goqu

- [ ] 2.1 Select array fields as `to_json(<expr>)::text AS "<name>"` in `Compile`; verify with golden SQL tests for a base-column array and an author-SQL array (both `sql-goqu-compilation` selection scenarios)
- [ ] 2.2 Build the value list for array predicates: split and trim string literals, reject empty items, choose each item's literal kind from the element type, and convert items with `predicateArgument`; treat an unquoted number or boolean as one value. Verify with predicate tests for every `contract-sql-driver` "Array value lists" scenario, including the undeclared enum item and the unchanged scalar text substring behaviour
- [ ] 2.3 Compile array `CONTAINS` to `(? = ANY(col) AND ...)` and array `EXCLUDES` to `COALESCE(NOT (? = ANY(col) OR ...), TRUE)`, reject every other operator with a value on arrays, and keep `= NULL`/`!= NULL` on the existing path. Verify with predicate tests for the "Array field operators" scenarios, the absence of any array type cast (enum scenario), `?::numeric` arguments for numeric arrays, and argument order `ok`, `passed`, `10`, `0` in a driver-level golden test
- [ ] 2.4 Document array selection, list literals, membership SQL, NULL handling, and the lack of GIN index use in the "PostgreSQL semantics" section of `easylist-sql-goqu/README.md`; verify `go test ./...` passes in `easylist-sql-goqu`

## 3. easylist-gqlgen

- [ ] 3.1 Map array fields to `[T!]`/`[T!]!` in `graphQLType`, and leave array fields out of `<P>SortField` in both the SDL and the generated `SortField` enum. Verify with golden SDL tests for the non-null text array, nullable enum array, uuid array with its declared scalar, and the "Array fields are not sortable" scenario
- [ ] 3.2 Generate `[]T` row fields using the new `Row` accessors, `[]<P><Field>` conversion loops for enum arrays, and `([]T, error)` methods for calculated arrays; verify the generated code compiles in unit tests for every element type
- [ ] 3.3 Add a nullable text array, a non-null enum array, and a calculated array field to the fixture lists, run `go generate ./...` in `internal/fixture/graph`, and commit the regenerated files. Verify the fixture tests return `null` for a NULL array and `[]` for an empty one, and that `go generate` is idempotent
- [ ] 3.4 Document array field types, nullability, and sort exclusion in `easylist-gqlgen/README.md` ("Generated types"); verify `go test ./...` passes in `easylist-gqlgen`

## 4. easylist-export

- [ ] 4.1 Add array cell formatting shared by CSV and XLSX: format elements with the scalar rules, quote text, enum, uuid, date, and timestamp elements with `"` doubled inside, join with `,`, and write an empty array as an empty cell. Write the result as a CSV field and as an XLSX text cell. Verify with tests for every `export-file-generation` "Array cells" scenario, including the raw CSV field `"""go"",""rust"""` and an XLSX text cell for `7,8`
- [ ] 4.2 Document the array cell format, and that empty and NULL arrays both produce an empty cell, in the "File contents" section of `easylist-export/README.md`; verify `go test ./...` passes in `easylist-export`

## 5. Workspace integration

- [ ] 5.1 Run `go vet ./...` and `go test -race ./...` in every module listed in `go.work`, and verify all pass
