# Spec Delta

## Purpose

Generates, at gqlgen code-generation time, the GraphQL types and Go bindings for every registered list, so an application can attach list queries and export mutations anywhere in its own gqlgen schema.

## ADDED Requirements

### Requirement: Generation entrypoint
The library SHALL provide a gqlgen plugin built from the application's list registry. Running gqlgen generation with the plugin SHALL add the list types to the schema and generate their Go bindings in one run, without a separate generation step.

#### Scenario: Plugin in a custom entrypoint
- **WHEN** an application runs gqlgen generation with the plugin built from lists `users` and `orders`
- **THEN** the generated executable schema contains the types for both lists

### Requirement: Type name prefix
Each list's GraphQL type names SHALL start with the PascalCase form of the list name, splitting on underscores.

#### Scenario: Multi-word list name
- **WHEN** a list is named `order_items`
- **THEN** its row type is `OrderItemsRow`

### Requirement: Row and page types
For each list the plugin SHALL generate `<P>Row`, with one field per SQL and calculated field named as the field, non-null unless the field is nullable, and `<P>Page` with `items: [<P>Row!]!` and `totalCount: Int!`.

#### Scenario: Nullable and non-null fields
- **WHEN** `users` has non-null `name` (text) and nullable `country` (text)
- **THEN** `UsersRow` has `name: String!` and `country: String`

### Requirement: GraphQL type mapping
Field types SHALL map to GraphQL as: text `String`, integer `Int64`, float `Float`, numeric `Decimal` (serialized as a string), boolean `Boolean`, date `Date` (`YYYY-MM-DD`), timestamp `Time` (RFC 3339), uuid `UUID`, and enum a generated enum `<P><Field>` whose values are the declared values in upper snake case.

#### Scenario: Enum field
- **WHEN** `users` has enum field `status` with values `active` and `on_hold`
- **THEN** the schema has `enum UsersStatus { ACTIVE ON_HOLD }` and `UsersRow.status` uses it

#### Scenario: Numeric field
- **WHEN** a numeric value `10.50` is returned
- **THEN** the GraphQL response contains the string `"10.50"`

### Requirement: List input
For each list the plugin SHALL generate `<P>SortField` (one value per SQL field, in upper snake case), `input <P>Sort { field: <P>SortField!, direction: SortDirection! = ASC }`, and `input <P>ListInput { filter: String, sort: [<P>Sort!], limit: Int, offset: Int }`.

#### Scenario: Calculated fields are not sortable
- **WHEN** `users` has calculated field `display`
- **THEN** `UsersSortField` has no `DISPLAY` value

### Requirement: Export input and job types
For each list the plugin SHALL generate `<P>ExportField` (one value per SQL and calculated field) and `input <P>ExportInput { fields: [<P>ExportField!]!, filter: String, sort: [<P>Sort!], format: ExportFormat! }`. It SHALL generate once `enum ExportFormat { CSV XLSX }` and `type ExportJob { id: ID! }`.

#### Scenario: Export field enum
- **WHEN** `users` has SQL fields `id`, `name` and calculated field `display`
- **THEN** `UsersExportField` has values `ID`, `NAME`, and `DISPLAY`

### Requirement: Shared types
The plugin SHALL declare `SortDirection { ASC DESC }` and the scalars it uses once, regardless of the number of lists. The names of the `Int64`, `Decimal`, `Date`, and `UUID` scalars SHALL be configurable.

#### Scenario: Renamed scalar
- **WHEN** the plugin is configured to name the UUID scalar `EasyUUID`
- **THEN** uuid fields use `EasyUUID` and no `UUID` scalar is declared

### Requirement: No root fields
The plugin SHALL NOT add fields to `Query`, `Mutation`, or any other application type. Attaching a list query or export mutation is done by the application in its own schema.

#### Scenario: Application attaches a list
- **WHEN** the application declares `extend type Query { people(query: UsersListInput!): UsersPage! }`
- **THEN** generation succeeds and `people` returns the `users` list

### Requirement: Name collisions
Generation SHALL fail with an error naming the type when a type the plugin would declare already exists in the application's schema.

#### Scenario: Existing UsersRow type
- **WHEN** the application schema already declares `type UsersRow`
- **THEN** generation fails with an error naming `UsersRow`

### Requirement: Generated code import direction
The generated Go code MAY import the application's list definitions, and the list definitions SHALL NOT need to import generated code. The documentation SHALL warn that importing gqlgen-generated packages from list definitions creates an import cycle.

#### Scenario: Definitions compile before generation
- **WHEN** the list definitions package is compiled with no generated code present
- **THEN** it compiles, so the gqlgen entrypoint can import it
