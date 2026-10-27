# contract-filter-ast Specification

## Purpose

Defines how parsed filters are represented in the public syntax tree produced by `easylist-filter`, so that drivers and other consumers can interpret predicates without re-reading the filter text.

## Requirements

### Requirement: NULL predicate representation
A NULL comparison SHALL be represented as an ordinary predicate whose operator is `=` or `!=` and whose value is a literal of a distinct NULL kind. The NULL literal SHALL keep the keyword's source spelling as its text and the keyword's 1-based byte position as its position. No other operator SHALL ever carry a NULL literal.

#### Scenario: NULL literal kind
- **WHEN** a consumer inspects the predicate parsed from `country = NULL`
- **THEN** the operator is `=`, the value's kind is the NULL kind, and the value's position is 11

#### Scenario: NULL literal keeps its spelling
- **WHEN** a consumer inspects the predicate parsed from `country != null`
- **THEN** the operator is `!=` and the value's text is `null`

#### Scenario: NULL kind is distinct
- **WHEN** a consumer inspects the predicates parsed from `country = NULL` and `country = 'NULL'`
- **THEN** the first value has the NULL kind and the second value has the string kind with text `NULL`
