# Design

## Context

- `easylist-sqlpg/conformance` already seeds PostgreSQL in a random schema (`easylist_conf_<hex>`) with a UTC session, skips live cases with `EASYLIST_PG_DSN is not set`, and pins the reference time. Its data set is adversarial on purpose (SQL-injection names, DST edges, 20-digit numerics) and its expectations are hand-written ids. It stays as it is.
- `easylist-gqlgen` already keeps golden SDL files in `testdata/` behind a package-level `-update` flag (`sdl_test.go`), and checks that its fixture project's generated code is fresh by regenerating a copy (`TestFixtureGeneration`, skipped under `-short`). Its fixture lives under `internal/`, so another module cannot import it.
- `easylist.Runtime` takes a `Clock`, `Runtime.Fetch` runs the page and count queries, and `CompiledList.Plan` returns planning errors with positions without touching a database. `sqlgorm.New()` needs no gorm handle and runs through `easylist.SQLDB(*sql.DB)`.
- CI runs `go test -race -v` on every `go.work` module with a `postgres:16` service and fails if the log contains `EASYLIST_PG_DSN is not set`.
- `main` now has a root `README.md` with a module table and a Develop section. This worktree branch does not have it yet.

## Goals / Non-Goals

**Goals:**
- A reviewer can understand a behaviour change from the outcome-file diff alone.
- Adding a fixture means editing one text file and running `-update`. No Go changes are needed, except for list definitions.
- Snapshots share tooling idioms with the rest of the repo: the `-update` flag, `EASYLIST_PG_DSN`, and a random schema per run.

**Non-Goals:**
- Checking that drivers agree with each other. That is conformance's job, and snapshots use one driver.
- Snapshotting exports, timings, or query plans.
- A test DSL beyond "one expression per case" and "one operation per file".

## Decisions

### Module layout

```
easylist-behavior/                    module github.com/mgpnd/easylist-go/easylist-behavior
  README.md  compose.yml
  doc.go                              package behavior: what the module is for
  schema.sql  seed.sql                embedded with go:embed
  lists/lists.go                      list definitions; imports easylist-core only
  graph/                              gqlgen project: gqlgen.yml, schema.graphqls, resolver.go,
                                      generated.go, model/, easylistgen/ (generated)
  cmd/generate/main.go                runs gqlgen with the easylist plugin (go:generate in graph/resolver.go)
  testdata/lists/<list>.golden
  testdata/filters/<family>.txt  <family>.golden  <family>.sql.golden
  testdata/queries/<name>.graphql  <name>.json
  *_test.go                           all in package behavior_test at the module root
```

All snapshot and coverage tests live in one package, so the `-update` flag exists in only one test binary. Running `go test ./... -update` would fail in packages that do not define the flag, so the README gives `go test . -update` from the module directory.

*Alternative:* put the snapshots in `easylist-gqlgen/internal/fixture`. That reuses its generated project, but drags pgx and a SQL driver into the gqlgen module's tests and mixes plugin tests with library-wide behaviour.

### Data set

There are two lists:

- `people` declares every field type as a scalar and as an array, nullable and non-nullable fields, an enum with declared values, a joined field (`country`, through `countries`), a field from a lateral join, and calculated fields (scalar, nullable, enum, and array). It has a static scope that hides soft-deleted rows.
- `countries` is a second, small list. It shows a plain list and a multi-word GraphQL name if one is used.

The seed has about a dozen people with real names, and a few edge rows that are still readable: a person with no country, all-NULL optional fields, empty versus NULL arrays, and one soft-deleted row. Values are chosen relative to the pinned reference time, so `TODAY`, `START_OF_WEEK` and the like each match a known person.

*Alternative:* reuse the conformance seed. It is rejected because it is unreadable as a story, and coupling would make every conformance tweak churn the snapshots.

### Pinned time

The reference time is Tuesday 2026-10-27 10:00 UTC, the same instant conformance uses. The runtime gets `Clock` returning it, and the session time zone is UTC. Filter fixtures always run in UTC. Client time zones are exercised only through GraphQL fixtures with a `timezone` argument, which is how applications pass them.

### Filter fixture format

```
# != keeps NULL rows            <- comment lines belong to the next case
active != TRUE

# BETWEEN with a list
age BETWEEN (30, 40)
```

Cases are separated by blank lines. A case is zero or more `#` comment lines followed by exactly one non-empty line holding the filter expression. Any other shape fails the test with the file and line.

Each case runs as `Runtime.Fetch` on `people`, with `Want{Rows, Count}`, the list's maximum page size, the default sort (primary key), and selected fields `name` plus every field the filter references (found by walking the parsed AST), in first-appearance order.

Outcome (`<family>.golden`):

```
# != keeps NULL rows
active != TRUE
=> 3 rows
  id | name | active
   2 | Bea  | false
   5 | Dan  | NULL
   7 | Gil  | NULL
```

An error case prints `=> error: <message>` instead, with the parse or plan error text, which includes its position. Values are rendered as follows: `NULL`; text and enums as-is; numerics as their exact text; dates as `2006-01-02`; timestamps as RFC 3339 in UTC; arrays as `[a, b]`, with `[]` for an empty array. Columns are padded for alignment, using ASCII only.

`<family>.sql.golden` repeats the comment and expression, then the page and count SQL and their arguments, rendered with `%#v`-like literal formatting. Error cases are omitted.

Families are `compare`, `null`, `in`, `text`, `between`, `logic`, `arrays`, `relative-time`, `arithmetic`, and `errors`. Grouping keeps a diff local to the area that changed.

*Alternative:* txtar files that hold input and output together. They are rejected because `-update` would rewrite a file a human also edits, and comments, ordering, and merges would suffer.

### List snapshot

`<list>.golden` has two sections:

1. **SDL.** The plugin's per-list SDL, produced the same way gqlgen's golden test produces it. The shared SDL goes in `testdata/lists/shared.golden`.
2. **Operator matrix.** For every field, and for every predicate operator plus the `= NULL` and `!= NULL` forms, the test builds a probe filter with a sample value of the field's type, plans it with `CompiledList.Plan`, and lowers the plan with `sqlpg.Lower`. It records `ok` or the first parse, plan, or lowering error. `Plan` checks fields, calculated fields, and values, but operator-by-type checks live in `sqlpg.Lower`, which every PostgreSQL driver calls. Without lowering, the matrix would show `ok` for operators every driver rejects. The section is printed as one block per field, which reads better in a diff than a wide table:

```
active (boolean, nullable)
  =          ok
  !=         ok
  >          error: operator ">" is not supported on field "active" ...
```

Sample values come from a fixed per-type table in the test: text `'x'`, integer `1`, a date, an RFC 3339 timestamp, a declared enum value, and so on, with list forms for `IN` and `BETWEEN`. Probing goes through the same planning and validation path as a real request, so the matrix shows real behaviour with no new API in core.

### GraphQL snapshots

The module's gqlgen server, built from `graph/` with the easylist plugin, is served in-process through `httptest` with the POST transport. It uses a `Runtime{Driver: sqlgorm.New(), DB: easylist.SQLDB(db), Clock: pinned}`. Each `<name>.graphql` holds one operation with inline arguments and no variables file. The response body is re-indented with `json.Indent`, which keeps the server's field order, and is written to `<name>.json`. The export mutation is not exercised.

A freshness test, like `easylist-gqlgen`'s `TestFixtureGeneration`, regenerates a copy of `graph/` and fails when the committed generated files are stale. This keeps the GraphQL snapshots from running an outdated plugin output.

### Coverage tests

These run without a database:

- **Field types:** iterate `easylist.Type` values from `Text` upward while `String()` is not `"invalid"`. For each, require a field of that type with `Array` false and one with `Array` true in some fixture list. Also require a nullable field, a joined field, and a calculated field.
- **Filter features:** parse every filter fixture and every `filter:` argument found in the GraphQL fixtures. Walk the ASTs and collect predicate operators, logical connectors, time keywords, duration units, and arithmetic operators. Each enum is iterated through its `String()` names until the enum's fallback form appears, and each must have been seen. A missing item fails with its name.

Enumerating through `String()` avoids hard-coding the lists, so a new constant is picked up automatically as long as it gets a name. `TimeKeyword`, `TimeUnit`, and `ArithOp` already have `String()`. This change adds it to `PredicateOp` and `LogicalOp` with the same `enumName` helper, whose fallback form (`PredicateOp(11)`) marks the end of the enum.

### Database setup

`compose.yml` runs `postgres:16` with `POSTGRES_HOST_AUTH_METHOD=trust` on host port 54329, with a `pg_isready` health check. The README uses `docker compose -f easylist-behavior/compose.yml up -d --wait` and the same DSN that `easylist-sqlpg/README.md` documents, so one database serves both suites.

The tests open the DSN with pgx's `stdlib`, create `easylist_behavior_<hex>`, set `search_path` and `timezone=UTC` as runtime parameters, load `schema.sql` and `seed.sql`, and drop the schema in `t.Cleanup`. This mirrors `conformance/live.go`, copied rather than imported, because that code is unexported test support in another module.

### Driver switch in the gqlgen fixture

`easylist-gqlgen/internal/fixture/graph/e2e_test.go` builds its runtime with `sqlgorm.New()`. Its expected SQL strings are updated to the gorm driver's output, and nothing else changes. The fake database matches on `SELECT COUNT(*)` and ` LIMIT `, both of which the gorm driver emits too.

### CI

The workflow needs no structural change: the new module is in `go.work`, and its skip message matches the existing grep. The guard step's error text changes from "live conformance cases were skipped" to "tests needing PostgreSQL were skipped". Its comment names both skip sites.

## Risks / Trade-offs

- [Reviewers rubber-stamp `-update` diffs] → The rows and SQL files are split, so real behaviour changes stand out. The PR description template is unchanged, but the module README tells reviewers to read `*.golden` and `*.json` diffs first.
- [The skip guard depends on a copied message] → The behaviour tests use the same constant text, and the comment in `ci.yml` lists both places.
- [The gqlgen freshness test is slow (it runs gqlgen and `go build`)] → It skips under `-short`, like gqlgen's own, and CI runs without `-short`.
- [A new dependency surface: gqlgen, gorm, and pgx in one module] → The module is a leaf that nothing imports, and the dependencies are test and tooling only.
- [Seed values tied to the pinned date drift in meaning if the reference time changes] → The reference time is a single constant, and changing it shows up as a large, obvious snapshot diff.
- [`go.sum` / `go.work.sum` churn from gqlgen's test importing gorm] → Covered by a tidy task. CI's `go vet` and `go test` fail on missing sums.

## Migration Plan

This is additive. Merge `main` into the branch first so the root README exists, then add the module and its `go.work` entry, generate the gqlgen code and outcome files, and commit them. To roll back, remove the module directory and its `go.work` line, and revert the CI message and the e2e test.
