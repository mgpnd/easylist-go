# Proposal

## Why

easylist-go can parse a filter and compile it into a `WHERE` clause for one table described by `CREATE TABLE` SQL, but an application still has to hand-write every list endpoint: projections, joins, sorting, pagination, counts, GraphQL types, and file exports. This change turns the two existing modules into a framework where a list is declared once in a Go file and the library produces the SQL, the GraphQL query, and the export mutation, joining only what each request actually uses.

## What Changes

- New module `easylist-core` (package `easylist`):
  - Go-value list definitions: name, base table and alias, declared primary key, static raw-SQL base scope, SQL fields (name, optional SQL expression, required joins, type, nullability, optional title), keyed joins (raw `LEFT JOIN` / `LEFT JOIN LATERAL` text plus `depends_on`), and per-row calculated fields with declared SQL-field dependencies.
  - Definition validation when a list is built (unknown or cyclic joins, unknown dependencies, invalid names, missing primary key, invalid enum values).
  - A planner that turns a request (selected fields, filter AST, sort, limit/offset) into a dialect-neutral query plan: selected expressions, transitive join closure in dependency order, scope, ordering with the primary key as default sort and final tiebreaker, and a separate count plan that carries only the joins the filter needs.
  - Execution through a minimal database interface that scans rows into typed values, and evaluates calculated fields one row at a time.
  - Field types for v1: text, integer, float, numeric, boolean, date, timestamp, uuid, enum.
  - The SQL driver interface and the export contract (`Exporter` interface and a serializable `ExportRequest`).
- `easylist-sql-goqu` becomes a driver: it compiles a query plan into a prepared PostgreSQL page query and count query against the list's field catalog.
  - **BREAKING**: removes the `CREATE TABLE` schema reader (`NewCompiler`, `Compile`, `CompileAST`, `Convert`), the `pg_query_go` dependency, and the `cmd/query2sql` CLI.
  - **BREAKING**: one-dimensional array columns and their `CONTAINS`/`EXCLUDES` semantics are not part of the v1 type set.
- New module `easylist-gqlgen`: a gqlgen plugin that injects SDL and generates Go types per list (row, page with `items` and `totalCount`, a single list input, sort and export-field enums, enum types, export input, `ExportJob`), plus runtime resolvers that collect the requested fields from the GraphQL selection. The client decides where in its schema each list query and export mutation is attached.
- New module `easylist-export`: a worker function that runs one streaming query for an `ExportRequest` and writes CSV or XLSX to an `io.Writer`. The client owns the queue, job status, and file storage.
- Documentation states that each join must yield at most one row per base row; this is the list author's responsibility and is not enforced.
- Out of scope: NULL comparisons in filters, batched calculated fields, cursor pagination, dialect portability of list definitions, tenant-style dynamic scopes.

## Capabilities

### New Capabilities
- `core-list-definition`: declaring lists in Go (table, primary key, scope, SQL fields, joins, calculated fields, types, limits) and validating them.
- `core-query-planning`: turning a list request into a page plan and a count plan, including join resolution, field validation, sorting, and limits.
- `core-list-execution`: running planned queries through the database interface, scanning typed rows, and evaluating calculated fields.
- `contract-sql-driver`: the plan structure and driver interface shared by `easylist-core` and every `easylist-sql-*` module, including per-type filter semantics.
- `contract-export-request`: the `Exporter` interface and serializable `ExportRequest` shared by `easylist-core`, `easylist-gqlgen`, and `easylist-export`.
- `sql-goqu-compilation`: compiling a query plan into prepared PostgreSQL with goqu.
- `gqlgen-schema-generation`: the gqlgen plugin that injects SDL and generates Go bindings for each list.
- `gqlgen-list-resolution`: runtime resolution of list queries and export mutations from gqlgen resolvers.
- `export-file-generation`: streaming a list request into CSV or XLSX.

### Modified Capabilities
<!-- None: openspec/specs/ has no existing capabilities. -->

## Impact

- Modules touched: `easylist-sql-goqu` (rewritten as a driver), new `easylist-core`, new `easylist-gqlgen`, new `easylist-export`. `easylist-filter` is not changed; its AST is consumed as-is.
- Root `go.work` gains the three new modules; `openspec/config.yaml` project context should describe them.
- Dependencies: `easylist-core` uses only the standard library and `easylist-filter`. `easylist-sql-goqu` drops `pg_query_go` (and its cgo build). `easylist-gqlgen` adds `github.com/99designs/gqlgen` and `github.com/vektah/gqlparser/v2`. `easylist-export` adds an XLSX streaming library (excelize).
- Existing users of `querysql.Convert`, `Compiler`, and `query2sql` must migrate to list definitions. The modules are unpublished, so no external consumers are expected.
