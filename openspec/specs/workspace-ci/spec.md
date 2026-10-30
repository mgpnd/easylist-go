# workspace-ci Specification

## Purpose

Checks every module of the Go workspace automatically on each change, including the PostgreSQL drivers' live conformance cases, so a regression in any module fails before it reaches `main`.

## Requirements

### Requirement: Runs on every change
CI SHALL run on every push to `main` and on every pull request to `main`, and SHALL be startable by hand. A failed check SHALL show the commit or pull request as failed.

#### Scenario: Pull request
- **WHEN** a pull request to `main` is opened or updated
- **THEN** CI runs on its head commit and reports a pass or fail status on the pull request

#### Scenario: Push to main
- **WHEN** a commit is pushed to `main`
- **THEN** CI runs on that commit

### Requirement: Every workspace module is checked
CI SHALL run `go vet` and the race-enabled tests on every package of every module listed in `go.work`, in workspace mode. A module added to `go.work` SHALL be checked without editing CI.

#### Scenario: All modules
- **WHEN** CI runs
- **THEN** the vet and test steps cover every package of every module that `go.work` lists

#### Scenario: New module
- **WHEN** a module is added to `go.work` and CI runs
- **THEN** its packages are vetted and tested along with the others

#### Scenario: Failing test
- **WHEN** a test fails, a vet check fails, or a race is detected in any module
- **THEN** CI fails

### Requirement: Live conformance cases run on a real database
CI SHALL provide a PostgreSQL database that meets the conformance suite's requirements (version 16 and a role that can create schemas and types) and SHALL pass it in `EASYLIST_PG_DSN` to the tests, so every driver's live conformance cases run.

#### Scenario: Driver matches the wrong rows
- **WHEN** a change makes a driver's query return rows that differ from a conformance case's expected rows, page order, or count
- **THEN** that case's live subtest fails and CI fails

### Requirement: Skipped live cases fail CI
CI SHALL fail when any live conformance case is skipped, so a missing or unusable database setting cannot produce a passing run.

#### Scenario: DSN missing
- **WHEN** CI runs the tests without `EASYLIST_PG_DSN` reaching them
- **THEN** CI fails and says that live conformance cases were skipped

### Requirement: Uses only the repository
CI SHALL need no repository secrets and no services other than the public actions and the public PostgreSQL image it uses, and SHALL request only read access to the repository contents.

#### Scenario: Pull request from a fork
- **WHEN** a pull request comes from a fork, where secrets are unavailable
- **THEN** CI runs the same checks, including the live conformance cases
