# Spec Delta

## ADDED Requirements

### Requirement: Relative time representation
A relative time value, bare or in braces, SHALL be represented as a literal of a distinct relative time kind. Its text SHALL be the value's source spelling, braces included. Its position SHALL be the 1-based byte position of the keyword or the `{`. The literal SHALL report whether it evaluates to a date or a timestamp, and SHALL evaluate to a time given a reference time and a first day of the week. Evaluation SHALL use the reference time's time zone and SHALL NOT read the system clock.

#### Scenario: Relative literal kind
- **WHEN** a consumer inspects the predicate parsed from `due > {today + 5::days}`
- **THEN** the value has the relative time kind, text `{today + 5::days}`, position 7, and reports a date

#### Scenario: Timestamp result
- **WHEN** a consumer inspects the values parsed from `t > NOW` and `t > {TODAY + 9::HOURS}`
- **THEN** both report a timestamp

#### Scenario: Deterministic evaluation
- **WHEN** the value of `due = TODAY` is evaluated twice with reference time 2026-10-27T23:30:00-05:00
- **THEN** both results are the date 2026-10-27

### Requirement: Relative time keyword meaning
Given a reference time `now` in time zone `Z` and a first day of the week `W`, evaluation SHALL give: `NOW` the instant `now`; `TODAY` the date of `now` in `Z`; `START_OF_WEEK` the latest date on or before `TODAY` that falls on `W`; `END_OF_WEEK` six days after `START_OF_WEEK`; `START_OF_MONTH` and `END_OF_MONTH` the first and last date of `TODAY`'s month; `START_OF_YEAR` and `END_OF_YEAR` January 1 and December 31 of `TODAY`'s year. A date SHALL be represented as midnight at the start of that date in `Z`.

#### Scenario: Week with Monday start
- **WHEN** `START_OF_WEEK` and `END_OF_WEEK` are evaluated with reference time Tuesday 2026-10-27T10:00:00Z and first day Monday
- **THEN** the results are 2026-10-26 and 2026-11-01

#### Scenario: Week with Sunday start
- **WHEN** `START_OF_WEEK` is evaluated with reference time Tuesday 2026-10-27T10:00:00Z and first day Sunday
- **THEN** the result is 2026-10-25

#### Scenario: Time zone decides the date
- **WHEN** `TODAY` is evaluated with reference time 2026-10-27T23:30:00Z expressed in UTC+02:00
- **THEN** the result is 2026-10-28

#### Scenario: End of February
- **WHEN** `END_OF_MONTH` is evaluated with reference time 2028-02-10T00:00:00Z
- **THEN** the result is 2028-02-29

### Requirement: Duration arithmetic meaning
Evaluation SHALL apply operations left to right as written. `n::DAYS` SHALL move by `n` calendar days in `Z`, keeping the wall-clock time of a timestamp. `n::HOURS` and `n::MINUTES` SHALL move by `n` × 60 or `n` minutes of elapsed time. `n::BUSINESS_DAYS` SHALL move to the `n`th Monday-to-Friday date after (for positive `n`) or before (for negative `n`) the start date, keeping the wall-clock time of a timestamp; `0::BUSINESS_DAYS` SHALL move a Saturday or Sunday forward to the next Monday and leave a weekday unchanged. An evaluation whose result falls outside the years 1 to 9999 SHALL fail with an error.

#### Scenario: Business days over a weekend
- **WHEN** `{TODAY + 3::BUSINESS_DAYS}` is evaluated on Thursday 2026-10-29
- **THEN** the result is Tuesday 2026-11-03

#### Scenario: Negative business days
- **WHEN** `{TODAY - 1::BUSINESS_DAYS}` is evaluated on Monday 2026-11-02
- **THEN** the result is Friday 2026-10-30

#### Scenario: Calendar days across a DST change
- **WHEN** `{NOW + 1::DAYS}` is evaluated at 2026-10-24T12:00 in a zone whose clocks go back one hour on 2026-10-25
- **THEN** the result is 2026-10-25T12:00 in that zone, 25 hours later

#### Scenario: Date plus hours
- **WHEN** `{TODAY + 9::HOURS}` is evaluated on 2026-10-27 in UTC+02:00
- **THEN** the result is the timestamp 2026-10-27T09:00:00+02:00

#### Scenario: Out of range
- **WHEN** `{TODAY + 10000000::DAYS}` is evaluated in 2026
- **THEN** evaluation fails with an error

### Requirement: Folded numeric literals
A brace expression without time keywords or durations SHALL be represented as an ordinary number literal whose text is the exact result in plain decimal notation, with no exponent, no leading `+`, and no trailing fractional zeros, and whose position is that of the `{`.

#### Scenario: Folded value
- **WHEN** a consumer inspects the predicate parsed from `price > {100 * 3}`
- **THEN** the value has the number kind, text `300`, and position 9

#### Scenario: Negative result
- **WHEN** a consumer inspects the value parsed from `delta > {-1.50 * 2}`
- **THEN** the value has the number kind and text `-3`
