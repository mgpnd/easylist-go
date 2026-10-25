# easylist-core

`easylist-core` (package `easylist`) lets an application declare a list once, as a Go value, and then plan and run requests against it: selected fields, a filter from [easylist-filter](../easylist-filter), sorting, and pagination. The planner joins only what a request uses. SQL itself comes from a driver such as [easylist-sql-goqu](../easylist-sql-goqu), GraphQL from [easylist-gqlgen](../easylist-gqlgen), and file exports from [easylist-export](../easylist-export).

The module uses only the standard library and `easylist-filter`.

## Declare a list

```go
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
		{Name: "last_order", Type: easylist.Timestamp, Nullable: true, SQL: "lo.created_at", Joins: []string{"last_order"}},
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
		{Key: "last_order", SQL: `LEFT JOIN LATERAL (
			SELECT o.created_at FROM orders o WHERE o.user_id = u.id ORDER BY o.created_at DESC LIMIT 1
		) lo ON TRUE`},
	},
}
```

- **Name** matches `^[a-z][a-z0-9_]*$`; field and join keys match `^[a-z_][a-z0-9_]*$`.
- **Table** and **Alias** are separate so a field without `SQL` reads the column named after the field from the alias, here `u."name"`.
- **PrimaryKey** names a non-nullable SQL field. It is always selected and is the default sort and final tiebreaker, so pages are stable.
- **Fields** with `SQL` and `Joins` read joined data. A join is added to a query only when a selected, filtered, or sorted field needs it. `Join.DependsOn` lists joins that must come first; they are added transitively and ordered deterministically.
- **Calculated fields** (`Calc`, `CalcNullable`) run once per row in Go, only when requested. They read the SQL fields they name as requirements through `Row` accessors such as `String`, `Int64`, and `Time`, which report whether the value is present. They cannot be filtered or sorted on. The function's return type must match the declared field type.
- **Title** is the export header and defaults to the field name; `WithTitle` sets it on calculated fields.

`NewRegistry` validates every list and fails at startup on invalid names, duplicate names, unknown or cyclic joins, unknown calculated requirements, a missing or nullable primary key, invalid enum values, and inconsistent page sizes:

```go
func newRegistry() (*easylist.Registry, error) {
	return easylist.NewRegistry(Users)
}
```

## Join cardinality

Each join must return at most one row per base row. Use a `LEFT JOIN` on a unique key, or a `LEFT JOIN LATERAL` subquery with `LIMIT 1` or an aggregate. **This is not enforced**: a join that returns several rows per base row duplicates items in pages and exports and inflates counts.

## Trusted SQL

`Table`, `Alias`, field `SQL`, join `SQL`, and `Scope` are trusted author SQL that drivers emit verbatim. Never build them from user input. Request values (filter literals, limit, offset) are always bound as query arguments.

## Scope

`Scope` is an optional static SQL condition applied to every page, count, and export query of the list. Drivers wrap it in parentheses and combine it with the request filter by `AND`, so an `OR` on either side cannot widen the result. It cannot depend on the request; per-tenant conditions are out of scope.

## Limits

`DefaultPageSize` and `MaxPageSize` default to 100 and 10000. A request without a limit gets the default; a limit below 1 or above the maximum is rejected, as is a negative offset. Exports have no limit or offset.

## Types

| Type | Go value | Filter operators |
| --- | --- | --- |
| `Text` | `string` | `=`, `!=`, `CONTAINS` |
| `Integer` | `int64` | `=`, `!=`, `>`, `>=`, `<`, `<=`, `BETWEEN` |
| `Float` | `float64` | same as `Integer` |
| `Numeric` | `string` (exact decimal text) | same as `Integer` |
| `Boolean` | `bool` | `=`, `!=` |
| `Date` | `time.Time` | same as `Integer` |
| `Timestamp` | `time.Time` | same as `Integer` |
| `UUID` | `string` | `=`, `!=` |
| `Enum` | `string` (a declared value) | `=`, `!=` |

A NULL is an absent value. A NULL in a field not declared `Nullable` fails the request with an error naming the field, which usually points at a join or definition mistake. Filters cannot compare with NULL.

## Run requests

A `Runtime` combines a driver with the application's database handle. The library never opens connections; `easylist.SQLDB` adapts a `*sql.DB` (pgx users can open one with `pgx/stdlib`):

```go
func firstPage(ctx context.Context, reg *easylist.Registry, db *sql.DB, driver easylist.Driver) ([]easylist.Row, int64, error) {
	rt := &easylist.Runtime{Driver: driver, DB: easylist.SQLDB(db)}
	expr, err := filter.Parse("name CONTAINS 'an' AND status = 'active'")
	if err != nil {
		return nil, 0, err
	}
	users, _ := reg.List("users")
	limit := 20
	res, err := rt.Fetch(ctx, users, easylist.Request{
		Fields: []string{"name", "display"},
		Filter: expr,
		Sort:   []easylist.Sort{{Field: "last_order", Direction: easylist.Desc}},
		Limit:  &limit,
	}, easylist.Want{Rows: true, Count: true})
	if err != nil {
		return nil, 0, err
	}
	for _, row := range res.Rows {
		if _, err := row.Calc(ctx, "display"); err != nil {
			return nil, 0, err
		}
	}
	return res.Rows, res.Total, nil
}
```

`Fetch` runs the page query only when `Want.Rows` is set and the count query only when `Want.Count` is set. The count query carries only the joins the filter needs. Database errors are wrapped with the list name and never include argument values.

Lower-level pieces are available for other integrations: `CompiledList.Plan` builds a `Plan` of kind `KindPage`, `KindCount`, or `KindExport`; `Runtime.Stream` and `Runtime.Count` run one plan.

## Exports

`ExportRequest` is a JSON-serializable export description (list, ordered fields, filter text, sort, and `CSV` or `XLSX`). `Registry.ValidateExport` applies the same rules when an export is requested and when a worker runs it. Applications accept validated requests through the `Exporter` interface on `Runtime` and own the queue, job status, and file storage.

## Run unit tests

From the `easylist-core` directory, run:

```sh
go test ./...
```

The Go snippets in this README are kept in `readme_example_test.go`, and a test checks that they match.
