# Spec Delta

## MODIFIED Requirements

### Requirement: Plan contents
A plan given to a driver SHALL describe the base table and alias, the ordered joins with their SQL text, the selected fields with their SQL expression and type, the optional scope, the filter syntax tree resolved against field definitions, the ordering, the optional limit and offset, the reference time with its time zone, and the first day of the week. A count plan SHALL be marked so the driver produces a count query.

#### Scenario: Driver receives resolved fields
- **WHEN** a plan filters on `country` with SQL `c.name`
- **THEN** the driver can obtain the expression `c.name` and type text for `country` without consulting the list definition

#### Scenario: Driver receives the reference time
- **WHEN** a request with reference time 2026-10-27T10:00:00+02:00 and week start Sunday is planned
- **THEN** the plan carries that reference time, its UTC+02:00 zone, and Sunday

### Requirement: Literal conversion per type
A driver SHALL convert each filter literal to its field's type and reject mismatches with an error at the literal's position: text, uuid, enum need a string; date and timestamp need a string or a relative time value; integer, float, numeric need a number; boolean needs `TRUE` or `FALSE`. Integers SHALL fit 64 bits, floats SHALL be finite, numerics SHALL keep exact digits, dates SHALL be `YYYY-MM-DD`, timestamps SHALL be RFC 3339 with an offset, uuids SHALL be canonical hyphenated hex, and enums SHALL be a declared value. A relative time value SHALL be evaluated with the plan's reference time and week start; a timestamp-valued one SHALL be rejected for a date field, and an evaluation error SHALL be reported at the literal's position. Array fields SHALL reject relative time values.

#### Scenario: Enum value not declared
- **WHEN** a filter is `status = 'archived'` and `status` declares only `active` and `blocked`
- **THEN** compilation fails with an error naming `archived` at the literal's position

#### Scenario: Timestamp without offset
- **WHEN** a filter is `created_at > '2026-10-25T10:00:00'`
- **THEN** compilation fails because the timestamp has no offset

#### Scenario: Invalid uuid
- **WHEN** a filter is `id = 'not-a-uuid'` for uuid field `id`
- **THEN** compilation fails at the literal's position

#### Scenario: Relative value on a non-time field
- **WHEN** a filter is `age > TODAY` for integer field `age`
- **THEN** compilation fails at the literal's position

#### Scenario: Timestamp value on a date field
- **WHEN** a filter is `due = NOW` for date field `due`
- **THEN** compilation fails at the literal's position because a timestamp cannot be compared with a date

#### Scenario: Folded number
- **WHEN** a filter is `qty > {2 * 3}` for integer field `qty`
- **THEN** compilation succeeds as if the filter were `qty > 6`

## ADDED Requirements

### Requirement: Relative time semantics
A date-valued relative time value compared with a date field SHALL compare with that date. A timestamp-valued one compared with a timestamp field SHALL compare with that instant. A date-valued value `D` compared with a timestamp field SHALL cover the whole day `D` in the reference time's zone: with `S` the instant at which `D` starts and `E` the instant at which the next day starts, `> D` SHALL match values `>= E`, `>= D` values `>= S`, `< D` values `< S`, `<= D` values `< E`, `= D` values in `[S, E)`, and `!= D` every row that `= D` does not match, including NULL.

#### Scenario: Whole last day included
- **WHEN** a filter is `created_at <= END_OF_WEEK` for a timestamp field, evaluated on Tuesday 2026-10-27 in UTC with a Monday week start, and a row has `created_at` 2026-11-01T23:59:59Z
- **THEN** that row matches

#### Scenario: Next day excluded
- **WHEN** the same filter meets a row with `created_at` 2026-11-02T00:00:00Z
- **THEN** that row does not match

#### Scenario: Equal to a day
- **WHEN** a filter is `created_at = TODAY` evaluated on 2026-10-27 in UTC+02:00
- **THEN** rows match when `created_at` is at or after 2026-10-26T22:00:00Z and before 2026-10-27T22:00:00Z

#### Scenario: Not equal to a day matches NULL
- **WHEN** a filter is `created_at != TODAY` and a row has `created_at` NULL
- **THEN** that row matches

#### Scenario: Date field
- **WHEN** a filter is `due > {TODAY + 5::DAYS}` for date field `due`, evaluated on 2026-10-27
- **THEN** rows match when `due` is after 2026-11-01
