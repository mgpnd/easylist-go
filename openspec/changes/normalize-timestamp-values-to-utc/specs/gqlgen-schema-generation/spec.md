# Spec Delta

## MODIFIED Requirements

### Requirement: GraphQL type mapping
Field types SHALL map to GraphQL as: text `String`, integer `Int64`, float `Float`, numeric `Decimal` (serialized as a string), boolean `Boolean`, date `Date` (`YYYY-MM-DD`), timestamp `Time` (RFC 3339), uuid `UUID`, and enum a generated enum `<P><Field>` whose values are the declared values in upper snake case.

#### Scenario: Enum field
- **WHEN** `users` has enum field `status` with values `active` and `on_hold`
- **THEN** the schema has `enum UsersStatus { ACTIVE ON_HOLD }` and `UsersRow.status` uses it

#### Scenario: Numeric field
- **WHEN** a numeric value `10.50` is returned
- **THEN** the GraphQL response contains the string `"10.50"`

#### Scenario: Timestamp field in UTC
- **WHEN** timestamp SQL field `created_at` holds `2026-10-25 10:00:00+02` and the server's local time zone is UTC+03:00
- **THEN** the GraphQL response contains the string `"2026-10-25T08:00:00Z"`, as it does for the same instant in a timestamp array field
