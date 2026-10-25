# Spec Delta

## Purpose

Compiles easylist query plans into prepared PostgreSQL queries using goqu, satisfying the SQL driver contract for applications that choose PostgreSQL.

## ADDED Requirements

### Requirement: PostgreSQL prepared output
The driver SHALL produce PostgreSQL SQL with numbered placeholders (`$1`, `$2`, ...) and a matching argument list. Arguments SHALL appear in the order the filter syntax tree is visited left to right, followed by limit and offset. Compiling the same plan twice SHALL produce identical SQL and arguments.

#### Scenario: Placeholder order
- **WHEN** a plan filters `name = 'Ann' AND age < 30` with limit 10 and offset 0
- **THEN** the arguments are `Ann`, `30`, `10`, `0` in that order, bound to `$1` through `$4`

#### Scenario: Deterministic output
- **WHEN** the same plan is compiled twice
- **THEN** both results are byte-identical

### Requirement: Page query shape
A page query SHALL select each planned field under its field name as a quoted column alias, read from the base table with its alias, add the planned joins in order, apply scope and filter in `WHERE`, apply the planned ordering, and apply `LIMIT` and `OFFSET` when present. A field without an SQL expression SHALL be read as the quoted column of the base table alias.

#### Scenario: Base and joined fields
- **WHEN** a plan for table `users` alias `u` selects `id`, `name`, and `country` (SQL `c.name`, join `LEFT JOIN countries c ON c.id = u.country_id`)
- **THEN** the query selects `u."id" AS "id"`, `u."name" AS "name"`, and `c.name AS "country"` from `users AS u` followed by that join text

#### Scenario: Export plan without pagination
- **WHEN** an export plan is compiled
- **THEN** the query has no `LIMIT` or `OFFSET`

### Requirement: Count query shape
A count query SHALL return `COUNT(*)` from the base table with its alias, the count plan's joins, and the scope and filter in `WHERE`, with no ordering, limit, or offset.

#### Scenario: Count with filter join
- **WHEN** a count plan filters on `country` and includes the `country` join
- **THEN** the query is a `SELECT COUNT(*)` over `users AS u` with that join and the filter condition

### Requirement: PostgreSQL type handling
Numeric literals SHALL be bound as exact decimal strings cast to `numeric`. Text `CONTAINS` SHALL use `ILIKE` with an explicit escape character so `%`, `_`, and the escape character in the literal match literally. Boolean equality SHALL compare against a bound argument rather than emitting `IS TRUE` or `IS FALSE`. `!=` SHALL compile to `<>`.

#### Scenario: Numeric literal
- **WHEN** a filter is `price = 12345678901234567890.10` for numeric field `price`
- **THEN** the argument is the string `12345678901234567890.10` and the SQL casts its placeholder to `numeric`

#### Scenario: Boolean equality
- **WHEN** a filter is `active = TRUE`
- **THEN** the SQL compares the field with a placeholder whose argument is `true`

### Requirement: Concurrent use
A driver value SHALL be safe to use from multiple goroutines at once without additional locking.

#### Scenario: Parallel compilation
- **WHEN** many goroutines compile different plans with one driver value
- **THEN** every result is the same as compiling that plan alone
