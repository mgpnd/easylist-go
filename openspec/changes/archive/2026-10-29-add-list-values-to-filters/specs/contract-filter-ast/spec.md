# Spec Delta

## ADDED Requirements

### Requirement: List values
A parenthesized value list SHALL be represented as a literal of a distinct list kind. Its text SHALL be the list's source spelling, parentheses included, and its position SHALL be the 1-based byte position of the `(`. The literal SHALL give access to its items in source order. Each item SHALL be a string, number, Boolean, or NULL literal with its own text and position. Literals of other kinds SHALL have no items.

#### Scenario: List literal
- **WHEN** a consumer inspects the predicate parsed from `status IN ('success', 'pending')`
- **THEN** the value has the list kind, text `('success', 'pending')`, position 11, and two string items: `success` at position 12 and `pending` at position 23

#### Scenario: Items keep their kinds
- **WHEN** a consumer inspects the value parsed from `age BETWEEN (30, 40)`
- **THEN** the value has the list kind and two number items with texts `30` and `40`

#### Scenario: Plain literal has no items
- **WHEN** a consumer inspects the value parsed from `tags CONTAINS 'a,b'`
- **THEN** the value has the string kind, text `a,b`, and no items

### Requirement: IN and NOT IN operators
`IN` and `NOT IN` SHALL be two distinct predicate operators. A predicate with either operator SHALL always have a list value. The operator position of `NOT IN` SHALL be the position of `NOT`.

#### Scenario: NOT IN operator
- **WHEN** a consumer inspects the predicate parsed from `status NOT IN ('error')`
- **THEN** the operator is `NOT IN`, the operator position is 8, and the value has the list kind

## MODIFIED Requirements

### Requirement: NULL predicate representation
A NULL comparison SHALL be represented as an ordinary predicate whose operator is `=` or `!=` and whose value is a literal of a distinct NULL kind. A NULL item of an `IN` or `NOT IN` list SHALL be a literal of the same NULL kind. A NULL literal SHALL keep the keyword's source spelling as its text and the keyword's 1-based byte position as its position. No other operator or list SHALL ever carry a NULL literal.

#### Scenario: NULL literal kind
- **WHEN** a consumer inspects the predicate parsed from `country = NULL`
- **THEN** the operator is `=`, the value's kind is the NULL kind, and the value's position is 11

#### Scenario: NULL literal keeps its spelling
- **WHEN** a consumer inspects the predicate parsed from `country != null`
- **THEN** the operator is `!=` and the value's text is `null`

#### Scenario: NULL kind is distinct
- **WHEN** a consumer inspects the predicates parsed from `country = NULL` and `country = 'NULL'`
- **THEN** the first value has the NULL kind and the second value has the string kind with text `NULL`

#### Scenario: NULL list item
- **WHEN** a consumer inspects the predicate parsed from `status IN ('a', null)`
- **THEN** the second item has the NULL kind, text `null`, and position 17
