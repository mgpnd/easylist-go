# Spec Delta

## ADDED Requirements

### Requirement: Array value transport
A driver SHALL select each array field so that its value reaches the runtime as JSON array text, or as SQL NULL when the array is NULL. Elements SHALL be JSON strings for text, enum, uuid, date, and timestamp, numbers for integer, float, and numeric, and booleans for boolean.

#### Scenario: Selected array field
- **WHEN** a page plan selects text array field `tags` holding `{go,rust}`
- **THEN** the query returns `["go","rust"]` as the value of column `tags`

#### Scenario: NULL array field
- **WHEN** a page plan selects nullable array field `tags` that is NULL for a row
- **THEN** the query returns SQL NULL for `tags` on that row

#### Scenario: Element encodings
- **WHEN** array fields of type numeric, date, and timestamp are selected
- **THEN** numerics arrive as JSON numbers with exact digits, dates as `YYYY-MM-DD` strings, and timestamps as ISO 8601 strings, either with an offset or, for values without a time zone, without one

### Requirement: Array field operators
A driver SHALL accept only `CONTAINS`, `EXCLUDES`, `= NULL`, and `!= NULL` on array fields of every element type. Any other operator, including `=` or `!=` with a value, SHALL be rejected with an error naming the operator, field, and operator position.

#### Scenario: Equality with a value on an array
- **WHEN** a filter is `tags = 'go'` for text array field `tags`
- **THEN** compilation fails with an error naming `=`, `tags`, and the operator position

#### Scenario: Ordering operator on an array
- **WHEN** a filter is `ids > 3` for integer array field `ids`
- **THEN** compilation fails with an error naming `>`, `ids`, and the operator position

#### Scenario: NULL test on an array
- **WHEN** a filter is `tags = NULL` for nullable text array field `tags`
- **THEN** compilation succeeds and only rows whose `tags` is NULL match, not rows holding an empty array

### Requirement: Array value lists
For array fields, a string literal SHALL be a comma-separated list of one or more values. Each item SHALL be trimmed and converted to the element type under the scalar literal rules, and an empty item SHALL be an error. An unquoted number or boolean SHALL be a single value. Errors SHALL report the literal's position.

#### Scenario: Quoted list on an integer array
- **WHEN** a filter is `ids CONTAINS '7, 8'` for integer array field `ids`
- **THEN** the values are the integers `7` and `8`

#### Scenario: Single unquoted value
- **WHEN** a filter is `ids CONTAINS 7` for integer array field `ids`
- **THEN** the value is the integer `7`

#### Scenario: Comma always separates text items
- **WHEN** a filter is `tags CONTAINS 'a,b'` for text array field `tags`
- **THEN** the values are `a` and `b`, never the single value `a,b`

#### Scenario: Empty item
- **WHEN** a filter is `tags CONTAINS 'a,,b'`
- **THEN** compilation fails at the literal's position

#### Scenario: Invalid item
- **WHEN** a filter is `ids CONTAINS '7,x'` for integer array field `ids`
- **THEN** compilation fails with an error naming `x` at the literal's position

#### Scenario: Undeclared enum item
- **WHEN** a filter is `statuses EXCLUDES 'skipped,archived'` and `statuses` declares only `ok`, `skipped`, and `failed`
- **THEN** compilation fails with an error naming `archived` at the literal's position

#### Scenario: Text fields do not split lists
- **WHEN** a filter is `name CONTAINS 'a,b'` for scalar text field `name`
- **THEN** rows match when `name` contains the substring `a,b`

### Requirement: Array CONTAINS semantics
`CONTAINS` on an array field SHALL match rows whose array has, for every listed value, an element equal to it. Equality SHALL be exact for every element type, including case-sensitive text. A NULL array SHALL NOT match.

#### Scenario: All values present
- **WHEN** a filter is `statuses CONTAINS 'ok,passed'` and rows hold `{ok,passed,retried}`, `{ok}`, and NULL
- **THEN** only the `{ok,passed,retried}` row matches

#### Scenario: Exact element match
- **WHEN** a filter is `tags CONTAINS 'go'` and a row holds `{golang,Go}`
- **THEN** that row does not match

### Requirement: Array EXCLUDES semantics
`EXCLUDES` on an array field SHALL match rows whose array has no element equal to any listed value, including empty arrays, and rows whose field is NULL.

#### Scenario: Any listed value excludes the row
- **WHEN** a filter is `statuses EXCLUDES 'skipped,failed'` and rows hold `{ok}`, `{ok,skipped}`, `{skipped,failed}`, an empty array, and NULL
- **THEN** the `{ok}`, empty array, and NULL rows match, and the `{ok,skipped}` and `{skipped,failed}` rows do not
