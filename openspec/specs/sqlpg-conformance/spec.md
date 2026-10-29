# sqlpg-conformance Specification

## Purpose

Provides one conformance suite that every PostgreSQL easylist driver runs from its own tests, so the shared PostgreSQL contract is checked the same way for every driver, including which rows each filter matches on a real database.

## Requirements

### Requirement: One entry point per driver
The suite SHALL be run from a driver's test with that driver as its only input, and SHALL report each case as a named subtest so that a failure names the case and the driver.

#### Scenario: Driver test runs the suite
- **WHEN** a driver module's test calls the suite with its driver
- **THEN** every case runs as a subtest whose name identifies the case

### Requirement: Offline argument checks
The suite SHALL check, without a database, that compiling each case returns the expected arguments with their Go types and order, the expected error text for invalid cases, the same result when compiled twice, and SQL text with exactly one `$n` placeholder number per argument.

#### Scenario: No database configured
- **WHEN** the suite runs with `EASYLIST_PG_DSN` unset
- **THEN** the argument, error, and determinism checks run and pass or fail, and only the database cases are skipped

#### Scenario: Placeholder count mismatch
- **WHEN** a driver returns SQL whose highest placeholder is `$3` with four arguments
- **THEN** the suite fails that case

### Requirement: Live database cases
When `EASYLIST_PG_DSN` is set, the suite SHALL run every case's compiled query against that PostgreSQL database and compare the matching row ids, the page rows in plan order, and the count with the expected results.

#### Scenario: Database configured
- **WHEN** the suite runs with `EASYLIST_PG_DSN` pointing at a reachable PostgreSQL database
- **THEN** each case's query is executed and its row ids and count are compared with the expected ones

#### Scenario: Unreachable database
- **WHEN** `EASYLIST_PG_DSN` is set but the database cannot be reached
- **THEN** the suite fails with an error naming the connection problem instead of skipping

### Requirement: Isolated fixture schema
The suite SHALL create its tables, enum types, and rows in a new schema of its own for each run, use only that schema, and drop it when the run ends, so that several driver test runs can share one database at the same time.

#### Scenario: Parallel driver runs
- **WHEN** two driver modules run the suite at the same time against the same database
- **THEN** both runs pass, and neither leaves a schema behind

### Requirement: Contract coverage
The suite SHALL include cases for every scenario in `contract-sql-driver` and `contract-sql-postgres` that can be shown by arguments, errors, or matching rows, including NULL rows for every negated operator, `IN` and `NOT IN` with and without NULL items, `CONTAINS` escaping, array `CONTAINS` and `EXCLUDES` on NULL and empty arrays, whole-day ranges across a DST change, offset 0, scope with `OR`, and author SQL containing `?`.

#### Scenario: NOT IN keeps NULL rows
- **WHEN** the suite runs the case `status NOT IN ('error')` on the fixture
- **THEN** it expects the rows whose `status` is `success` and NULL, and no others

#### Scenario: Whole day across DST
- **WHEN** the suite runs the case `created_at = TODAY` with a reference time on a DST change day in `Europe/Berlin`
- **THEN** it expects exactly the rows inside that 23- or 25-hour day

#### Scenario: Selected arrays
- **WHEN** the suite runs a page case that selects array fields of text, numeric, date, and timestamp elements
- **THEN** it expects the JSON array text of each row, and SQL NULL for a NULL array

### Requirement: Fixed reference time
The suite SHALL plan every case with an explicit reference time and week start, so its results never depend on the machine's clock or time zone.

#### Scenario: Different host time zone
- **WHEN** the suite runs on two machines with different local time zones
- **THEN** both runs compile the same arguments and expect the same rows
