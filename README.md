# easylist-go

easylist lets a Go application declare a list once, such as "users with their country and last order", and then serve it with user-chosen fields, a text filter, sorting, pagination, counts, GraphQL, and CSV or XLSX exports. Each request becomes one parameterized PostgreSQL query, joins are added only when the request needs them, and calculated fields are computed in Go only when someone asks for them.

```
name CONTAINS 'an' AND status IN ('active', NULL) AND last_order >= {TODAY - 30::DAYS}
```

## Repository layout

The repository is a Go workspace (`go.work`) of independent modules. Each module has its own `go.mod`, README, and tests, so an application pulls in only the libraries it uses.

| Module | Package | What it does |
| --- | --- | --- |
| [easylist-filter](easylist-filter) | `filter` | Parses filter text into an immutable AST. No dependencies. |
| [easylist-core](easylist-core) | `easylist` | List definitions, the registry, the planner, the runtime, and export requests. Depends on `easylist-filter` and the standard library only. |
| [easylist-sqlpg](easylist-sqlpg) | `sqlpg` | Shared core of the PostgreSQL drivers: turns a plan into a typed query description, and the conformance suite every driver runs. For driver authors, not applications. |
| [easylist-sql-goqu](easylist-sql-goqu) | `sqlgoqu` | PostgreSQL driver built on [goqu](https://github.com/doug-martin/goqu). |
| [easylist-sql-gorm](easylist-sql-gorm) | `sqlgorm` | PostgreSQL driver built on [gorm](https://gorm.io). |
| [easylist-sql-ent](easylist-sql-ent) | `sqlent` | PostgreSQL driver built on [ent](https://entgo.io). |
| [easylist-sql-bob](easylist-sql-bob) | `sqlbob` | PostgreSQL driver built on [bob](https://github.com/stephenafamo/bob). |
| [easylist-sql-sqlbuilder](easylist-sql-sqlbuilder) | `sqlsqlbuilder` | PostgreSQL driver built on [go-sqlbuilder](https://github.com/huandu/go-sqlbuilder). |
| [easylist-sql-jet](easylist-sql-jet) | `sqljet` | PostgreSQL driver built on [jet](https://github.com/go-jet/jet). |
| [easylist-gqlgen](easylist-gqlgen) | `easylistgql` | [gqlgen](https://gqlgen.com) plugin that generates GraphQL types and one-call resolvers for every list. |
| [easylist-export](easylist-export) | `easylistexport` | Writes an export request to CSV or XLSX, for an application's export worker. |

Other directories:

- [`openspec/`](openspec) holds the specifications (`specs/`) and the archived change proposals and designs (`changes/archive/`) behind every feature. Look there for the reasoning behind a decision.

## Layering

Dependencies point one way, from the edges toward `easylist-filter`:

```
                       easylist-filter
                              ^
                              |
                        easylist-core  <-----------------+------------------+
                         ^         ^                     |                  |
                         |         |                     |                  |
                 easylist-sqlpg    |              easylist-gqlgen    easylist-export
                    ^              |                (+ gqlgen)       (+ excelize)
                    |              |
       easylist-sql-{goqu,gorm,ent,bob,sqlbuilder,jet}
            (each + its own query library only)
```

- **easylist-filter** knows only syntax. It has no idea which fields exist or what SQL is. Relative values such as `{TODAY + 5::DAYS}` stay unevaluated trees.
- **easylist-core** knows lists, fields, and requests, and nothing about any SQL dialect. The planner checks a request against a list, type-checks and evaluates expression values, picks the joins it needs, and produces a dialect-neutral `easylist.Plan`. The `Runtime` runs a plan through any `easylist.Driver` and any `easylist.Queryer`, such as `*sql.DB`. It never opens connections.
- **easylist-sqlpg** is PostgreSQL-specific and lives above core. `sqlpg.Lower(plan)` makes every decision that depends on the plan (types, operators, NULL handling, casts, whole-day ranges) and returns a `sqlpg.Query`.
- **Drivers** only map a `sqlpg.Query` onto their library's builder. They contain no type or NULL logic, and the conformance suite checks that all of them bind the same arguments, return the same errors, and match the same rows. Each driver package links its own query library and nothing else, which `TestCompiledInPackages` checks.
- **easylist-gqlgen** and **easylist-export** are integrations over core. They depend on no driver: the application passes a `Runtime` that already holds one.

The practical rules that follow:

- Applications import `easylist-core`, `easylist-filter`, one driver, and optionally `easylist-gqlgen` and `easylist-export`. They never import `easylist-sqlpg`.
- Keep list definitions in their own package that imports only `easylist-core`. The gqlgen-generated code imports that package, so it must not import generated code back.
- New validation rules for PostgreSQL belong in `easylist-sqlpg`, so every driver gets them at once. New list or request features belong in `easylist-core`.

## How a request flows

```
filter text --filter.Parse--> filter.Expr
                                  |
easylist.Request{Fields, Filter, Sort, Limit, Offset, Now}
                                  |
      CompiledList.Plan --> *easylist.Plan          (validated, joins chosen, expressions evaluated)
                                  |
      Driver.Compile    --> SQL + bound arguments   (sqlpg.Lower, then the query library)
                                  |
      Runtime           --> Queryer.QueryContext --> []easylist.Row
                                  |
      Row.Calc          --> calculated fields, in Go, on demand
```

`Runtime.Fetch` does the planning, compiling, and running in one call, for a page, a count, or both.

## Use easylist

The modules are not published yet. Until they are, build inside this repository, whose `go.work` links them, or add the modules to your own workspace with `go work use`.

### 1. Declare a list

A list is a Go value: a table, its fields, and the joins those fields need. Field, join, and scope SQL is trusted author SQL that drivers emit verbatim; request values are always bound.

```go
package lists

import (
	"context"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
)

var Users = easylist.List{
	Name:       "users",
	Table:      "users",
	Alias:      "u",
	PrimaryKey: "id",
	Scope:      "u.deleted_at IS NULL",
	Fields: []easylist.Field{
		{Name: "id", Type: easylist.Integer},
		{Name: "name", Type: easylist.Text, Title: "Full name"},
		{Name: "status", Type: easylist.Enum, Values: []string{"active", "blocked"}},
		{Name: "country", Type: easylist.Text, Nullable: true, SQL: "c.name", Joins: []string{"country"}},
		easylist.Calc("display", easylist.Text, func(ctx context.Context, r easylist.Row) (string, error) {
			name, _ := r.String("name")
			if country, ok := r.String("country"); ok {
				return name + " (" + country + ")", nil
			}
			return name, nil
		}, "name", "country"),
	},
	Joins: []easylist.Join{
		{Key: "country", SQL: "LEFT JOIN countries c ON c.id = u.country_id"},
	},
}

var Registry = must(easylist.NewRegistry(Users))

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
```

`NewRegistry` validates every list at startup: names, joins and their order, calculated-field requirements, the primary key, enum values, and page sizes. Field types, arrays, NULL semantics, scopes, and page limits are described in [easylist-core's README](easylist-core/README.md).

### 2. Pick a driver and build a runtime

Choose the driver for the query library your application already uses; all of them produce equivalent queries. The runtime wraps your existing `*sql.DB`. Any PostgreSQL `database/sql` driver works, including `pgx/stdlib`.

```go
rt := &easylist.Runtime{
	Driver: sqlgoqu.New(),
	DB:     easylist.SQLDB(db),
}
```

Drivers hold no state, and a runtime is safe to share between goroutines.

### 3. Fetch a page

```go
expr, err := filter.Parse("name CONTAINS 'an' AND status = 'active'")
if err != nil {
	return err // a syntax error with its byte position
}
users, _ := lists.Registry.List("users")
limit := 20
res, err := rt.Fetch(ctx, users, easylist.Request{
	Fields: []string{"name", "display"},
	Filter: expr,
	Sort:   []easylist.Sort{{Field: "name", Direction: easylist.Asc}},
	Limit:  &limit,
}, easylist.Want{Rows: true, Count: true})
if err != nil {
	return err
}
for _, row := range res.Rows {
	name, _ := row.String("name")
	display, err := row.Calc(ctx, "display")
	if err != nil {
		return err
	}
	fmt.Println(name, display)
}
fmt.Println("total:", res.Total)
```

Only the `country` join is added here, because `display` requires `country`. The count query carries only the joins the filter needs. The filter syntax, with operators, value lists, `NULL`, and relative time values, is described in [the query language reference](easylist-filter/docs/query-language.md).

### 4. Serve lists over GraphQL (optional)

[easylist-gqlgen](easylist-gqlgen) is a gqlgen plugin. It generates `UsersRow`, `UsersPage`, sort and filter inputs, and export inputs for each list, and each resolver becomes one call:

```go
func (r queryResolver) People(ctx context.Context, query easylistgen.UsersListInput) (*easylistgen.UsersPage, error) {
	return easylistgen.ResolveUsers(ctx, r.Runtime, query)
}
```

The resolver queries only the fields under `items` that the operation selected, and runs the count only when `totalCount` is selected.

### 5. Export to CSV or XLSX (optional)

An export starts as an `easylist.ExportRequest`, a JSON-serializable description that the application hands to its own queue through the `easylist.Exporter` interface on the runtime. A worker later writes the file with [easylist-export](easylist-export):

```go
err := easylistexport.Run(ctx, lists.Registry, rt, req, w)
```

`Run` revalidates the request, evaluates relative values such as `TODAY` at the moment the user asked, and streams rows in one query, so memory use does not grow with the number of rows. The application owns the queue, job status, and file storage.

## Develop

Every module is tested on its own. From the repository root, this vets and tests all of them:

```sh
go vet $(go list -m -f '{{.Dir}}/...')
go test -race $(go list -m -f '{{.Dir}}/...')
```

The root has no `go.mod`, so `go test ./...` from here matches nothing. The driver tests also run the shared conformance suite; its live cases need PostgreSQL 16 and `EASYLIST_PG_DSN`, as described in [easylist-sqlpg's README](easylist-sqlpg/README.md#start-a-local-postgresql):

```sh
EASYLIST_PG_DSN='postgres://postgres@127.0.0.1:54329/postgres?sslmode=disable' go test -race $(go list -m -f '{{.Dir}}/...')
```

Module READMEs keep their Go snippets in `readme_example_test.go`, where a test checks that they compile and match. Changes are planned as [OpenSpec](openspec) proposals before they are implemented.
