# Spec Delta

## MODIFIED Requirements

### Requirement: Live conformance cases run on a real database
CI SHALL provide a PostgreSQL database that meets the conformance suite's requirements (version 16 and a role that can create schemas and types) and SHALL pass it in `EASYLIST_PG_DSN` to the tests, so every driver's live conformance cases and the behaviour snapshot tests run.

#### Scenario: Driver matches the wrong rows
- **WHEN** a change makes a driver's query return rows that differ from a conformance case's expected rows, page order, or count
- **THEN** that case's live subtest fails and CI fails

#### Scenario: Behaviour snapshot differs
- **WHEN** a change alters a behaviour snapshot's outcome and its outcome file is not refreshed
- **THEN** the snapshot test fails and CI fails

### Requirement: Skipped live cases fail CI
CI SHALL fail when any live conformance case or behaviour snapshot test is skipped for lack of a database, so a missing or unusable database setting cannot produce a passing run.

#### Scenario: DSN missing
- **WHEN** CI runs the tests without `EASYLIST_PG_DSN` reaching them
- **THEN** CI fails and says that tests needing PostgreSQL were skipped
