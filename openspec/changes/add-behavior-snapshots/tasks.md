# Tasks

## 1. Repository root: preparation

- [x] 1.1 Bring the branch up to date with `main` (in this worktree, through the app's sync with the base branch). Verify that the root `README.md` with the module table exists and that `git log` contains `docs: add a top-level README`

## 2. easylist-filter

- [x] 2.1 Add `String()` to `PredicateOp` (`=`, `>`, `>=`, `<`, `<=`, `CONTAINS`, `EXCLUDES`, `BETWEEN`, `!=`, `IN`, `NOT IN`) and to `LogicalOp` (`AND`, `OR`) in `ast.go`, using the existing `enumName` helper like `TimeKeyword`, `TimeUnit`, and `ArithOp`, so an out-of-range value prints as `PredicateOp(n)`. Verify with a table test covering every constant and one out-of-range value, and that `go test ./...` and `go vet ./...` pass in `easylist-filter`

## 3. easylist-gqlgen

- [x] 3.1 Switch `internal/fixture/graph/e2e_test.go` from `sqlgoqu.New()` to `sqlgorm.New()`, and update its expected SQL strings to the gorm driver's output. Keep every expected argument, response, and CSV unchanged. Update the module's `go.sum` or the workspace `go.work.sum` as needed. Verify that `go test ./...` and `go vet ./...` pass in `easylist-gqlgen`, and that `grep -rn sqlgoqu easylist-gqlgen` finds nothing

## 4. easylist-behavior: scaffolding

- [x] 4.1 Create the module `github.com/mgpnd/easylist-go/easylist-behavior` with `go.mod` (go 1.25.0, no sibling requires), `doc.go`, and an entry in `go.work`. Verify that `go list -m` from the root lists it and `go build ./...` passes in it
- [x] 4.2 Add `compose.yml` (postgres:16, trust auth, host port 54329, `pg_isready` health check). Verify that `docker compose -f easylist-behavior/compose.yml up -d --wait` starts a healthy server reachable at `postgres://postgres@127.0.0.1:54329/postgres?sslmode=disable`, then stop it with `down`

## 5. easylist-behavior: data set and list definitions

- [x] 5.1 Write `lists/lists.go` with `people` and `countries` as design.md describes:
  - every field type as a scalar and as an array
  - nullable and non-nullable fields
  - an enum, a joined field, and a lateral-join field
  - scalar, nullable, enum, and array calculated fields
  - a scope that hides soft-deleted rows
  - a `Registry`

  The package imports only `easylist-core`. Verify that `NewRegistry` succeeds, through a package test or the coverage test in 4.3
- [x] 5.2 Write `schema.sql` and `seed.sql`: about a dozen people with real names, values placed relative to the pinned reference time (Tuesday 2026-10-27 10:00 UTC), and readable edge rows (no country, all-NULL optionals, empty vs NULL arrays, one soft-deleted row). Embed both with `go:embed`. Verify that they load into a fresh schema on the compose database without errors
- [x] 5.3 Add the field-type coverage test: iterate `easylist.Type` through `String()`, and require a scalar and an array field of each type, plus a nullable, a joined, and a calculated field. Verify that it passes, and that it fails naming the type when one array field is temporarily removed

## 6. easylist-behavior: test harness

- [x] 6.1 Add the live database helper: open `EASYLIST_PG_DSN` with pgx `stdlib`, create `easylist_behavior_<hex>` with `search_path` and `timezone=UTC`, load the schema and seed, and drop the schema in `t.Cleanup`. Skip with exactly `EASYLIST_PG_DSN is not set` when the variable is unset. Verify that with the DSN unset the tests report that skip message, and with the DSN set no `easylist_behavior_%` schema remains after the run
- [x] 6.2 Add the golden-file helper behind a package-level `-update` flag. It compares, reports the file and a diff, and fails a missing file with a hint to run `-update`. With `-update` it writes files and removes outcome files whose fixture is gone. It never writes when the database is unavailable. Verify with a temporary fixture that it creates, detects a change in, and removes its outcome file

## 7. easylist-behavior: list snapshots

- [x] 7.1 Generate the per-list SDL and the shared SDL into `testdata/lists/<list>.golden` and `testdata/lists/shared.golden`, using the plugin the same way `easylist-gqlgen`'s SDL golden test does. Verify that `go test . -update` writes them and a second run without `-update` passes
- [x] 7.2 Add the operator matrix section: for every field and every predicate operator, plus `= NULL` and `!= NULL`, plan a probe filter with a per-type sample value, and print one block per field with `ok` or the planner's error. Verify that `active` shows `=` as ok and `>` as an error, and that the output is identical across two runs

## 8. easylist-behavior: filter snapshots

- [x] 8.1 Implement the filter case parser (comment lines, then one expression line, with blank-line separators). Any other shape is an error naming the file and line. Verify with table tests for a valid file, two expressions in one case, and a trailing comment with no expression
- [x] 8.2 Run each case through `Runtime.Fetch` on `people` with `sqlgorm.New()`, the pinned `Clock`, the maximum page size, and the fields `name` plus the fields the filter references. Write `<family>.golden` (count, an aligned table of id, name, and referenced fields, or `=> error: ...`) and `<family>.sql.golden` (page and count SQL with arguments; error cases omitted). Verify that `go test . -update` writes both, and that a run without `-update` passes
- [x] 8.3 Write the fixture files `compare`, `null`, `in`, `text`, `between`, `logic`, `arrays`, `relative-time`, `arithmetic`, and `errors`. Generate their outcomes and read every outcome by hand against the seed. Record any result that contradicts the query language reference in the PR description; do not change library code in this change. Verify that the outcomes are committed and that the tests pass
- [x] 8.4 Add the filter-feature coverage test: walk the ASTs of every filter fixture and every GraphQL `filter:` argument, and require every predicate operator, logical connector, time keyword, duration unit, and arithmetic operator, enumerated through their `String()` names. Verify that it passes, and that it fails naming the operator when the only `NOT IN` case is temporarily removed

## 9. easylist-behavior: GraphQL snapshots

- [x] 9.1 Add the gqlgen project under `graph/` (`gqlgen.yml`, `schema.graphqls` with `people` and `countries` queries, `resolver.go` with `//go:generate go run ../cmd/generate`) and `cmd/generate/main.go` with the easylist plugin. Run `go generate ./graph` and commit the generated files. Verify that `go build ./...` passes
- [x] 9.2 Add a freshness test that regenerates a copy of `graph/` and fails when a committed generated file differs, skipped under `-short`. Verify that it passes, and that it fails after a temporary edit to `generated.go`
- [x] 9.3 Serve the generated schema in-process with `httptest` and the POST transport over a runtime using `sqlgorm.New()`, the live database, and the pinned `Clock`. Write each `testdata/queries/<name>.graphql` response as indented JSON to `<name>.json`. Verify that `go test . -update` writes them and a run without `-update` passes
- [x] 9.4 Write the GraphQL fixtures:
  - a basic page with `totalCount`
  - fragments and aliases
  - sort, limit, and offset
  - a calculated field and a calculated field error
  - array and enum fields
  - a `timezone` argument with `TODAY`
  - a filter syntax error
  - a limit above the maximum

  Read every response by hand. Verify that the outcomes are committed and the tests pass

## 10. easylist-behavior: documentation

- [x] 10.1 Write `easylist-behavior/README.md` covering:
  - what the module is for, and that nothing imports it
  - how to start the database with `compose.yml` and set `EASYLIST_PG_DSN`
  - how to add each kind of fixture
  - how to refresh outcomes with `go test . -update`
  - how to review: read `*.golden` and `*.json` diffs first, and treat `*.sql.golden` as implementation

  Verify that every command in it runs as written from the stated directory

## 11. Repository root: README and CI

- [x] 11.1 In the root `README.md`, add `easylist-behavior` to the module table, marked test-only. In the Develop section, add the compose command and the `-update` refresh command. Verify that the links resolve and the commands run as written
- [x] 11.2 In `.github/workflows/ci.yml`, reword the guard's error to say that tests needing PostgreSQL were skipped, and update its comment to name both skip sites (the conformance suite and `easylist-behavior`). Add a matching comment at the behaviour skip. Verify that the YAML parses, and that the guard command fails on a log from a run without the DSN and passes on a log from a run with it

## 12. Integration

- [x] 12.1 From the root, against the compose database, run `go vet $(go list -m -f '{{.Dir}}/...')` and `go test -race $(go list -m -f '{{.Dir}}/...')` with `EASYLIST_PG_DSN` set. Verify that every module reports `ok` and no test reports `EASYLIST_PG_DSN is not set`
- [ ] 12.2 Push the branch and open a pull request to `main` (only when the user asks). Verify that the `test` check passes and its log shows the `easylist-behavior` package passing

## Workflow follow-up

- Archive the change once the pull request is merged and CI has passed on `main`.
- Retiring `easylist-sql-goqu` from READMEs, specs, and `easylist-sqlpg` tests is a separate change.
