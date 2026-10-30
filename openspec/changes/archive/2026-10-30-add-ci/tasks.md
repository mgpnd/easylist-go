# Tasks

## 1. Workflow (repository root)

- [x] 1.1 Look up the current major versions of `actions/checkout` and `actions/setup-go` and note them for 1.2. Verify that each version's release notes support what the workflow needs: `go-version-file` pointing at a `go.work` file, and a multi-line `cache-dependency-path`
- [x] 1.2 Add `.github/workflows/ci.yml` as design.md describes:
  - triggers: `push` and `pull_request` to `main`, plus `workflow_dispatch`
  - `permissions: contents: read`, and a `concurrency` group that cancels older runs on a pull request but never on `main`
  - one `test` job on `ubuntu-latest` with a 20-minute timeout and a `postgres:16` service (trust auth, `pg_isready` health check, port 5432)
  - setup-go reading `go.work`, with its cache keyed on `go.work.sum` and `**/go.sum`
  - a `go vet` step and a `go test -race -v` step over `$(go list -m -f '{{.Dir}}/...')`, the test step with `set -o pipefail`, `tee` to a log file, and `EASYLIST_PG_DSN=postgres://postgres@localhost:5432/postgres?sslmode=disable`
  - an `if: always()` guard step that fails when the log contains `EASYLIST_PG_DSN is not set`, and prints how many live cases were skipped

  Verify the file parses as YAML (for example with `python3 -c 'import yaml,sys; yaml.safe_load(open(sys.argv[1]))' .github/workflows/ci.yml`), and with `actionlint` if it is available
- [x] 1.3 Add a comment above the guard step that names the skip message's source (`easylist-sqlpg/conformance/conformance.go`, `t.Skip(DSNEnv + " is not set")`), and a matching comment at that `t.Skip` call that names the guard. Verify that the only Go change is that comment (`git diff --stat` shows one line or so in `conformance.go`) and that `go test ./...` still passes in `easylist-sqlpg`
- [x] 1.4 Run the workflow's vet and test commands locally from the repository root against `postgres:16` in Docker (`docker run --rm -d --name easylist-pg -e POSTGRES_HOST_AUTH_METHOD=trust -p 54329:5432 postgres:16`, with the DSN on port 54329), with the same `tee` log. Verify that every module reports `ok`, and that the guard's command finds no skip message. Then run the test command again without the DSN and verify that the guard's command fails on that log. Stop the container afterwards

## 2. easylist-sqlpg

- [x] 2.1 In `easylist-sqlpg/README.md`, under "Run the conformance suite", add one sentence saying that CI runs the suite with `EASYLIST_PG_DSN` set against PostgreSQL 16 for every driver, and fails if any live case is skipped, with a link to `.github/workflows/ci.yml`. Verify that the link path resolves from the README's folder, and that `go test ./...` passes in `easylist-sqlpg`

## 3. Integration

- [x] 3.1 Push the branch and open a pull request to `main` (only when the user asks). Verify the `test` check runs on it and passes, that its log shows live conformance subtests passing for all six drivers (`--- PASS: TestConformance/.*/live`), and that the guard step reports zero skipped live cases

## Workflow follow-up

- After merge, the repository owner may make the `test` check required for `main` in the branch protection settings.
- Archive the change once the pull request is merged and CI has passed on `main`.
