# Spec Delta

## MODIFIED Requirements

### Requirement: PostgreSQL type handling
Numeric literals SHALL be bound as exact decimal strings cast to `numeric`. Text `CONTAINS` SHALL use `ILIKE` with an explicit escape character so `%`, `_`, and the escape character in the literal match literally. Text `EXCLUDES` SHALL negate the same escaped `ILIKE` match and SHALL treat a NULL field as matching. Boolean equality SHALL compare against a bound argument rather than emitting `IS TRUE` or `IS FALSE`. `!=` SHALL compile to `IS DISTINCT FROM`. `= NULL` SHALL compile to `IS NULL` and `!= NULL` to `IS NOT NULL`, with no bound argument.

#### Scenario: Numeric literal
- **WHEN** a filter is `price = 12345678901234567890.10` for numeric field `price`
- **THEN** the argument is the string `12345678901234567890.10` and the SQL casts its placeholder to `numeric`

#### Scenario: Boolean equality
- **WHEN** a filter is `active = TRUE`
- **THEN** the SQL compares the field with a placeholder whose argument is `true`

#### Scenario: Not equal
- **WHEN** a filter is `active != TRUE`
- **THEN** the SQL compares the field with `IS DISTINCT FROM` and a placeholder whose argument is `true`

#### Scenario: Text EXCLUDES
- **WHEN** a filter is `name EXCLUDES '50%_off'`
- **THEN** the SQL negates an `ILIKE` match with an explicit escape character, the argument is `%50\%\_off%`, and a row whose `name` is NULL matches

#### Scenario: NULL comparisons
- **WHEN** a plan filters `country = NULL AND name != NULL` with limit 10 and offset 0
- **THEN** the SQL tests `country` with `IS NULL` and `name` with `IS NOT NULL`, and the arguments are only `10` and `0`, bound to `$1` and `$2`
