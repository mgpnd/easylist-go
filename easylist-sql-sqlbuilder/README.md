# easylist-sql-sqlbuilder

`easylist-sql-sqlbuilder` (package `sqlsqlbuilder`) is a PostgreSQL driver for [easylist-core](../easylist-core) for applications that already use [go-sqlbuilder](https://github.com/huandu/go-sqlbuilder). It compiles an easylist query plan into one prepared PostgreSQL query with go-sqlbuilder's `PostgreSQL` flavor: a page query, an export query, or a count query. It generates SQL; the application's database handle runs it.

The driver depends on `github.com/huandu/go-sqlbuilder` only, pinned to v1.44.1. It needs no database handle and no PostgreSQL wire driver of its own.

## Why the package is named `sqlsqlbuilder`

Every easylist SQL driver is named `sql` plus the name of its library: `sqlgoqu`, `sqlbob`, `sqlent`. For go-sqlbuilder that gives the awkward `sqlsqlbuilder`. The obvious shorter name, `sqlbuilder`, is the name of go-sqlbuilder's own package, so a file that builds its own queries with go-sqlbuilder and also sets up easylist would have to rename one of the two imports. With `sqlsqlbuilder`, both keep their names, and such a file can use `sqlbuilder` for its queries next to `sqlsqlbuilder.New()` for easylist:

```go
func activeUsers() (string, []any) {
	sb := sqlbuilder.PostgreSQL.NewSelectBuilder()
	sb.Select("id", "name").From("users").Where(sb.Equal("active", true))
	return sb.Build()
}
```

## Local development

`easylist-filter`, `easylist-core`, and `easylist-sqlpg` are not published yet, so this module builds only inside the [easylist-go](https://github.com/mgpnd/easylist-go) repository, whose root `go.work` links the modules. Clone that repository and work from it.

## Use the driver

go-sqlbuilder only builds SQL, so a go-sqlbuilder application runs its queries through its own `*sql.DB`, with any PostgreSQL `database/sql` driver, such as `pgx/stdlib`. Construct the easylist driver without arguments and put it in an `easylist.Runtime` with that `*sql.DB`:

```go
func newRuntime(db *sql.DB) *easylist.Runtime {
	return &easylist.Runtime{
		Driver: sqlsqlbuilder.New(),
		DB:     easylist.SQLDB(db),
	}
}
```

Lists and your go-sqlbuilder queries then share one connection pool. The driver does not read go-sqlbuilder's `DefaultFlavor`, so it emits PostgreSQL whatever flavor the rest of your program uses.

Run compiled queries through the runtime or the `*sql.DB`, not through `sqlbuilder.Build` or `sqlbuilder.Buildf`: go-sqlbuilder reads every `$` in their format as one of its references, and the compiled SQL holds `$n` placeholders and list-author SQL that may contain a `$`, as in a `u.code !~ '^\$'` scope.

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
	return sqlsqlbuilder.New().Compile(plan)
}
```

For a `users` list with alias `u` and a `country` field read as `c.name` through a `LEFT JOIN countries c ...` join, this returns one line, shown here broken before each clause:

```sql
SELECT u."id" AS "id", u."name" AS "name", c.name AS "country"
FROM users AS u LEFT JOIN countries c ON c.id = u.country_id
WHERE (u."name" = $1 AND u."age" < $2)
ORDER BY u."id" ASC LIMIT $3 OFFSET $4
```

with arguments `["Ann", 30, 10, 0]`.

## Query shape and semantics

The driver calls [`sqlpg.Lower`](../easylist-sqlpg) and maps the lowered query onto go-sqlbuilder, so its query shape, PostgreSQL semantics, and error texts are the ones every easylist PostgreSQL driver shares. They are described in easylist-sqlpg's README:

- [Query shape](../easylist-sqlpg/README.md#query-shape): selected fields, `FROM` and joins, the scope, pagination, and placeholder order.
- [PostgreSQL semantics](../easylist-sqlpg/README.md#postgresql-semantics): `!=` and `NULL`, `IN` and `NOT IN`, text `CONTAINS` and `EXCLUDES`, numeric, date, and timestamp binding, expression values and whole-day ranges, and array fields.

Operators and literal checks per type are listed in [easylist-core's README](../easylist-core/README.md#types). Errors name the field and include the 1-based byte position in the filter text.

## How the driver uses go-sqlbuilder

- Each `Compile` builds a new `sqlbuilder.PostgreSQL.NewSelectBuilder()`, whose `Args` hold that query's values, so concurrent compilations share no builder. go-sqlbuilder assembles `SELECT`, `FROM`, `WHERE`, and `ORDER BY` on one line and numbers the placeholders `$1`, `$2`, ... in the order they appear. The SQL has no trailing `;`.
- go-sqlbuilder reads `$0`, `$?`, `${name}`, and `$$` in every string it is given as references to its `Args`. `FROM`, each selected expression, and each `ORDER BY` entry are therefore passed through `sqlbuilder.Escape`, which doubles each `$` so that the builder writes it back as one. The PostgreSQL flavor gives `?` no meaning, so a jsonb `?` operator needs no escape. Columns are aliased with the builder's `As`, with the field name written in double quotes, since `As` writes an alias as given. A count query selects `COUNT(*)`.
- `And` and `Or` conditions map to the builder's `And` and `Or`, which parenthesize every connector, so the grouping of the filter is kept.
- An uncast comparison (`Equal`, `IsDistinctFrom`, `GreaterThan`, `GreaterEqualThan`, `LessThan`, `LessEqualThan`), `IsNull` and `IsNotNull`, `Between` without casts, and an `In` list without casts whose NULL result needs no `COALESCE` map to the builder's native methods. These write their column into the statement as is, without reading `$` in it, so they get the column unescaped. Each writes exactly the shared template's text, with the same values in the same order, the column once, and no cast; `TestNativeMatchesFragment` checks this.
- Every other condition, including any with a `::numeric` cast, `IN` and `NOT IN` wrapped in `COALESCE`, `ILIKE ... ESCAPE`, array membership, whole-day ranges, and the scope, is rendered through its `Fragment()`: each author SQL slot becomes its escaped text, each value slot becomes `sb.Var(value)`, a reference the builder binds as one `$n`, and the template's own text is escaped too.
- The builder's `Limit` and `Offset` bind an `int`, and every easylist PostgreSQL driver binds `int64`. So the driver writes `LIMIT $n OFFSET $m` itself, with both values bound through `sb.Var` as the last two arguments, including an offset of 0.
- The driver refuses a condition value that go-sqlbuilder would not bind as one plain argument, such as nil, a slice, a `sqlbuilder.Builder`, the builder's own `Raw`, `List`, `Tuple`, or `Named` values, or a `sql.NamedArg`. The lowered query never contains one.

Because the native methods write the same text as the shared templates, the driver's SQL is the same as `easylist-sql-goqu`'s. The driver package compiles in go-sqlbuilder and its dependencies (`github.com/huandu/go-clone`, `github.com/huandu/xstrings`), and no other query library or PostgreSQL wire driver, which `TestCompiledInPackages` checks.

## Run unit tests

From the `easylist-sql-sqlbuilder` directory, run:

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
