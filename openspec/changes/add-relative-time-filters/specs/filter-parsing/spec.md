# Spec Delta

## ADDED Requirements

### Requirement: Syntax only
The parser SHALL only check syntax and build the syntax tree. It SHALL NOT calculate, simplify, or type-check value expressions, and SHALL NOT read the system clock. Expressions that are syntactically valid but cannot be evaluated, such as `{NOW - TODAY}` or `{1 / 0}`, SHALL parse successfully.

#### Scenario: Ill-typed expression parses
- **WHEN** the filter `t > {NOW - TODAY}` is parsed
- **THEN** parsing succeeds with an expression value

#### Scenario: Division by zero parses
- **WHEN** the filter `price > {1 / 0}` is parsed
- **THEN** parsing succeeds with an expression value

#### Scenario: Bare duration parses
- **WHEN** the filter `due > {5::DAYS}` is parsed
- **THEN** parsing succeeds with an expression value

### Requirement: Relative time keywords
The parser SHALL accept the case-insensitive keywords `NOW`, `TODAY`, `START_OF_WEEK`, `END_OF_WEEK`, `START_OF_MONTH`, `END_OF_MONTH`, `START_OF_YEAR`, and `END_OF_YEAR` as the value of a `=`, `!=`, `>`, `>=`, `<`, or `<=` predicate, either bare or inside braces. Any other operator with a keyword value SHALL be a syntax error at the keyword's position.

#### Scenario: Bare keyword
- **WHEN** the filter `created_at >= START_OF_WEEK` is parsed
- **THEN** parsing succeeds with a predicate on `created_at` with operator `>=` and an expression value

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
The parser SHALL accept an arithmetic expression in braces, `{ … }`, as the value of a `=`, `!=`, `>`, `>=`, `<`, or `<=` predicate. An expression SHALL be built from unsigned number literals, relative time keywords, durations, the binary operators `+`, `-`, `*`, `/`, unary `-`, and parentheses.

#### Scenario: Date plus days
- **WHEN** the filter `due_date > {today + 5::days}` is parsed
- **THEN** parsing succeeds with an expression value

#### Scenario: Unclosed brace
- **WHEN** the filter `due > {TODAY + 5::DAYS` is parsed
- **THEN** parsing fails with a syntax error naming the end of input

#### Scenario: Empty braces
- **WHEN** the filter `due > {}` is parsed
- **THEN** parsing fails with a syntax error at the position of `}`

### Requirement: Brace expression precedence
Inside braces, `::` SHALL bind more tightly than every operator, unary `-` more tightly than `*` and `/`, and `*` and `/` more tightly than `+` and `-`. Binary operators of equal precedence SHALL associate left to right. `+` and `-` SHALL always be operators inside braces, whatever the spacing.

#### Scenario: Precedence
- **WHEN** the filter `due = {TODAY + 2 * 7::DAYS}` is parsed
- **THEN** the tree is an addition whose right operand is a multiplication of `2` and `7::DAYS`

#### Scenario: Spacing
- **WHEN** the filters `due = {TODAY+5::DAYS}` and `due = {TODAY + 5::DAYS}` are parsed
- **THEN** both parse to trees of the same shape

### Requirement: Arithmetic only inside braces
Arithmetic operators and durations outside braces SHALL be a syntax error at the position of the first such token, and braces SHALL NOT nest.

#### Scenario: Arithmetic outside braces
- **WHEN** the filter `due > TODAY + 5::DAYS` is parsed
- **THEN** parsing fails with a syntax error at the position of `+`

#### Scenario: Nested braces
- **WHEN** the filter `due > {{TODAY}}` is parsed
- **THEN** parsing fails with a syntax error at position 8

### Requirement: Duration syntax
A duration SHALL be an unsigned integer literal followed by `::` and one of the case-insensitive units `DAYS`, `BUSINESS_DAYS`, `HOURS`, or `MINUTES`. A decimal amount SHALL be a syntax error at the amount's position, and an unknown unit at the unit's position. The parser SHALL NOT limit the amount's size.

#### Scenario: Unit case
- **WHEN** the filter `t > {NOW - 30::minutes}` is parsed
- **THEN** parsing succeeds

#### Scenario: Unknown unit
- **WHEN** the filter `t > {NOW - 3::WEEKS}` is parsed
- **THEN** parsing fails with a syntax error at the position of `WEEKS`

#### Scenario: Decimal amount
- **WHEN** the filter `t > {NOW - 1.5::HOURS}` is parsed
- **THEN** parsing fails with a syntax error at the position of `1.5`
