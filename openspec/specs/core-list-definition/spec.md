# core-list-definition Specification

## Purpose

Lets an application declare a list once, as a Go value, describing the base table, the SQL and calculated fields it exposes, the optional joins those fields need, and the limits that apply, and rejects inconsistent definitions before any request runs.

## Requirements

### Requirement: List identity and base table
A list SHALL declare a unique name, a base table, and a base table alias. The name SHALL match `^[a-z][a-z0-9_]*$`. The table and alias are trusted SQL text written by the list author and are emitted verbatim.

#### Scenario: Valid list identity
- **WHEN** a list is declared with name `users`, table `users`, and alias `u`
- **THEN** the definition is accepted

#### Scenario: Invalid list name
- **WHEN** a list is declared with name `Users-List`
- **THEN** building the list fails with an error naming the invalid list name

#### Scenario: Duplicate list names in a registry
- **WHEN** two lists named `users` are registered together
- **THEN** building the registry fails with an error naming `users`

### Requirement: SQL field declaration
A list SHALL declare one or more SQL fields. Each SQL field SHALL have a name unique within the list across SQL and calculated fields, a type, and a nullability flag. It MAY have an SQL expression, a list of join keys, and a title. Field names SHALL match `^[a-z_][a-z0-9_]*$`.

#### Scenario: Field without SQL expression
- **WHEN** a SQL field `name` has no SQL expression
- **THEN** its value is read from the column `name` of the base table alias

#### Scenario: Field with SQL expression and joins
- **WHEN** a SQL field `country` declares SQL `c.name` and joins `["country"]`
- **THEN** its value is read from `c.name` and selecting, filtering, or sorting by it requires the `country` join

#### Scenario: Duplicate field name
- **WHEN** a SQL field and a calculated field are both named `display`
- **THEN** building the list fails with an error naming `display`

#### Scenario: Invalid field name
- **WHEN** a SQL field is named `firstName`
- **THEN** building the list fails with an error naming `firstName`

#### Scenario: Default title
- **WHEN** a SQL field `last_order` has no title
- **THEN** its title is `last_order`

### Requirement: Supported field types
Every SQL and calculated field SHALL have exactly one of the types text, integer, float, numeric, boolean, date, timestamp, uuid, or enum. An enum field SHALL declare a non-empty list of distinct string values, and each value SHALL map to a distinct GraphQL enum value name.

#### Scenario: Enum without values
- **WHEN** an enum field `status` declares no values
- **THEN** building the list fails with an error naming `status`

#### Scenario: Enum values that collide
- **WHEN** an enum field declares values `in-progress` and `in_progress`
- **THEN** building the list fails because both map to the GraphQL enum value `IN_PROGRESS`

### Requirement: Primary key declaration
A list SHALL name exactly one of its SQL fields as the primary key. The primary key field SHALL NOT be nullable.

#### Scenario: Missing primary key
- **WHEN** a list declares no primary key
- **THEN** building the list fails

#### Scenario: Primary key references an unknown field
- **WHEN** a list declares primary key `uid` and has no SQL field `uid`
- **THEN** building the list fails with an error naming `uid`

#### Scenario: Nullable primary key
- **WHEN** the primary key field is declared nullable
- **THEN** building the list fails

### Requirement: Join declaration
A list MAY declare joins keyed by name. Each join SHALL have trusted SQL text (for example a `LEFT JOIN` or `LEFT JOIN LATERAL` clause) and MAY list the keys of joins it depends on. Every join key referenced by a field or by `depends_on` SHALL exist, and the dependency graph SHALL be acyclic.

#### Scenario: Field references an unknown join
- **WHEN** a field declares join `region` and no join `region` exists
- **THEN** building the list fails with an error naming the field and `region`

#### Scenario: Join depends on an unknown join
- **WHEN** join `city` depends on `region` and no join `region` exists
- **THEN** building the list fails with an error naming `city` and `region`

#### Scenario: Join dependency cycle
- **WHEN** join `a` depends on `b` and join `b` depends on `a`
- **THEN** building the list fails with an error listing the cycle

### Requirement: Join cardinality is the author's responsibility
Each join SHALL yield at most one row per base row. The library SHALL NOT verify this; the documentation of list definitions SHALL state that joins producing several rows per base row cause duplicated items and wrong counts.

#### Scenario: Documented join contract
- **WHEN** a developer reads the list definition documentation
- **THEN** it states that each join must return at most one row per base row and that this is not enforced

### Requirement: Calculated field declaration
A list MAY declare calculated fields. Each calculated field SHALL have a name, a type, a nullability flag, an optional title, a per-row function, and the names of the SQL fields it requires. Every required name SHALL be a SQL field of the same list.

#### Scenario: Calculated field requires an unknown field
- **WHEN** calculated field `display` requires `nickname` and no SQL field `nickname` exists
- **THEN** building the list fails with an error naming `display` and `nickname`

#### Scenario: Calculated field requires another calculated field
- **WHEN** calculated field `label` requires calculated field `display`
- **THEN** building the list fails because calculated fields may only require SQL fields

### Requirement: Static base scope
A list MAY declare a base scope: a static, trusted SQL condition that does not depend on the request. When present, it SHALL apply to every page, count, and export query of the list.

#### Scenario: Scope declared
- **WHEN** a list declares scope `u.deleted_at IS NULL`
- **THEN** every query generated for the list includes that condition combined with the request filter by AND

#### Scenario: Scope absent
- **WHEN** a list declares no scope
- **THEN** generated queries contain only the request filter conditions

### Requirement: Page size limits
A list SHALL have a default page size and a maximum page size. When not declared, they SHALL be 100 and 10000. A declared default SHALL be at least 1 and no greater than the maximum.

#### Scenario: Library defaults
- **WHEN** a list declares no page size limits
- **THEN** its default page size is 100 and its maximum page size is 10000

#### Scenario: Default above maximum
- **WHEN** a list declares default page size 500 and maximum page size 200
- **THEN** building the list fails

### Requirement: Array fields
Any SQL or calculated field SHALL be declarable as an array of its type, for every supported type. An array enum field SHALL declare its values like a scalar enum field. The field's nullability SHALL apply to the array as a whole; array elements are never nullable.

#### Scenario: Array of each type
- **WHEN** a list declares array fields of types text, integer, float, numeric, boolean, date, timestamp, uuid, and enum
- **THEN** the definition is accepted

#### Scenario: Array enum without values
- **WHEN** an array enum field `statuses` declares no values
- **THEN** building the list fails with an error naming `statuses`

#### Scenario: Calculated array field
- **WHEN** a calculated text field `labels` is declared with a function returning a slice of strings
- **THEN** the definition is accepted and `labels` is an array field

#### Scenario: Calculated array field with a mismatched result
- **WHEN** a calculated array field of type integer is declared with a function returning a slice of strings
- **THEN** building the list fails with an error naming the field and both Go types

### Requirement: Array primary key rejected
A list SHALL NOT name an array field as its primary key.

#### Scenario: Array primary key
- **WHEN** a list names array field `ids` as its primary key
- **THEN** building the list fails with an error naming `ids`
