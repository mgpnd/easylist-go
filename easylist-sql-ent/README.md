# easylist-sql-ent

`easylist-sql-ent` (package `sqlent`) is a PostgreSQL driver for [easylist-core](../easylist-core) for applications that already use [ent](https://entgo.io). It compiles an easylist query plan into one prepared PostgreSQL query with ent's SQL builder (`entgo.io/ent/dialect/sql`): a page query, an export query, or a count query. It generates SQL; the application's database handle runs it.

The driver imports only `entgo.io/ent/dialect` and `entgo.io/ent/dialect/sql`. It needs no ent schema, no generated code, no ent client, and no PostgreSQL wire driver of its own.

## Local development

`easylist-filter`, `easylist-core`, and `easylist-sqlpg` are not published yet, so this module builds only inside the [easylist-go](https://github.com/mgpnd/easylist-go) repository, whose root `go.work` links the modules. Clone that repository and work from it.

## Use the driver

An ent application usually opens its database once as an ent SQL driver, `drv, err := entsql.Open(dialect.Postgres, dsn)` or `entsql.OpenDB(dialect.Postgres, db)`, and hands it to its generated client with `ent.NewClient(ent.Driver(drv))`. Construct the easylist driver without arguments and put it in an `easylist.Runtime` together with the `*sql.DB` under that same ent driver:

```go
func newRuntime(drv *entsql.Driver) *easylist.Runtime {
	return &easylist.Runtime{
		Driver: sqlent.New(),
		DB:     easylist.SQLDB(drv.DB()),
	}
}
```

Lists and the generated client then share one connection pool. The driver never sees the ent driver or client, so your schema, hooks, interceptors, and privacy rules do not change the compiled SQL, and the driver works the same in a program that has no generated code at all. Your lists' table, field, and join SQL refers to the tables of your ent schema by their SQL names.

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
	return sqlent.New().Compile(plan)
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

The driver calls [`sqlpg.Lower`](../easylist-sqlpg) and maps the lowered query onto ent, so its query shape, PostgreSQL semantics, and error texts are the ones every easylist PostgreSQL driver shares. They are described in easylist-sqlpg's README:

- [Query shape](../easylist-sqlpg/README.md#query-shape): selected fields, `FROM` and joins, the scope, pagination, and placeholder order.
- [PostgreSQL semantics](../easylist-sqlpg/README.md#postgresql-semantics): `!=` and `NULL`, `IN` and `NOT IN`, text `CONTAINS` and `EXCLUDES`, numeric, date, and timestamp binding, expression values and whole-day ranges, and array fields.

Operators and literal checks per type are listed in [easylist-core's README](../easylist-core/README.md#types). Errors name the field and include the 1-based byte position in the filter text.

## How the driver uses ent

- Each `Compile` builds a new `sql.Dialect(dialect.Postgres).Select()` selector, so concurrent compilations share no builder. ent assembles `SELECT`, `FROM`, `WHERE`, and `ORDER BY` and numbers the placeholders `$1`, `$2`, ...
- `FROM`, each selected expression, and each `ORDER BY` entry are `sql.Raw` values, which ent writes verbatim. Columns are aliased with `AppendSelectExprAs`, which quotes the field name. A count query selects `sql.Count("*")`, which ent writes as `COUNT(*)`.
- `And` and `Or` conditions map to `sql.And` and `sql.Or`. ent parenthesizes a nested connector and writes the outermost one without parentheses, as the whole `WHERE` clause. Every other condition is rendered through its `Fragment()` as one `sql.P` predicate: the template text and author SQL slots are written with `Builder.WriteString`, so a `?` or `$` inside author SQL is never treated as a placeholder, and value slots go through `Builder.Arg`, which binds each as one `$n`. The driver uses no native ent predicates, which quote their column, and not `sql.Expr` or `sql.ExprP`, which leave `?` placeholders unnumbered.
- ent's `Limit` and `Offset` write their values into the SQL text, so the driver appends `LIMIT $n OFFSET $m` itself and binds both, including an offset of 0.

The driver package compiles in ent's `dialect` and `dialect/sql` packages, plus `github.com/google/uuid`, which `dialect` imports, and no other ent package, query library, or PostgreSQL wire driver, which `TestCompiledInPackages` checks.

## Run unit tests

From the `easylist-sql-ent` directory, run:

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
