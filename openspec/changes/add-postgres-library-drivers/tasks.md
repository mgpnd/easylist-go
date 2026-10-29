# Tasks

## 1. easylist-core

- [x] 1.1 Add a "Drivers" section to `easylist-core/README.md` that lists the six PostgreSQL drivers (`easylist-sql-goqu`, `-gorm`, `-ent`, `-bob`, `-sqlbuilder`, `-jet`), says they compile the same plans to the same arguments and matching rows, and says to pick the one whose library the application already uses. Verify `go test ./...` passes in `easylist-core`, including the README example tests

## 2. easylist-sqlpg

- [x] 2.1 Create the `easylist-sqlpg` module (`github.com/mgpnd/easylist-go/easylist-sqlpg`, package `sqlpg`, `go 1.25.0`, no sibling requires) and add it to `go.work`. Verify `go build ./...` succeeds from the workspace root with the Go 1.25 toolchain (`GOTOOLCHAIN=auto`). If the toolchain cannot be obtained, stop and report it
- [x] 2.2 Add the `Query`, `Column`, `Ordering`, `Page`, `Expr`, and `Arg` types and the `Cond` node types (`And`, `Or`, `Raw`, `Compare`, `IsNull`, `In`, `Between`, `ILike`, `ArrayHas`, `DayRange`) from design.md, each with `Fragment()` reproducing the template that `easylist-sql-goqu/predicate.go` uses today. Verify with table tests that render every node's fragment and check the SQL template and slot order, including all four `In` NULL forms, NULL-only lists, `ILike` with `Not`, `ArrayHas` with `None`, and `DayRange` with `Not`
- [x] 2.3 Move the plan-dependent logic from `easylist-sql-goqu/predicate.go` and `lower.go` (field lookup, operator checks, `predicateArgument`, `listArguments`, `arrayValues`, `exprPredicate`, `wholeDay`, the iterative left-first walk) into `sqlpg.Lower`, which builds the `Cond` tree, the verbatim `From`, `Select` (with `to_json(...)::text` for arrays), `Order`, and `Page`, and `Query.Args()`. Keep every error message text unchanged. Leave `easylist-sql-goqu` untouched in this task. Verify with tests that `Lower` returns the same error text as the goqu driver for each case in goqu's `TestCompileErrors`, `TestBuildPredicateRejects*`, and `TestExpr*` error tests, and that `Args()` lists filter arguments left to right, then limit and offset as `int64`
- [x] 2.4 Add a test-only reference driver in `easylist-sqlpg` that renders `Query` through `Fragment()` with `$n` numbering, and golden tests through it for the `contract-sql-postgres` scenarios that are about SQL text: placeholder order, page and count shape, offset 0 bound, numeric cast and no other casts, NULL forms, array selection and membership, whole-day SQL, `IN` lists, and author SQL with `?`, `$`, `#`, and `:`. Verify `go test -race ./...` passes in `easylist-sqlpg`
- [x] 2.5 Add package `easylist-sqlpg/conformance` with `Run(t *testing.T, d easylist.Driver)`, the shared list definition, and the case table from design.md, starting with the offline checks: arguments and their Go types equal `sqlpg.Lower(plan).Args()`, placeholders are exactly `$1`..`$len(args)`, a second compile is byte-identical, and error cases match `Lower`'s error text. Cover every `contract-sql-driver` and `contract-sql-postgres` scenario that arguments or errors can show. Verify that running `conformance.Run` with the reference driver from 2.4 passes with `EASYLIST_PG_DSN` unset and reports the live subtests as skipped
- [x] 2.6 Add the live half of `conformance.Run`: connect with `pgx/v5/stdlib` (imported only by `conformance`), fail on an unreachable database, create `easylist_conf_<random>` with the fixture DDL and seed rows from design.md, run each case's page and count queries, compare ids in order, selected values (including array JSON text and SQL NULL), and counts, and drop the schema in `t.Cleanup`. Add the expected ids for every case, written by hand from the specs, including the NULL-row, escaping, empty and NULL array, DST whole-day, offset 0, scope-with-`OR`, and jsonb `?` scope cases from `sqlpg-conformance`. Verify by starting PostgreSQL 16 locally (`/usr/lib/postgresql/16/bin/initdb` and `pg_ctl`, or Docker), setting `EASYLIST_PG_DSN`, and running the reference driver's conformance test, including two runs in parallel that both pass and leave no `easylist_conf_` schema behind
- [x] 2.7 Write `easylist-sqlpg/README.md`: who the module is for (driver authors), the `Lower` → `Query` → driver flow, the opaque-leaf rule for author SQL, how to run `conformance.Run` from a driver test, and how to start a local PostgreSQL and set `EASYLIST_PG_DSN`. Verify the documented commands run as written in this container

## 3. easylist-sql-goqu

- [ ] 3.1 Rewrite `driver.go` to call `sqlpg.Lower` and map the `Query` onto goqu, rendering every `Cond` through `Fragment()` with `goqu.L` and keeping the hand-appended `LIMIT $n OFFSET $m`. Delete the moved code from `predicate.go` and `lower.go`. Verify `TestCompile*`, `TestReadmeSnippets`, and the other `driver_test.go` and `expr_test.go` driver-level tests pass with their expected values unchanged
- [ ] 3.2 Port the internal tests in `predicate_test.go` and `expr_test.go` that call `buildPredicate` or `exprPredicate` to a test helper that lowers a one-predicate plan with `sqlpg.Lower` and renders it through the goqu mapping, keeping every expected SQL string, argument, and error unchanged. Verify `go test -race ./...` passes and `git diff` shows no change to any expected value
- [ ] 3.3 Add `conformance_test.go` calling `conformance.Run(t, sqlgoqu.New())`, and a test that `go list -deps` of the package (without tests) contains goqu and none of bob, go-sqlbuilder, jet, ent, gorm, or pgx. Verify it passes offline, and live with `EASYLIST_PG_DSN` set
- [ ] 3.4 Update `easylist-sql-goqu/README.md` to link the shared semantics in `easylist-sqlpg` and keep only the goqu-specific notes. Verify the README example test passes

## 4. easylist-sql-gorm

- [ ] 4.1 Create the `easylist-sql-gorm` module (package `sqlgorm`, requiring only `gorm.io/gorm`), add it to `go.work`, and add the private dialector from design.md (`BindVarTo` writing `$n`, `QuoteTo` writing `"name"`, default callbacks, no-op migrator and the other methods) with a package-level DryRun `*gorm.DB`. Verify `go build ./...` succeeds and `go list -deps` of the package contains no `gorm.io/driver/postgres` or pgx package
- [ ] 4.2 Implement `New()` and `Compile`: a fresh `Session` per call, verbatim `TableExpr`, `clause.Select`/`Where`/`OrderBy` built from `clause.Expr` with author SQL as zero-var leaves, `And`/`Or` mapped to gorm's connectors, and `LIMIT $n OFFSET $m` appended by hand. Verify with golden tests for page, export, and count shape, offset 0, `IN` lists (pinning the `(?` handling), and author SQL containing `?`, plus a test that a separate gorm handle with a custom naming strategy and a plugin does not change the output
- [ ] 4.3 Add `conformance_test.go`, a concurrent-compile test under `-race`, and the compiled-in package test (gorm present; no goqu, bob, go-sqlbuilder, jet, ent, `gorm.io/driver/postgres`, or pgx). Verify all pass offline, and live with `EASYLIST_PG_DSN` set
- [ ] 4.4 Write `easylist-sql-gorm/README.md` with usage alongside an existing gorm handle (the driver needs none) and a README example test like goqu's. Verify the example test passes

## 5. easylist-sql-ent

- [ ] 5.1 Create the `easylist-sql-ent` module (package `sqlent`, requiring `entgo.io/ent` and importing only `entgo.io/ent/dialect` and `dialect/sql`) and add it to `go.work`. Verify `go build ./...` succeeds
- [ ] 5.2 Implement `New()` and `Compile` with `sql.Dialect(dialect.Postgres).Select()`: `FromExpr(sql.Raw(from))`, `AppendSelectExprAs`, `OrderExpr`, `Where` with `sql.And`/`sql.Or` over `sql.P` fragments (author SQL through `b.WriteString`, values through `b.Arg`), and `LIMIT $n OFFSET $m` appended by hand. Verify with golden tests for page, export, and count shape, limit 25 and offset 75 not appearing as literals, offset 0, and author SQL containing `?` and `$`
- [ ] 5.3 Add `conformance_test.go`, a concurrent-compile test under `-race`, and the compiled-in package test (ent present; no goqu, bob, go-sqlbuilder, jet, gorm, or pgx). Verify all pass offline, and live with `EASYLIST_PG_DSN` set
- [ ] 5.4 Write `easylist-sql-ent/README.md` (usage next to an ent client, no generated code needed) with a README example test. Verify the example test passes

## 6. easylist-sql-bob

- [ ] 6.1 Create the `easylist-sql-bob` module (package `sqlbob`, requiring `github.com/stephenafamo/bob` at a pinned version) and add it to `go.work`. Verify `go build ./...` succeeds
- [ ] 6.2 Implement `New()` and `Compile` with `psql.Select`: `sm.From(expr.Raw(from))`, columns and ordering from `expr.Raw` leaves, `sm.Where` with bob's `And`/`Or`, bound `sm.Limit`/`sm.Offset`, and predicates via `Fragment()` rendered with `psql.Raw` and `psql.Arg`. Map `Compare`, `IsNull`, `In`, and `Between` to bob's native operators only where a golden test shows the same arguments and no cast. Verify with golden tests for page, export, and count shape, offset 0, every node type, and a join containing `?`
- [ ] 6.3 Add `conformance_test.go`, a concurrent-compile test under `-race`, and the compiled-in package test (bob present; no goqu, go-sqlbuilder, jet, ent, gorm, or pgx). Verify all pass offline, and live with `EASYLIST_PG_DSN` set
- [ ] 6.4 Write `easylist-sql-bob/README.md` with a README example test. Verify the example test passes

## 7. easylist-sql-sqlbuilder

- [ ] 7.1 Create the `easylist-sql-sqlbuilder` module (package `sqlsqlbuilder`, requiring `github.com/huandu/go-sqlbuilder`) and add it to `go.work`. Verify `go build ./...` succeeds
- [ ] 7.2 Implement `New()` and `Compile` with `sqlbuilder.PostgreSQL.NewSelectBuilder()`: escaped author SQL for `From`, `Select`, and ordering, bound `Limit`/`Offset`, predicates via `Fragment()` with `sb.Var`, and native `Cond` methods only where a golden test shows the same arguments and no cast. Verify with golden tests for page, export, and count shape, offset 0, every node type, and a scope containing `$` and `?`
- [ ] 7.3 Add `conformance_test.go`, a concurrent-compile test under `-race`, and the compiled-in package test (go-sqlbuilder present; no goqu, bob, jet, ent, gorm, or pgx). Verify all pass offline, and live with `EASYLIST_PG_DSN` set
- [ ] 7.4 Write `easylist-sql-sqlbuilder/README.md`, including why the package is named `sqlsqlbuilder`, with a README example test. Verify the example test passes

## 8. easylist-sql-jet

- [ ] 8.1 Create the `easylist-sql-jet` module (package `sqljet`, requiring `github.com/go-jet/jet/v2`) and add it to `go.work`. Verify `go build ./...` succeeds
- [ ] 8.2 Implement `New()` and `Compile` with `postgres.SELECT(...).FROM(NewTable("", sentinel, ""))`: choose the sentinel as in design.md, replace its single occurrence with the verbatim `From` (error if the count is not one), projections as `Raw(expr).AS(alias)`, predicates with `CustomExpression` over `Token`, opaque `Raw` leaves, and one-argument `Raw` slots, bound `LIMIT`/`OFFSET`, and the trailing `;\n` trimmed. Verify with golden tests for page, export, and count shape, offset 0, no `::text` or other cast besides `::numeric`, twelve `IN` items bound `$1`..`$14` in order, a plan whose author SQL contains `easylist_from`, and author SQL containing `#1` and `?`
- [ ] 8.3 Add `conformance_test.go`, a concurrent-compile test under `-race`, and the compiled-in package test (jet present; no goqu, bob, go-sqlbuilder, ent, gorm, or pgx). Verify all pass offline, and live with `EASYLIST_PG_DSN` set
- [ ] 8.4 Write `easylist-sql-jet/README.md` (no generated code needed, and the note on jet's casts) with a README example test. Verify the example test passes

## 9. Workspace integration

- [ ] 9.1 With a local PostgreSQL 16 running and `EASYLIST_PG_DSN` set, run `go vet ./...` and `go test -race ./...` in every module listed in `go.work`, and verify that all pass, that no conformance subtest is skipped, and that no `easylist_conf_` schema is left in the database
- [ ] 9.2 Run `openspec validate add-postgres-library-drivers --strict` and verify it passes

## Workflow follow-up

- Archive the change with `/opsx:archive` once the review is done, and check that `contract-sql-postgres`, `sqlpg-conformance`, the five new driver specs, and the reduced `sql-goqu-compilation` land in `openspec/specs/`.
