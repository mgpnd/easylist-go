# Spec Delta

## ADDED Requirements

### Requirement: Relative time keywords
The parser SHALL accept the keywords `NOW`, `TODAY`, `START_OF_WEEK`, `END_OF_WEEK`, `START_OF_MONTH`, `END_OF_MONTH`, `START_OF_YEAR`, and `END_OF_YEAR` as the value of a `=`, `!=`, `>`, `>=`, `<`, or `<=` predicate, either bare or inside braces. The keywords SHALL be case-insensitive. They SHALL be keywords only on the value side of a predicate: an unquoted identifier with the same spelling SHALL remain a field name on the left side. An unquoted identifier on the value side that is not one of these keywords SHALL be a syntax error at its position.

#### Scenario: Bare keyword
- **WHEN** the filter `created_at >= START_OF_WEEK` is parsed
- **THEN** parsing succeeds with a predicate on `created_at` with operator `>=` and a relative time value

#### Scenario: Keyword case
- **WHEN** the filters `due = today` and `due = Today` are parsed
- **THEN** both parse the same as `due = TODAY`

#### Scenario: Keyword spelling as a field
- **WHEN** the filter `today = 'x' AND now > 3` is parsed
- **THEN** parsing succeeds with predicates on the fields `today` and `now`

#### Scenario: Unknown word as a value
- **WHEN** the filter `due = tomorrow` is parsed
- **THEN** parsing fails with a syntax error at position 7 naming `tomorrow`

#### Scenario: Keyword with a non-comparison operator
- **WHEN** the filter `name CONTAINS TODAY` or `due BETWEEN TODAY` is parsed
- **THEN** parsing fails with a syntax error at the position of `TODAY`

### Requirement: Brace expressions
The parser SHALL accept an arithmetic expression in braces, `{ … }`, as the value of a `=`, `!=`, `>`, `>=`, `<`, or `<=` predicate. An expression SHALL be built from number literals, relative time keywords, durations `<integer>::<unit>`, the binary operators `+`, `-`, `*`, `/`, unary `-`, and parentheses. `*` and `/` SHALL bind more tightly than `+` and `-`, operators of equal precedence SHALL associate left to right, and `::` SHALL bind more tightly than every operator. Inside braces `+` and `-` SHALL always be operators, so `{TODAY+5::DAYS}` and `{TODAY + 5::DAYS}` parse the same. Arithmetic outside braces SHALL be a syntax error, and braces SHALL NOT nest.

#### Scenario: Date plus days
- **WHEN** the filter `due_date > {today + 5::days}` is parsed
- **THEN** parsing succeeds with a relative time value

#### Scenario: Precedence
- **WHEN** the filter `due = {TODAY + 2 * 7::DAYS}` is parsed
- **THEN** the value is today plus 14 days

#### Scenario: Arithmetic outside braces
- **WHEN** the filter `due > TODAY + 5::DAYS` is parsed
- **THEN** parsing fails with a syntax error at the position of `+`

#### Scenario: Unclosed brace
- **WHEN** the filter `due > {TODAY + 5::DAYS` is parsed
- **THEN** parsing fails with a syntax error naming the end of input

#### Scenario: Empty braces
- **WHEN** the filter `due > {}` is parsed
- **THEN** parsing fails with a syntax error at the position of `}`

### Requirement: Duration units
A duration SHALL be an unsigned integer literal followed by `::` and one of the case-insensitive units `DAYS`, `BUSINESS_DAYS`, `HOURS`, or `MINUTES`. The integer SHALL be at most 10000000. A decimal amount, an unknown unit, or an amount above the limit SHALL be an error at the amount's or unit's position.

#### Scenario: Unit case
- **WHEN** the filter `t > {NOW - 30::minutes}` is parsed
- **THEN** parsing succeeds

#### Scenario: Unknown unit
- **WHEN** the filter `t > {NOW - 3::WEEKS}` is parsed
- **THEN** parsing fails with an error at the position of `WEEKS` naming the allowed units

#### Scenario: Decimal amount
- **WHEN** the filter `t > {NOW - 1.5::HOURS}` is parsed
- **THEN** parsing fails with an error at the position of `1.5`

### Requirement: Brace expression types
The parser SHALL type-check each brace expression and SHALL report the first type error at the position of the offending operator. Values SHALL be numbers, durations, dates, or timestamps, with these rules:
- number with `+`, `-`, `*`, or `/` and a number gives a number;
- duration `*` integer, integer `*` duration, and duration `/` integer give a duration, and `/` SHALL divide the amount exactly;
- duration `+` or `-` duration of the same unit gives a duration;
- a date or timestamp `+` duration, a duration `+` a date or timestamp, and a date or timestamp `-` duration give a time;
- unary `-` applies to numbers and durations.
Every other combination SHALL be an error, including adding or subtracting durations of different units, subtracting a time from anything, and multiplying or dividing a time. A date plus `DAYS` or `BUSINESS_DAYS` SHALL stay a date; a date plus `HOURS` or `MINUTES` SHALL become a timestamp. The whole expression SHALL be a number or a time; a bare duration SHALL be an error at the `{` position.

#### Scenario: Mixed units in one step
- **WHEN** the filter `t > {NOW + (1::DAYS + 2::HOURS)}` is parsed
- **THEN** parsing fails with an error at the position of the inner `+`

#### Scenario: Mixed units applied in turn
- **WHEN** the filter `t > {NOW + 1::DAYS + 2::HOURS}` is parsed
- **THEN** parsing succeeds

#### Scenario: Time minus time
- **WHEN** the filter `t > {NOW - TODAY}` is parsed
- **THEN** parsing fails with an error at the position of `-`

#### Scenario: Inexact duration division
- **WHEN** the filter `due > {TODAY + 5::DAYS / 2}` is parsed
- **THEN** parsing fails with an error at the position of `/`

#### Scenario: Bare duration
- **WHEN** the filter `due > {5::DAYS}` is parsed
- **THEN** parsing fails with an error at position 7

### Requirement: Numeric brace expressions
A brace expression without relative time keywords or durations SHALL be calculated exactly when the filter is parsed. Division by zero, or a quotient with no finite decimal form, SHALL be an error at the position of `/`.

#### Scenario: Multiplication
- **WHEN** the filter `price > {100 * 3}` is parsed
- **THEN** the value is the number `300`

#### Scenario: Exact decimals
- **WHEN** the filter `price > {10 / 4 - 0.1}` is parsed
- **THEN** the value is the number `2.4`

#### Scenario: Non-terminating quotient
- **WHEN** the filter `price > {1 / 3}` is parsed
- **THEN** parsing fails with an error at the position of `/`

#### Scenario: Division by zero
- **WHEN** the filter `price > {1 / (2 - 2)}` is parsed
- **THEN** parsing fails with an error at the position of `/`
