# Tasks

`easylist-filter` needs no changes; its AST is consumed as-is.

## 1. easylist-core

- [x] 1.1 Create the `easylist-core` module (package `easylist`, module path `github.com/mgpnd/easylist-go/easylist-core`, requiring only `easylist-filter`), add it to `go.work`, and verify `go build ./...` succeeds from the repository root
- [x] 1.2 Define the list model (`List`, `Field`, `Join`, field types including enum values, titles, scope string, primary key, page limits) and the `Calc`/`CalcNullable` constructors; verify with a compile-only example test that declares a list using every field type
- [x] 1.3 Implement `NewRegistry` validation for every scenario in `core-list-definition` (names, duplicates, enum values and GraphQL name collisions, primary key, unknown and cyclic joins, calculated requirements, calculated return type vs declared type, page limits); verify with table-driven tests, one case per scenario
- [x] 1.4 Precompute each compiled list's join topological order (ties by declaration order) and calculated-field dependency sets; verify with tests for transitive and diamond-shaped join graphs giving a stable order
- [x] 1.5 Define the driver contract (`Plan` with `Kind` page/count/export, selected fields, ordered joins, scope, filter AST with resolved field lookup, ordering, limit/offset, and the `Driver` interface); verify it compiles and a fake driver in core tests can read field SQL, type, and enum values through the lookup
- [x] 1.6 Implement the planner for page, count, and export plans per `core-query-planning` (selection with primary key and calculated requirements, join closure per plan kind, filter field validation with positions, sort validation, default sort and tiebreaker, limit/offset rules); verify with table-driven tests covering each scenario
- [x] 1.7 Implement `Row`, typed accessors with presence flags, and the `Runtime`, `Queryer`, `Rows` interfaces plus the `SQLDB(*sql.DB)` adapter; verify with unit tests using a fake `Rows`
- [x] 1.8 Implement page execution (typed scanning per field type, NULL in non-nullable field error, page/count queries run only when requested, error wrapping with the list name) and per-row calculated evaluation; verify with tests against a fake `Queryer` covering each `core-list-execution` scenario
- [x] 1.9 Define `ExportRequest` (JSON-serializable), the `Exporter` interface, and the shared export validation function per `contract-export-request`; verify with a JSON round-trip test and validation tests for unknown, duplicate, empty, and calculated fields
- [x] 1.10 Write `easylist-core/README.md` covering list definitions, the join cardinality contract (at most one row per base row, not enforced), trusted author SQL, scope, limits, types, and runtime wiring; verify every Go snippet in it compiles via an example test

## 2. easylist-sql-goqu

- [x] 2.1 Remove `schema.go`, `identifier.go` truncation, `internal/pgident`, `Convert`/`Compiler`, `cmd/query2sql`, their tests, and the `pg_query_go` requirement; rename the package to `sqlgoqu`, require `easylist-core`, and verify `go mod tidy` drops `pg_query_go` and `go build ./...` succeeds
- [x] 2.2 Adapt predicate building and lowering to plan fields (expression from field SQL or `alias."column"`, type from the field) and add timestamp, uuid, and enum literal handling while dropping arrays; verify with ported predicate tests plus new cases for every `contract-sql-driver` operator and literal scenario
- [x] 2.3 Implement page and export query compilation (trusted FROM literal with ordered joins, aliased selected fields, parenthesized scope ANDed with the filter, ordering, prepared limit/offset); verify with golden SQL and argument tests covering `sql-goqu-compilation` page scenarios, placeholder order, and determinism
- [x] 2.4 Implement count query compilation; verify with golden tests that count SQL has only filter joins and no ordering or pagination
- [x] 2.5 Add a concurrency test compiling many plans in parallel with one driver value and verify it passes under `go test -race ./...`
- [x] 2.6 Rewrite `easylist-sql-goqu/README.md` for driver usage (constructing the driver, plugging it into `easylist.Runtime`, PostgreSQL semantics such as `<>` and `ILIKE`), removing the CLI and `CREATE TABLE` sections; verify every Go snippet compiles via an example test

## 3. easylist-gqlgen

- [ ] 3.1 Create the `easylist-gqlgen` module (package `easylistgql`) with a pinned gqlgen version, add it to `go.work`, and spike a plugin that writes one Go type in `MutateConfig`, binds it in `cfg.Models`, and injects one SDL type; verify a test-fixture gqlgen project generates and compiles with it
- [ ] 3.2 Implement the `Int64`, `Decimal`, `Date`, and `UUID` scalar marshalers with configurable names; verify marshal/unmarshal tests including invalid inputs
- [ ] 3.3 Implement SDL generation per list and the shared SDL (`SortDirection`, `ExportFormat`, `ExportJob`, scalars) per `gqlgen-schema-generation`; verify with golden SDL tests for a list using every field type, nullable fields, enums, and calculated fields
- [ ] 3.4 Implement Go code generation per list (row struct with typed fields and calculated-field methods, page, inputs, enum types, `Resolve<P>` and `Export<P>` wrappers) and model binding in `MutateConfig`; verify the fixture project generates, compiles, and its generated code does not import back into the list definitions' importers
- [ ] 3.5 Implement selection collection under `items` (fragments, inline fragments, aliases, `@skip`/`@include`, `__typename` ignored) and the generic list resolver (filter parse, planning, page/count only when selected, errors with positions); verify with fixture-schema tests executing operations through the gqlgen handler against a fake `Queryer`
- [ ] 3.6 Implement the generic export resolver (input to `ExportRequest`, shared validation, `Exporter` call, `ExportJob` result, missing-exporter error); verify with fixture-schema mutation tests for accepted, invalid, and unconfigured cases
- [ ] 3.7 Verify name-collision behavior: a fixture schema that already declares `UsersRow` makes generation fail with an error naming `UsersRow`
- [ ] 3.8 Write `easylist-gqlgen/README.md` covering the custom generate entrypoint, attaching queries and mutations in the application schema, scalar renaming, and the import-cycle warning; verify the documented entrypoint matches the fixture project

## 4. easylist-export

- [ ] 4.1 Create the `easylist-export` module (package `easylistexport`) requiring `easylist-core` and excelize, add it to `go.work`, and verify `go build ./...` succeeds
- [ ] 4.2 Implement `Run` (revalidation, export plan, single streaming query, per-row calculated evaluation, encoder dispatch, stop on first error or context cancellation); verify with tests against a fake `Queryer` that count queries, assert one query runs, and assert rows are written before the fake finishes producing them
- [ ] 4.3 Implement the CSV encoder per `export-file-generation` (header titles, RFC 4180 quoting, NULL as empty, per-type formatting); verify with golden CSV tests
- [ ] 4.4 Implement the XLSX encoder with excelize `StreamWriter` (sheet named after the list, text cells for strings and exact numerics, number cells within ±2^53, date, boolean, and empty cells); verify by reading the produced workbook back with excelize in tests, including a formula-like string
- [ ] 4.5 Write `easylist-export/README.md` showing an application worker that decodes an `ExportRequest`, calls `Run`, and stores the file, and naming the CSV formula risk; verify every Go snippet compiles via an example test

## 5. Integration checks

- [ ] 5.1 Add an end-to-end test in the `easylist-gqlgen` fixture project that wires core, `easylist-sql-goqu`, the generated schema, and a fake `Queryer`, then checks that a query selecting a joined field and a calculated field produces the expected SQL, arguments, and response, and that an export mutation followed by `easylistexport.Run` produces the expected CSV; verify with `go test ./...` from every module in the workspace
- [ ] 5.2 Update the project context in `openspec/config.yaml` to describe the four modules and their dependency direction, and verify `openspec validate add-list-framework --strict` passes
