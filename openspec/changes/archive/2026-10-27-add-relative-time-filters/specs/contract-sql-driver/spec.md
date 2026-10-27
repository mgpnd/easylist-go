# Spec Delta

## MODIFIED Requirements

### Requirement: Plan contents
A plan given to a driver SHALL describe the base table and alias, the ordered joins with their SQL text, the selected fields with their SQL expression and type, the optional scope, the filter syntax tree resolved against field definitions, the evaluated result of every expression value in the filter, the ordering, and the optional limit and offset. A count plan SHALL be marked so the driver produces a count query.

#### Scenario: Driver receives resolved fields
- **WHEN** a plan filters on `country` with SQL `c.name`
- **THEN** the driver can obtain the expression `c.name` and type text for `country` without consulting the list definition

#### Scenario: Driver receives evaluated values
- **WHEN** a plan filters on `due > {TODAY + 5::DAYS}` with reference time 2026-10-27T10:00:00+02:00
- **THEN** the driver can obtain, for that predicate, the date 2026-11-01 as midnight in UTC+02:00, without evaluating the expression

### Requirement: Literal conversion per type
A driver SHALL convert each filter literal to its field's type and reject mismatches with an error at the literal's position: text, uuid, enum, date, timestamp need a string; integer, float, numeric need a number; boolean needs `TRUE` or `FALSE`. Integers SHALL fit 64 bits, floats SHALL be finite, numerics SHALL keep exact digits, dates SHALL be `YYYY-MM-DD`, timestamps SHALL be RFC 3339 with an offset, uuids SHALL be canonical hyphenated hex, and enums SHALL be a declared value. For an expression literal the driver SHALL use the plan's evaluated result: a number result SHALL follow the number rules above.

#### Scenario: Enum value not declared
- **WHEN** a filter is `status = 'archived'` and `status` declares only `active` and `blocked`
- **THEN** compilation fails with an error naming `archived` at the literal's position

#### Scenario: Timestamp without offset
- **WHEN** a filter is `created_at > '2026-10-25T10:00:00'`
- **THEN** compilation fails because the timestamp has no offset

#### Scenario: Invalid uuid
- **WHEN** a filter is `id = 'not-a-uuid'` for uuid field `id`
- **THEN** compilation fails at the literal's position

#### Scenario: Evaluated number
- **WHEN** a filter is `qty > {2 * 3}` for integer field `qty`
- **THEN** compilation succeeds as if the filter were `qty > 6`

#### Scenario: Evaluated number not an integer
- **WHEN** a filter is `qty > {10 / 4}` for integer field `qty`
- **THEN** compilation fails at the literal's position because `2.5` is not an integer

## ADDED Requirements

### Requirement: Relative time comparison
A date result compared with a date field SHALL compare with that date. A timestamp result compared with a timestamp field SHALL compare with that instant.

#### Scenario: Date field
- **WHEN** a filter is `due > {TODAY + 5::DAYS}` for date field `due`, evaluated on 2026-10-27
- **THEN** rows match when `due` is after 2026-11-01

#### Scenario: Timestamp field
- **WHEN** a filter is `seen_at > {NOW - 30::MINUTES}` evaluated at 2026-10-27T10:00:00Z
- **THEN** rows match when `seen_at` is after 2026-10-27T09:30:00Z

### Requirement: Whole-day comparison on timestamp fields
A date result `D` compared with a timestamp field SHALL cover the whole day `D` in the time zone of the result. With `S` the instant at which `D` starts and `E` the instant at which the next day starts, `> D` SHALL match values `>= E`, `>= D` values `>= S`, `< D` values `< S`, `<= D` values `< E`, `= D` values in `[S, E)`, and `!= D` every row that `= D` does not match, including NULL.

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
