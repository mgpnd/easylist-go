# core-query-planning Specification

## Purpose

Turns one list request (selected fields, filter, sort, page) into a dialect-neutral page plan and count plan that include only the expressions and joins the request actually uses.

## Requirements

### Requirement: Request validation
A list request SHALL name the selected fields, and MAY carry a filter syntax tree, sort entries, a limit, and an offset. Every selected name SHALL be a field of the list. Planning SHALL fail with an error naming the first unknown field.

#### Scenario: Unknown selected field
- **WHEN** a request selects `nickname` and the list has no such field
- **THEN** planning fails with an error naming `nickname`

#### Scenario: No selected fields
- **WHEN** a request selects no fields
- **THEN** planning succeeds and the page plan selects only the primary key

### Requirement: Selected expressions
The page plan SHALL select the primary key, every selected SQL field, and every SQL field required by a selected calculated field, each once. It SHALL NOT select fields that are only filtered or sorted on.

#### Scenario: Calculated field pulls in its requirements
- **WHEN** a request selects only calculated field `display`, which requires `name` and `country`
- **THEN** the page plan selects the primary key, `name`, and `country`

#### Scenario: Filter-only field is not selected
- **WHEN** a request selects `name` and filters on `country`
- **THEN** the page plan selects the primary key and `name`, but not `country`

### Requirement: Join resolution
The page plan SHALL include exactly the joins needed by selected SQL fields, SQL fields required by selected calculated fields, filtered fields, and sorted fields, plus every join those joins transitively depend on. Joins SHALL be ordered so that each join appears after all joins it depends on, and the order SHALL be deterministic for the same request.

#### Scenario: Unused join omitted
- **WHEN** a request selects only base-table fields
- **THEN** the page plan contains no joins

#### Scenario: Transitive dependency included
- **WHEN** a selected field needs join `city`, and `city` depends on `region`
- **THEN** the page plan contains `region` followed by `city`

#### Scenario: Join needed only by the filter
- **WHEN** a request filters on `country`, which needs join `country`, without selecting it
- **THEN** the page plan contains the `country` join

### Requirement: Filter field validation
Every field referenced in the filter syntax tree SHALL be a SQL field of the list. Referencing a calculated field or an unknown name SHALL fail planning with an error that includes the field's byte position in the filter text.

#### Scenario: Filtering on a calculated field
- **WHEN** a filter references calculated field `display`
- **THEN** planning fails with an error naming `display` and its position

#### Scenario: Filtering on an unknown field
- **WHEN** a filter `nickname = 'a'` is planned for a list without `nickname`
- **THEN** planning fails with an error naming `nickname` at position 1

### Requirement: Sorting
Each sort entry SHALL name a SQL field and a direction, ascending or descending. Sorting on a calculated or unknown field, or naming the same field twice, SHALL fail planning. When the request has no sort entries the plan SHALL order by the primary key ascending; otherwise the primary key ascending SHALL be appended as the final tiebreaker unless the request already sorts on it.

#### Scenario: Default sort
- **WHEN** a request has no sort entries
- **THEN** the page plan orders by the primary key ascending

#### Scenario: Primary key tiebreaker
- **WHEN** a request sorts by `name` descending
- **THEN** the page plan orders by `name` descending, then the primary key ascending

#### Scenario: Request already sorts on the primary key
- **WHEN** a request sorts by the primary key descending
- **THEN** the page plan orders only by the primary key descending

#### Scenario: Sorting on a calculated field
- **WHEN** a request sorts by calculated field `display`
- **THEN** planning fails with an error naming `display`

### Requirement: Page limits
When the request has no limit, the page plan SHALL use the list's default page size. A limit below 1 or above the list's maximum page size SHALL fail planning. When the request has no offset the plan SHALL use 0; a negative offset SHALL fail planning.

#### Scenario: Default limit
- **WHEN** a request for a list with default page size 100 has no limit
- **THEN** the page plan has limit 100 and offset 0

#### Scenario: Limit above maximum
- **WHEN** a request asks for limit 20000 on a list whose maximum page size is 10000
- **THEN** planning fails with an error stating the maximum

#### Scenario: Negative offset
- **WHEN** a request has offset -1
- **THEN** planning fails

### Requirement: Count plan
Planning SHALL also produce a count plan for the same request. The count plan SHALL carry the list's scope, the request filter, and only the joins (with their transitive dependencies) needed by filtered fields. It SHALL carry no selected fields, ordering, limit, or offset.

#### Scenario: Count ignores selection and sort joins
- **WHEN** a request selects `country` (join `country`), sorts by `last_order` (join `last_order`), and filters on `name`
- **THEN** the count plan contains no joins

#### Scenario: Count keeps filter joins
- **WHEN** a request filters on `country`
- **THEN** the count plan contains the `country` join

### Requirement: Export plan
Planning an export request SHALL produce a plan with the same selection, join, filter, scope, and ordering rules as a page plan, but with no limit and no offset.

#### Scenario: Export has no pagination
- **WHEN** an export request is planned for a list with maximum page size 10000
- **THEN** the plan has no limit and no offset

### Requirement: Array fields are not sortable
Planning SHALL fail with an error naming the field when a sort entry names an array field. Array fields MAY still be selected and filtered.

#### Scenario: Sort on an array field
- **WHEN** a request sorts by array field `tags`
- **THEN** planning fails with an error naming `tags`

#### Scenario: Select and filter an array field
- **WHEN** a request selects `tags` and filters `tags CONTAINS 'go'`
- **THEN** planning succeeds and the plan marks `tags` as an array field
