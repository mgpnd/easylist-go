# sql-bob-compilation Specification

## Purpose

Compiles easylist query plans into prepared PostgreSQL queries using bob's PostgreSQL query builder, for applications that build queries with bob and should not need another query library.

## Requirements

### Requirement: PostgreSQL contract with bob
The bob driver SHALL satisfy `contract-sql-postgres` and pass the shared conformance suite.

#### Scenario: Conformance suite
- **WHEN** the bob driver's tests run the conformance suite
- **THEN** every offline case passes, and every database case passes when `EASYLIST_PG_DSN` is set

### Requirement: No other query libraries
The bob driver package, without its tests, SHALL NOT import, directly or transitively, goqu, the query library of another easylist driver, or a PostgreSQL wire driver. Its tests MAY import more.

#### Scenario: Compiled-in packages
- **WHEN** the transitive imports of the bob driver package, without tests, are listed
- **THEN** they include bob packages and no goqu, go-sqlbuilder, jet, ent, gorm, or PostgreSQL wire driver package

### Requirement: Construction without configuration
The bob driver SHALL be constructed without arguments and SHALL hold no state between compilations.

#### Scenario: Shared value
- **WHEN** an application constructs the bob driver once and uses it in a runtime for every request
- **THEN** every compilation behaves as with a newly constructed driver

### Requirement: Author SQL outside bob placeholders
The bob driver SHALL never pass list-author SQL through a bob construct that reads `?` as a placeholder.

#### Scenario: Question mark in a join
- **WHEN** a plan has join `LEFT JOIN prefs p ON p.data ? 'beta'` and filter `name = 'Ann'`
- **THEN** the join text appears unchanged and `Ann` is bound to `$1`
