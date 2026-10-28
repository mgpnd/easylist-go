# Spec Delta

## ADDED Requirements

### Requirement: IN semantics
`IN` SHALL match exactly the rows that `=` would match for at least one item of its list. A NULL item SHALL match rows whose field is NULL, as `= NULL` does. Duplicate items SHALL NOT change the result.

#### Scenario: Any listed value
- **WHEN** a filter is `status IN ('success', 'pending')` and rows have `status` equal to `success`, `pending`, `error`, and NULL
- **THEN** only the `success` and `pending` rows match

#### Scenario: NULL item
- **WHEN** a filter is `status IN ('success', NULL)` and rows have `status` equal to `success`, `error`, and NULL
- **THEN** the `success` and NULL rows match

#### Scenario: Only a NULL item
- **WHEN** a filter is `status IN (NULL)`
- **THEN** it matches the same rows as `status = NULL`

### Requirement: NOT IN semantics
`NOT IN` SHALL match exactly the rows that `!=` would match for every item of its list, so it matches the rows that `IN` with the same list does not. Rows whose field is NULL SHALL match unless the list contains a NULL item.

#### Scenario: NULL field matches
- **WHEN** a filter is `status NOT IN ('error')` and rows have `status` equal to `success`, `error`, and NULL
- **THEN** the `success` and NULL rows match

#### Scenario: NULL item leaves out NULL rows
- **WHEN** a filter is `status NOT IN ('error', NULL)` and rows have `status` equal to `success`, `error`, and NULL
- **THEN** only the `success` row matches

#### Scenario: Only a NULL item
- **WHEN** a filter is `status NOT IN (NULL)`
- **THEN** it matches the same rows as `status != NULL`

### Requirement: List item conversion
A driver SHALL convert each non-NULL item of a list value separately, under the literal conversion rules for the field's type or, on an array field, its element type, and SHALL reject a mismatch with an error at the item's position. An item SHALL keep the kind it was written as, so a quoted number is a string.

#### Scenario: Undeclared enum item
- **WHEN** a filter is `status IN ('active', 'archived')` and `status` declares only `active` and `blocked`
- **THEN** compilation fails with an error naming `archived` at position 22

#### Scenario: Quoted number item
- **WHEN** a filter is `age BETWEEN ('30', '40')` for integer field `age`
- **THEN** compilation fails at position 14 because the bound is a string

#### Scenario: Invalid array item
- **WHEN** a filter is `ids CONTAINS (7, 'x')` for integer array field `ids`
- **THEN** compilation fails at position 18

### Requirement: No lists on scalar text CONTAINS and EXCLUDES
A driver SHALL reject a list value for `CONTAINS` or `EXCLUDES` on a scalar text field with an error naming the field at the list's position.

#### Scenario: List on a text field
- **WHEN** a filter is `name CONTAINS ('a', 'b')` for scalar text field `name`
- **THEN** compilation fails with an error naming `name` at position 15

## MODIFIED Requirements

### Requirement: Operators per type
A driver SHALL accept these operators and reject any other with an error naming the operator, field, and operator position: text `=`, `!=`, `IN`, `NOT IN`, `CONTAINS`, `EXCLUDES`; integer, float, numeric, date, timestamp `=`, `!=`, `IN`, `NOT IN`, `>`, `>=`, `<`, `<=`, `BETWEEN`; boolean, uuid, enum `=`, `!=`, `IN`, `NOT IN`. Every type SHALL also accept `= NULL` and `!= NULL`, whether or not the field is declared nullable.

#### Scenario: Unsupported operator
- **WHEN** a filter is `active > TRUE` for boolean field `active`
- **THEN** compilation fails with an error naming `>`, `active`, and the operator position

#### Scenario: CONTAINS on text
- **WHEN** a filter is `name CONTAINS '50%_off'` for text field `name`
- **THEN** rows match when `name` contains `50%_off` case-insensitively, with `%` and `_` treated literally

#### Scenario: EXCLUDES on text
- **WHEN** a filter is `name EXCLUDES '50%_off'` for text field `name`
- **THEN** compilation succeeds, and `%` and `_` are treated literally

#### Scenario: EXCLUDES on a non-text field
- **WHEN** a filter is `age EXCLUDES 3` for integer field `age`
- **THEN** compilation fails with an error naming `EXCLUDES`, `age`, and the operator position

#### Scenario: NULL comparison on every type
- **WHEN** a filter is `active = NULL` for boolean field `active`, or `created_at != NULL` for timestamp field `created_at`
- **THEN** compilation succeeds

#### Scenario: NULL comparison on a non-nullable field
- **WHEN** a filter is `name = NULL` for text field `name`, which is not declared nullable
- **THEN** compilation succeeds and the filter matches only rows where `name` is NULL

#### Scenario: IN on every scalar type
- **WHEN** a filter is `active IN (TRUE)` for boolean field `active`, or `id NOT IN ('7f1c0d2e-0000-4000-8000-000000000000')` for uuid field `id`
- **THEN** compilation succeeds

### Requirement: NULL literals
A NULL literal SHALL be valid for every type and SHALL NOT be converted or bound as an argument. This SHALL hold for a NULL item of an `IN` or `NOT IN` list.

#### Scenario: NULL literal binds nothing
- **WHEN** a filter is `status = NULL` for enum field `status`
- **THEN** compilation succeeds without checking declared values, and the query has no argument for the NULL literal

#### Scenario: NULL item binds nothing
- **WHEN** a filter is `status IN ('active', NULL)` for enum field `status`
- **THEN** compilation succeeds, and the query has an argument for `active` and none for the NULL item

### Requirement: BETWEEN semantics
`BETWEEN 'a,b'` SHALL match values from `a` to `b` inclusive, after trimming spaces around each bound and converting both to the field's type. `BETWEEN (a, b)` SHALL match the same range, with each bound converted as a list item and not trimmed.

#### Scenario: Timestamp range
- **WHEN** a filter is `created_at BETWEEN '2026-01-01T00:00:00Z, 2026-02-01T00:00:00Z'`
- **THEN** rows with `created_at` in that inclusive range match

#### Scenario: List bounds
- **WHEN** a filter is `age BETWEEN (30, 40)` for integer field `age`
- **THEN** it matches the same rows as `age BETWEEN '30,40'`

#### Scenario: Date list bounds
- **WHEN** a filter is `due BETWEEN ('2026-01-01', '2026-02-01')` for date field `due`
- **THEN** rows with `due` from 2026-01-01 to 2026-02-01 inclusive match

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

#### Scenario: IN on an array
- **WHEN** a filter is `tags IN ('go')` for text array field `tags`
- **THEN** compilation fails with an error naming `IN`, `tags`, and position 6

### Requirement: Array value lists
For array fields, a string literal SHALL be a comma-separated list of one or more values. Each item SHALL be trimmed and converted to the element type under the scalar literal rules, and an empty item SHALL be an error. An unquoted number or boolean SHALL be a single value. Errors SHALL report the literal's position. A list value SHALL give one value per item, never split or trimmed.

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

#### Scenario: List value keeps commas
- **WHEN** a filter is `tags CONTAINS ('a,b', ' c')` for text array field `tags`
- **THEN** the values are `a,b` and ` c`

#### Scenario: List value matches like the quoted form
- **WHEN** a filter is `statuses CONTAINS ('ok', 'passed')` for text array field `statuses`
- **THEN** it matches the same rows as `statuses CONTAINS 'ok,passed'`
