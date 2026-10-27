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
The parser SHALL reject `NULL` as the value of any operator other than `=` and `!=`, and SHALL reject `NULL` in any position other than a predicate's value. The error SHALL report the 1-based byte position of the `NULL` token.

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
