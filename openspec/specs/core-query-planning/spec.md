# core-query-planning Specification

## Purpose

Turns one list request (selected fields, filter, sort, page) into a dialect-neutral page plan and count plan that include only the expressions and joins the request actually uses.

## Requirements

### Requirement: Request validation
A list request SHALL name the selected fields, and MAY carry a filter syntax tree, sort entries, a limit, an offset, a reference time, and a first day of the week. Every selected name SHALL be a field of the list. Planning SHALL fail with an error naming the first unknown field.

#### Scenario: Unknown selected field
- **WHEN** a request selects `nickname` and the list has no such field
- **THEN** planning fails with an error naming `nickname`

#### Scenario: No selected fields
- **WHEN** a request selects no fields
- **THEN** planning succeeds and the page plan selects only the primary key

### Requirement: Selected expressions
The page plan SHALL select the primary key, every selected SQL field, and every SQL field required by a selected calculated field, each once. It SHALL NOT select fields that are only filtered or sorted on.

#### Scenario: Calculated field pulls in its requirements
- **WHEN** a request selects only calculated field `display`, which requires `name` and `country`
- **THEN** the page plan selects the primary key, `name`, and `country`

#### Scenario: Filter-only field is not selected
- **WHEN** a request selects `name` and filters on `country`
- **THEN** the page plan selects the primary key and `name`, but not `country`

### Requirement: Join resolution
The page plan SHALL include exactly the joins needed by selected SQL fields, SQL fields required by selected calculated fields, filtered fields, and sorted fields, plus every join those joins transitively depend on. Joins SHALL be ordered so that each join appears after all joins it depends on, and the order SHALL be deterministic for the same request.

#### Scenario: Unused join omitted
- **WHEN** a request selects only base-table fields
- **THEN** the page plan contains no joins

#### Scenario: Transitive dependency included
- **WHEN** a selected field needs join `city`, and `city` depends on `region`
- **THEN** the page plan contains `region` followed by `city`

#### Scenario: Join needed only by the filter
- **WHEN** a request filters on `country`, which needs join `country`, without selecting it
- **THEN** the page plan contains the `country` join

### Requirement: Filter field validation
Every field referenced in the filter syntax tree SHALL be a SQL field of the list. Referencing a calculated field or an unknown name SHALL fail planning with an error that includes the field's byte position in the filter text.

#### Scenario: Filtering on a calculated field
- **WHEN** a filter references calculated field `display`
- **THEN** planning fails with an error naming `display` and its position

#### Scenario: Filtering on an unknown field
- **WHEN** a filter `nickname = 'a'` is planned for a list without `nickname`
- **THEN** planning fails with an error naming `nickname` at position 1

### Requirement: Sorting
Each sort entry SHALL name a SQL field and a direction, ascending or descending. Sorting on a calculated or unknown field, or naming the same field twice, SHALL fail planning. When the request has no sort entries the plan SHALL order by the primary key ascending; otherwise the primary key ascending SHALL be appended as the final tiebreaker unless the request already sorts on it.

#### Scenario: Default sort
- **WHEN** a request has no sort entries
- **THEN** the page plan orders by the primary key ascending

#### Scenario: Primary key tiebreaker
- **WHEN** a request sorts by `name` descending
- **THEN** the page plan orders by `name` descending, then the primary key ascending

#### Scenario: Request already sorts on the primary key
- **WHEN** a request sorts by the primary key descending
- **THEN** the page plan orders only by the primary key descending

#### Scenario: Sorting on a calculated field
- **WHEN** a request sorts by calculated field `display`
- **THEN** planning fails with an error naming `display`

### Requirement: Page limits
When the request has no limit, the page plan SHALL use the list's default page size. A limit below 1 or above the list's maximum page size SHALL fail planning. When the request has no offset the plan SHALL use 0; a negative offset SHALL fail planning.

#### Scenario: Default limit
- **WHEN** a request for a list with default page size 100 has no limit
- **THEN** the page plan has limit 100 and offset 0

#### Scenario: Limit above maximum
- **WHEN** a request asks for limit 20000 on a list whose maximum page size is 10000
- **THEN** planning fails with an error stating the maximum

#### Scenario: Negative offset
- **WHEN** a request has offset -1
- **THEN** planning fails

### Requirement: Count plan
Planning SHALL also produce a count plan for the same request. The count plan SHALL carry the list's scope, the request filter, and only the joins (with their transitive dependencies) needed by filtered fields. It SHALL carry no selected fields, ordering, limit, or offset.

#### Scenario: Count ignores selection and sort joins
- **WHEN** a request selects `country` (join `country`), sorts by `last_order` (join `last_order`), and filters on `name`
- **THEN** the count plan contains no joins

#### Scenario: Count keeps filter joins
- **WHEN** a request filters on `country`
- **THEN** the count plan contains the `country` join

### Requirement: Export plan
Planning an export request SHALL produce a plan with the same selection, join, filter, scope, and ordering rules as a page plan, but with no limit and no offset.

#### Scenario: Export has no pagination
- **WHEN** an export request is planned for a list with maximum page size 10000
- **THEN** the plan has no limit and no offset

### Requirement: Array fields are not sortable
Planning SHALL fail with an error naming the field when a sort entry names an array field. Array fields MAY still be selected and filtered.

#### Scenario: Sort on an array field
- **WHEN** a request sorts by array field `tags`
- **THEN** planning fails with an error naming `tags`

#### Scenario: Select and filter an array field
- **WHEN** a request selects `tags` and filters `tags CONTAINS 'go'`
- **THEN** planning succeeds and the plan marks `tags` as an array field

### Requirement: Expression evaluation in the planner
The planner SHALL type-check and evaluate every expression value in the filter, using the request's reference time with its time zone and the request's first day of the week, or Monday when it has none. The plan SHALL carry each result, so drivers never evaluate expressions. The planner SHALL NOT read the system clock. Every error SHALL name the position of the node that caused it.

#### Scenario: Default week start
- **WHEN** a request with a reference time, no week start, and filter `created_at >= START_OF_WEEK` is planned
- **THEN** the plan's value for that predicate is the Monday of the reference week

#### Scenario: Missing reference time
- **WHEN** a request with filter `due = TODAY` and no reference time is planned
- **THEN** planning fails with an error at position 7

#### Scenario: Numbers need no reference time
- **WHEN** a request with filter `qty > {2 * 3}` and no reference time is planned
- **THEN** planning succeeds and the plan's value for that predicate is the number `6`

### Requirement: Expression value types
An expression SHALL have one of the types number, duration, date, or timestamp. Number literals are numbers, durations are durations, `NOW` is a timestamp, and the other keywords are dates. An expression's result SHALL be a number or a time; a bare duration SHALL be an error at the `{` position.

#### Scenario: Bare duration
- **WHEN** a request with filter `due > {5::DAYS}` is planned
- **THEN** planning fails with an error at position 7

### Requirement: Number arithmetic
Number `+`, `-`, `*`, `/` number SHALL give a number, calculated exactly. Division by zero, or a quotient with no finite decimal form, SHALL be an error at the position of `/`. The result SHALL be handed to the driver as an exact decimal text in plain notation, with no exponent and no trailing fractional zeros.

#### Scenario: Exact decimals
- **WHEN** a request with filter `price > {10 / 4 - 0.1}` is planned
- **THEN** the plan's value is the number `2.4`

#### Scenario: Non-terminating quotient
- **WHEN** a request with filter `price > {1 / 3}` is planned
- **THEN** planning fails with an error at the position of `/`

#### Scenario: Division by zero
- **WHEN** a request with filter `price > {1 / (2 - 2)}` is planned
- **THEN** planning fails with an error at the position of `/`

### Requirement: Duration arithmetic
A duration SHALL be an ordered list of unit amounts. Duration `+` or `-` duration SHALL concatenate the lists, negating the right one for `-`, for the same or different units. Duration `*` integer and integer `*` duration SHALL scale every amount, and duration `/` integer SHALL divide every amount and fail unless each divides exactly. Unary `-` SHALL negate every amount. Each amount SHALL be at most 10000000 in absolute value.

#### Scenario: Mixed units
- **WHEN** a request with filter `t > {NOW + (1::DAYS + 2::HOURS)}` is planned
- **THEN** planning succeeds with the same value as `t > {NOW + 1::DAYS + 2::HOURS}`

#### Scenario: Inexact duration division
- **WHEN** a request with filter `t > {NOW + (2::DAYS + 3::HOURS) / 2}` is planned
- **THEN** planning fails with an error at the position of `/`

### Requirement: Time arithmetic
A date or timestamp `+` duration, duration `+` date or timestamp, and date or timestamp `-` duration SHALL give a time. A date stays a date only if every amount of the duration is in `DAYS` or `BUSINESS_DAYS`; otherwise the result is a timestamp. Every other operation involving a time or a duration SHALL be an error at the operator's position.

#### Scenario: Time minus time
- **WHEN** a request with filter `t > {NOW - TODAY}` is planned
- **THEN** planning fails with an error at the position of `-`

#### Scenario: Date plus hours
- **WHEN** a request with filter `t > {TODAY + 9::HOURS}` is planned
- **THEN** the plan's value is a timestamp

### Requirement: Expression type per field
A number result SHALL be accepted only for integer, float, and numeric fields. A date result SHALL be accepted only for date and timestamp fields. A timestamp result SHALL be accepted only for timestamp fields. Array fields SHALL reject expression values. A mismatch SHALL be an error at the literal's position.

#### Scenario: Time on a number field
- **WHEN** a request with filter `age > TODAY` for integer field `age` is planned
- **THEN** planning fails at position 7

#### Scenario: Timestamp on a date field
- **WHEN** a request with filter `due = NOW` for date field `due` is planned
- **THEN** planning fails at position 7 because a timestamp cannot be compared with a date

### Requirement: NOW and TODAY meaning
Given a reference time `now` in time zone `Z`, `NOW` SHALL evaluate to the instant `now` and `TODAY` to the date of `now` in `Z`. A date SHALL be represented as midnight at the start of that date in `Z`.

#### Scenario: Time zone decides the date
- **WHEN** `TODAY` is evaluated with reference time 2026-10-27T23:30:00Z expressed in UTC+02:00
- **THEN** the result is 2026-10-28, represented as 2026-10-28T00:00:00+02:00

### Requirement: Week keyword meaning
Given a first day of the week `W`, `START_OF_WEEK` SHALL evaluate to the latest date on or before `TODAY` that falls on `W`, and `END_OF_WEEK` to six days after `START_OF_WEEK`.

#### Scenario: Week with Monday start
- **WHEN** `START_OF_WEEK` and `END_OF_WEEK` are evaluated with reference time Tuesday 2026-10-27T10:00:00Z and first day Monday
- **THEN** the results are 2026-10-26 and 2026-11-01

#### Scenario: Week with Sunday start
- **WHEN** `START_OF_WEEK` is evaluated with reference time Tuesday 2026-10-27T10:00:00Z and first day Sunday
- **THEN** the result is 2026-10-25

### Requirement: Month and year keyword meaning
`START_OF_MONTH` and `END_OF_MONTH` SHALL evaluate to the first and last date of `TODAY`'s month. `START_OF_YEAR` and `END_OF_YEAR` SHALL evaluate to January 1 and December 31 of `TODAY`'s year.

#### Scenario: End of February
- **WHEN** `END_OF_MONTH` is evaluated with reference time 2028-02-10T00:00:00Z
- **THEN** the result is 2028-02-29

#### Scenario: End of year
- **WHEN** `END_OF_YEAR` is evaluated with reference time 2026-10-27T10:00:00Z
- **THEN** the result is 2026-12-31

### Requirement: Applying durations
Operations SHALL be applied left to right as written, and a duration's amounts one at a time in their order. `n::DAYS` SHALL move by `n` calendar days in `Z`, keeping the wall-clock time of a timestamp. `n::HOURS` and `n::MINUTES` SHALL move by elapsed time. A result outside the years 1 to 9999 SHALL be an error at the `{` position.

#### Scenario: Calendar days across a DST change
- **WHEN** `{NOW + 1::DAYS}` is evaluated at 2026-10-24T12:00 in a zone whose clocks go back one hour on 2026-10-25
- **THEN** the result is 2026-10-25T12:00 in that zone, 25 hours later

#### Scenario: Mixed-unit duration
- **WHEN** `{NOW + (1::DAYS + 2::HOURS)}` is evaluated at 2026-10-27T10:00:00Z
- **THEN** the result is 2026-10-28T12:00:00Z

#### Scenario: Out of range
- **WHEN** `{TODAY + 10000000::DAYS}` is evaluated in 2026
- **THEN** planning fails with an error

### Requirement: Business day durations
`n::BUSINESS_DAYS` SHALL move to the `n`th Monday-to-Friday date after the start date for positive `n`, or before it for negative `n`, keeping the wall-clock time of a timestamp. `0::BUSINESS_DAYS` SHALL move a Saturday or Sunday forward to the next Monday and leave a weekday unchanged.

#### Scenario: Business days over a weekend
- **WHEN** `{TODAY + 3::BUSINESS_DAYS}` is evaluated on Thursday 2026-10-29
- **THEN** the result is Tuesday 2026-11-03

#### Scenario: Negative business days
- **WHEN** `{TODAY - 1::BUSINESS_DAYS}` is evaluated on Monday 2026-11-02
- **THEN** the result is Friday 2026-10-30

#### Scenario: Zero business days on a weekend
- **WHEN** `{TODAY + 0::BUSINESS_DAYS}` is evaluated on Saturday 2026-10-31
- **THEN** the result is Monday 2026-11-02
