# Design

## Context

- The repository root has only `go.work`, with no `go.mod`, so `go test ./...` at the root fails: "directory prefix . does not contain modules listed in go.work". In workspace mode, `go list -m` lists exactly the 11 workspace modules, and `go list $(go list -m -f '{{.Dir}}/...')` resolves all of their packages from the root (checked locally on Go 1.25.0).
- `go.work` and every `go.mod` declare `go 1.25.0` and have no `toolchain` line.
- The conformance suite (`easylist-sqlpg/conformance`) skips each `<case>/live` subtest with the message `EASYLIST_PG_DSN is not set` (`t.Skip(DSNEnv + " is not set")`). When the variable is set and the database cannot be reached, it fails. Each run uses its own schema, so all six drivers' tests can share one database at the same time.
- The suite needs PostgreSQL 16 or later and a role that can create schemas and types. The READMEs already document the `postgres:16` Docker image with trust authentication.
- Other skips are deliberate: `easylist-gqlgen` skips its gqlgen and `go build` tests only under `-short`, and `easylist-core` skips one time-zone test when no tz database exists. CI runs without `-short` on a runner that has tzdata, so both run.

## Goals / Non-Goals

**Goals:**
- One workflow file, with the same commands a developer can run locally from the repository root.
- Readable failure output: a failing test is easy to find in the log.

**Non-Goals:**
- Splitting tests into per-module jobs or caching test results across runs.
- Pinning actions by commit SHA or adding Dependabot for them.

## Decisions

### One job with a PostgreSQL service container
A single `test` job on `ubuntu-latest` runs vet and tests with a `postgres:16` service container. Offline conformance cases and every other test run in the same `go test` call as the live ones, so there is no separate unit job.

*Alternative:* a matrix with one job per module. It runs in parallel, but repeats checkout, Go setup, and module download 11 times and needs the module list in two places. Revisit if the single job gets slow.

*Alternative:* two jobs, one without a DSN and one with. The offline checks already run inside the DSN job, so a no-DSN job would only prove that the skip path works, which the suite's own tests cover.

### Find modules with `go list -m`
Both steps use the same pattern list, `go list -m -f '{{.Dir}}/...'`, as `go vet $(…)` and `go test -race $(…)`. A module added to `go.work` is picked up automatically, as `workspace-ci` requires.

*Alternative:* a shell loop that runs `cd <module> && go test ./...`. It gives the same coverage, but needs `go.work` parsed by hand and runs the Go command once per module.

### Go version from `go.work`
`actions/setup-go` reads the version with `go-version-file: go.work`, so CI follows the workspace's `go` line. Its module and build cache is keyed on `go.work.sum` and `**/go.sum`.

### PostgreSQL service settings
- Image `postgres:16` with `POSTGRES_HOST_AUTH_METHOD=trust`. That is safe on a throwaway runner and needs no secret, so pull requests from forks get the same checks.
- Health check `pg_isready`, so the job waits for the server before the test step.
- Port `5432:5432`, and `EASYLIST_PG_DSN=postgres://postgres@localhost:5432/postgres?sslmode=disable`, set on the test step only.

PostgreSQL 16 is the documented minimum. Testing newer versions is a later matrix entry.

### Guard against skipped live cases
The test step runs `go test -race -v` with `set -o pipefail` and copies the output to a log file with `tee`. A following step, run even when tests fail, fails the job if the log contains `EASYLIST_PG_DSN is not set`, and reports how many live cases were skipped.

*Alternative:* `go test -json` checked with `jq` for skipped `*/live` tests. That is sturdier against a reworded message, but the raw JSON log is hard to read without an extra formatter such as gotestsum.

*Alternative:* a suite-side switch, such as an `EASYLIST_PG_REQUIRED` variable that turns the skip into a failure. That is cleaner, but it changes `easylist-sqlpg` code and the `sqlpg-conformance` spec for a CI concern. It's a reasonable follow-up if the grep proves brittle.

### Triggers, concurrency, and permissions
- `push` to `main`, `pull_request` to `main`, and `workflow_dispatch`.
- `concurrency: ci-${{ github.ref }}` cancels an older run on the same pull request. Runs on `main` are never cancelled, so every commit on `main` gets a result.
- `permissions: contents: read`. A job timeout of 20 minutes stops a hung run.
- `actions/checkout` and `actions/setup-go` are pinned to their current major tags, which are checked when the change is applied.

## Risks / Trade-offs

- [The guard matches the suite's skip message] → The step and the suite each get a comment that points at the other. If the message changes, the guard stops matching and skipped cases would pass silently, so the verification task tests the guard both ways.
- [`-v` makes the log long] → Failures still print `--- FAIL` and `FAIL <package>` lines, and the guard step prints its own summary. Moving to gotestsum is a follow-up if the log gets hard to read.
- [The race detector and the gqlgen tests (which run gqlgen and `go build`) slow the job] → The Go cache makes later builds fast, and the 20-minute timeout bounds the worst case.
- [`ubuntu-latest` and floating action major tags change under us] → Both are standard and keep CI current. A failure caused by drift shows up on the next run, not in a release.

## Migration Plan

Merge the workflow. The first push to `main` runs it. Then the repository owner can make the `test` check required in the branch protection settings for `main`, which is outside this change. To roll back, delete the workflow file.

## Open Questions

- Whether to make the check required on `main`. That is a repository setting and changes nothing here.
