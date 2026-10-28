# Spec Delta

## ADDED Requirements

### Requirement: IN and NOT IN predicates
The parser SHALL accept `IN` and `NOT IN` after a field, followed by a value list. Each item SHALL be a string, a number, a Boolean, or `NULL`. Items of different kinds MAY be mixed, and the parser SHALL NOT check them against any field type. The keywords SHALL be case-insensitive.

#### Scenario: IN with strings
- **WHEN** the filter `status IN ('success', 'pending')` is parsed
- **THEN** parsing succeeds with one predicate on field `status` with operator `IN` and a list value of the strings `success` and `pending`

#### Scenario: NOT IN
- **WHEN** the filter `status NOT IN ('error')` is parsed
- **THEN** parsing succeeds with one predicate on field `status` with operator `NOT IN` and a list value of the string `error`

#### Scenario: NULL item
- **WHEN** the filter `status IN ('success', 'pending', NULL)` is parsed
- **THEN** parsing succeeds, and the third item is a NULL value at position 34

#### Scenario: Mixed item kinds
- **WHEN** the filter `x IN ('a', 1, TRUE)` is parsed
- **THEN** parsing succeeds with a string item, a number item, and a Boolean item

#### Scenario: Keyword case
- **WHEN** the filters `status in ('a')` and `status not In ('a')` are parsed
- **THEN** they parse the same as `status IN ('a')` and `status NOT IN ('a')`

#### Scenario: IN without a list
- **WHEN** the filter `status IN 'a'` is parsed
- **THEN** parsing fails with a syntax error at position 11

#### Scenario: NOT without IN
- **WHEN** the filter `status NOT = 'a'` is parsed
- **THEN** parsing fails with a syntax error at position 12

### Requirement: Value list syntax
A value list SHALL be `(`, one or more items separated by `,`, and `)`. An empty list or a trailing comma SHALL be a syntax error at the `)`. A `,` anywhere other than between list items SHALL be a syntax error at its position.

#### Scenario: Single item
- **WHEN** the filter `status IN ('a')` is parsed
- **THEN** parsing succeeds with a list value of one item

#### Scenario: Empty list
- **WHEN** the filter `status IN ()` is parsed
- **THEN** parsing fails with a syntax error at position 12

#### Scenario: Trailing comma
- **WHEN** the filter `status IN ('a',)` is parsed
- **THEN** parsing fails with a syntax error at position 16

#### Scenario: Comma outside a list
- **WHEN** the filter `a = 1, b = 2` is parsed
- **THEN** parsing fails with a syntax error at position 6

### Requirement: List values for CONTAINS and EXCLUDES
The parser SHALL accept a value list as the value of `CONTAINS` and `EXCLUDES`, in addition to a single string, number, or Boolean. Each item SHALL be a string, a number, or a Boolean. The parser SHALL keep each string item exactly as decoded and SHALL NOT split or trim it.

#### Scenario: CONTAINS with a list
- **WHEN** the filter `tags CONTAINS ('a,b', 'c')` is parsed
- **THEN** parsing succeeds with operator `CONTAINS` and a list value of the strings `a,b` and `c`

#### Scenario: EXCLUDES with a list
- **WHEN** the filter `tags EXCLUDES ('x')` is parsed
- **THEN** parsing succeeds with operator `EXCLUDES` and a list value of the string `x`

#### Scenario: Quoted form unchanged
- **WHEN** the filter `tags CONTAINS 'a,b'` is parsed
- **THEN** parsing succeeds with a single string value `a,b`, exactly as before

### Requirement: List bounds for BETWEEN
The parser SHALL accept a value list of exactly two items as the value of `BETWEEN`, in addition to a single string. Each item SHALL be a string, a number, or a Boolean. A list with one item SHALL be a syntax error at the `)`, and a list with more than two items at the second `,`.

#### Scenario: Number bounds
- **WHEN** the filter `age BETWEEN (30, 40)` is parsed
- **THEN** parsing succeeds with operator `BETWEEN` and a list value of the numbers `30` and `40`

#### Scenario: Quoted form unchanged
- **WHEN** the filter `age BETWEEN '30,40'` is parsed
- **THEN** parsing succeeds with a single string value `30,40`, exactly as before

#### Scenario: One bound
- **WHEN** the filter `age BETWEEN (30)` is parsed
- **THEN** parsing fails with a syntax error at position 16

#### Scenario: Three bounds
- **WHEN** the filter `age BETWEEN (30, 40, 50)` is parsed
- **THEN** parsing fails with a syntax error at position 20

### Requirement: No relative values in lists
A relative time keyword or a brace expression as a list item SHALL be a syntax error at the position of the keyword or the `{`.

#### Scenario: Keyword item
- **WHEN** the filter `due IN (TODAY)` is parsed
- **THEN** parsing fails with a syntax error at position 9

#### Scenario: Brace expression bound
- **WHEN** the filter `due BETWEEN ({TODAY}, {NOW})` is parsed
- **THEN** parsing fails with a syntax error at position 14

### Requirement: IN and NOT are reserved
Outside braces, a word spelled `IN` or `NOT`, in any case, SHALL be a keyword and never an identifier. A field with one of these names SHALL be written double-quoted.

#### Scenario: Unquoted reserved field name
- **WHEN** the filter `in = 'x'` or `not = 1` is parsed
- **THEN** parsing fails with a syntax error at position 1

#### Scenario: Quoted reserved field name
- **WHEN** the filter `"in" = 'x'` is parsed
- **THEN** parsing succeeds with a predicate on the field named `in`

## MODIFIED Requirements

### Requirement: NULL only with equality operators
The parser SHALL reject `NULL` as the value of any operator other than `=` and `!=`, except as an item of an `IN` or `NOT IN` list, and SHALL reject `NULL` in any other position, including a `CONTAINS`, `EXCLUDES`, or `BETWEEN` list. The error SHALL report the 1-based byte position of the `NULL` token.

#### Scenario: Ordering operator with NULL
- **WHEN** the filter `age > NULL` is parsed
- **THEN** parsing fails with a syntax error at position 7

#### Scenario: CONTAINS with NULL
- **WHEN** the filter `name CONTAINS NULL` is parsed
- **THEN** parsing fails with a syntax error at the position of `NULL`

#### Scenario: BETWEEN with NULL
- **WHEN** the filter `age BETWEEN NULL` is parsed
- **THEN** parsing fails with a syntax error at the position of `NULL`

#### Scenario: Unquoted NULL as a field
- **WHEN** the filter `NULL = 'x'` is parsed
- **THEN** parsing fails with a syntax error at position 1

#### Scenario: NULL in a CONTAINS list
- **WHEN** the filter `tags CONTAINS ('a', NULL)` is parsed
- **THEN** parsing fails with a syntax error at position 21

#### Scenario: NULL as a BETWEEN bound
- **WHEN** the filter `age BETWEEN (NULL, 40)` is parsed
- **THEN** parsing fails with a syntax error at position 14
