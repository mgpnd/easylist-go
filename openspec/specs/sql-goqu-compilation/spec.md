# sql-goqu-compilation Specification

## Purpose

Compiles easylist query plans into prepared PostgreSQL queries using goqu, satisfying the SQL driver contract for applications that choose PostgreSQL.

## Requirements

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
Numeric literals SHALL be bound as exact decimal strings cast to `numeric`. Text `CONTAINS` SHALL use `ILIKE` with an explicit escape character so `%`, `_`, and the escape character in the literal match literally. Boolean equality SHALL compare against a bound argument rather than emitting `IS TRUE` or `IS FALSE`.

#### Scenario: Numeric literal
- **WHEN** a filter is `price = 12345678901234567890.10` for numeric field `price`
- **THEN** the argument is the string `12345678901234567890.10` and the SQL casts its placeholder to `numeric`

#### Scenario: Boolean equality
- **WHEN** a filter is `active = TRUE`
- **THEN** the SQL compares the field with a placeholder whose argument is `true`

### Requirement: PostgreSQL NULL handling
`!=` SHALL compile to `IS DISTINCT FROM`. `= NULL` SHALL compile to `IS NULL` and `!= NULL` to `IS NOT NULL`, with no bound argument. Text `EXCLUDES` SHALL negate the same escaped `ILIKE` match as `CONTAINS` and SHALL treat a NULL field as matching.

#### Scenario: Not equal
- **WHEN** a filter is `active != TRUE`
- **THEN** the SQL compares the field with `IS DISTINCT FROM` and a placeholder whose argument is `true`

#### Scenario: Text EXCLUDES
- **WHEN** a filter is `name EXCLUDES '50%_off'`
- **THEN** the SQL negates an `ILIKE` match with an explicit escape character, the argument is `%50\%\_off%`, and a row whose `name` is NULL matches

#### Scenario: NULL comparisons
- **WHEN** a plan filters `country = NULL AND name != NULL` with limit 10 and offset 0
- **THEN** the SQL tests `country` with `IS NULL` and `name` with `IS NOT NULL`, and the arguments are only `10` and `0`, bound to `$1` and `$2`

### Requirement: Concurrent use
A driver value SHALL be safe to use from multiple goroutines at once without additional locking.

#### Scenario: Parallel compilation
- **WHEN** many goroutines compile different plans with one driver value
- **THEN** every result is the same as compiling that plan alone

### Requirement: PostgreSQL array selection
The driver SHALL select each array field as `to_json` of its expression cast to `text`, under the field's name as the column alias, so that NULL arrays stay SQL NULL.

#### Scenario: Base column array
- **WHEN** a plan for alias `u` selects array field `tags` with no SQL expression
- **THEN** the query selects `to_json(u."tags")::text AS "tags"`

#### Scenario: Author SQL array
- **WHEN** a plan selects array field `labels` with SQL `l.names`
- **THEN** the query selects `to_json(l.names)::text AS "labels"`

### Requirement: PostgreSQL array membership
The driver SHALL bind each listed value as its own argument, in list order, and compare it with the array's elements so that PostgreSQL infers the argument's type from the column, as it does for scalar comparisons. `EXCLUDES` SHALL treat NULL arrays and NULL comparison results as matching.

#### Scenario: CONTAINS list arguments
- **WHEN** a plan filters `statuses CONTAINS 'ok,passed'` with limit 10 and offset 0
- **THEN** the arguments are `ok`, `passed`, `10`, `0`, bound to `$1` through `$4`, and each of `$1` and `$2` is required to equal some element of `statuses`

#### Scenario: Enum array without a cast
- **WHEN** a plan filters `statuses CONTAINS 'ok'` for an enum array field stored as a PostgreSQL enum array
- **THEN** the SQL contains no cast to a named array type, and the argument `ok` is compared with the elements directly

#### Scenario: EXCLUDES with a NULL array
- **WHEN** a plan filters `tags EXCLUDES 'go'` and a row's `tags` is NULL
- **THEN** that row matches

### Requirement: No database clock for relative values
The driver SHALL bind the plan's evaluated result of each expression value as an argument, and SHALL NOT evaluate expressions itself. The SQL SHALL NOT use `now()`, `CURRENT_DATE`, or any other database clock or time zone function.

#### Scenario: No database clock
- **WHEN** the filter `created_at > {NOW - 30::MINUTES}` is compiled
- **THEN** the SQL contains no `now()` and the value is a bound argument

#### Scenario: Recompiling moves the window
- **WHEN** the same parsed filter `due = TODAY` is planned and compiled with two reference times on different dates
- **THEN** the two queries bind different dates

### Requirement: Relative value binding
A date result for a date field SHALL be bound like a `YYYY-MM-DD` literal. A timestamp result, and each day boundary of a whole-day comparison, SHALL be bound as a `time.Time` instant truncated to microseconds. A number result SHALL be bound like a number literal of the same text.

#### Scenario: Microsecond truncation
- **WHEN** the filter `created_at > {NOW - 30::MINUTES}` is compiled with reference time 2026-10-27T10:00:00.1234567Z
- **THEN** the argument is 2026-10-27T09:30:00.123456Z

#### Scenario: Whole-day upper bound
- **WHEN** the filter `created_at <= TODAY` for a timestamp field is compiled with reference time 2026-10-27T10:00:00+02:00
- **THEN** the SQL compares `created_at < ?` with argument 2026-10-27T22:00:00Z

#### Scenario: Numeric result
- **WHEN** the filter `price > {10 / 4}` for numeric field `price` is compiled
- **THEN** the argument is the string `2.5` cast to `numeric`

### Requirement: Whole-day equality SQL
`= D` on a timestamp field SHALL compile to `(? >= ? AND ? < ?)` over the two day boundaries, and `!= D` SHALL compile to its NULL-safe negation, `COALESCE(NOT (? >= ? AND ? < ?), TRUE)`.

#### Scenario: Whole-day equality
- **WHEN** the filter `created_at = TODAY` for a timestamp field is compiled
- **THEN** the SQL is a range test over two bound day boundaries, with the column referenced twice

#### Scenario: Whole-day inequality
- **WHEN** the filter `created_at != TODAY` for a timestamp field is compiled
- **THEN** the SQL wraps the negated range test in `COALESCE(…, TRUE)`
