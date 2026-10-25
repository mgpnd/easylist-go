# Design

## Context

See proposal.md - Why. Today the workspace has two modules:

- `easylist-filter` produces an immutable AST (`filter.Expr`, `filter.Predicate`, `filter.Logical`) with byte positions. It stays unchanged and dependency-free.
- `easylist-sql-goqu` (package `querysql`) reads `CREATE TABLE` SQL through `pg_query_go`, classifies columns into kinds (`schema.go`), and lowers the AST into a goqu `WHERE` (`lower.go`, `predicate.go`) on `SELECT * FROM table`. The lowering, literal conversion, `ILIKE` escaping, `<>` semantics, and numeric casting are reusable; the schema reader, identifier truncation (`internal/pgident`), table-name parsing, and `cmd/query2sql` are not.

Constraints: modules build only inside the workspace (`go.work`); `easylist-filter` must not learn about schemas or SQL; dialect lock-in per application is accepted; list SQL is trusted author input.

## Goals / Non-Goals

**Goals:**
- One planning path shared by GraphQL pages, counts, and exports, so selection, join, filter, and sort rules cannot drift between them.
- `easylist-core` free of third-party dependencies, so applications that never use gqlgen or exports do not pull them in.
- Resolver bodies of one line in the application; no hand-written per-list GraphQL types.

**Non-Goals:**
- Verifying join cardinality, rewriting author SQL, or detecting dialect mismatches in author SQL.
- Runtime (non-codegen) GraphQL schemas, or GraphQL libraries other than gqlgen.
- Server-side cursors or keyset batching for exports in v1; the driver can add them later behind the same interface.
- Custom NULL ordering in sorts: the database default applies (PostgreSQL: NULLs last ascending, first descending).

## Decisions

### Module layout and dependency direction

```
 easylist-filter  <--  easylist-core  <--  easylist-sql-goqu   (goqu)
                            ^
                            +----------  easylist-gqlgen       (gqlgen, gqlparser)
                            +----------  easylist-export       (excelize)
```

New folders follow the `easylist-*` convention: `easylist-core` (package `easylist`), `easylist-gqlgen` (package `easylistgql`), `easylist-export` (package `easylistexport`). `easylist-sql-goqu` keeps its module path; its package is renamed from `querysql` to `sqlgoqu` because it no longer converts schema-plus-filter strings.

Contracts (`Plan`, `Driver`, `Exporter`, `ExportRequest`) live in `easylist-core`. Alternative: a separate contracts module. Rejected: core already has no third-party dependencies, so a second module adds wiring without reducing anyone's dependency set. Putting `Exporter` in `easylist-export` was rejected because `easylist-gqlgen` would then pull in excelize.

### List definitions are plain Go values, compiled by a registry

Authors write `easylist.List{...}` literals. `easylist.NewRegistry(lists...)` validates everything in `core-list-definition` and returns an immutable registry of compiled lists (field index, join graph with a precomputed topological order, calculated dependency sets). All later steps work on compiled lists, so invalid definitions fail at startup or at gqlgen generation time, never mid-request.

Base table and alias are separate fields (`Table: "users", Alias: "u"`) so a field without SQL can be read as `u."name"`. Scope is a plain string (static, trusted). Page limits default to 100/10000 when zero.

Calculated fields use a generic constructor so the function's Go return type is checked against the declared field type when the registry is built:

```go
easylist.Calc("display", easylist.Text, func(ctx context.Context, r easylist.Row) (string, error) {...}, "name", "country")
easylist.CalcNullable("nick", easylist.Text, func(ctx context.Context, r easylist.Row) (*string, error) {...}, "name")
```

`easylist.Row` is a read-only view over the scanned values with typed accessors (`String`, `Int64`, `Time`, ..., each with a presence flag). Calculated functions never see generated types, which keeps the import direction required by `gqlgen-schema-generation`.

Alternative: YAML definitions with a function registry. Rejected in exploration: functions would be referenced by string, and gqlgen needs a generation step anyway.

### Planner produces one plan type for three purposes

`Plan` carries `Kind` (`Page`, `Count`, `Export`), the table, alias, ordered joins, selected fields (name, SQL, type), scope, the filter AST plus a resolved field lookup, ordering, and optional limit/offset. The planner:

1. validates selection, filter fields (walking the AST iteratively, as `lower.go` does), and sort;
2. computes the needed field set per plan kind, maps fields to join keys, closes over `depends_on`, and orders joins by the registry's precomputed topological order (deterministic: ties broken by declaration order);
3. applies the default sort and the primary-key tiebreaker, and resolves limits.

The filter AST is passed through untouched. Drivers resolve each predicate's field through the plan's lookup, which gives them the SQL expression, type, and enum values. Alternative: translate the AST into a core-owned IR. Rejected: it duplicates `easylist-filter`'s tree for no gain, and positions are already on the AST.

### Runtime and database interface

```go
type Runtime struct {
    Driver   Driver
    DB       Queryer
    Exporter Exporter // optional
}
type Queryer interface {
    QueryContext(ctx context.Context, query string, args ...any) (Rows, error)
}
```

`Rows` is a minimal `Next/Scan/Err/Close` interface. `easylist.SQLDB(*sql.DB)` adapts `database/sql` (pgx users use `pgx/stdlib`). A core-owned interface keeps core free of driver packages and makes fakes trivial in tests across all modules.

Scanning: the driver selects columns in plan order, and core scans each column into a holder chosen by field type (`sql.NullString`, `sql.NullInt64`, `sql.NullFloat64`, `sql.NullBool`, `sql.NullTime`). Numeric, uuid, and enum scan as strings, which `database/sql` supports for PostgreSQL text-format values. A NULL in a non-nullable field is an error, which catches join or definition mistakes early.

### easylist-sql-goqu as a driver

The FROM clause is built as one trusted literal, `goqu.L(table + " AS " + alias + " " + join1 + " " + join2 ...)`, because goqu cannot append arbitrary join text, and join SQL is emitted verbatim by contract. Field expressions are `goqu.L(sql)`, or `goqu.L(alias + "." + quotedColumn)` for plain fields, selected `AS` the quoted field name. The scope is `goqu.L("(" + scope + ")")` ANDed with the filter expression, so `OR` in either side cannot escape.

`predicate.go` is adapted from `columnMeta` to plan fields: same operator checks and literal conversion, extended with timestamp (RFC 3339 with offset, bound as `time.Time`), uuid (canonical form, bound as string), and enum (declared value, bound as string), and with array handling removed. PostgreSQL infers parameter types for uuid and enum comparisons from the column side. Output uses `Prepared(true)` so limit and offset are bound too. Count plans use `SELECT COUNT(*)`.

Removed: `schema.go`, `identifier.go` truncation, `internal/pgident`, `Convert`/`Compiler`, `cmd/query2sql`, and the `pg_query_go` dependency. Their tests go with them; predicate and lowering tests are ported to plan-based fixtures.

### gqlgen plugin

The application's generate entrypoint runs `api.Generate(cfg, api.AddPlugin(easylistgql.NewPlugin(registry, opts)))`. The plugin implements:

- `EarlySourcesInjector`: one SDL source per list plus one shared source (`SortDirection`, `ExportFormat`, `ExportJob`, the scalars). Because the sources are merged into the application schema by gqlparser, a duplicate type fails generation with a redeclaration error naming the type, which covers the collision requirement.
- `ConfigMutator`: writes the generated Go file for all lists into `opts.Package` (default `easylistgen` next to the gqlgen exec package), then binds every injected GraphQL type to its Go type in `cfg.Models` and reloads packages. This follows gqlgen's own `modelgen` plugin, which generates models inside `MutateConfig` before binding.

Generated Go per list:
- `<P>Row` struct with typed fields for SQL fields (pointers when nullable) and an unexported `easylist.Row`. Each calculated field is a method `func (r *<P>Row) Display(ctx context.Context) (string, error)`; gqlgen calls bound methods only for selected fields, which gives per-row evaluation without client resolver stubs.
- `<P>Page`, input structs, Go string types for enums with `MarshalGQL`/`UnmarshalGQL`.
- `Resolve<P>(ctx, rt, input) (*<P>Page, error)` and `Export<P>(ctx, rt, input) (*ExportJob, error)`, thin wrappers over generic runtime functions in `easylistgql`.

Scalars `Int64`, `Decimal`, `Date`, `UUID` are implemented in `easylistgql` as gqlgen marshalers; `Time` uses gqlgen's built-in scalar. Names are configurable through `opts`.

Selection: `graphql.CollectFieldsCtx` on the page, then `graphql.CollectFields` on the `items` selection set. This is gqlgen's own fragment and `@skip`/`@include` resolution, so fragments and aliases behave as the operation specifies. `__typename` is dropped.

Alternative: emit plain SDL files plus resolver stubs for the client to implement. Rejected: more client boilerplate, and calculated fields would need hand-written resolvers.

### Export worker

`easylistexport.Run(ctx, registry, rt, req, w)` revalidates the request, plans with `Kind: Export`, compiles, runs one query, and for each row evaluates the requested calculated fields and hands the values to an encoder. CSV uses `encoding/csv` directly on `w`. XLSX uses excelize's `StreamWriter`, which keeps rows in temporary storage rather than memory and writes the final zip to `w` at the end. The mutation path and the worker share one validation function in core, so a request accepted by the mutation is checked by exactly the same rules when it runs.

## Risks / Trade-offs

- [gqlgen plugin hooks and ordering change across versions] → pin a gqlgen version in `easylist-gqlgen`, keep the plugin small, and spike `MutateConfig` generation plus binding first (task 3.1).
- [Scalar names collide with the application's existing scalars] → configurable names; gqlparser reports the duplicate at generation time.
- [A join returns several rows per base row] → documented author contract; duplicated items and inflated counts are the visible symptom. Not enforced.
- [Author SQL is spliced verbatim] → it is trusted by contract and request values are always bound; documentation states that table, alias, field SQL, joins, and scope must never be built from user input.
- [Deep offsets are slow] → the maximum page size caps work per page; exports do not use offsets.
- [Long single-query exports hold one connection and snapshot] → acceptable in v1; a cursor-based driver option can be added without API changes.
- [CSV cells that start with `=`, `+`, `-`, `@` can be interpreted as formulas by spreadsheet tools] → XLSX always writes text cells; CSV writes values unchanged and the documentation names the risk.
- [NULL from a non-nullable field fails the whole request] → intentional strictness; the error names the field so definition mistakes surface immediately.

## Migration Plan

The modules are unpublished, so there are no external consumers. Inside the repository, `querysql.Convert`, `Compiler`, and `query2sql` are deleted in the same change, and their README sections are replaced by list-based usage. `go.work` gains the three new modules. Rollback is reverting the change.

## Open Questions

- Whether CSV output should offer opt-in formula escaping. It can be added later as an encoder option without changing these specs' defaults.
