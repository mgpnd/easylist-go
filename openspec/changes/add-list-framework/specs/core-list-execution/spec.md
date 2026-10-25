# Spec Delta

## Purpose

Runs planned list queries against the application's database, returns typed rows and a total count, and evaluates calculated fields one row at a time.

## ADDED Requirements

### Requirement: Database access through the application
The library SHALL execute queries only through a database handle supplied by the application, and SHALL NOT open connections itself. An adapter SHALL let a standard `database/sql` handle serve as that database handle.

#### Scenario: Application supplies the handle
- **WHEN** an application runs a list request with its own `database/sql` handle
- **THEN** the page and count queries run on that handle with the driver's SQL and arguments

### Requirement: Typed row values
Each returned row SHALL contain one value per planned SQL field, keyed by field name. Values SHALL have a fixed Go representation per type: text, numeric, uuid, and enum as string; integer as int64; float as float64; boolean as bool; date and timestamp as time.Time. A SQL NULL SHALL be returned as an absent value.

#### Scenario: Nullable field holds NULL
- **WHEN** nullable text field `country` is NULL for a row
- **THEN** the row reports `country` as absent rather than an empty string

#### Scenario: Numeric precision preserved
- **WHEN** a numeric field holds `12345678901234567890.123`
- **THEN** the row value is the string `12345678901234567890.123`

#### Scenario: NULL in a non-nullable field
- **WHEN** a field declared non-nullable returns NULL
- **THEN** the request fails with an error naming the field

### Requirement: Page result
Running a list request SHALL return the page rows in plan order. The total count SHALL be computed with the count plan only when the caller asks for it, and the page query SHALL run only when the caller asks for rows.

#### Scenario: Count not requested
- **WHEN** a caller requests rows but not the total count
- **THEN** only the page query runs

#### Scenario: Only count requested
- **WHEN** a caller requests the total count but not the rows
- **THEN** only the count query runs

### Requirement: Calculated field evaluation
A calculated field SHALL be evaluated by calling its function once per row, with the request context and that row, only when the field is needed. Its function SHALL be able to read every SQL field it declared as required. An error from the function SHALL fail evaluation of that field for that row.

#### Scenario: Calculated field reads its requirements
- **WHEN** `display` requires `name` and `country` and is evaluated on a row with `name = 'Ann'` and `country` NULL
- **THEN** the function sees `name` as `Ann` and `country` as absent

#### Scenario: Calculated field not selected
- **WHEN** a request does not select `display`
- **THEN** the `display` function is never called

### Requirement: Query errors
Database errors SHALL be returned to the caller wrapped with the list name. The wrapping SHALL NOT add bound argument values to the message.

#### Scenario: Database rejects the query
- **WHEN** the database returns an error for a page query on list `users`
- **THEN** the returned error names `users` and wraps the database error
