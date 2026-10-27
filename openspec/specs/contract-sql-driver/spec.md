# contract-sql-driver Specification

## Purpose

Defines what every `easylist-sql-*` driver receives from the planner and must produce, including the filter semantics each field type guarantees, so drivers stay interchangeable within their dialect.

## Requirements

### Requirement: Plan contents
A plan given to a driver SHALL describe the base table and alias, the ordered joins with their SQL text, the selected fields with their SQL expression and type, the optional scope, the filter syntax tree resolved against field definitions, the evaluated result of every expression value in the filter, the ordering, and the optional limit and offset. A count plan SHALL be marked so the driver produces a count query.

#### Scenario: Driver receives resolved fields
- **WHEN** a plan filters on `country` with SQL `c.name`
- **THEN** the driver can obtain the expression `c.name` and type text for `country` without consulting the list definition

#### Scenario: Driver receives evaluated values
- **WHEN** a plan filters on `due > {TODAY + 5::DAYS}` with reference time 2026-10-27T10:00:00+02:00
- **THEN** the driver can obtain, for that predicate, the date 2026-11-01 as midnight in UTC+02:00, without evaluating the expression

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
A driver SHALL accept these operators and reject any other with an error naming the operator, field, and operator position: text `=`, `!=`, `CONTAINS`, `EXCLUDES`; integer, float, numeric, date, timestamp `=`, `!=`, `>`, `>=`, `<`, `<=`, `BETWEEN`; boolean, uuid, enum `=`, `!=`. Every type SHALL also accept `= NULL` and `!= NULL`, whether or not the field is declared nullable.

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

### Requirement: Literal conversion per type
A driver SHALL convert each filter literal to its field's type and reject mismatches with an error at the literal's position: text, uuid, enum, date, timestamp need a string; integer, float, numeric need a number; boolean needs `TRUE` or `FALSE`. Integers SHALL fit 64 bits, floats SHALL be finite, numerics SHALL keep exact digits, dates SHALL be `YYYY-MM-DD`, timestamps SHALL be RFC 3339 with an offset, uuids SHALL be canonical hyphenated hex, and enums SHALL be a declared value. For an expression literal the driver SHALL use the plan's evaluated result: a number result SHALL follow the number rules above.

#### Scenario: Enum value not declared
- **WHEN** a filter is `status = 'archived'` and `status` declares only `active` and `blocked`
- **THEN** compilation fails with an error naming `archived` at the literal's position

#### Scenario: Timestamp without offset
- **WHEN** a filter is `created_at > '2026-10-25T10:00:00'`
- **THEN** compilation fails because the timestamp has no offset

#### Scenario: Invalid uuid
- **WHEN** a filter is `id = 'not-a-uuid'` for uuid field `id`
- **THEN** compilation fails at the literal's position

#### Scenario: Evaluated number
- **WHEN** a filter is `qty > {2 * 3}` for integer field `qty`
- **THEN** compilation succeeds as if the filter were `qty > 6`

#### Scenario: Evaluated number not an integer
- **WHEN** a filter is `qty > {10 / 4}` for integer field `qty`
- **THEN** compilation fails at the literal's position because `2.5` is not an integer

### Requirement: NULL literals
A NULL literal SHALL be valid for every type and SHALL NOT be converted or bound as an argument.

#### Scenario: NULL literal binds nothing
- **WHEN** a filter is `status = NULL` for enum field `status`
- **THEN** compilation succeeds without checking declared values, and the query has no argument for the NULL literal

### Requirement: Scope combination
When the plan has a scope, the driver SHALL combine it with the filter so that rows must satisfy both, regardless of `OR` connectors inside the filter.

#### Scenario: OR filter under scope
- **WHEN** scope is `u.deleted_at IS NULL` and the filter is `name = 'a' OR name = 'b'`
- **THEN** only rows that satisfy the scope and either name comparison match

### Requirement: Inequality semantics
`!=` SHALL match exactly the rows that `=` with the same value does not match, so rows whose field is NULL match it.

#### Scenario: Not equal matches NULL
- **WHEN** a filter is `country != 'FR'` and a row has `country` NULL
- **THEN** that row matches

#### Scenario: Not equal skips the equal value
- **WHEN** a filter is `country != 'FR'` and a row has `country` equal to `FR`
- **THEN** that row does not match

#### Scenario: Keeping the old behaviour
- **WHEN** a filter is `country != 'FR' AND country != NULL` and a row has `country` NULL
- **THEN** that row does not match

### Requirement: EXCLUDES semantics
Text `EXCLUDES 'a'` SHALL match rows whose field does not contain `a` under the same matching rules as `CONTAINS`, and rows whose field is NULL.

#### Scenario: EXCLUDES matches NULL
- **WHEN** a filter is `country EXCLUDES 'fr'` and a row has `country` NULL
- **THEN** that row matches

#### Scenario: EXCLUDES skips containing values
- **WHEN** a filter is `country EXCLUDES 'fr'` and rows have `country` equal to `France` and `Spain`
- **THEN** the `Spain` row matches and the `France` row does not

### Requirement: NULL comparison semantics
`= NULL` SHALL match only rows whose field is NULL, and `!= NULL` SHALL match only rows whose field is not NULL.

#### Scenario: Equal to NULL
- **WHEN** a filter is `country = NULL` and rows have `country` NULL and `FR`
- **THEN** only the row with `country` NULL matches

#### Scenario: Not equal to NULL
- **WHEN** a filter is `country != NULL` and rows have `country` NULL and `FR`
- **THEN** only the row with `country` equal to `FR` matches

### Requirement: BETWEEN semantics
`BETWEEN 'a,b'` SHALL match values from `a` to `b` inclusive, after trimming spaces around each bound and converting both to the field's type.

#### Scenario: Timestamp range
- **WHEN** a filter is `created_at BETWEEN '2026-01-01T00:00:00Z, 2026-02-01T00:00:00Z'`
- **THEN** rows with `created_at` in that inclusive range match

### Requirement: Logical grouping
`AND` and `OR` SHALL keep the grouping of the syntax tree.

#### Scenario: Grouped OR under AND
- **WHEN** a filter is `(name = 'a' OR name = 'b') AND active = TRUE`
- **THEN** only active rows whose name is `a` or `b` match

### Requirement: Array value transport
A driver SHALL select each array field so that its value reaches the runtime as JSON array text, or as SQL NULL when the array is NULL. Elements SHALL be JSON strings for text, enum, uuid, date, and timestamp, numbers for integer, float, and numeric, and booleans for boolean.

#### Scenario: Selected array field
- **WHEN** a page plan selects text array field `tags` holding `{go,rust}`
- **THEN** the query returns `["go","rust"]` as the value of column `tags`

#### Scenario: NULL array field
- **WHEN** a page plan selects nullable array field `tags` that is NULL for a row
- **THEN** the query returns SQL NULL for `tags` on that row

#### Scenario: Element encodings
- **WHEN** array fields of type numeric, date, and timestamp are selected
- **THEN** numerics arrive as JSON numbers with exact digits, dates as `YYYY-MM-DD` strings, and timestamps as ISO 8601 strings, either with an offset or, for values without a time zone, without one

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

### Requirement: Array value lists
For array fields, a string literal SHALL be a comma-separated list of one or more values. Each item SHALL be trimmed and converted to the element type under the scalar literal rules, and an empty item SHALL be an error. An unquoted number or boolean SHALL be a single value. Errors SHALL report the literal's position.

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

### Requirement: Array CONTAINS semantics
`CONTAINS` on an array field SHALL match rows whose array has, for every listed value, an element equal to it. Equality SHALL be exact for every element type, including case-sensitive text. A NULL array SHALL NOT match.

#### Scenario: All values present
- **WHEN** a filter is `statuses CONTAINS 'ok,passed'` and rows hold `{ok,passed,retried}`, `{ok}`, and NULL
- **THEN** only the `{ok,passed,retried}` row matches

#### Scenario: Exact element match
- **WHEN** a filter is `tags CONTAINS 'go'` and a row holds `{golang,Go}`
- **THEN** that row does not match

### Requirement: Array EXCLUDES semantics
`EXCLUDES` on an array field SHALL match rows whose array has no element equal to any listed value, including empty arrays, and rows whose field is NULL.

#### Scenario: Any listed value excludes the row
- **WHEN** a filter is `statuses EXCLUDES 'skipped,failed'` and rows hold `{ok}`, `{ok,skipped}`, `{skipped,failed}`, an empty array, and NULL
- **THEN** the `{ok}`, empty array, and NULL rows match, and the `{ok,skipped}` and `{skipped,failed}` rows do not

### Requirement: Relative time comparison
A date result compared with a date field SHALL compare with that date. A timestamp result compared with a timestamp field SHALL compare with that instant.

#### Scenario: Date field
- **WHEN** a filter is `due > {TODAY + 5::DAYS}` for date field `due`, evaluated on 2026-10-27
- **THEN** rows match when `due` is after 2026-11-01

#### Scenario: Timestamp field
- **WHEN** a filter is `seen_at > {NOW - 30::MINUTES}` evaluated at 2026-10-27T10:00:00Z
- **THEN** rows match when `seen_at` is after 2026-10-27T09:30:00Z

### Requirement: Whole-day comparison on timestamp fields
A date result `D` compared with a timestamp field SHALL cover the whole day `D` in the time zone of the result. With `S` the instant at which `D` starts and `E` the instant at which the next day starts, `> D` SHALL match values `>= E`, `>= D` values `>= S`, `< D` values `< S`, `<= D` values `< E`, `= D` values in `[S, E)`, and `!= D` every row that `= D` does not match, including NULL.

#### Scenario: Whole last day included
- **WHEN** a filter is `created_at <= END_OF_WEEK` for a timestamp field, evaluated on Tuesday 2026-10-27 in UTC with a Monday week start, and a row has `created_at` 2026-11-01T23:59:59Z
- **THEN** that row matches

#### Scenario: Next day excluded
- **WHEN** the same filter meets a row with `created_at` 2026-11-02T00:00:00Z
- **THEN** that row does not match

#### Scenario: Equal to a day
- **WHEN** a filter is `created_at = TODAY` evaluated on 2026-10-27 in UTC+02:00
- **THEN** rows match when `created_at` is at or after 2026-10-26T22:00:00Z and before 2026-10-27T22:00:00Z

#### Scenario: Not equal to a day matches NULL
- **WHEN** a filter is `created_at != TODAY` and a row has `created_at` NULL
- **THEN** that row matches
