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

- `!=` compiles to `<>`, so a row whose field is NULL never matches; `IS DISTINCT FROM` is not used.
- `CONTAINS` on text compiles to `ILIKE '%…%' ESCAPE E'\\'`: case-insensitive, with `%`, `_`, and `\` in the literal matched literally.
- Numeric literals are bound as exact decimal strings and cast with `::numeric`.
- Boolean equality compares with a bound `true` or `false` rather than emitting `IS TRUE`.
- Dates (`YYYY-MM-DD`) and timestamps (RFC 3339 with an offset) are bound as `time.Time`. UUIDs (canonical hyphenated hex) and enums (a declared value) are bound as strings; PostgreSQL infers their type from the column.
- `BETWEEN 'a,b'` is inclusive, with spaces around each bound trimmed.
- NULL ordering follows PostgreSQL defaults: last when ascending, first when descending.

Operators and literal checks per type are listed in [easylist-core's README](../easylist-core/README.md#types). Errors name the field and include the 1-based byte position in the filter text.

## Run unit tests

From the `easylist-sql-goqu` directory, run:

```sh
go test -race ./...
```

The Go snippets in this README are kept in `readme_example_test.go`, and a test checks that they match.
