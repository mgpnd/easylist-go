# Spec Delta

## MODIFIED Requirements

### Requirement: List input
For each list the plugin SHALL generate `<P>SortField` (one value per SQL field that is not an array, in upper snake case), `input <P>Sort { field: <P>SortField!, direction: SortDirection! = ASC }`, and `input <P>ListInput { filter: String, sort: [<P>Sort!], limit: Int, offset: Int }`.

#### Scenario: Calculated fields are not sortable
- **WHEN** `users` has calculated field `display`
- **THEN** `UsersSortField` has no `DISPLAY` value

#### Scenario: Array fields are not sortable
- **WHEN** `users` has array field `tags`
- **THEN** `UsersSortField` has no `TAGS` value

## ADDED Requirements

### Requirement: Array field types
An array field SHALL map to a GraphQL list of its element type with non-null elements: `[T!]!` when the field is not nullable and `[T!]` when it is. The element type SHALL follow the scalar type mapping, and an enum array SHALL use the field's generated enum. A NULL array SHALL resolve to `null` and an empty array to `[]`.

#### Scenario: Non-null text array
- **WHEN** `users` has non-nullable text array field `tags`
- **THEN** `UsersRow` has `tags: [String!]!`

#### Scenario: Nullable enum array
- **WHEN** `users` has nullable enum array field `statuses` with values `ok` and `failed`
- **THEN** the schema has `enum UsersStatuses { OK FAILED }` and `UsersRow` has `statuses: [UsersStatuses!]`

#### Scenario: Custom scalar array
- **WHEN** `users` has uuid array field `group_ids`
- **THEN** `UsersRow` has `group_ids: [UUID!]!` and the `UUID` scalar is declared

#### Scenario: NULL and empty arrays
- **WHEN** nullable `tags` is NULL on one row and an empty array on another
- **THEN** the response has `"tags": null` for the first row and `"tags": []` for the second
