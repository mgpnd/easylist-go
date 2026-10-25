# easylist-gqlgen

`easylist-gqlgen` (package `easylistgql`) exposes [easylist-core](../easylist-core) lists through [gqlgen](https://gqlgen.com). A gqlgen plugin adds each list's GraphQL types to your schema and generates their Go bindings in the same `gqlgen generate` run. At runtime, each list query and export mutation resolver is a single call, and SQL covers only what the operation selected.

It pins gqlgen v0.17.94. Use the same gqlgen version in your application.

## Generated types

For a list named `users` (prefix `Users`; `order_items` becomes `OrderItems`) the plugin declares:

- `UsersRow` with one field per SQL and calculated field, non-null unless the field is nullable, and `UsersPage { items: [UsersRow!]!, totalCount: Int! }`.
- An enum per enum field, such as `UsersStatus { ACTIVE ON_HOLD }`.
- `UsersSortField` (SQL fields only), `input UsersSort { field: UsersSortField!, direction: SortDirection! = ASC }`, and `input UsersListInput { filter: String, sort: [UsersSort!], limit: Int, offset: Int }`.
- `UsersExportField` (SQL and calculated fields) and `input UsersExportInput { fields: [UsersExportField!]!, filter: String, sort: [UsersSort!], format: ExportFormat! }`.

Once for all lists it declares `SortDirection { ASC DESC }`, `ExportFormat { CSV XLSX }`, `ExportJob { id: ID! }`, and the scalars the lists use:

| Field type | GraphQL type |
| --- | --- |
| text | `String` |
| integer | `Int64` |
| float | `Float` |
| numeric | `Decimal` (a string, so no digits are lost) |
| boolean | `Boolean` |
| date | `Date` (`YYYY-MM-DD`) |
| timestamp | `Time` (RFC 3339) |
| uuid | `UUID` |
| enum | `<Prefix><Field>`, values in upper snake case |

The plugin writes this SDL to `easylist/*.graphqls` next to `gqlgen.yml` so gqlgen can embed it. Commit those files with the rest of the generated code. If your schema already declares a type the plugin would declare, generation fails with an error naming it, for example `Cannot redeclare type UsersRow`.

## Generate

gqlgen plugins run from a custom entrypoint instead of the `gqlgen` command. Add one, for example `cmd/gqlgen/main.go`, and run it from the directory of `gqlgen.yml` with `go run`, typically through a `//go:generate` line:

```go
import (
	"fmt"
	"os"

	easylistgql "github.com/mgpnd/easylist-go/easylist-gqlgen"
	"example.com/app/lists"

	"github.com/99designs/gqlgen/api"
	"github.com/99designs/gqlgen/codegen/config"
)

func main() {
	cfg, err := config.LoadConfigFromDefaultLocations()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load config:", err)
		os.Exit(2)
	}
	plugin := easylistgql.NewPlugin(lists.Registry, easylistgql.Options{
		Registry: "example.com/app/lists.Registry",
	})
	if err := api.Generate(cfg, api.PrependPlugin(plugin)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
}
```

- `lists.Registry` is a package-level `*easylist.Registry` built from your list definitions. `Options.Registry` names that same variable, because the generated code reads it at runtime.
- Use `api.PrependPlugin`, not `api.AddPlugin`: the plugin must bind its types before gqlgen's model generator runs, or the model generator would generate them too.
- The Go bindings go to `easylistgen/easylist_gen.go` next to the gqlgen exec package unless `Options.Filename` and `Options.Package` say otherwise.

### Import cycle warning

The generated package imports your list definitions package. **The list definitions must never import gqlgen-generated packages** (the exec package, the models, or `easylistgen`), or the build fails with an import cycle. Keep definitions in their own package that depends only on `easylist-core`, so it compiles before anything is generated.

## Attach lists in your schema

The plugin adds no fields to `Query`, `Mutation`, or any other type. Attach lists wherever you want:

```graphql
extend type Query {
  people(query: UsersListInput!): UsersPage
}

extend type Mutation {
  exportPeople(input: UsersExportInput!): ExportJob!
}
```

Each resolver body is one call to a generated function, with an `easylist.Runtime` holding the driver, the database handle, and optionally an exporter:

```go
func (r queryResolver) People(ctx context.Context, query easylistgen.UsersListInput) (*easylistgen.UsersPage, error) {
	return easylistgen.ResolveUsers(ctx, r.Runtime, query)
}
```

```go
func (r mutationResolver) ExportPeople(ctx context.Context, input easylistgen.UsersExportInput) (*easylistgql.ExportJob, error) {
	return easylistgen.ExportUsers(ctx, r.Runtime, input)
}
```

`Resolve<P>` reads the operation's selection: only fields under `items` are queried (through fragments, inline fragments, aliases, and `@skip`/`@include`), the page query runs only when `items` is selected, and the count query only when `totalCount` is. Filter syntax and planning errors become GraphQL errors on the list field, with their positions, and no query runs. Calculated fields are methods on `<P>Row` that gqlgen calls only when selected; an error from one becomes an error on that field of that row. Because row fields are non-null unless declared nullable, such an error nulls the nearest nullable parent, as GraphQL requires; declare the list field itself nullable, as above, to keep the error from nulling the whole response.

`Export<P>` validates the input with the same rules as the export worker, passes the `easylist.ExportRequest` to `Runtime.Exporter`, and returns `ExportJob` with the exporter's job ID. With no exporter configured it returns `ErrExportsNotConfigured`.

## Scalars

`Int64`, `Decimal`, `Date`, and `UUID` are implemented in this package; `Time` uses gqlgen's built-in marshaler. If your schema already declares a scalar with one of these names, rename the plugin's:

```go
var renamed = easylistgql.Options{
	Registry: "example.com/app/lists.Registry",
	Scalars:  easylistgql.Scalars{UUID: "EasyUUID", Time: "EasyTime"},
}
```

## Run unit tests

From the `easylist-gqlgen` directory, run:

```sh
go test ./...
```

`internal/fixture` is a complete gqlgen project used by the tests. `TestFixtureGeneration` regenerates it with the entrypoint in `internal/fixture/cmd/generate` and fails if the committed output is stale; regenerate with `go generate ./internal/fixture/graph`.
