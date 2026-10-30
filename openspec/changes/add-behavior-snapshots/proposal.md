# Proposal

## Why

A reviewer cannot see from a diff how a change affects what easylist does. The conformance suite writes expected rows as hand-written ids in Go structs (`ids(1, 4, 6, 9)`), so a behaviour change means cross-referencing a seed-table comment. The GraphQL end-to-end tests run against a fake database with inline strings. Only the generated SDL has readable golden files. We want every behaviour change to show up as a readable input → outcome pair in the diff.

## What Changes

- A new top-level module, `easylist-behavior`, holds behaviour snapshots. No other module imports it. It runs a small, readable seeded PostgreSQL database through `easylist-sql-gorm` with a fixed clock, and compares the results with golden files that `go test -update` rewrites. It has three kinds of fixtures:
  - **List definitions**: Go list definitions that use every field type, as scalars, arrays, nullable fields, joins, and calculated fields. The outcome is the generated GraphQL SDL plus a field × operator matrix that shows which filter operators each field accepts, or the error the planner returns.
  - **Filters**: hand-written filter expressions grouped by operator family. The outcome is the matching rows by id, name, and the fields the filter references, or the error and its position. The generated SQL and arguments go in a separate golden file, so a refactor that changes only SQL is distinguishable from a behaviour change.
  - **GraphQL requests**: `*.graphql` operations. The outcome is the pretty-printed JSON response, errors included. They run through the module's own gqlgen project with the easylist plugin.
- Golden files echo their input, so every diff hunk shows the fixture next to its outcome.
- A coverage test fails when the fixtures lack a field type (scalar or array), a filter operator, a time keyword, a duration unit, or an arithmetic operator. A new filtering ability cannot ship without a snapshot.
- The module adds a `compose.yml` with PostgreSQL 16 on port 54329, matching the existing `EASYLIST_PG_DSN` instructions. Without `EASYLIST_PG_DSN` the snapshot tests skip with the same message the conformance suite uses, so CI's skip check also covers them.
- The hand-written conformance suite stays the contract for drivers. Snapshots describe the library's behaviour; they do not replace it.
- The gqlgen fixture's end-to-end tests switch from `easylist-sql-goqu` to `easylist-sql-gorm`, because goqu is no longer the supported driver for new code.
- The root README lists `easylist-behavior` as a test-only module and explains how to start the database and refresh snapshots.

Out of scope: export (CSV/XLSX) snapshots, and retiring `easylist-sql-goqu` from the remaining READMEs, specs, and `easylist-sqlpg` tests.

Modules touched:
- `easylist-behavior` (new): fixtures, golden files, the snapshot and coverage tests, its gqlgen project, `compose.yml`, and a README.
- `easylist-filter`: `PredicateOp` and `LogicalOp` gain `String()` methods, like the other AST enums, so the coverage test can enumerate them.
- `easylist-gqlgen`: the fixture project's end-to-end tests use `sqlgorm` instead of `sqlgoqu`. No library code changes.
- `go.work` and the root `README.md`: the new module is listed.

`easylist-core`, `easylist-sqlpg`, the drivers, and `easylist-export` are not changed.

## Capabilities

### New Capabilities
- `behavior-snapshots`: reviewer-readable behaviour snapshots of list definitions, filters, and GraphQL requests, run against a live PostgreSQL database, with enforced coverage of every field type and filter feature.

### Modified Capabilities
- `workspace-ci`: the live-database and skip-guard requirements cover behaviour snapshots as well as conformance cases.

## Impact

- New module `easylist-behavior` with dependencies on gqlgen, gqlparser, gorm (through `easylist-sql-gorm`), and pgx. These are test-only for the workspace and affect no published module.
- `go.work` gains one entry. CI picks the module up automatically through `go list -m`.
- CI runs one more live test package against the existing PostgreSQL service. The workflow file needs only its skip message reworded.
- Changing library behaviour now also requires `go test -update` in `easylist-behavior` and committing the golden-file diff. That diff is what reviewers read.
