# easylist-sql-jet

`easylist-sql-jet` (package `sqljet`) is a PostgreSQL driver for [easylist-core](../easylist-core) for applications that already use [jet](https://github.com/go-jet/jet). It compiles an easylist query plan into one prepared PostgreSQL query with jet's `postgres` package: a page query, an export query, or a count query. It generates SQL; the application's database handle runs it.

The driver depends on `github.com/go-jet/jet/v2` only, pinned to v2.16.1, and imports only its `postgres` package. It needs no generated code: no `jet` generator run, no table or model types, and no database handle. jet's `postgres` package imports `github.com/lib/pq` for its array literals, so lib/pq is compiled in with jet, as it is in any program that uses jet's `postgres` package. The driver itself uses no wire driver.

## Local development

`easylist-filter`, `easylist-core`, and `easylist-sqlpg` are not published yet, so this module builds only inside the [easylist-go](https://github.com/mgpnd/easylist-go) repository, whose root `go.work` links the modules. Clone that repository and work from it.

## Use the driver

A jet application runs its statements through its own `*sql.DB`, with any PostgreSQL `database/sql` driver, such as `pgx/stdlib` or `lib/pq`. Construct the easylist driver without arguments and put it in an `easylist.Runtime` with that `*sql.DB`:

```go
func newRuntime(db *sql.DB) *easylist.Runtime {
	return &easylist.Runtime{
		Driver: sqljet.New(),
		DB:     easylist.SQLDB(db),
	}
}
```

Lists and your jet statements then share one connection pool. Your lists' table, field, and join SQL refers to your tables by their SQL names; the driver never sees the code jet generated for them, so a program without generated code works the same.

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
	return sqljet.New().Compile(plan)
}
```

For a `users` list with alias `u` and a `country` field read as `c.name` through a `LEFT JOIN countries c ...` join, this returns the SQL as jet lays it out, one clause per line:

```sql
SELECT (u."id") AS "id",
     (u."name") AS "name",
     (c.name) AS "country"
FROM users AS u LEFT JOIN countries c ON c.id = u.country_id
WHERE ((u."name") = ($1)) AND ((u."age") < ($2))
ORDER BY u."id" ASC
LIMIT $3
OFFSET $4
```

with arguments `["Ann", 30, 10, 0]`. jet writes each raw expression and each bound value in parentheses, and both sides of `AND` and `OR`. The parentheses change no meaning; other easylist PostgreSQL drivers write the same query with fewer of them.

## Query shape and semantics

The driver calls [`sqlpg.Lower`](../easylist-sqlpg) and maps the lowered query onto jet, so its query shape, PostgreSQL semantics, and error texts are the ones every easylist PostgreSQL driver shares. They are described in easylist-sqlpg's README:

- [Query shape](../easylist-sqlpg/README.md#query-shape): selected fields, `FROM` and joins, the scope, pagination, and placeholder order.
- [PostgreSQL semantics](../easylist-sqlpg/README.md#postgresql-semantics): `!=` and `NULL`, `IN` and `NOT IN`, text `CONTAINS` and `EXCLUDES`, numeric, date, and timestamp binding, expression values and whole-day ranges, and array fields.

Operators and literal checks per type are listed in [easylist-core's README](../easylist-core/README.md#types). Errors name the field and include the 1-based byte position in the filter text.

## jet's casts

jet's typed literals add a cast to their placeholder in the `postgres` dialect. `postgres.String`, for example, binds its value as `$1::text`:

```go
func typedLiteral() (string, []any) {
	return postgres.SELECT(postgres.String("Ann")).Sql()
}
```

A `::text` cast on a value compared with an enum or uuid column makes PostgreSQL compare text with that type, which fails, and every easylist PostgreSQL driver binds values without casts so that PostgreSQL infers each value's type from its column. The driver therefore never uses jet's typed literals. It uses none of jet's native operators (`EQ`, `IN`, `BETWEEN`, `LIKE`, ...) either: they take typed expressions, which are typed literals in the usual jet code, and jet has none for `COALESCE(...)` around `IN`, `ILIKE ... ESCAPE`, or `= ANY(...)`. Every condition is written from the shared PostgreSQL template instead, and each value is bound through a `Raw` named argument, which jet binds as a plain `$n`. The only cast in a condition is `::numeric` after a numeric literal's placeholder, which the shared rules require.

## How the driver uses jet

- Each `Compile` builds a new `postgres.SELECT(...)` statement and serializes it with `Sql()`, so concurrent compilations share no builder. jet assembles `SELECT`, `FROM`, `WHERE`, `ORDER BY`, `LIMIT`, and `OFFSET` and numbers the placeholders `$1`, `$2`, ... The driver trims the `;` and line break jet ends the statement with, and the line break it starts with.
- jet has no construct for a verbatim `FROM` with joins, and `postgres.NewTable` quotes a name with spaces in it. So the statement reads `FROM` a table named by a sentinel, `easylist_from`, which jet writes unquoted, and the driver replaces the one occurrence of the sentinel in jet's output with the plan's `FROM` text. The sentinel gets `_x` appended until it occurs in none of the plan's author SQL: `FROM` and joins, selected expressions and field names, `ORDER BY` entries, the scope, and every filtered field. If the sentinel does not occur exactly once in jet's output, `Compile` fails with an error naming the list instead of guessing.
- Each selected expression is `postgres.Raw(sql).AS(name)`: a `Raw` without named arguments is written verbatim, and jet writes the alias in double quotes. A count query selects `Raw("COUNT(*)")`. `ORDER BY` entries are `Raw(sql).ASC()` and `Raw(sql).DESC()`.
- `And` and `Or` conditions map to jet's `AND` and `OR`, which parenthesize both sides, so the grouping of the filter is kept. Every other condition, including the scope, is one `CustomExpression` over its `Fragment()`: the template's text becomes `Token`s, each author SQL slot is a `Raw` without named arguments, and each value slot is its own `Raw("#v", RawArgs{"#v": value})`, followed by the value's `::numeric` cast if it has one, which jet binds as the next `$n`.
- jet finds a `Raw`'s named arguments by searching its text and replaces every occurrence, so names that share a prefix, such as `#1` and `#10`, would collide, and a name inside author SQL would be replaced there. Giving each value its own `Raw`, whose text is only the name, and writing author SQL only in `Raw`s without named arguments avoids both: `?`, `$`, `#`, and `:` in author SQL are written as they are, and any number of values bind in order.
- `LIMIT` and `OFFSET` are jet's `LIMIT` and `OFFSET`, which bind both as `int64` after the filter values, including an offset of 0.
- The driver refuses a condition value that would not be one plain argument, such as nil, a slice or array (`[]byte` included), a jet expression or other jet value, or a `sql.NamedArg`. The lowered query never contains one.

The driver package compiles in jet's `postgres` package and what it imports, including `github.com/google/uuid` and `github.com/lib/pq`, and no other query library or PostgreSQL wire driver, which `TestCompiledInPackages` checks.

## Run unit tests

From the `easylist-sql-jet` directory, run:

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
