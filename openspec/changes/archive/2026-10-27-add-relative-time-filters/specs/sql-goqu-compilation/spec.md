# Spec Delta

## ADDED Requirements

### Requirement: No database clock for relative values
The driver SHALL bind the plan's evaluated result of each expression value as an argument, and SHALL NOT evaluate expressions itself. The SQL SHALL NOT use `now()`, `CURRENT_DATE`, or any other database clock or time zone function.

#### Scenario: No database clock
- **WHEN** the filter `created_at > {NOW - 30::MINUTES}` is compiled
- **THEN** the SQL contains no `now()` and the value is a bound argument

#### Scenario: Recompiling moves the window
- **WHEN** the same parsed filter `due = TODAY` is planned and compiled with two reference times on different dates
- **THEN** the two queries bind different dates

### Requirement: Relative value binding
A date result for a date field SHALL be bound like a `YYYY-MM-DD` literal. A timestamp result, and each day boundary of a whole-day comparison, SHALL be bound as a `time.Time` instant truncated to microseconds. A number result SHALL be bound like a number literal of the same text.

#### Scenario: Microsecond truncation
- **WHEN** the filter `created_at > {NOW - 30::MINUTES}` is compiled with reference time 2026-10-27T10:00:00.1234567Z
- **THEN** the argument is 2026-10-27T09:30:00.123456Z

#### Scenario: Whole-day upper bound
- **WHEN** the filter `created_at <= TODAY` for a timestamp field is compiled with reference time 2026-10-27T10:00:00+02:00
- **THEN** the SQL compares `created_at < ?` with argument 2026-10-27T22:00:00Z

#### Scenario: Numeric result
- **WHEN** the filter `price > {10 / 4}` for numeric field `price` is compiled
- **THEN** the argument is the string `2.5` cast to `numeric`

### Requirement: Whole-day equality SQL
`= D` on a timestamp field SHALL compile to `(? >= ? AND ? < ?)` over the two day boundaries, and `!= D` SHALL compile to its NULL-safe negation, `COALESCE(NOT (? >= ? AND ? < ?), TRUE)`.

#### Scenario: Whole-day equality
- **WHEN** the filter `created_at = TODAY` for a timestamp field is compiled
- **THEN** the SQL is a range test over two bound day boundaries, with the column referenced twice

#### Scenario: Whole-day inequality
- **WHEN** the filter `created_at != TODAY` for a timestamp field is compiled
- **THEN** the SQL wraps the negated range test in `COALESCE(…, TRUE)`
