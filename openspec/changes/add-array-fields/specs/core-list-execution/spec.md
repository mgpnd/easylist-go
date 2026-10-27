# Spec Delta

## ADDED Requirements

### Requirement: Array row values
An array field's row value SHALL be a Go slice of its element type's fixed representation, in the database's element order. A NULL array SHALL be an absent value, and an empty array SHALL be a present, empty slice.

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
- **WHEN** timestamp array field `seen_at` holds `{2026-10-25 10:00:00+02}`
- **THEN** the row value holds the instant `2026-10-25T08:00:00Z`

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
