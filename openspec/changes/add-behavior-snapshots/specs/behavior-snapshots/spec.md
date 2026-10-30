# Spec Delta

## Purpose

Shows how easylist behaves as readable input → outcome snapshots of list definitions, filters, and GraphQL requests run against a real PostgreSQL database, so a reviewer can see every behaviour change in a diff of the outcome files.

## ADDED Requirements

### Requirement: Fixtures and outcomes are files
Every snapshot SHALL be a hand-written fixture file paired with a generated outcome file in the module's test data. Fixtures SHALL be list definitions, filter expressions, or GraphQL operations. Outcome files SHALL be text whose content does not depend on map iteration order, the machine, or the current date.

#### Scenario: Same result twice
- **WHEN** the snapshot tests run twice against the same code and fixtures
- **THEN** the outcome files they would write are byte-for-byte identical

### Requirement: Outcome files echo their input
Each case in a filter outcome file SHALL repeat the fixture text it came from, its comments and filter expression, before the case's outcome. A GraphQL outcome SHALL be exactly the response body, so it is paired with its operation by file name instead: `<name>.json` sits beside `<name>.graphql`.

#### Scenario: Reading a diff
- **WHEN** a change alters the rows one filter case matches
- **THEN** the changed lines in the outcome file sit under that case's filter text, without opening the fixture file

#### Scenario: GraphQL pairing
- **WHEN** a change alters the response to `calculated-field-error.graphql`
- **THEN** the diff shows `calculated-field-error.json`, beside the operation file of the same name

### Requirement: Snapshots are compared and refreshed
By default, the snapshot tests SHALL fail when a computed outcome differs from its outcome file, and SHALL report the file and the difference. When run with the `-update` flag, they SHALL rewrite every outcome file instead, delete outcome files whose fixture no longer exists, and pass.

#### Scenario: Behaviour changed
- **WHEN** a code change makes `age > 30` match one more row and the tests run without `-update`
- **THEN** the filter snapshot test fails and names the outcome file

#### Scenario: Refresh
- **WHEN** the tests run with `-update` after that change
- **THEN** the outcome file shows the new row, and a following run without `-update` passes

#### Scenario: Missing outcome file
- **WHEN** a new fixture has no outcome file and the tests run without `-update`
- **THEN** the test fails and tells the developer to run with `-update`

### Requirement: Readable seeded database
The snapshots SHALL run against a seeded PostgreSQL database. Its rows SHALL use human-readable names, so a matched row is recognisable without looking up its id. The data set SHALL be separate from the driver conformance fixture.

#### Scenario: Row identity
- **WHEN** a filter outcome lists a matched row
- **THEN** the row shows its id and its name

### Requirement: Pinned time and isolation
Every snapshot SHALL be computed with a fixed reference time and a UTC database session, so relative values such as `TODAY` give the same result on any date. Each test run SHALL create the fixture in its own new schema and drop it afterwards.

#### Scenario: Different day
- **WHEN** the snapshot tests run on two different calendar days
- **THEN** the outcomes of filters that use `TODAY` or `NOW` are identical

#### Scenario: Shared database
- **WHEN** the snapshot tests and the driver conformance suite use the same database at the same time
- **THEN** neither sees the other's tables

### Requirement: Live database is required
The snapshot tests SHALL connect to the database given in `EASYLIST_PG_DSN`. When the variable is unset, each snapshot test SHALL skip with the message `EASYLIST_PG_DSN is not set`. When it is set and the database cannot be reached, the tests SHALL fail.

#### Scenario: No database
- **WHEN** the tests run without `EASYLIST_PG_DSN`
- **THEN** every snapshot test skips with `EASYLIST_PG_DSN is not set` and no outcome file is changed, even with `-update`

### Requirement: Local database setup
The module SHALL provide a one-command way to start a PostgreSQL 16 database that matches the DSN documented in its README. That DSN SHALL also work for the driver conformance suite.

#### Scenario: Start and run
- **WHEN** a developer starts the provided database and runs the module's tests with the documented `EASYLIST_PG_DSN`
- **THEN** the snapshot tests run against it and none skip

### Requirement: List definition snapshots
For every fixture list, the outcome SHALL contain the GraphQL SDL that the gqlgen plugin generates for it, and a matrix that, for every field and every filter operator, shows whether a request filtering that field with that operator is accepted, or the error it is rejected with, as validated before any PostgreSQL driver builds SQL.

#### Scenario: Operator matrix
- **WHEN** the fixture list has a boolean field `active`
- **THEN** the matrix row for `active` marks `=` as accepted and `>` as rejected with its error message

#### Scenario: New field
- **WHEN** a field is added to a fixture list
- **THEN** the list's outcome file gains its SDL field and its matrix row

### Requirement: Filter snapshots
Filter fixtures SHALL be grouped by operator family, one expression per case. For each case the outcome SHALL show the matched row count and the rows in primary-key order, each with its id, its name, and the values of the fields the filter references. A case that fails to parse or plan SHALL show the error message, including its position.

#### Scenario: Matching rows
- **WHEN** a case's filter is `age > 30`
- **THEN** the outcome lists each matching row's id, name, and age, and the count

#### Scenario: Error case
- **WHEN** a case's filter is `active > TRUE`
- **THEN** the outcome shows the planner's error with its position, and no rows

#### Scenario: NULL values
- **WHEN** a matched row has NULL in a referenced field
- **THEN** the outcome shows that value as `NULL`

### Requirement: Generated SQL is a separate snapshot
For each filter case that plans successfully, the generated SQL and bound arguments SHALL be recorded in a separate outcome file from the matched rows, so a change to SQL alone does not change the rows file.

#### Scenario: SQL-only refactor
- **WHEN** a change rewrites the SQL a filter compiles to without changing the rows it matches
- **THEN** only the SQL outcome file changes

### Requirement: GraphQL request snapshots
Each GraphQL fixture SHALL be one operation file. Its outcome SHALL be the response that the module's GraphQL server returns for it: the `data` and `errors` members, as indented JSON in the server's field order.

#### Scenario: Query with a calculated field
- **WHEN** an operation selects `items { name display }` and `totalCount`
- **THEN** the outcome holds the rows' names, their calculated values, and the total

#### Scenario: Request error
- **WHEN** an operation's filter has a syntax error
- **THEN** the outcome holds the GraphQL error with its message and path, and `data` as the server returned it

### Requirement: Field types are covered
The fixture lists SHALL together declare at least one field of every field type as a scalar and as an array, a nullable and a non-nullable field, a joined field, and a calculated field. A test SHALL fail and name what is missing otherwise.

#### Scenario: New field type
- **WHEN** a new field type is added to easylist and no fixture list declares it
- **THEN** the coverage test fails and names the type and whether the scalar or the array form is missing

### Requirement: Filter features are covered
The filter fixtures SHALL together use every predicate operator, both logical connectors, every time keyword, every duration unit, and every arithmetic operator. A test SHALL fail and name what is missing otherwise.

#### Scenario: New operator
- **WHEN** a new filter operator is added and no filter case uses it
- **THEN** the coverage test fails and names the operator
