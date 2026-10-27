# Spec Delta

## MODIFIED Requirements

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
A driver SHALL convert each filter literal to its field's type and reject mismatches with an error at the literal's position: text, uuid, enum, date, timestamp need a string; integer, float, numeric need a number; boolean needs `TRUE` or `FALSE`. Integers SHALL fit 64 bits, floats SHALL be finite, numerics SHALL keep exact digits, dates SHALL be `YYYY-MM-DD`, timestamps SHALL be RFC 3339 with an offset, uuids SHALL be canonical hyphenated hex, and enums SHALL be a declared value. A NULL literal SHALL be valid for every type and SHALL NOT be converted or bound as an argument.

#### Scenario: Enum value not declared
- **WHEN** a filter is `status = 'archived'` and `status` declares only `active` and `blocked`
- **THEN** compilation fails with an error naming `archived` at the literal's position

#### Scenario: Timestamp without offset
- **WHEN** a filter is `created_at > '2026-10-25T10:00:00'`
- **THEN** compilation fails because the timestamp has no offset

#### Scenario: Invalid uuid
- **WHEN** a filter is `id = 'not-a-uuid'` for uuid field `id`
- **THEN** compilation fails at the literal's position

#### Scenario: NULL literal binds nothing
- **WHEN** a filter is `status = NULL` for enum field `status`
- **THEN** compilation succeeds without checking declared values, and the query has no argument for the NULL literal

## REMOVED Requirements

### Requirement: Comparison semantics
**Reason**: `!=` no longer behaves as SQL `<>`: it now matches rows whose field is NULL, and the requirement gained `EXCLUDES` and NULL comparisons. It is split into one requirement per behaviour below, without the "Not equal skips NULL" scenario that no longer holds.
**Migration**: See the ADDED requirements "Inequality semantics", "EXCLUDES semantics", "NULL comparison semantics", "BETWEEN semantics", and "Logical grouping". Filters that relied on `!=` skipping NULL rows can add `AND field != NULL`.

## ADDED Requirements

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
