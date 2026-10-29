# sql-gorm-compilation Specification

## Purpose

Compiles easylist query plans into prepared PostgreSQL queries using gorm's statement builder, for applications that use gorm and should not need another query library or a second PostgreSQL driver.

## Requirements

### Requirement: PostgreSQL contract with gorm
The gorm driver SHALL satisfy `contract-sql-postgres` and pass the shared conformance suite.

#### Scenario: Conformance suite
- **WHEN** the gorm driver's tests run the conformance suite
- **THEN** every offline case passes, and every database case passes when `EASYLIST_PG_DSN` is set

### Requirement: No other query libraries
The gorm driver package, without its tests, SHALL NOT import, directly or transitively, goqu, the query library of another easylist driver, gorm's PostgreSQL dialector, or a PostgreSQL wire driver. Its tests MAY import more.

#### Scenario: Compiled-in packages
- **WHEN** the transitive imports of the gorm driver package, without tests, are listed
- **THEN** they include gorm packages and no goqu, bob, go-sqlbuilder, jet, ent, `gorm.io/driver/postgres`, pgx, or other PostgreSQL wire driver package

### Requirement: Construction without a gorm handle
The gorm driver SHALL be constructed without arguments, without a gorm database handle, and SHALL hold no state between compilations, so the application's gorm configuration, plugins, and callbacks never change the compiled SQL.

#### Scenario: Application uses gorm with plugins
- **WHEN** an application with its own gorm handle, naming strategy, and plugins constructs the gorm driver
- **THEN** the driver compiles the same SQL and arguments as in a program without any gorm handle

### Requirement: Bound pagination with gorm
The gorm driver SHALL bind limit and offset as arguments, including an offset of 0.

#### Scenario: First page
- **WHEN** a page plan with limit 50 and offset 0 is compiled
- **THEN** the SQL has an `OFFSET` placeholder and the last argument is `0`
