# Spec Delta

## MODIFIED Requirements

### Requirement: List input
For each list the plugin SHALL generate `<P>SortField` (one value per SQL field that is not an array, in upper snake case), `input <P>Sort { field: <P>SortField!, direction: SortDirection! = ASC }`, and `input <P>ListInput { filter: String, sort: [<P>Sort!], limit: Int, offset: Int, timezone: Int }`. `timezone` SHALL be documented in the SDL as the client's UTC offset in minutes, positive east of UTC.

#### Scenario: Calculated fields are not sortable
- **WHEN** `users` has calculated field `display`
- **THEN** `UsersSortField` has no `DISPLAY` value

#### Scenario: Array fields are not sortable
- **WHEN** `users` has array field `tags`
- **THEN** `UsersSortField` has no `TAGS` value

#### Scenario: Timezone input field
- **WHEN** the plugin generates `UsersListInput`
- **THEN** it has an optional `timezone: Int` field

### Requirement: Export input and job types
For each list the plugin SHALL generate `<P>ExportField` (one value per SQL and calculated field) and `input <P>ExportInput { fields: [<P>ExportField!]!, filter: String, sort: [<P>Sort!], format: ExportFormat!, timezone: Int, asOf: Time }`, with `timezone` documented as for `<P>ListInput` and the optional `asOf` documented as the moment at which relative time values are evaluated, defaulting to the server's current time. `asOf` SHALL use the plugin's timestamp scalar, so that scalar is declared whenever an export input is generated. It SHALL generate once `enum ExportFormat { CSV XLSX }` and `type ExportJob { id: ID! }`.

#### Scenario: Export field enum
- **WHEN** `users` has SQL fields `id`, `name` and calculated field `display`
- **THEN** `UsersExportField` has values `ID`, `NAME`, and `DISPLAY`

#### Scenario: Optional asOf
- **WHEN** the plugin generates `UsersExportInput` for a list with no timestamp fields
- **THEN** it has an optional `asOf: Time` field and the schema declares the `Time` scalar
