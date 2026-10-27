# Design

## Context

See proposal.md (Why) for the motivation and specs/ for the required behaviour. Today:

- `easylist-filter`'s `value` rule accepts only `tSTRING`, `tNUMBER`, and `tBOOL`. Any identifier on the value side is a syntax error. The lexer scans `+` and `-` as the start of a signed number, and it has no `{`, `}`, `*`, `/`, or `:` tokens.
- `easylist-core` passes the filter tree from `Request` to `Plan` unchanged. Nothing in the library reads the clock.
- `easylist-sql-goqu` converts literals in `predicateArgument`, which needs a string for date and timestamp fields. `Driver.Compile(*Plan)` takes no context, so anything time-related has to travel on the plan.
- `ExportRequest.Filter` is filter text, parsed again by `ValidateExport` in both the mutation and the worker. Without an extra field, the worker would evaluate `TODAY` when the job runs.

## Goals / Non-Goals

**Goals:**
- Keep `easylist-filter` free of schemas, SQL, and the system clock: it parses, type-checks, and evaluates against a reference time it is given.
- Report syntax and type errors at parse time with byte positions, the same as existing errors.
- Make results deterministic and testable: one reference time per request, set at the edge (GraphQL resolver, runtime, or export request).
- Make relative date comparisons on timestamp columns behave the way users read them ("this week" includes all of Sunday).

**Non-Goals:**
- Holiday calendars for `BUSINESS_DAYS`.
- Relative values in `BETWEEN`, in array value lists, or with `CONTAINS`/`EXCLUDES`.
- Named IANA time zones in the GraphQL input. Only fixed UTC offsets are supported, so DST is not applied to the client's zone; the server-local default does follow DST.
- Field references or functions inside brace expressions.

## Decisions

### Relative values are a literal kind evaluated by consumers
The parser produces a `Literal` of the new kind `LiteralTime`. `Literal.Time()` returns a `*TimeValue` (nil for other kinds) with:

```go
func (v *TimeValue) IsDate() bool
func (v *TimeValue) Eval(now time.Time, weekStart time.Weekday) (time.Time, error)
```

`now` carries the time zone. Dates come back as midnight in that zone. `TimeValue` is an immutable, opaque tree (keyword base plus a list of operations), so the AST stays read-only and safe to share.

*Alternative:* a `filter.Resolve(expr, now)` pass that rewrites relative values into string literals before planning. This was rejected because the driver needs to know that the value is a date, so it can apply whole-day comparisons to timestamp fields. Also, a rewritten tree would have to be rebuilt for every request, while an evaluable literal lets one parsed filter be compiled many times.

*Alternative:* SQL expressions (`now()`, `date_trunc`, `interval`). This was rejected because results would depend on the database session's time zone, business days have no clean SQL form, and tests would need a database clock.

### Numeric brace expressions are folded at parse time
A brace expression with no time keywords or durations is evaluated with `math/big.Rat` and becomes a `LiteralNumber` whose text is the exact decimal result. A quotient whose denominator has prime factors other than 2 and 5 has no finite decimal form and is rejected. This keeps every driver unchanged for numbers and keeps numeric fields exact.

### Types are checked in the parser
Each brace sub-expression has a static type: number, duration of a unit, date, or timestamp. Grammar actions build typed nodes and return errors at the operator position (through the existing `parseState` error path). "Date + HOURS → timestamp" is known statically, so `IsDate()` never depends on the reference time. Integer-only rules (the amount of `n::UNIT`, the multiplier of a duration, exact division of a duration) are checked while folding the integer parts, so `{TODAY + 2*7::DAYS}` folds to one `+14::DAYS` step.

### Keywords only on the value side
The lexer keeps scanning `today` as `tIDENT`. A new grammar alternative `tIDENT ordinary_op tIDENT` (restricted to comparison operators in the action) maps the right-hand identifier through a keyword table and reports an unknown word at its position. A quoted identifier on the right is rejected. This avoids reserving words that are likely to be column names (`now`, `today`). The cost is that the action, rather than the grammar, rejects `CONTAINS TODAY`. Both still produce a positioned syntax error.

### Lexer brace mode
`{` switches the lexer into expression mode until the matching `}`. In this mode, `+ - * / ( )` are operator tokens, numbers are unsigned, `::` is a token, and identifiers are units or keywords. Outside braces, scanning is unchanged, so `age > -3` still lexes `-3` as a number. A nested `{` is an error.

### Whole-day comparisons in the driver
`predicateArgument` gains a time path. For a date-valued `TimeValue` on a `Timestamp` field, the driver computes `S` (midnight of `D` in the plan zone) and `E = S.AddDate(0, 0, 1)`, which is DST-correct. It then rewrites the operator as in the `contract-sql-driver` table. `=` and `!=` compile to a range test and its `COALESCE(NOT …, TRUE)` negation, matching how `EXCLUDES` already handles NULL. Fixed string dates on timestamp fields keep their current behaviour; only relative values get day semantics, so existing filters are unaffected.

### Where the reference time comes from
- `easylist.Request` gains `Now time.Time` (instant plus zone) and `WeekStart *time.Weekday`. `Plan` gains `Now time.Time` and `WeekStart time.Weekday`, with nil defaulting to Monday. A pointer is needed because `time.Sunday` is the zero value.
- `CompiledList.Plan` never reads the clock. It fails if the filter contains a `LiteralTime` and `Now` is zero, because silently using the system clock would make planning nondeterministic.
- `easylist.Runtime` gains `Clock func() time.Time` (nil means `time.Now`, which is server-local) and `WeekStart *time.Weekday`. `Runtime.Fetch` fills missing `Now` and `WeekStart` before planning. Tests inject `Clock`.
- `easylistgql.ListInput.Timezone *int`: when set, the resolver sets `req.Now = rt.Now().In(time.FixedZone("", m*60))`. A shared `easylist.UTCOffset(minutes) (*time.Location, error)` validates the range -720…840 for both GraphQL and exports.

### Exports record AsOf and Timezone
`ExportRequest` gains `AsOf time.Time` with tag `json:"asOf,omitzero"` and `Timezone *int` with tag `json:"timezone,omitempty"`. The export resolver sets `AsOf = rt.Now().UTC().Round(0)`, which drops the monotonic reading so the JSON round trip is equal. `ValidateExport` builds `Request.Now = AsOf.In(zone)` when `AsOf` is set, so the mutation and the worker plan identically. The worker (`easylistexport.Run`) fills a zero `Now` from `rt.Now()` in the request zone, and `WeekStart` from the runtime, before planning.

*Alternative:* resolve the filter to fixed text at request time. This was rejected because it loses whole-day semantics and changes the filter the user wrote.

## Risks / Trade-offs

- [New `LiteralKind` breaks exhaustive switches in third-party drivers] → Documented in the README and the AST doc comment. Drivers already need a default branch, which `LiteralNull` established.
- [Fixed offsets ignore DST in the client's zone] → Clients send their current offset with each request, which is correct for "now". It may be off by an hour only for date boundaries on the far side of a DST change from now, which is acceptable for this change.
- [Values that used to be syntax errors now parse] → Only on the value side, and only for the listed keywords or braces, so no previously valid filter changes meaning.
- [Large amounts overflow time arithmetic] → The amount is capped at 10,000,000 at parse time, and `Eval` checks the year range 1–9999 after each step.
