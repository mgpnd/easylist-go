# Spec Delta

## ADDED Requirements

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
