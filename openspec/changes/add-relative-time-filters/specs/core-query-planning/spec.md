# Spec Delta

## MODIFIED Requirements

### Requirement: Request validation
A list request SHALL name the selected fields, and MAY carry a filter syntax tree, sort entries, a limit, an offset, a reference time, and a first day of the week. Every selected name SHALL be a field of the list. Planning SHALL fail with an error naming the first unknown field.

#### Scenario: Unknown selected field
- **WHEN** a request selects `nickname` and the list has no such field
- **THEN** planning fails with an error naming `nickname`

#### Scenario: No selected fields
- **WHEN** a request selects no fields
- **THEN** planning succeeds and the page plan selects only the primary key

## ADDED Requirements

### Requirement: Reference time in plans
The planner SHALL copy the request's reference time, with its time zone, into the plan, and SHALL set the plan's first day of the week to the request's, or Monday when the request has none. The planner SHALL NOT read the system clock. Planning a request whose filter contains a relative time value and that has no reference time SHALL fail with an error naming the value's position.

#### Scenario: Default week start
- **WHEN** a request with a reference time and no week start is planned
- **THEN** the plan's first day of the week is Monday

#### Scenario: Missing reference time
- **WHEN** a request with filter `due = TODAY` and no reference time is planned directly
- **THEN** planning fails with an error at position 7

#### Scenario: Fixed filters need no reference time
- **WHEN** a request with filter `due = '2026-10-27'` and no reference time is planned
- **THEN** planning succeeds
