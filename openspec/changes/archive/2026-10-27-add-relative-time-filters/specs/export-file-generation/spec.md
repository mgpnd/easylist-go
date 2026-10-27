# Spec Delta

## ADDED Requirements

### Requirement: Relative values at request time
The worker SHALL evaluate relative time values at the request moment, in the time zone given by the request's UTC offset, or the server's local time zone when no offset is set. A request without a request moment SHALL use the runtime's clock. The worker SHALL use the runtime's week start.

#### Scenario: Job runs the next day
- **WHEN** a request with filter `created_at = TODAY`, request moment 2026-10-27T21:00:00Z, and offset `120` runs on 2026-10-28
- **THEN** the file holds the rows created on 2026-10-27 in UTC+02:00

#### Scenario: Old request
- **WHEN** a request without a request moment runs
- **THEN** relative values are evaluated at the runtime clock's current time
