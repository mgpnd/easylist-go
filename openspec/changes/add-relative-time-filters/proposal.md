# Proposal

## Why

Saved views and dashboards need filters that move with the calendar, such as "created this week" or "due within five business days". Today a filter can only hold fixed values like `'2026-10-27'`, so a client has to rewrite the filter text every day and work out dates in its own time zone. Clients also cannot write simple arithmetic such as `price > {100 * 3}`.

## What Changes

- Add relative time keywords as filter values: `NOW`, `TODAY`, `START_OF_WEEK`, `END_OF_WEEK`, `START_OF_MONTH`, `END_OF_MONTH`, `START_OF_YEAR`, `END_OF_YEAR`. They are case-insensitive and recognised only on the value side of a predicate, so fields named `today` or `now` still work unquoted. `NOW` is a timestamp. All the others are dates; for example, `END_OF_MONTH` is the last day of the month.
- Add arithmetic expressions in braces: `{TODAY + 5::DAYS}`, `{NOW - 90::MINUTES}`, `{TODAY + 2 * 7::DAYS}`, `{100 * 3}`. They support `+`, `-`, `*`, `/`, unary minus, and parentheses. Durations use the units `DAYS`, `BUSINESS_DAYS` (Monday to Friday, no holidays), `HOURS`, and `MINUTES`. Arithmetic is allowed only inside braces.
- Brace expressions without time keywords are calculated when the filter is parsed and become ordinary number literals, so drivers need no change for them.
- Relative values are calculated against a reference time and a time zone. A date value compared with a timestamp field covers the whole day in that time zone, so `created_at <= END_OF_WEEK` includes all of Sunday.
- Weeks start on Monday by default. An application can change the week start.
- GraphQL `<P>ListInput` and `<P>ExportInput` gain an optional `timezone: Int`: the client's UTC offset in minutes, positive east of UTC (UTC+02:00 is `120`). When it is omitted, the server's local time zone is used.
- `ExportRequest` gains `AsOf` (the moment the export was requested) and `Timezone`. The worker calculates relative values at `AsOf`, so the file matches the list the user saw, however long the job waits in a queue.
- `BETWEEN` keeps its single-string form and does not accept relative values.

Modules touched:
- `easylist-filter`: lexer brace mode, grammar, numeric folding, relative time values and their evaluation, AST, and docs.
- `easylist-core`: reference time and week start on `Request`, `Plan`, and `Runtime`; `AsOf` and `Timezone` on `ExportRequest` and their validation; README.
- `easylist-sql-goqu`: converting relative time values for date and timestamp fields, including whole-day comparisons; README.
- `easylist-gqlgen`: the `timezone` input field in the generated SDL and Go glue, and passing it to core; README.
- `easylist-export`: running the export at `AsOf` in the request's time zone; README.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
- `filter-parsing`: relative time keywords, brace expressions, duration units, and their syntax and type errors.
- `contract-filter-ast`: a relative time literal kind that consumers evaluate against a reference time and week start, and folded numeric literals.
- `contract-sql-driver`: the plan carries a reference time and week start; relative time literal conversion and whole-day comparison semantics.
- `sql-goqu-compilation`: how relative values are bound for PostgreSQL.
- `core-query-planning`: request reference time and week start, and rejecting relative filters without a reference time.
- `core-list-execution`: the runtime clock and week start, used when a request has no reference time.
- `contract-export-request`: `AsOf` and `Timezone` fields and their validation.
- `export-file-generation`: the worker evaluates relative values at `AsOf` in the request's time zone.
- `gqlgen-schema-generation`: `timezone: Int` on list and export inputs.
- `gqlgen-list-resolution`: mapping `timezone` to the reference time, and setting `AsOf` on exports.

## Impact

- **Public API (`easylist-filter`)**: a new `LiteralTime` kind and a `Literal.Time()` accessor. Third-party drivers that switch on `LiteralKind` must handle or reject it. Values that used to be syntax errors (`x = today`, `x = {1 + 2}`) now parse.
- **Public API (`easylist-core`)**: new fields on `Request`, `Plan`, `Runtime`, and `ExportRequest`. All are optional, so existing callers compile unchanged. Calling `CompiledList.Plan` directly with a relative filter and no `Request.Now` fails.
- **GraphQL schema**: an additive optional input field. Applications must run the gqlgen generator again.
- **Serialized export jobs**: jobs queued before the upgrade have no `AsOf` and use the worker's clock and local zone.
- **Generated code**: `grammar.y` changes, so `parser_gen.go` is regenerated.
- **Dependencies**: none added. Numeric folding uses `math/big`.
