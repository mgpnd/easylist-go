# Spec Delta

## ADDED Requirements

### Requirement: Relative time keywords
The parser SHALL accept the case-insensitive keywords `NOW`, `TODAY`, `START_OF_WEEK`, `END_OF_WEEK`, `START_OF_MONTH`, `END_OF_MONTH`, `START_OF_YEAR`, and `END_OF_YEAR` as the value of a `=`, `!=`, `>`, `>=`, `<`, or `<=` predicate, either bare or inside braces. Any other operator with a keyword value SHALL be a syntax error at the keyword's position.

#### Scenario: Bare keyword
- **WHEN** the filter `created_at >= START_OF_WEEK` is parsed
- **THEN** parsing succeeds with a predicate on `created_at` with operator `>=` and a relative time value

#### Scenario: Keyword case
- **WHEN** the filters `due = today` and `due = Today` are parsed
- **THEN** both parse the same as `due = TODAY`

#### Scenario: Keyword with a non-comparison operator
- **WHEN** the filter `name CONTAINS TODAY` or `due BETWEEN TODAY` is parsed
- **THEN** parsing fails with a syntax error at the position of `TODAY`

### Requirement: Keywords only on the value side
Relative time keywords SHALL be keywords only on the value side of a predicate. An unquoted identifier with the same spelling SHALL remain a field name on the left side. An unquoted identifier on the value side that is not a relative time keyword SHALL be a syntax error at its position naming it.

#### Scenario: Keyword spelling as a field
- **WHEN** the filter `today = 'x' AND now > 3` is parsed
- **THEN** parsing succeeds with predicates on the fields `today` and `now`

#### Scenario: Unknown word as a value
- **WHEN** the filter `due = tomorrow` is parsed
- **THEN** parsing fails with a syntax error at position 7 naming `tomorrow`

### Requirement: Brace expressions
The parser SHALL accept an arithmetic expression in braces, `{ … }`, as the value of a `=`, `!=`, `>`, `>=`, `<`, or `<=` predicate. An expression SHALL be built from number literals, relative time keywords, durations `<integer>::<unit>`, the binary operators `+`, `-`, `*`, `/`, unary `-`, and parentheses.

#### Scenario: Date plus days
- **WHEN** the filter `due_date > {today + 5::days}` is parsed
- **THEN** parsing succeeds with a relative time value

#### Scenario: Unclosed brace
- **WHEN** the filter `due > {TODAY + 5::DAYS` is parsed
- **THEN** parsing fails with a syntax error naming the end of input

#### Scenario: Empty braces
- **WHEN** the filter `due > {}` is parsed
- **THEN** parsing fails with a syntax error at the position of `}`

### Requirement: Brace expression precedence
Inside braces, `::` SHALL bind more tightly than every operator, `*` and `/` SHALL bind more tightly than `+` and `-`, and operators of equal precedence SHALL associate left to right. `+` and `-` SHALL always be operators inside braces, whatever the spacing.

#### Scenario: Precedence
- **WHEN** the filter `due = {TODAY + 2 * 7::DAYS}` is parsed
- **THEN** the value is today plus 14 days

#### Scenario: Spacing
- **WHEN** the filters `due = {TODAY+5::DAYS}` and `due = {TODAY + 5::DAYS}` are parsed
- **THEN** both parse to the same value

### Requirement: Arithmetic only inside braces
Arithmetic operators and durations outside braces SHALL be a syntax error at the position of the first such token, and braces SHALL NOT nest.

#### Scenario: Arithmetic outside braces
- **WHEN** the filter `due > TODAY + 5::DAYS` is parsed
- **THEN** parsing fails with a syntax error at the position of `+`

#### Scenario: Nested braces
- **WHEN** the filter `due > {{TODAY}}` is parsed
- **THEN** parsing fails with a syntax error at position 8

### Requirement: Duration units
A duration SHALL be an unsigned integer literal of at most 10000000, followed by `::` and one of the case-insensitive units `DAYS`, `BUSINESS_DAYS`, `HOURS`, or `MINUTES`. A decimal amount, an amount above the limit, or an unknown unit SHALL be an error at the amount's or unit's position.

#### Scenario: Unit case
- **WHEN** the filter `t > {NOW - 30::minutes}` is parsed
- **THEN** parsing succeeds

#### Scenario: Unknown unit
- **WHEN** the filter `t > {NOW - 3::WEEKS}` is parsed
- **THEN** parsing fails with an error at the position of `WEEKS` naming the allowed units

#### Scenario: Decimal amount
- **WHEN** the filter `t > {NOW - 1.5::HOURS}` is parsed
- **THEN** parsing fails with an error at the position of `1.5`

### Requirement: Duration arithmetic types
Duration `+` or `-` duration SHALL give a duration, for the same or different units. Duration `*` integer, integer `*` duration, and duration `/` integer SHALL give a duration, and `/` SHALL divide every unit's amount exactly. Unary `-` SHALL negate a duration. Any other operation on a duration SHALL be a type error at the operator's position.

#### Scenario: Mixed units in one step
- **WHEN** the filter `t > {NOW + (1::DAYS + 2::HOURS)}` is parsed
- **THEN** parsing succeeds with the same value as `t > {NOW + 1::DAYS + 2::HOURS}`

#### Scenario: Scaling a mixed duration
- **WHEN** the filter `t > {NOW - 2 * (1::DAYS - 30::MINUTES)}` is parsed
- **THEN** parsing succeeds with the same value as `t > {NOW - 2::DAYS + 60::MINUTES}`

#### Scenario: Inexact duration division
- **WHEN** the filter `due > {TODAY + 5::DAYS / 2}` or `t > {NOW + (2::DAYS + 3::HOURS) / 2}` is parsed
- **THEN** parsing fails with an error at the position of `/`

### Requirement: Time arithmetic types
A date or timestamp plus a duration, a duration plus a date or timestamp, and a date or timestamp minus a duration SHALL give a time. A date plus a duration made only of `DAYS` and `BUSINESS_DAYS` SHALL stay a date, and a date plus a duration with any `HOURS` or `MINUTES` SHALL become a timestamp. Any other operation involving a time SHALL be a type error at the operator's position.

#### Scenario: Mixed units applied in turn
- **WHEN** the filter `t > {NOW + 1::DAYS + 2::HOURS}` is parsed
- **THEN** parsing succeeds

#### Scenario: Time minus time
- **WHEN** the filter `t > {NOW - TODAY}` is parsed
- **THEN** parsing fails with an error at the position of `-`

#### Scenario: Multiplying a time
- **WHEN** the filter `due > {TODAY * 2}` is parsed
- **THEN** parsing fails with an error at the position of `*`

### Requirement: Brace expression result
A whole brace expression SHALL evaluate to a number or a time. A bare duration SHALL be an error at the position of `{`.

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
