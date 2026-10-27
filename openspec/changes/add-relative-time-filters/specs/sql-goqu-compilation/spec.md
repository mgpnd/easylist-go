# Spec Delta

## ADDED Requirements

### Requirement: PostgreSQL relative time binding
A relative time value SHALL be evaluated once, when the plan is compiled, and bound as an argument; the SQL SHALL NOT use `now()`, `CURRENT_DATE`, or any other database clock or time zone function. A date for a date field SHALL be bound like a `YYYY-MM-DD` literal. A timestamp, and each day boundary of a whole-day comparison, SHALL be bound as a `time.Time` instant truncated to microseconds. `= D` on a timestamp field SHALL compile to `(? >= ? AND ? < ?)`, and `!= D` SHALL compile to its NULL-safe negation, `COALESCE(NOT (? >= ? AND ? < ?), TRUE)`.

#### Scenario: No database clock
- **WHEN** the filter `created_at > {NOW - 30::MINUTES}` is compiled with reference time 2026-10-27T10:00:00.1234567Z
- **THEN** the SQL contains no `now()` and the argument is 2026-10-27T09:30:00.123456Z

#### Scenario: Whole-day upper bound
- **WHEN** the filter `created_at <= TODAY` for a timestamp field is compiled with reference time 2026-10-27T10:00:00+02:00
- **THEN** the SQL compares `created_at < ?` with argument 2026-10-27T22:00:00Z

#### Scenario: Whole-day equality
- **WHEN** the filter `created_at = TODAY` for a timestamp field is compiled
- **THEN** the SQL is a range test over two bound day boundaries, with the column referenced twice

#### Scenario: Recompiling moves the window
- **WHEN** the same parsed filter `due = TODAY` is compiled in two plans whose reference times are on different dates
- **THEN** the two queries bind different dates
