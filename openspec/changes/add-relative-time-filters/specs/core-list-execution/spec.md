# Spec Delta

## ADDED Requirements

### Requirement: Runtime clock and week start
The runtime SHALL have an optional clock and an optional first day of the week. Without a clock it SHALL use the system clock in the server's local time zone. When it runs a request that has no reference time, it SHALL use the clock's current time as the reference time; when the request has no week start, it SHALL use the runtime's, and Monday when the runtime has none. A request's own reference time and week start SHALL take precedence.

#### Scenario: Server local time by default
- **WHEN** a runtime without a clock fetches a request with filter `due = TODAY` and no reference time
- **THEN** `TODAY` is the current date in the server's local time zone

#### Scenario: Application week start
- **WHEN** the runtime's week start is Sunday and a request with filter `created_at >= START_OF_WEEK` and no week start is fetched
- **THEN** the week starts on Sunday

#### Scenario: Request reference time wins
- **WHEN** the runtime has a clock and a request carries reference time 2026-01-01T00:00:00Z
- **THEN** relative values are evaluated at 2026-01-01T00:00:00Z
