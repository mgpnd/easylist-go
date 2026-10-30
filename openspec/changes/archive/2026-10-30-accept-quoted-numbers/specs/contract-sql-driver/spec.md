# Spec Delta

## MODIFIED Requirements

### Requirement: Literal conversion per type
A driver SHALL convert each filter literal to its field's type and reject mismatches with an error at the literal's position: text, uuid, enum, date, timestamp need a string; integer, float, numeric need a number, or a string whose text the field would accept as an unquoted number, converted the same way; boolean needs `TRUE` or `FALSE`. Integers SHALL fit 64 bits, floats SHALL be finite, numerics SHALL keep exact digits, dates SHALL be `YYYY-MM-DD`, timestamps SHALL be RFC 3339 with an offset, uuids SHALL be canonical hyphenated hex, and enums SHALL be a declared value.

#### Scenario: Enum value not declared
- **WHEN** a filter is `status = 'archived'` and `status` declares only `active` and `blocked`
- **THEN** compilation fails with an error naming `archived` at the literal's position

#### Scenario: Timestamp without offset
- **WHEN** a filter is `created_at > '2026-10-25T10:00:00'`
- **THEN** compilation fails because the timestamp has no offset

#### Scenario: Invalid uuid
- **WHEN** a filter is `id = 'not-a-uuid'` for uuid field `id`
- **THEN** compilation fails at the literal's position

#### Scenario: Quoted integer
- **WHEN** a filter is `age > '30'` for integer field `age`
- **THEN** compilation succeeds with the same query and arguments as `age > 30`, and the argument is the integer `30`

#### Scenario: Quoted float and numeric
- **WHEN** a filter is `score >= '-1.5'` for float field `score`, or `price < '19.990'` for numeric field `price`
- **THEN** compilation succeeds with the same query and arguments as `score >= -1.5` or `price < 19.990`

#### Scenario: Quoted number compares numerically
- **WHEN** a filter is `age > '9'` and rows hold ages `30` and `5`
- **THEN** only the row with age `30` matches

#### Scenario: Quoted value an unquoted number would not be
- **WHEN** a filter is `age = '30.0'`, `age = ' 30'`, or `age = 'abc'` for integer field `age`, or `price = '1e3'` for numeric field `price`
- **THEN** compilation fails with an error naming the value at the literal's position

#### Scenario: Quoted value on a boolean field
- **WHEN** a filter is `active = 'true'` for boolean field `active`
- **THEN** compilation fails at the literal's position

### Requirement: List item conversion
A driver SHALL convert each non-NULL item of a list value separately, under the literal conversion rules for the field's type or, on an array field, its element type, and SHALL reject a mismatch with an error at the item's position. An item SHALL keep the kind it was written as, so a quoted item is a string, which number types accept under the literal conversion rules.

#### Scenario: Undeclared enum item
- **WHEN** a filter is `status IN ('active', 'archived')` and `status` declares only `active` and `blocked`
- **THEN** compilation fails with an error naming `archived` at position 22

#### Scenario: Quoted number item
- **WHEN** a filter is `age BETWEEN ('30', '40')` for integer field `age`
- **THEN** it matches the same rows as `age BETWEEN (30, 40)`

#### Scenario: Mixed quoted and unquoted items
- **WHEN** a filter is `age IN (1, '30')` for integer field `age`
- **THEN** compilation succeeds with the same arguments as `age IN (1, 30)`

#### Scenario: Invalid array item
- **WHEN** a filter is `ids CONTAINS (7, 'x')` for integer array field `ids`
- **THEN** compilation fails at position 18
