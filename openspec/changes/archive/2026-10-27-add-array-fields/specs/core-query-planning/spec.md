# Spec Delta

## ADDED Requirements

### Requirement: Array fields are not sortable
Planning SHALL fail with an error naming the field when a sort entry names an array field. Array fields MAY still be selected and filtered.

#### Scenario: Sort on an array field
- **WHEN** a request sorts by array field `tags`
- **THEN** planning fails with an error naming `tags`

#### Scenario: Select and filter an array field
- **WHEN** a request selects `tags` and filters `tags CONTAINS 'go'`
- **THEN** planning succeeds and the plan marks `tags` as an array field
