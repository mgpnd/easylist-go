# Spec Delta

## Purpose

Resolves list queries and export mutations at runtime inside an application's gqlgen resolvers, so a resolver body is a single call and SQL only covers what the GraphQL operation selected.

## ADDED Requirements

### Requirement: One-call list resolver
For each list the library SHALL provide a function that an application resolver calls with the request context, the application's easylist runtime (driver and database), and the `<P>ListInput` value, returning the `<P>Page` value or an error.

#### Scenario: Resolver delegates
- **WHEN** a resolver for `people(query: UsersListInput!): UsersPage!` calls the `users` resolve function
- **THEN** the field returns the `users` page for that input

### Requirement: Selection-driven fields
The resolver SHALL derive the selected fields from the GraphQL selection under `items`, including fields reached through fragments and inline fragments, and ignoring `__typename`. Only those fields SHALL be requested from the planner.

#### Scenario: Fragment selection
- **WHEN** the operation selects `items { ...UserBits }` with fragment `UserBits` selecting `name`
- **THEN** the page plan selects the primary key and `name` only

#### Scenario: Aliased field
- **WHEN** the operation selects `items { n: name }`
- **THEN** the page plan selects `name`

### Requirement: Selection-driven queries
The resolver SHALL run the page query only when `items` is selected and the count query only when `totalCount` is selected.

#### Scenario: Only totalCount selected
- **WHEN** the operation selects only `totalCount`
- **THEN** only the count query runs

### Requirement: Input mapping and errors
`filter` SHALL be parsed with easylist-filter; an empty or absent filter means no filter. Sort entries, limit, and offset SHALL be passed to the planner. Parse and planning errors SHALL be returned as GraphQL errors on the list field, keeping their position information, without running any query.

#### Scenario: Filter syntax error
- **WHEN** `filter` is `name = `
- **THEN** the field resolves to an error that includes the position, and no query runs

#### Scenario: Limit above maximum
- **WHEN** `limit` is 20000 for a list with maximum page size 10000
- **THEN** the field resolves to an error stating the maximum

### Requirement: Calculated field resolution
A calculated field on `<P>Row` SHALL be resolved by calling its function for that row only when the field is selected. An error from the function SHALL become a GraphQL error on that field of that row.

#### Scenario: Calculated field error
- **WHEN** `display` returns an error for the second row
- **THEN** the response has a field error at `items[1].display` and the other rows resolve normally

### Requirement: One-call export resolver
For each list the library SHALL provide a function that an application mutation resolver calls with the request context, the runtime, and the `<P>ExportInput`. It SHALL validate the input into an export request, pass it to the application's exporter, and return `ExportJob` with the exporter's job identifier.

#### Scenario: Export accepted
- **WHEN** the mutation is called with valid fields and the exporter returns `job-42`
- **THEN** the mutation returns `{ id: "job-42" }`

#### Scenario: Invalid export filter
- **WHEN** the mutation is called with filter `nickname = 'a'` on a list without `nickname`
- **THEN** the mutation returns an error and the exporter is not called

#### Scenario: No exporter configured
- **WHEN** the runtime has no exporter and the export function is called
- **THEN** it returns an error stating that exports are not configured
