# contract-export-request Specification

## Purpose

Defines the export request that travels from the GraphQL mutation to the application's job system and back into the export worker, and the interface through which the application accepts export jobs.

## Requirements

### Requirement: Export request contents
An export request SHALL contain the list name, the ordered field names to export, the filter text (possibly empty), the sort entries, the file format (CSV or XLSX), the moment at which relative time values are evaluated (possibly unset), and the client's UTC offset in minutes (possibly unset). It SHALL NOT contain a limit, an offset, or the list's scope.

#### Scenario: Request built from a mutation
- **WHEN** a client asks to export fields `name, country` of `users` filtered by `name CONTAINS 'a'` as XLSX with timezone `120`
- **THEN** the export request holds list `users`, fields `[name, country]`, that filter text, the sort entries, format XLSX, the moment of the request, and offset `120`

### Requirement: Export request serialization
An export request SHALL round-trip through JSON without loss, so an application can store it in a queue or database and hand it to a worker in another process. The request moment SHALL be stored in UTC so the decoded value equals the original, and unset optional fields SHALL be omitted.

#### Scenario: JSON round trip
- **WHEN** an export request is encoded to JSON and decoded again
- **THEN** the decoded request equals the original

#### Scenario: Request from before this change
- **WHEN** JSON without `asOf` or `timezone` is decoded
- **THEN** the request has no request moment and no offset

### Requirement: Export request validation
An export request SHALL be validated against the list definition before it is handed to the application: the list SHALL exist, at least one field SHALL be named, every field SHALL exist (SQL or calculated) and appear once, the filter SHALL parse and plan, the sort SHALL plan, and the offset, when set, SHALL be between -720 and 840 minutes.

#### Scenario: Unknown export field
- **WHEN** an export request names field `nickname` that `users` lacks
- **THEN** validation fails and the application's exporter is not called

#### Scenario: Calculated field allowed
- **WHEN** an export request names calculated field `display`
- **THEN** validation succeeds

#### Scenario: Offset out of range
- **WHEN** an export request has offset `900`
- **THEN** validation fails with an error stating the allowed range

### Requirement: Application-owned exporter
The application SHALL accept validated export requests through an exporter interface that returns a job identifier string or an error. The library SHALL NOT queue jobs, run them in the background, track their status, or store files.

#### Scenario: Exporter accepts a job
- **WHEN** a validated export request is passed to the application's exporter and it returns `job-42`
- **THEN** the caller receives job identifier `job-42`

#### Scenario: Exporter rejects a job
- **WHEN** the application's exporter returns an error
- **THEN** the caller receives that error and no job identifier
