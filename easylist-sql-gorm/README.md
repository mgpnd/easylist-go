# easylist-sql-gorm

`easylist-sql-gorm` (package `sqlgorm`) is a PostgreSQL driver for [easylist-core](../easylist-core) for applications that already use [gorm](https://gorm.io). It compiles an easylist query plan into one prepared PostgreSQL query with gorm's statement builder: a page query, an export query, or a count query. It generates SQL; the application's database handle runs it.

The driver depends on `gorm.io/gorm` only. It needs no gorm handle, no `gorm.io/driver/postgres`, and no PostgreSQL wire driver of its own.

## Local development

`easylist-filter`, `easylist-core`, and `easylist-sqlpg` are not published yet, so this module builds only inside the [easylist-go](https://github.com/mgpnd/easylist-go) repository, whose root `go.work` links the modules. Clone that repository and work from it.

## Use the driver

Construct the driver without arguments and put it in an `easylist.Runtime` together with the `*sql.DB` under your existing gorm handle:

```go
func newRuntime(gormDB *gorm.DB) (*easylist.Runtime, error) {
	db, err := gormDB.DB()
	if err != nil {
		return nil, err
	}
	return &easylist.Runtime{
		Driver: sqlgorm.New(),
		DB:     easylist.SQLDB(db),
	}, nil
}
```

The driver never sees your gorm handle. Its naming strategy, plugins, callbacks, and clause builders do not change the compiled SQL, and the driver works the same in a program that has no gorm handle at all.

Run compiled queries through the runtime or the `*sql.DB`, not through `gormDB.Raw` or `gormDB.Exec`: gorm scans raw SQL for `?` and `@name` placeholders, and list-author SQL may contain both, as in the jsonb operator `meta ? 'vip'`.

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
	return sqlgorm.New().Compile(plan)
}
```

For a `users` list with alias `u` and a `country` field read as `c.name` through a `LEFT JOIN countries c ...` join, this returns:

```sql
SELECT u."id" AS "id", u."name" AS "name", c.name AS "country"
FROM users AS u LEFT JOIN countries c ON c.id = u.country_id
WHERE u."name" = $1 AND u."age" < $2
ORDER BY u."id" ASC LIMIT $3 OFFSET $4
```

with arguments `["Ann", 30, 10, 0]`.

## Query shape and semantics

The driver calls [`sqlpg.Lower`](../easylist-sqlpg) and maps the lowered query onto gorm, so its query shape, PostgreSQL semantics, and error texts are the ones every easylist PostgreSQL driver shares. They are described in easylist-sqlpg's README:

- [Query shape](../easylist-sqlpg/README.md#query-shape): selected fields, `FROM` and joins, the scope, pagination, and placeholder order.
- [PostgreSQL semantics](../easylist-sqlpg/README.md#postgresql-semantics): `!=` and `NULL`, `IN` and `NOT IN`, text `CONTAINS` and `EXCLUDES`, numeric, date, and timestamp binding, expression values and whole-day ranges, and array fields.

Operators and literal checks per type are listed in [easylist-core's README](../easylist-core/README.md#types). Errors name the field and include the 1-based byte position in the filter text.

## How the driver uses gorm

- The package opens one private gorm handle at start-up, with its own minimal dialector and `DryRun`. The dialector writes `$n` placeholders and `"name"` identifiers and registers gorm's default callbacks. It opens no connection.
- Each `Compile` starts a new session on that handle, so concurrent compilations share no statement. gorm's query callback assembles `SELECT`, `FROM`, `WHERE`, and `ORDER BY`; the driver declares no model, so gorm adds no columns, primary-key ordering, or soft-delete condition.
- `FROM` is the statement's `TableExpr`, and each selected expression and `ORDER BY` entry is a `clause.Expr` without vars, which gorm writes verbatim. Columns are aliased with `clause.Column`, which the dialector quotes.
- `And` and `Or` conditions map to `clause.And` and `clause.Or`. At the top of `WHERE`, gorm writes an `And` without its outer parentheses. Every other condition is rendered through its `Fragment()` as one `clause.Expr` template: author SQL slots are nested `clause.Expr` values without vars, so a `?` or `@` inside them is never treated as a placeholder, and value slots are bound. gorm expands a slice that follows `(`; no bound value is a slice, so each `IN` item is one placeholder. The driver uses no native gorm predicates, which quote their column.
- gorm drops `OFFSET 0`, so the driver appends `LIMIT $n OFFSET $m` itself and binds both, including an offset of 0.

The driver package compiles in gorm and no other query library, gorm dialector, or PostgreSQL wire driver, which `TestCompiledInPackages` checks.

## Run unit tests

From the `easylist-sql-gorm` directory, run:

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
