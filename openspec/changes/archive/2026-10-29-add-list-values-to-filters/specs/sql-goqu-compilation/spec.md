# Spec Delta

## ADDED Requirements

### Requirement: PostgreSQL IN lists
The driver SHALL compile `IN` and `NOT IN` to an SQL `IN` or `NOT IN` over one bound argument per non-NULL item, in list order, so that PostgreSQL infers each argument's type from the column. NULL items SHALL never appear inside the SQL list. The field's SQL SHALL be emitted once per predicate.

#### Scenario: Argument order
- **WHEN** a plan filters `status IN ('a', NULL, 'b') AND age < 30` with limit 10 and offset 0
- **THEN** the arguments are `a`, `b`, `30`, `10`, `0`, bound to `$1` through `$5`

#### Scenario: Enum list without a cast
- **WHEN** a plan filters `status IN ('active', 'blocked')` for an enum field stored as a PostgreSQL enum
- **THEN** the SQL contains no cast to a named type, and the arguments are compared with `status` directly

### Requirement: PostgreSQL IN NULL handling
The driver SHALL produce the `IN` and `NOT IN` NULL semantics without placing NULL in an SQL list. A list of only NULL items SHALL compile to `IS NULL` for `IN` and `IS NOT NULL` for `NOT IN`, with no bound argument. Otherwise the result of the SQL `IN` or `NOT IN` for a NULL field SHALL be replaced by whether that row must match: `IN` with a NULL item and `NOT IN` without one match it, and the others do not.

#### Scenario: Only NULL
- **WHEN** a plan filters `status IN (NULL) AND name NOT IN (NULL)` with limit 10 and offset 0
- **THEN** the SQL tests `status` with `IS NULL` and `name` with `IS NOT NULL`, and the arguments are only `10` and `0`

#### Scenario: NOT IN without a NULL item
- **WHEN** a plan filters `status NOT IN ('error')` and a row's `status` is NULL
- **THEN** that row matches

#### Scenario: IN with a NULL item
- **WHEN** a plan filters `status IN ('ok', NULL)` and a row's `status` is NULL
- **THEN** that row matches, and the SQL list holds only the argument for `ok`
