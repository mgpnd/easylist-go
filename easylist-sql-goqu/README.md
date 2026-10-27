# easylist-sql-goqu

`easylist-sql-goqu` (package `sqlgoqu`) is the PostgreSQL driver for [easylist-core](../easylist-core). It compiles an easylist query plan into one prepared PostgreSQL query with [goqu](https://github.com/doug-martin/goqu): a page query, an export query, or a count query. It generates SQL; the application's database handle runs it.

## Local development

`easylist-filter` and `easylist-core` are not published yet, so this module builds only inside the [easylist-go](https://github.com/mgpnd/easylist-go) repository, whose root `go.work` links the modules. Clone that repository and work from it.

## Use the driver

Construct the driver once and put it in an `easylist.Runtime` with your `*sql.DB`. Any PostgreSQL `database/sql` driver works, including `pgx/stdlib`:

```go
func newRuntime(db *sql.DB) *easylist.Runtime {
	return &easylist.Runtime{
		Driver: sqlgoqu.New(),
		DB:     easylist.SQLDB(db),
	}
}
```

The driver holds no state, so one value is safe to share between goroutines. To see the SQL for a plan, compile it directly:

```go
func showSQL(users *easylist.CompiledList) (string, []any, error) {
	expr, err := filter.Parse("name = 'Ann' AND age < 30")
	if err != nil {
		return "", nil, err
	}
	limit := 10
	plan, err := users.Plan(easylist.KindPage, easylist.Request{
		Fields: []string{"name", "country"},
		Filter: expr,
		Limit:  &limit,
	})
	if err != nil {
		return "", nil, err
	}
	return sqlgoqu.New().Compile(plan)
}
```

For a `users` list with alias `u` and a `country` field read as `c.name` through a `LEFT JOIN countries c ...` join, this returns:

```sql
SELECT u."id" AS "id", u."name" AS "name", c.name AS "country"
FROM users AS u LEFT JOIN countries c ON c.id = u.country_id
WHERE (u."name" = $1 AND u."age" < $2)
ORDER BY u."id" ASC LIMIT $3 OFFSET $4
```

with arguments `["Ann", 30, 10, 0]`.

## Query shape

- Each selected field is returned under its field name. A field without SQL reads `alias."name"`; a field with SQL emits it verbatim (and in parentheses inside `WHERE`).
- `FROM` is the table, `AS`, the alias, then the planned joins in order, all verbatim.
- The list's scope is wrapped in parentheses and ANDed with the filter, so an `OR` in the filter cannot escape it.
- Page queries always bind `LIMIT` and `OFFSET`, after the filter arguments. Export queries have neither. Count queries are `SELECT COUNT(*)` with only the joins the filter needs, and no ordering or pagination.
- Placeholders are `$1`, `$2`, ... in the order the filter is read, left to right. Compiling the same plan twice produces the same SQL and arguments.

List-author SQL must not contain `$n` placeholders of its own, because they would collide with the bound arguments.

## PostgreSQL semantics

- `!=` compiles to `IS DISTINCT FROM`, so it matches exactly the rows `=` does not, including rows whose field is NULL. To skip NULL rows as `<>` did, add `AND x != NULL`, as in `country != 'FR' AND country != NULL`.
- `= NULL` compiles to `IS NULL` and `!= NULL` to `IS NOT NULL`, with no bound argument. Every type accepts them, whether or not the field is declared `Nullable`.
- `CONTAINS` on text compiles to `ILIKE '%…%' ESCAPE E'\\'`: case-insensitive, with `%`, `_`, and `\` in the literal matched literally.
- `EXCLUDES` on text compiles to `COALESCE(x NOT ILIKE '%…%' ESCAPE E'\\', TRUE)`, with the same escaping as `CONTAINS`. It matches rows that do not contain the literal and rows whose field is NULL, and emits the field's SQL once.
- `>`, `>=`, `<`, `<=`, `BETWEEN`, and `CONTAINS` never match a NULL field.
- Numeric literals are bound as exact decimal strings and cast with `::numeric`.
- Boolean equality compares with a bound `true` or `false` rather than emitting `IS TRUE`.
- Dates (`YYYY-MM-DD`) and timestamps (RFC 3339 with an offset) are bound as `time.Time`. UUIDs (canonical hyphenated hex) and enums (a declared value) are bound as strings; PostgreSQL infers their type from the column.
- `BETWEEN 'a,b'` is inclusive, with spaces around each bound trimmed.
- NULL ordering follows PostgreSQL defaults: last when ascending, first when descending.

Array fields:

- An array field is selected as `to_json(x)::text AS "name"`, so the runtime receives JSON array text and a NULL array stays NULL. The `::text` cast makes every `database/sql` driver return a string.
- On an array field a quoted literal is a comma-separated list: `ids CONTAINS '7, 8'` binds the integers `7` and `8`. Items are trimmed, an empty item is an error, and each item is checked like a scalar literal of the element type, so a list on an enum array accepts only declared values. An unquoted number or boolean is one value, as in `ids CONTAINS 7`.
- `CONTAINS` binds each value once and compiles to `($1 = ANY(x) AND $2 = ANY(x))`: the array must hold every value, compared exactly and case-sensitively. A NULL array never matches.
- `EXCLUDES` compiles to `COALESCE(NOT ($1 = ANY(x) OR $2 = ANY(x)), TRUE)`: the array must hold none of the values. Empty arrays and NULL arrays match.
- `= NULL` and `!= NULL` compile to `IS NULL` and `IS NOT NULL` as for scalars. An empty array is not NULL. Every other operator is rejected on arrays.
- `= ANY` lets PostgreSQL infer each argument's type from the column's element type, so the same filter works on `int4[]` and `int8[]`, `varchar[]` and `text[]`, and PostgreSQL enum arrays, with no array cast. Numeric values keep their `::numeric` cast.
- A list of N values repeats the field's SQL N times. These predicates cannot use GIN array indexes, so they scan the rows the other conditions leave.

Operators and literal checks per type are listed in [easylist-core's README](../easylist-core/README.md#types). Errors name the field and include the 1-based byte position in the filter text.

## Run unit tests

From the `easylist-sql-goqu` directory, run:

```sh
go test -race ./...
```

The Go snippets in this README are kept in `readme_example_test.go`, and a test checks that they match.
