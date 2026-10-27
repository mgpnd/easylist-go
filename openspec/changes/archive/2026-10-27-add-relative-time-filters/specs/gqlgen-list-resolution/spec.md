# Spec Delta

## MODIFIED Requirements

### Requirement: Input mapping and errors
`filter` SHALL be parsed with easylist-filter; an empty or absent filter means no filter. Sort entries, limit, and offset SHALL be passed to the planner. When `timezone` is set, the reference time SHALL be the runtime clock's current time in a fixed zone with that UTC offset in minutes; when it is absent, the runtime's default reference time applies. A `timezone` outside -720 to 840 SHALL be an error. Parse, timezone, and planning errors SHALL be returned as GraphQL errors on the list field, keeping their position information, without running any query.

#### Scenario: Filter syntax error
- **WHEN** `filter` is `name = `
- **THEN** the field resolves to an error that includes the position, and no query runs

#### Scenario: Limit above maximum
- **WHEN** `limit` is 20000 for a list with maximum page size 10000
- **THEN** the field resolves to an error stating the maximum

#### Scenario: Client time zone
- **WHEN** `filter` is `created_at = TODAY`, `timezone` is `-300`, and the runtime clock reads 2026-10-28T02:00:00Z
- **THEN** `TODAY` is 2026-10-27, the date in UTC-05:00

#### Scenario: Timezone out of range
- **WHEN** `timezone` is `-800`
- **THEN** the field resolves to an error stating the allowed range, and no query runs

### Requirement: One-call export resolver
For each list the library SHALL provide a function that an application mutation resolver calls with the request context, the runtime, and the `<P>ExportInput`. It SHALL validate the input into an export request whose request moment is the input's `asOf`, or the runtime clock's current time when `asOf` is absent, stored in UTC, and whose UTC offset is the input's `timezone`, pass it to the application's exporter, and return `ExportJob` with the exporter's job identifier.

#### Scenario: Export accepted
- **WHEN** the mutation is called with valid fields and the exporter returns `job-42`
- **THEN** the mutation returns `{ id: "job-42" }`

#### Scenario: Invalid export filter
- **WHEN** the mutation is called with filter `nickname = 'a'` on a list without `nickname`
- **THEN** the mutation returns an error and the exporter is not called

#### Scenario: No exporter configured
- **WHEN** the runtime has no exporter and the export function is called
- **THEN** it returns an error stating that exports are not configured

#### Scenario: Request moment defaults to server time
- **WHEN** the mutation is called with `timezone: 120` and no `asOf` while the runtime clock reads 2026-10-27T10:00:00+02:00
- **THEN** the exporter receives a request with request moment 2026-10-27T08:00:00Z and offset `120`

#### Scenario: Client-supplied asOf
- **WHEN** the mutation is called with `asOf: "2026-10-01T09:00:00+02:00"`
- **THEN** the exporter receives a request with request moment 2026-10-01T07:00:00Z, whatever the runtime clock reads
