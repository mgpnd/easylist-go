# Spec Delta

## MODIFIED Requirements

### Requirement: Request validation
A list request SHALL name the selected fields, and MAY carry a filter syntax tree, sort entries, a limit, an offset, a reference time, and a first day of the week. Every selected name SHALL be a field of the list. Planning SHALL fail with an error naming the first unknown field.

#### Scenario: Unknown selected field
- **WHEN** a request selects `nickname` and the list has no such field
- **THEN** planning fails with an error naming `nickname`

#### Scenario: No selected fields
- **WHEN** a request selects no fields
- **THEN** planning succeeds and the page plan selects only the primary key

## ADDED Requirements

### Requirement: Expression evaluation in the planner
The planner SHALL type-check and evaluate every expression value in the filter, using the request's reference time with its time zone and the request's first day of the week, or Monday when it has none. The plan SHALL carry each result, so drivers never evaluate expressions. The planner SHALL NOT read the system clock. Every error SHALL name the position of the node that caused it.

#### Scenario: Default week start
- **WHEN** a request with a reference time, no week start, and filter `created_at >= START_OF_WEEK` is planned
- **THEN** the plan's value for that predicate is the Monday of the reference week

#### Scenario: Missing reference time
- **WHEN** a request with filter `due = TODAY` and no reference time is planned
- **THEN** planning fails with an error at position 7

#### Scenario: Numbers need no reference time
- **WHEN** a request with filter `qty > {2 * 3}` and no reference time is planned
- **THEN** planning succeeds and the plan's value for that predicate is the number `6`

### Requirement: Expression value types
An expression SHALL have one of the types number, duration, date, or timestamp. Number literals are numbers, durations are durations, `NOW` is a timestamp, and the other keywords are dates. An expression's result SHALL be a number or a time; a bare duration SHALL be an error at the `{` position.

#### Scenario: Bare duration
- **WHEN** a request with filter `due > {5::DAYS}` is planned
- **THEN** planning fails with an error at position 7

### Requirement: Number arithmetic
Number `+`, `-`, `*`, `/` number SHALL give a number, calculated exactly. Division by zero, or a quotient with no finite decimal form, SHALL be an error at the position of `/`. The result SHALL be handed to the driver as an exact decimal text in plain notation, with no exponent and no trailing fractional zeros.

#### Scenario: Exact decimals
- **WHEN** a request with filter `price > {10 / 4 - 0.1}` is planned
- **THEN** the plan's value is the number `2.4`

#### Scenario: Non-terminating quotient
- **WHEN** a request with filter `price > {1 / 3}` is planned
- **THEN** planning fails with an error at the position of `/`

#### Scenario: Division by zero
- **WHEN** a request with filter `price > {1 / (2 - 2)}` is planned
- **THEN** planning fails with an error at the position of `/`

### Requirement: Duration arithmetic
A duration SHALL be an ordered list of unit amounts. Duration `+` or `-` duration SHALL concatenate the lists, negating the right one for `-`, for the same or different units. Duration `*` integer and integer `*` duration SHALL scale every amount, and duration `/` integer SHALL divide every amount and fail unless each divides exactly. Unary `-` SHALL negate every amount. Each amount SHALL be at most 10000000 in absolute value.

#### Scenario: Mixed units
- **WHEN** a request with filter `t > {NOW + (1::DAYS + 2::HOURS)}` is planned
- **THEN** planning succeeds with the same value as `t > {NOW + 1::DAYS + 2::HOURS}`

#### Scenario: Inexact duration division
- **WHEN** a request with filter `t > {NOW + (2::DAYS + 3::HOURS) / 2}` is planned
- **THEN** planning fails with an error at the position of `/`

### Requirement: Time arithmetic
A date or timestamp `+` duration, duration `+` date or timestamp, and date or timestamp `-` duration SHALL give a time. A date stays a date only if every amount of the duration is in `DAYS` or `BUSINESS_DAYS`; otherwise the result is a timestamp. Every other operation involving a time or a duration SHALL be an error at the operator's position.

#### Scenario: Time minus time
- **WHEN** a request with filter `t > {NOW - TODAY}` is planned
- **THEN** planning fails with an error at the position of `-`

#### Scenario: Date plus hours
- **WHEN** a request with filter `t > {TODAY + 9::HOURS}` is planned
- **THEN** the plan's value is a timestamp

### Requirement: Expression type per field
A number result SHALL be accepted only for integer, float, and numeric fields. A date result SHALL be accepted only for date and timestamp fields. A timestamp result SHALL be accepted only for timestamp fields. Array fields SHALL reject expression values. A mismatch SHALL be an error at the literal's position.

#### Scenario: Time on a number field
- **WHEN** a request with filter `age > TODAY` for integer field `age` is planned
- **THEN** planning fails at position 7

#### Scenario: Timestamp on a date field
- **WHEN** a request with filter `due = NOW` for date field `due` is planned
- **THEN** planning fails at position 7 because a timestamp cannot be compared with a date

### Requirement: NOW and TODAY meaning
Given a reference time `now` in time zone `Z`, `NOW` SHALL evaluate to the instant `now` and `TODAY` to the date of `now` in `Z`. A date SHALL be represented as midnight at the start of that date in `Z`.

#### Scenario: Time zone decides the date
- **WHEN** `TODAY` is evaluated with reference time 2026-10-27T23:30:00Z expressed in UTC+02:00
- **THEN** the result is 2026-10-28, represented as 2026-10-28T00:00:00+02:00

### Requirement: Week keyword meaning
Given a first day of the week `W`, `START_OF_WEEK` SHALL evaluate to the latest date on or before `TODAY` that falls on `W`, and `END_OF_WEEK` to six days after `START_OF_WEEK`.

#### Scenario: Week with Monday start
- **WHEN** `START_OF_WEEK` and `END_OF_WEEK` are evaluated with reference time Tuesday 2026-10-27T10:00:00Z and first day Monday
- **THEN** the results are 2026-10-26 and 2026-11-01

#### Scenario: Week with Sunday start
- **WHEN** `START_OF_WEEK` is evaluated with reference time Tuesday 2026-10-27T10:00:00Z and first day Sunday
- **THEN** the result is 2026-10-25

### Requirement: Month and year keyword meaning
`START_OF_MONTH` and `END_OF_MONTH` SHALL evaluate to the first and last date of `TODAY`'s month. `START_OF_YEAR` and `END_OF_YEAR` SHALL evaluate to January 1 and December 31 of `TODAY`'s year.

#### Scenario: End of February
- **WHEN** `END_OF_MONTH` is evaluated with reference time 2028-02-10T00:00:00Z
- **THEN** the result is 2028-02-29

#### Scenario: End of year
- **WHEN** `END_OF_YEAR` is evaluated with reference time 2026-10-27T10:00:00Z
- **THEN** the result is 2026-12-31

### Requirement: Applying durations
Operations SHALL be applied left to right as written, and a duration's amounts one at a time in their order. `n::DAYS` SHALL move by `n` calendar days in `Z`, keeping the wall-clock time of a timestamp. `n::HOURS` and `n::MINUTES` SHALL move by elapsed time. A result outside the years 1 to 9999 SHALL be an error at the `{` position.

#### Scenario: Calendar days across a DST change
- **WHEN** `{NOW + 1::DAYS}` is evaluated at 2026-10-24T12:00 in a zone whose clocks go back one hour on 2026-10-25
- **THEN** the result is 2026-10-25T12:00 in that zone, 25 hours later

#### Scenario: Mixed-unit duration
- **WHEN** `{NOW + (1::DAYS + 2::HOURS)}` is evaluated at 2026-10-27T10:00:00Z
- **THEN** the result is 2026-10-28T12:00:00Z

#### Scenario: Out of range
- **WHEN** `{TODAY + 10000000::DAYS}` is evaluated in 2026
- **THEN** planning fails with an error

### Requirement: Business day durations
`n::BUSINESS_DAYS` SHALL move to the `n`th Monday-to-Friday date after the start date for positive `n`, or before it for negative `n`, keeping the wall-clock time of a timestamp. `0::BUSINESS_DAYS` SHALL move a Saturday or Sunday forward to the next Monday and leave a weekday unchanged.

#### Scenario: Business days over a weekend
- **WHEN** `{TODAY + 3::BUSINESS_DAYS}` is evaluated on Thursday 2026-10-29
- **THEN** the result is Tuesday 2026-11-03

#### Scenario: Negative business days
- **WHEN** `{TODAY - 1::BUSINESS_DAYS}` is evaluated on Monday 2026-11-02
- **THEN** the result is Friday 2026-10-30

#### Scenario: Zero business days on a weekend
- **WHEN** `{TODAY + 0::BUSINESS_DAYS}` is evaluated on Saturday 2026-10-31
- **THEN** the result is Monday 2026-11-02
