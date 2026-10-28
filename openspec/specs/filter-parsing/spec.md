# filter-parsing Specification

## Purpose

Defines the filter language that `easylist-filter` accepts and the syntax errors it reports, independent of any schema or SQL dialect.

## Requirements

### Requirement: NULL keyword
The parser SHALL accept the keyword `NULL` as the value of a `=` or `!=` predicate. The keyword SHALL be case-insensitive. A double-quoted `"NULL"` SHALL remain an identifier.

#### Scenario: Equal to NULL
- **WHEN** the filter `country = NULL` is parsed
- **THEN** parsing succeeds with one predicate on field `country` with operator `=` and a NULL value

#### Scenario: Not equal to NULL
- **WHEN** the filter `country != NULL` is parsed
- **THEN** parsing succeeds with one predicate on field `country` with operator `!=` and a NULL value

#### Scenario: Keyword case
- **WHEN** the filters `country = null` and `country = Null` are parsed
- **THEN** both parse the same as `country = NULL`

#### Scenario: Quoted NULL is a field
- **WHEN** the filter `"NULL" = 'x'` is parsed
- **THEN** parsing succeeds with a predicate on the field named `NULL`

#### Scenario: NULL combined with other predicates
- **WHEN** the filter `country = NULL OR country EXCLUDES 'fr'` is parsed
- **THEN** parsing succeeds with an `OR` of the NULL predicate and the `EXCLUDES` predicate

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

### Requirement: Grammar documented in EBNF
The filter language reference, `easylist-filter/docs/query-language.md`, SHALL give the grammar in EBNF (ISO/IEC 14977 notation) in an `ebnf` code block over lexer tokens, with the lexical rules in a separate `ebnf` block. It SHALL NOT use goyacc notation. A test SHALL fail when the syntactic EBNF block and the parser disagree on any filter in a shared corpus of accepted and rejected inputs.

#### Scenario: EBNF in the reference
- **WHEN** a reader opens `docs/query-language.md`
- **THEN** the grammar is written in EBNF, including brace expressions, durations, and relative time keywords

#### Scenario: Documentation drift
- **WHEN** `grammar.y` accepts a new operator that the documented EBNF does not derive
- **THEN** the grammar documentation test fails, naming the filter that only one of them accepts
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
