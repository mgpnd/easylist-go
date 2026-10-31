# core-list-execution Specification

## Purpose

Runs planned list queries against the application's database, returns typed rows and a total count, and evaluates calculated fields one row at a time.

## Requirements

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

#### Scenario: Undeclared enum value
- **WHEN** an enum field returns a value that is not one of its declared values
- **THEN** the request fails with an error naming the field and the value

### Requirement: Timestamp values in UTC
A timestamp value read from an SQL field, scalar or array element, SHALL be the database's instant in UTC, whatever zone the database handle or the database session renders it in. Date values and calculated field values SHALL NOT be converted.

#### Scenario: Timestamp scanned in a local zone
- **WHEN** timestamp field `created_at` holds `2026-10-25 10:00:00+02` and the database handle scans it as `2026-10-25T11:00:00+03:00`
- **THEN** the row value is `2026-10-25T08:00:00Z` with location UTC

#### Scenario: Date keeps its calendar day
- **WHEN** date field `due` holds `2026-10-25` and the database handle scans it as midnight in UTC+03:00
- **THEN** the row value is still on 2026-10-25

#### Scenario: Calculated timestamp is not converted
- **WHEN** calculated timestamp field `next_call` returns `2026-10-25T11:00:00+03:00`
- **THEN** its value is `2026-10-25T11:00:00+03:00`, in the zone the function returned

### Requirement: Page result
Running a list request SHALL return the page rows in plan order. The total count SHALL be computed with the count plan only when the caller asks for it, and the page query SHALL run only when the caller asks for rows.

#### Scenario: Count not requested
- **WHEN** a caller requests rows but not the total count
- **THEN** only the page query runs

#### Scenario: Only count requested
- **WHEN** a caller requests the total count but not the rows
- **THEN** only the count query runs

### Requirement: Calculated field evaluation
A calculated field SHALL be evaluated by calling its function once per row, with the request context and that row, only when the field is needed. Its function SHALL be able to read every SQL field it declared as required. An error from the function, or an enum result that is not a declared value, SHALL fail evaluation of that field for that row.

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

### Requirement: Array row values
An array field's row value SHALL be a Go slice of its element type's fixed representation, in the database's element order. Each timestamp element SHALL be in UTC, as for scalar timestamp values. A NULL array SHALL be an absent value, and an empty array SHALL be a present, empty slice.

#### Scenario: Text array value
- **WHEN** text array field `tags` holds `{go,rust}` for a row
- **THEN** the row value of `tags` is the string slice `go`, `rust`

#### Scenario: Numeric array precision
- **WHEN** numeric array field `prices` holds `{12345678901234567890.10}`
- **THEN** the row value is a string slice holding `12345678901234567890.10`

#### Scenario: Empty array
- **WHEN** nullable array field `tags` holds an empty array for a row
- **THEN** the row reports `tags` as present with no elements

#### Scenario: NULL array
- **WHEN** nullable array field `tags` is NULL for a row
- **THEN** the row reports `tags` as absent

#### Scenario: Timestamp array value
- **WHEN** timestamp array field `seen_at` holds `{2026-10-25 10:00:00+02}` and the database session's time zone is `Asia/Tokyo`
- **THEN** the row value holds `2026-10-25T08:00:00Z` with location UTC

### Requirement: Array element validation
Reading an array field SHALL fail the request with an error naming the field when an element is NULL or cannot be decoded as the element type. Every element of an enum array SHALL be a declared value, and the error for one that is not SHALL name the field and the value.

#### Scenario: NULL element
- **WHEN** text array field `tags` holds `{go,NULL}` for a row
- **THEN** the request fails with an error naming `tags`

#### Scenario: Undeclared enum element
- **WHEN** enum array field `statuses` declares `ok` and `failed` and a row holds `{ok,archived}`
- **THEN** the request fails with an error naming `statuses` and `archived`

#### Scenario: Calculated array with an undeclared enum element
- **WHEN** a calculated enum array field returns an element that is not a declared value
- **THEN** evaluation of that field for that row fails with an error naming the value

### Requirement: Runtime clock and week start
The runtime SHALL have an optional clock and an optional first day of the week. Without a clock it SHALL use the system clock in the server's local time zone. When it runs a request that has no reference time, it SHALL use the clock's current time as the reference time; when the request has no week start, it SHALL use the runtime's, and Monday when the runtime has none. A request's own reference time and week start SHALL take precedence.

#### Scenario: Server local time by default
- **WHEN** a runtime without a clock fetches a request with filter `due = TODAY` and no reference time
- **THEN** `TODAY` is the current date in the server's local time zone

#### Scenario: Application week start
- **WHEN** the runtime's week start is Sunday and a request with filter `created_at >= START_OF_WEEK` and no week start is fetched
- **THEN** the week starts on Sunday

#### Scenario: Request reference time wins
- **WHEN** the runtime has a clock and a request carries reference time 2026-01-01T00:00:00Z
- **THEN** relative values are evaluated at 2026-01-01T00:00:00Z
