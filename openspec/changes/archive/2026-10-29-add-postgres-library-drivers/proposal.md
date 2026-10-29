# Proposal

## Why

`easylist-sql-goqu` is the only SQL driver, so every application that uses
easylist adds goqu to its dependencies, even when it already builds queries
with bob, go-sqlbuilder, jet, ent, or gorm. A driver per library lets those
applications use easylist with the library they already depend on. All of
these drivers target PostgreSQL. Today the PostgreSQL rules for validation,
literal conversion, and NULL handling live inside the goqu driver, so each new
driver would have to copy them. Those rules have changed in every recent
change, and the copies would drift apart.

## What Changes

- New module `easylist-sqlpg` (package `sqlpg`) takes over the
  library-independent work from the goqu driver: field lookup, operator
  checks, literal and list conversion, evaluated-value binding, whole-day
  ranges, and the NULL-safe forms of `!=`, `EXCLUDES`, `IN`, and `NOT IN`. It
  lowers a plan into a typed, dialect-specific description of the query that
  a driver maps onto its library. Its exported API is meant for driver
  authors; applications never import it directly.
- `easylist-sqlpg/conformance` is a test package that every PostgreSQL driver
  runs. It checks arguments without a database. When `EASYLIST_PG_DSN` is set
  it also creates a fixture schema and checks which rows each filter matches
  on a live PostgreSQL. It is skipped, not failed, when the variable is unset.
- `easylist-sql-goqu` is refactored onto `easylist-sqlpg`. Its SQL output,
  arguments, and error messages stay the same.
- Five new driver modules, each depending only on `easylist-core`,
  `easylist-sqlpg`, and its own library:
  - `easylist-sql-bob` (package `sqlbob`), using bob's `psql` builder.
  - `easylist-sql-sqlbuilder` (package `sqlsqlbuilder`), using
    `huandu/go-sqlbuilder` with the PostgreSQL flavor.
  - `easylist-sql-jet` (package `sqljet`), using jet's `postgres` package
    without generated code.
  - `easylist-sql-ent` (package `sqlent`), using ent's `dialect/sql` builder
    without generated code.
  - `easylist-sql-gorm` (package `sqlgorm`), using gorm in dry-run mode with
    its own minimal dialector. It needs no database handle and no
    `gorm.io/driver/postgres` or pgx.
- Every PostgreSQL driver satisfies one shared contract. For the same plan,
  every driver gives the same arguments, with the same Go types and in the
  same order, and the same error text. Every driver also matches the same
  rows. The SQL text may differ in whitespace and formatting.
- bun and go-pg are not included. Both always inline values into the SQL
  text, which breaks the rule that request values are bound.

## Capabilities

### New Capabilities

- `contract-sql-postgres`: what every PostgreSQL driver produces. This is the
  PostgreSQL behaviour now in `sql-goqu-compilation` (placeholders, query
  shape, type and NULL handling, arrays, relative values, `IN` lists). It
  adds identical arguments and errors across drivers and verbatim author SQL
  even when that SQL contains placeholder-like characters.
- `sqlpg-conformance`: the shared conformance suite. Covers offline argument
  checks, skipping without a database, an isolated fixture schema, and the
  row-matching cases every PostgreSQL driver has to pass.
- `sql-bob-compilation`: the bob driver.
- `sql-sqlbuilder-compilation`: the go-sqlbuilder driver.
- `sql-jet-compilation`: the jet driver.
- `sql-ent-compilation`: the ent driver.
- `sql-gorm-compilation`: the gorm driver.

### Modified Capabilities

- `sql-goqu-compilation`: its PostgreSQL requirements move to
  `contract-sql-postgres`. What remains is the goqu driver's own guarantee:
  it satisfies that contract, depends only on goqu, and keeps its output
  unchanged.

## Impact

- **easylist-filter**: no changes.
- **easylist-core**: no code changes. Its README lists the available
  drivers.
- **easylist-sqlpg** (new): the lowering code moved out of
  `easylist-sql-goqu/predicate.go` and `lower.go`, the `conformance` package
  with its fixture schema, and a README covering how to run the suite against
  a local PostgreSQL. Its `go.mod` requires a PostgreSQL `database/sql`
  driver, which only the `conformance` package imports.
- **easylist-sql-goqu**: `driver.go` maps the lowered query onto goqu.
  `predicate.go` and `lower.go` shrink to that mapping. Validation tests move
  to `easylist-sqlpg`, and the golden SQL tests stay. Its README links the
  shared semantics.
- **easylist-sql-bob**, **easylist-sql-sqlbuilder**, **easylist-sql-jet**,
  **easylist-sql-ent**, **easylist-sql-gorm** (new): driver, golden SQL tests,
  conformance test, README, and README example test.
- **go.work**: adds the six new modules.
- **easylist-gqlgen** and **easylist-export**: no changes. They use any
  `easylist.Driver`.
- New third-party dependencies, each confined to its own module:
  `github.com/stephenafamo/bob`, `github.com/huandu/go-sqlbuilder`,
  `github.com/go-jet/jet/v2`, `entgo.io/ent`, and `gorm.io/gorm`, plus a
  PostgreSQL `database/sql` driver for `easylist-sqlpg/conformance`.
