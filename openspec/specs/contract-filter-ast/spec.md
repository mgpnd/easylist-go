# contract-filter-ast Specification

## Purpose

Defines how parsed filters are represented in the public syntax tree produced by `easylist-filter`, so that drivers and other consumers can interpret predicates without re-reading the filter text.

## Requirements

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

### Requirement: Expression values
A predicate value that is a bare relative time keyword or a brace expression SHALL be represented as a literal of a distinct expression kind. Its text SHALL be the value's source spelling, braces included, and its position SHALL be the 1-based byte position of the keyword or the `{`. The literal SHALL give access to the root node of the value expression tree. Literals of other kinds SHALL have no expression tree.

#### Scenario: Brace expression literal
- **WHEN** a consumer inspects the predicate parsed from `due > {today + 5::days}`
- **THEN** the value has the expression kind, text `{today + 5::days}`, position 7, and its root is an addition

#### Scenario: Bare keyword literal
- **WHEN** a consumer inspects the predicate parsed from `due = TODAY`
- **THEN** the value has the expression kind and its root is the `TODAY` keyword node

#### Scenario: Plain literal has no tree
- **WHEN** a consumer inspects the value parsed from `price > 300`
- **THEN** the value has the number kind and no expression tree

### Requirement: Expression leaf nodes
An expression tree's leaves SHALL be: a number node with its unsigned source spelling; a keyword node identifying one of the eight relative time keywords, with its source spelling; and a duration node with the amount's decimal digits, the unit, and the unit's position. Every node SHALL be immutable, SHALL report its 1-based byte position, and SHALL be creatable only by the parser.

#### Scenario: Duration node
- **WHEN** a consumer inspects the root of `t > {NOW - 30::minutes}`
- **THEN** it is a `-` node whose right operand is a duration node with amount `30` and unit minutes

#### Scenario: Keyword node
- **WHEN** a consumer inspects the root of `due = today`
- **THEN** it is a keyword node identifying `TODAY` with text `today` and position 7

### Requirement: Expression operator nodes
An expression tree's inner nodes SHALL be: a binary node with the operator (`+`, `-`, `*`, or `/`), the operator's position, and left and right operands; and a negation node with the `-` position and its operand.

#### Scenario: Binary node position
- **WHEN** a consumer inspects the root of `price > {100 * 3}`
- **THEN** it is a `*` node at position 14

#### Scenario: Negation node
- **WHEN** a consumer inspects the root of `delta > {-1.50 * 2}`
- **THEN** it is a `*` node whose left operand is a negation of the number node `1.50`

### Requirement: Tree mirrors the source
The expression tree SHALL keep the structure and spelling the user wrote. Parentheses SHALL only shape the tree and SHALL NOT appear as nodes. The tree SHALL NOT be simplified, reordered, or evaluated, so `{100 * 3}` stays a multiplication.

#### Scenario: No folding
- **WHEN** a consumer inspects the root of `price > {100 * 3}`
- **THEN** it is a `*` node with number operands `100` and `3`

#### Scenario: Parentheses shape the tree
- **WHEN** a consumer inspects the root of `t > {NOW + (1::DAYS + 2::HOURS)}`
- **THEN** it is a `+` node whose left operand is `NOW` and whose right operand is a `+` node of the two durations
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
