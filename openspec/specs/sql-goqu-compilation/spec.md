# sql-goqu-compilation Specification

## Purpose

Compiles easylist query plans into prepared PostgreSQL queries using goqu, satisfying the SQL driver contract for applications that choose PostgreSQL.

## Requirements

### Requirement: PostgreSQL contract with goqu
The goqu driver SHALL satisfy `contract-sql-postgres` and pass the shared conformance suite.

#### Scenario: Conformance suite
- **WHEN** the goqu driver's tests run the conformance suite
- **THEN** every offline case passes, and every database case passes when `EASYLIST_PG_DSN` is set

### Requirement: No other query libraries
The goqu driver package, without its tests, SHALL NOT import, directly or transitively, the query library of another easylist driver or a PostgreSQL wire driver. Its tests MAY import more.

#### Scenario: Compiled-in packages
- **WHEN** the transitive imports of the goqu driver package, without tests, are listed
- **THEN** they include goqu packages and no bob, go-sqlbuilder, jet, ent, gorm, or PostgreSQL wire driver package

### Requirement: Unchanged goqu output
Moving the PostgreSQL rules into the shared module SHALL NOT change the goqu driver's SQL text, arguments, or error text for any plan.

#### Scenario: README example
- **WHEN** the goqu README example plan is compiled
- **THEN** the SQL and arguments are exactly those the README shows

#### Scenario: Existing golden tests
- **WHEN** the goqu driver's golden SQL and error tests from before this change run against the refactored driver
- **THEN** they pass without changes to their expected values
