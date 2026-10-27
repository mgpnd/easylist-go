# Proposal

## Why

Saved views and dashboards need filters that move with the calendar, such as "created this week" or "due within five business days". Today a filter can only hold fixed values like `'2026-10-27'`, so a client has to rewrite the filter text every day and work out dates in its own time zone. Clients also cannot write simple arithmetic such as `price > {100 * 3}`.

## What Changes

- Add relative time keywords as filter values: `NOW`, `TODAY`, `START_OF_WEEK`, `END_OF_WEEK`, `START_OF_MONTH`, `END_OF_MONTH`, `START_OF_YEAR`, `END_OF_YEAR`. They are case-insensitive and recognised only on the value side of a predicate, so fields named `today` or `now` still work unquoted. `NOW` is a timestamp. All the others are dates; for example, `END_OF_MONTH` is the last day of the month.
- Add arithmetic expressions in braces: `{TODAY + 5::DAYS}`, `{NOW - 90::MINUTES}`, `{TODAY + 2 * 7::DAYS}`, `{100 * 3}`. They support `+`, `-`, `*`, `/`, unary minus, and parentheses. Durations use the units `DAYS`, `BUSINESS_DAYS` (Monday to Friday, no holidays), `HOURS`, and `MINUTES`, and durations of different units can be combined, as in `{NOW + (1::DAYS + 2::HOURS)}`. Arithmetic is allowed only inside braces.
- The parser only checks syntax and builds the tree: a bare keyword or brace expression becomes an expression literal holding an expression tree that mirrors the source, unevaluated.
- The `easylist-core` planner type-checks every expression against its field's type and evaluates it exactly (numbers with `math/big`), using the request's reference time, time zone, and week start. Plans carry the results, so drivers only bind values and never evaluate.
- Relative values are calculated against a reference time and a time zone. A date value compared with a timestamp field covers the whole day in that time zone, so `created_at <= END_OF_WEEK` includes all of Sunday.
- Weeks start on Monday by default. An application can change the week start.
- GraphQL `<P>ListInput` and `<P>ExportInput` gain an optional `timezone: Int`: the client's UTC offset in minutes, positive east of UTC (UTC+02:00 is `120`). When it is omitted, the server's local time zone is used.
- `<P>ExportInput` also gains an optional `asOf: Time`. When it is omitted, the resolver uses the server's current time.
- `ExportRequest` gains `AsOf` (the moment relative values are evaluated) and `Timezone`. The worker calculates relative values at `AsOf`, so the file matches the list the user saw, however long the job waits in a queue.
- `BETWEEN` keeps its single-string form and does not accept relative values.
- The grammar in `easylist-filter/docs/query-language.md` is rewritten in EBNF instead of goyacc notation. The test that compared it rule by rule with `grammar.y` is replaced by one that checks the documented EBNF and the parser accept and reject the same filters.

Modules touched:
- `easylist-filter`: lexer brace mode, grammar, the expression literal kind and expression tree nodes, EBNF docs, and the grammar documentation test. No evaluation.
- `easylist-core`: expression type-checking and evaluation in the planner, evaluated values on `Plan`; reference time and week start on `Request` and `Runtime`; `AsOf` and `Timezone` on `ExportRequest` and their validation; README.
- `easylist-sql-goqu`: binding the plan's evaluated values, including whole-day comparisons for dates on timestamp fields; README.
- `easylist-gqlgen`: the `timezone` input field in the generated SDL and Go glue, and passing it to core; README.
- `easylist-export`: running the export at `AsOf` in the request's time zone; README.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
- `filter-parsing`: relative time keywords, brace expressions, duration syntax, the rule that the parser does syntax only, and the grammar documented in EBNF.
- `contract-filter-ast`: an expression literal kind and the expression tree nodes it exposes.
- `contract-sql-driver`: the plan carries evaluated expression values; their conversion and whole-day comparison semantics.
- `sql-goqu-compilation`: how relative values are bound for PostgreSQL.
- `core-query-planning`: request reference time and week start; expression typing, field compatibility, and evaluation (keywords, number arithmetic, durations, business days).
- `core-list-execution`: the runtime clock and week start, used when a request has no reference time.
- `contract-export-request`: `AsOf` and `Timezone` fields and their validation.
- `export-file-generation`: the worker evaluates relative values at `AsOf` in the request's time zone.
- `gqlgen-schema-generation`: `timezone: Int` on list and export inputs, and `asOf: Time` on export inputs.
- `gqlgen-list-resolution`: mapping `timezone` to the reference time, and setting `AsOf` on exports from `asOf` or the server's current time.

## Impact

- **Public API (`easylist-filter`)**: a new `LiteralExpr` kind, a `Literal.Expr()` accessor, and expression node interfaces. Third-party drivers that switch on `LiteralKind` must handle or reject it. Values that used to be syntax errors (`x = today`, `x = {1 + 2}`) now parse.
- **Public API (`easylist-core`)**: new fields on `Request`, `Plan`, `Runtime`, and `ExportRequest`. All are optional, so existing callers compile unchanged. Calling `CompiledList.Plan` directly with a relative filter and no `Request.Now` fails.
- **GraphQL schema**: additive optional input fields. The `Time` scalar is now declared for every list, because `asOf` uses it. Applications must run the gqlgen generator again.
- **Serialized export jobs**: jobs queued before the upgrade have no `AsOf` and use the worker's clock and local zone.
- **Generated code**: `grammar.y` changes, so `parser_gen.go` is regenerated.
- **Dependencies**: none added. The planner's number arithmetic uses `math/big`.
