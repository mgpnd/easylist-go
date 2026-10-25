# Spec Delta

## Purpose

Defines what every `easylist-sql-*` driver receives from the planner and must produce, including the filter semantics each field type guarantees, so drivers stay interchangeable within their dialect.

## ADDED Requirements

### Requirement: Plan contents
A plan given to a driver SHALL describe the base table and alias, the ordered joins with their SQL text, the selected fields with their SQL expression and type, the optional scope, the filter syntax tree resolved against field definitions, the ordering, and the optional limit and offset. A count plan SHALL be marked so the driver produces a count query.

#### Scenario: Driver receives resolved fields
- **WHEN** a plan filters on `country` with SQL `c.name`
- **THEN** the driver can obtain the expression `c.name` and type text for `country` without consulting the list definition

### Requirement: Driver output
A driver SHALL turn a page or export plan into one SQL query and its bound arguments, and a count plan into one query returning a single integer column. Selected values SHALL appear in plan order and be addressable by field name.

#### Scenario: Page query
- **WHEN** a driver compiles a page plan with limit 50 and offset 100
- **THEN** it returns one query that returns at most 50 rows starting after the first 100 in plan order

#### Scenario: Count query
- **WHEN** a driver compiles a count plan
- **THEN** it returns one query whose single row holds the number of matching base rows

### Requirement: Trusted and untrusted input
A driver SHALL emit list-author SQL (table, alias, field expressions, join text, scope) verbatim. It SHALL pass every value that comes from a request (filter literals, limit, offset) as a bound argument and never splice it into the SQL text.

#### Scenario: Filter literal with SQL syntax
- **WHEN** a filter is `name = 'x''; DROP TABLE users; --'`
- **THEN** the literal is passed as one bound argument and the SQL text contains only a placeholder for it

### Requirement: Operators per type
A driver SHALL accept these operators and reject any other with an error naming the operator, field, and operator position: text `=`, `!=`, `CONTAINS`; integer, float, numeric, date, timestamp `=`, `!=`, `>`, `>=`, `<`, `<=`, `BETWEEN`; boolean, uuid, enum `=`, `!=`.

#### Scenario: Unsupported operator
- **WHEN** a filter is `active > TRUE` for boolean field `active`
- **THEN** compilation fails with an error naming `>`, `active`, and the operator position

#### Scenario: CONTAINS on text
- **WHEN** a filter is `name CONTAINS '50%_off'` for text field `name`
- **THEN** rows match when `name` contains `50%_off` case-insensitively, with `%` and `_` treated literally

### Requirement: Literal conversion per type
A driver SHALL convert each filter literal to its field's type and reject mismatches with an error at the literal's position: text, uuid, enum, date, timestamp need a string; integer, float, numeric need a number; boolean needs `TRUE` or `FALSE`. Integers SHALL fit 64 bits, floats SHALL be finite, numerics SHALL keep exact digits, dates SHALL be `YYYY-MM-DD`, timestamps SHALL be RFC 3339 with an offset, uuids SHALL be canonical hyphenated hex, and enums SHALL be a declared value.

#### Scenario: Enum value not declared
- **WHEN** a filter is `status = 'archived'` and `status` declares only `active` and `blocked`
- **THEN** compilation fails with an error naming `archived` at the literal's position

#### Scenario: Timestamp without offset
- **WHEN** a filter is `created_at > '2026-10-25T10:00:00'`
- **THEN** compilation fails because the timestamp has no offset

#### Scenario: Invalid uuid
- **WHEN** a filter is `id = 'not-a-uuid'` for uuid field `id`
- **THEN** compilation fails at the literal's position

### Requirement: Comparison semantics
`!=` SHALL behave as SQL `<>`, so rows whose field is NULL never match it. `BETWEEN 'a,b'` SHALL match values from `a` to `b` inclusive, after trimming spaces around each bound and converting both to the field's type. `AND` and `OR` SHALL keep the grouping of the syntax tree.

#### Scenario: Not equal skips NULL
- **WHEN** a filter is `country != 'FR'` and a row has `country` NULL
- **THEN** that row does not match

#### Scenario: Timestamp range
- **WHEN** a filter is `created_at BETWEEN '2026-01-01T00:00:00Z, 2026-02-01T00:00:00Z'`
- **THEN** rows with `created_at` in that inclusive range match

### Requirement: Scope combination
When the plan has a scope, the driver SHALL combine it with the filter so that rows must satisfy both, regardless of `OR` connectors inside the filter.

#### Scenario: OR filter under scope
- **WHEN** scope is `u.deleted_at IS NULL` and the filter is `name = 'a' OR name = 'b'`
- **THEN** only rows that satisfy the scope and either name comparison match
