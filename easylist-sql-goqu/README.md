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

## Query shape and semantics

The driver calls [`sqlpg.Lower`](../easylist-sqlpg) and maps the lowered query onto goqu, so its query shape, PostgreSQL semantics, and error texts are the ones every easylist PostgreSQL driver shares. They are described in easylist-sqlpg's README:

- [Query shape](../easylist-sqlpg/README.md#query-shape): selected fields, `FROM` and joins, the scope, pagination, and placeholder order.
- [PostgreSQL semantics](../easylist-sqlpg/README.md#postgresql-semantics): `!=` and `NULL`, `IN` and `NOT IN`, text `CONTAINS` and `EXCLUDES`, numeric, date, and timestamp binding, expression values and whole-day ranges, and array fields.

Operators and literal checks per type are listed in [easylist-core's README](../easylist-core/README.md#types). Errors name the field and include the 1-based byte position in the filter text.

## How the driver uses goqu

- goqu assembles `SELECT`, `FROM`, `WHERE`, and `ORDER BY` with the `postgres` dialect and `Prepared(true)`, which numbers the placeholders `$1`, `$2`, ...
- `FROM`, each selected expression, and each `ORDER BY` expression are `goqu.L(sql)` literals without arguments, which goqu writes verbatim. Columns are aliased with `goqu.C(name)`, which quotes the field name.
- `And` and `Or` conditions map to `goqu.And` and `goqu.Or`. Every other condition is rendered through its `Fragment()` as one `goqu.L(template, slots...)`: author SQL slots are nested `goqu.L` literals without arguments, so a `?` inside them is never treated as a placeholder, and value slots are bound. The driver uses no native goqu operators, whose output differs (goqu writes `IS TRUE` for Boolean equality, for example), so the SQL is exactly the canonical fragments.
- goqu drops `OFFSET 0`, so the driver appends `LIMIT $n OFFSET $m` itself and binds both, including an offset of 0.

The SQL, arguments, and error texts are the same as before the shared rules moved to easylist-sqlpg; the golden tests in this module check them. The driver package compiles in goqu and no other query library or PostgreSQL wire driver, which `TestCompiledInPackages` checks.

## Run unit tests

From the `easylist-sql-goqu` directory, run:

```sh
go vet ./...
go test -race ./...
```

`TestConformance` runs the shared [conformance suite](../easylist-sqlpg/README.md#run-the-conformance-suite). Its live cases are skipped unless `EASYLIST_PG_DSN` names a PostgreSQL 16 database, as in:

```sh
EASYLIST_PG_DSN='postgres://postgres@127.0.0.1:54329/postgres?sslmode=disable' go test -race ./...
```

easylist-sqlpg's README describes how to [start a local PostgreSQL](../easylist-sqlpg/README.md#start-a-local-postgresql) for it.

The Go snippets in this README are kept in `readme_example_test.go`, and a test checks that they match.
