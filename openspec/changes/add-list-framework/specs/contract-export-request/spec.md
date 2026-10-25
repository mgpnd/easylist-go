# Spec Delta

## Purpose

Defines the export request that travels from the GraphQL mutation to the application's job system and back into the export worker, and the interface through which the application accepts export jobs.

## ADDED Requirements

### Requirement: Export request contents
An export request SHALL contain the list name, the ordered field names to export, the filter text (possibly empty), the sort entries, and the file format (CSV or XLSX). It SHALL NOT contain a limit, an offset, or the list's scope.

#### Scenario: Request built from a mutation
- **WHEN** a client asks to export fields `name, country` of `users` filtered by `name CONTAINS 'a'` as XLSX
- **THEN** the export request holds list `users`, fields `[name, country]`, that filter text, the sort entries, and format XLSX

### Requirement: Export request serialization
An export request SHALL round-trip through JSON without loss, so an application can store it in a queue or database and hand it to a worker in another process.

#### Scenario: JSON round trip
- **WHEN** an export request is encoded to JSON and decoded again
- **THEN** the decoded request equals the original

### Requirement: Export request validation
An export request SHALL be validated against the list definition before it is handed to the application: the list SHALL exist, at least one field SHALL be named, every field SHALL exist (SQL or calculated) and appear once, the filter SHALL parse and plan, and the sort SHALL plan.

#### Scenario: Unknown export field
- **WHEN** an export request names field `nickname` that `users` lacks
- **THEN** validation fails and the application's exporter is not called

#### Scenario: Calculated field allowed
- **WHEN** an export request names calculated field `display`
- **THEN** validation succeeds

### Requirement: Application-owned exporter
The application SHALL accept validated export requests through an exporter interface that returns a job identifier string or an error. The library SHALL NOT queue jobs, run them in the background, track their status, or store files.

#### Scenario: Exporter accepts a job
- **WHEN** a validated export request is passed to the application's exporter and it returns `job-42`
- **THEN** the caller receives job identifier `job-42`

#### Scenario: Exporter rejects a job
- **WHEN** the application's exporter returns an error
- **THEN** the caller receives that error and no job identifier
