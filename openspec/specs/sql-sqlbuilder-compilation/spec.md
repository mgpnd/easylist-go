# sql-sqlbuilder-compilation Specification

## Purpose

Compiles easylist query plans into prepared PostgreSQL queries using go-sqlbuilder's PostgreSQL flavor, for applications that build queries with go-sqlbuilder and should not need another query library.

## Requirements

### Requirement: PostgreSQL contract with go-sqlbuilder
The go-sqlbuilder driver SHALL satisfy `contract-sql-postgres` and pass the shared conformance suite.

#### Scenario: Conformance suite
- **WHEN** the go-sqlbuilder driver's tests run the conformance suite
- **THEN** every offline case passes, and every database case passes when `EASYLIST_PG_DSN` is set

### Requirement: No other query libraries
The go-sqlbuilder driver package, without its tests, SHALL NOT import, directly or transitively, goqu, the query library of another easylist driver, or a PostgreSQL wire driver. Its tests MAY import more.

#### Scenario: Compiled-in packages
- **WHEN** the transitive imports of the go-sqlbuilder driver package, without tests, are listed
- **THEN** they include go-sqlbuilder packages and no goqu, bob, jet, ent, gorm, or PostgreSQL wire driver package

### Requirement: Construction without configuration
The go-sqlbuilder driver SHALL be constructed without arguments and SHALL hold no state between compilations.

#### Scenario: Shared value
- **WHEN** an application constructs the go-sqlbuilder driver once and uses it in a runtime for every request
- **THEN** every compilation behaves as with a newly constructed driver

### Requirement: Author SQL with dollar signs
The go-sqlbuilder driver SHALL emit list-author SQL containing `$` unchanged, never interpreting it as a go-sqlbuilder reference or placeholder.

#### Scenario: Dollar in a scope
- **WHEN** a plan has scope `u.code !~ '^\$'` and filter `name = 'Ann'`
- **THEN** the scope text appears unchanged and `Ann` is bound to `$1`
