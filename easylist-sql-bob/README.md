# easylist-sql-bob

`easylist-sql-bob` (package `sqlbob`) is a PostgreSQL driver for [easylist-core](../easylist-core) for applications that already use [bob](https://github.com/stephenafamo/bob). It compiles an easylist query plan into one prepared PostgreSQL query with bob's PostgreSQL query builder (`github.com/stephenafamo/bob/dialect/psql`): a page query, an export query, or a count query. It generates SQL; the application's database handle runs it.

The driver depends on `github.com/stephenafamo/bob` only, pinned to v0.50.0. It needs no generated bob models, no bob executor, and no PostgreSQL wire driver of its own.

## Local development

`easylist-filter`, `easylist-core`, and `easylist-sqlpg` are not published yet, so this module builds only inside the [easylist-go](https://github.com/mgpnd/easylist-go) repository, whose root `go.work` links the modules. Clone that repository and work from it.

## Use the driver

A bob application usually opens its database once as a `bob.DB`, with `bob.Open("pgx", dsn)` or `bob.NewDB(sqlDB)`. Construct the easylist driver without arguments and put it in an `easylist.Runtime` together with the `*sql.DB` inside that `bob.DB`:

```go
func newRuntime(db bob.DB) *easylist.Runtime {
	return &easylist.Runtime{
		Driver: sqlbob.New(),
		DB:     easylist.SQLDB(db.DB),
	}
}
```

Lists and your bob queries then share one connection pool. The driver never sees the `bob.DB`, so your hooks and generated models do not change the compiled SQL, and the driver works the same in a program that has no generated code at all.

Run compiled queries through the runtime or the `*sql.DB`, not through `psql.RawQuery` or `psql.Raw`: bob reads every `?` in raw SQL as a placeholder, and list-author SQL may contain one, as in the jsonb operator `meta ? 'vip'`.

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
	return sqlbob.New().Compile(plan)
}
```

For a `users` list with alias `u` and a `country` field read as `c.name` through a `LEFT JOIN countries c ...` join, this returns bob's layout, one clause per line:

```sql
SELECT
u."id" AS "id", u."name" AS "name", c.name AS "country"
FROM users AS u LEFT JOIN countries c ON c.id = u.country_id
WHERE ((u."name" = $1) AND (u."age" < $2))
ORDER BY u."id" ASC
LIMIT $3
OFFSET $4
```

with arguments `["Ann", 30, 10, 0]`.

## Query shape and semantics

The driver calls [`sqlpg.Lower`](../easylist-sqlpg) and maps the lowered query onto bob, so its query shape, PostgreSQL semantics, and error texts are the ones every easylist PostgreSQL driver shares. They are described in easylist-sqlpg's README:

- [Query shape](../easylist-sqlpg/README.md#query-shape): selected fields, `FROM` and joins, the scope, pagination, and placeholder order.
- [PostgreSQL semantics](../easylist-sqlpg/README.md#postgresql-semantics): `!=` and `NULL`, `IN` and `NOT IN`, text `CONTAINS` and `EXCLUDES`, numeric, date, and timestamp binding, expression values and whole-day ranges, and array fields.

Operators and literal checks per type are listed in [easylist-core's README](../easylist-core/README.md#types). Errors name the field and include the 1-based byte position in the filter text.

## How the driver uses bob

- Each `Compile` builds a new `psql.Select` query from new mods, so concurrent compilations share no builder. bob assembles `SELECT`, `FROM`, `WHERE`, `ORDER BY`, `LIMIT`, and `OFFSET`, writes each clause on its own line, and numbers the placeholders `$1`, `$2`, ... The SQL has no trailing `;`.
- `FROM`, each selected expression, and each `ORDER BY` entry are `expr.Raw` values, which bob writes verbatim, without scanning them. Columns are aliased with bob's `As`, which writes the field name in double quotes. A count query selects `COUNT(*)`.
- `And` and `Or` conditions map to `psql.And` and `psql.Or`, which parenthesize every connector, so the grouping of the filter is kept.
- An uncast comparison (`=`, `IS DISTINCT FROM`, `>`, `>=`, `<`, `<=`), `IS NULL` and `IS NOT NULL`, `BETWEEN` without casts, and an `IN` list without casts whose NULL result needs no `COALESCE` map to bob's native operators on the `expr.Raw` column. bob writes each of them in parentheses, with the same values in the same order as the shared template, the column once, and no cast.
- Every other condition, including any with a `::numeric` cast, `IN` and `NOT IN` wrapped in `COALESCE`, `ILIKE ... ESCAPE`, array membership, whole-day ranges, and the scope, is rendered through its `Fragment()` with `psql.Raw`. The template is easylist's own text and holds no author SQL: each author SQL slot is an `expr.Raw` argument, which bob writes in place of its `?` without scanning it, and each value slot is a `psql.Arg`, bound as one `$n`.
- `LIMIT` and `OFFSET` are `psql.Arg` values, so bob binds both as the last two arguments, including an offset of 0.
- The driver refuses a condition value that bob would not bind as one plain argument, such as nil, a slice, a bob expression, or a named argument. The lowered query never contains one.

The driver package compiles in bob's core, `expr`, `clause`, `mods`, `orm`, and `dialect/psql` packages and their dependencies (`github.com/stephenafamo/scan`, `github.com/aarondl/opt`, `github.com/qdm12/reprint`), and no other query library or PostgreSQL wire driver, which `TestCompiledInPackages` checks.

## Run unit tests

From the `easylist-sql-bob` directory, run:

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
