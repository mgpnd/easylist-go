# Spec Delta

## MODIFIED Requirements

### Requirement: Typed row values
Each returned row SHALL contain one value per planned SQL field, keyed by field name. Values SHALL have a fixed Go representation per type: text, numeric, uuid, and enum as string; integer as int64; float as float64; boolean as bool; date and timestamp as time.Time. A timestamp value read from an SQL field SHALL be the database's instant in UTC, whatever zone the database handle scans it in. Date values and calculated values SHALL NOT be converted. A SQL NULL SHALL be returned as an absent value.

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

#### Scenario: Timestamp scanned in a local zone
- **WHEN** timestamp field `created_at` holds `2026-10-25 10:00:00+02` and the database handle scans it as `2026-10-25T11:00:00+03:00`
- **THEN** the row value is `2026-10-25T08:00:00Z` with location UTC

#### Scenario: Date keeps its calendar day
- **WHEN** date field `due` holds `2026-10-25` and the database handle scans it as midnight in UTC+03:00
- **THEN** the row value is still on 2026-10-25

#### Scenario: Calculated timestamp is not converted
- **WHEN** calculated timestamp field `next_call` returns `2026-10-25T11:00:00+03:00`
- **THEN** its value is `2026-10-25T11:00:00+03:00`, in the zone the function returned

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
