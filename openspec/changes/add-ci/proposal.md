# Proposal

## Why

Nothing runs the tests automatically. Each of the 11 workspace modules is tested only when someone remembers to run `go test` in it. The six PostgreSQL drivers promise to match the same rows, but that promise is checked only when someone sets `EASYLIST_PG_DSN` locally. Without a DSN the live conformance cases skip quietly, so a run with no database still looks green. The `add-postgres-library-drivers` design put CI off for later. Now that six drivers depend on the shared suite, a regression in `easylist-sqlpg` or in any one driver can land unnoticed.

## What Changes

- Add a GitHub Actions workflow that runs on pushes to `main`, on pull requests, and on manual dispatch.
- The workflow tests every module listed in `go.work` with `go vet` and `go test -race`. It finds the modules with `go list -m`, so a module added to `go.work` later is tested without editing the workflow. (`go test ./...` does not work at the workspace root, which has no `go.mod`.)
- The workflow starts a PostgreSQL 16 service container and sets `EASYLIST_PG_DSN` for the test step, so every driver's live conformance cases run against a real database.
- The workflow fails when any live conformance case is skipped, so a missing or mistyped DSN cannot turn into a green run.
- The `easylist-sqlpg` README says that CI runs the live suite.

Out of scope, possible follow-ups: `staticcheck` or `golangci-lint`, checks that generated code (`parser_gen.go`, the gqlgen fixture) and `go.mod`/`go.sum` are up to date, a PostgreSQL version matrix, coverage reports, and release tagging.

## Capabilities

### New Capabilities
- `workspace-ci`: automated checks that run on every push and pull request across all workspace modules, including the live PostgreSQL conformance suite.

### Modified Capabilities
None. The `sqlpg-conformance` requirements still hold as written. CI only makes sure their "when `EASYLIST_PG_DSN` is set" branch runs on every change.

## Impact

- New file `.github/workflows/ci.yml`. No change in Go behaviour.
- Modules touched: `easylist-sqlpg`, for the README and a comment at the conformance suite's skip call that names the CI guard depending on its message. Every module in `go.work` (`easylist-filter`, `easylist-core`, `easylist-export`, `easylist-gqlgen`, `easylist-sqlpg`, `easylist-sql-goqu`, `easylist-sql-gorm`, `easylist-sql-ent`, `easylist-sql-bob`, `easylist-sql-sqlbuilder`, `easylist-sql-jet`) is tested by the workflow, but none of them changes.
- Needs GitHub Actions enabled on `mgpnd/easylist-go`. The workflow uses only public actions and the `postgres:16` image, and needs no secrets.
