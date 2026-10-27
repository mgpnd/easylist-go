# Design

## Context

See proposal.md (Why) for the motivation and specs/ for the required behaviour. Today:

- `easylist-filter`'s `value` rule accepts only `tSTRING`, `tNUMBER`, and `tBOOL`. Any identifier on the value side is a syntax error. The lexer scans `+` and `-` as the start of a signed number, and it has no `{`, `}`, `*`, `/`, or `:` tokens.
- `easylist-core` passes the filter tree from `Request` to `Plan` unchanged. Nothing in the library reads the clock.
- `easylist-sql-goqu` converts literals in `predicateArgument`, which needs a string for date and timestamp fields. `Driver.Compile(*Plan)` takes no context, so anything time-related has to travel on the plan.
- `ExportRequest.Filter` is filter text, parsed again by `ValidateExport` in both the mutation and the worker. Without an extra field, the worker would evaluate `TODAY` when the job runs.

## Goals / Non-Goals

**Goals:**
- Keep the parser's job to syntax: it builds an expression tree that mirrors the source and never calculates, simplifies, or type-checks.
- Evaluate once, in one place: the `easylist-core` planner, so every driver gets identical semantics without reimplementing arithmetic.
- Report syntax errors at parse time and type and evaluation errors at planning time, all with byte positions.
- Make results deterministic and testable: one reference time per request, set at the edge (GraphQL resolver, runtime, or export request).
- Make relative date comparisons on timestamp columns behave the way users read them ("this week" includes all of Sunday).

**Non-Goals:**
- Holiday calendars for `BUSINESS_DAYS`.
- Relative values in `BETWEEN`, in array value lists, or with `CONTAINS`/`EXCLUDES`.
- Named IANA time zones in the GraphQL input. Only fixed UTC offsets are supported, so DST is not applied to the client's zone; the server-local default does follow DST.
- Field references or functions inside brace expressions.

## Decisions

### Expression values are an unevaluated tree
A bare keyword or a brace expression parses to a `Literal` of the new kind `LiteralExpr`. `Literal.Expr()` returns the root `ExprNode` (nil for other kinds). The nodes are sealed interfaces in the style of `Predicate` and `Logical`:

```go
type ExprNode interface{ exprNode(); Pos() int }
type NumberNode interface{ ExprNode; Text() string }            // unsigned source spelling
type KeywordNode interface{ ExprNode; Keyword() TimeKeyword; Text() string }
type DurationNode interface{ ExprNode; Amount() string; Unit() TimeUnit; UnitPos() int }
type BinaryNode interface{ ExprNode; Operator() ArithOp; Left() ExprNode; Right() ExprNode } // Pos is the operator's
type NegationNode interface{ ExprNode; Operand() ExprNode }
```

Grammar actions only allocate nodes. Parentheses shape the tree and leave no node behind. Nothing is folded, so `{100 * 3}` stays a `*` node, and `{NOW - TODAY}` and `{1 / 0}` parse successfully. The only checks in actions are syntactic: decimal duration amounts, unit names, keyword names, and which operators may take an expression value.

*Alternative:* fold numbers and type-check in grammar actions. This was rejected because it makes the parser interpret the language, ties the AST to one set of semantics, and stops anything from inspecting the tree as written.

### The planner type-checks and evaluates
`easylist-core` gains an evaluator (`eval.go`) that walks an `ExprNode` with an environment of reference time, time zone (the location of `Request.Now`), and week start. It returns a typed `Value`:

```go
type ValueKind uint8 // ValueNumber, ValueDate, ValueTimestamp
type Value struct {
	Kind   ValueKind
	Number string    // exact decimal text, for ValueNumber
	Time   time.Time // midnight in the zone for ValueDate; the instant for ValueTimestamp
}
```

`CompiledList.Plan` runs it for each predicate whose value is a `LiteralExpr`, during the tree walk it already does in `filterFields`. It checks the result kind against the field (`Integer`/`Float`/`Numeric` take numbers; `Date` takes dates; `Timestamp` takes dates and timestamps; array fields reject expressions) and stores the results in a map keyed by predicate node. Drivers read them with `Plan.Value(pred)`. Core already owns field types, so it is the only layer that can type-check without duplicating them, and it is the layer every driver shares.

Number arithmetic uses `math/big.Rat`. A result whose reduced denominator has prime factors other than 2 and 5 has no finite decimal form and is an error at the `/`. Results are rendered as plain decimal text, which drivers convert exactly as they convert number literals, so integer fields still reject `2.5`.

*Alternative:* each driver evaluates (directly, or with a shared helper in `easylist-filter`). This was rejected because every driver would repeat the type rules and could drift, and the filter module would gain clock-dependent semantics.

*Alternative:* SQL expressions (`now()`, `date_trunc`, `interval`). This was rejected because results would depend on the database session's time zone, business days have no clean SQL form, and tests would need a database clock.

### Durations are ordered lists of unit amounts
During evaluation, a duration is a list of `(amount, unit)` terms in the order written. `+` concatenates the lists, `-` concatenates the negated right-hand list, unary `-` negates every term, `* k` multiplies every term, and `/ k` divides every term and fails unless each divides exactly. Terms are never merged or reordered, so adding `(1::DAYS + 2::HOURS)` applies the day step and then the hour step, exactly like `+ 1::DAYS + 2::HOURS`. Calendar days and elapsed hours do not commute across a DST change, so the order must be preserved. A date stays a date only if every term is `DAYS` or `BUSINESS_DAYS`. The result type is determined by the tree's structure alone, never by the reference time.

### Keywords only on the value side
The lexer keeps scanning `today` as `tIDENT`. A new grammar alternative `tIDENT ordinary_op tIDENT` (restricted to comparison operators in the action) maps the right-hand identifier through a keyword table into a `KeywordNode`, and reports an unknown word at its position. A quoted identifier on the right is rejected. This avoids reserving words that are likely to be column names (`now`, `today`). The cost is that the action, rather than the grammar, rejects `CONTAINS TODAY`. Both still produce a positioned syntax error.

### Grammar documentation in EBNF, checked against the parser
`docs/query-language.md` gives the grammar as two `ebnf` blocks. The syntactic block is written over token names (`identifier`, `string`, `number`, keywords, punctuation), and a separate lexical block defines those tokens at the character level. EBNF uses `{ }` repetition where `grammar.y` needs left recursion, so the old rule-by-rule text comparison can no longer work.

`grammar_doc_test.go` is replaced by a conformance test:
- A small test-only reader parses the syntactic `ebnf` block (sequence `,`, alternation `|`, `[ ]`, `{ }`, `( )`, quoted terminals) into rules.
- A backtracking recognizer matches token sequences from `lexFilter` against the `filter` rule. Lexical rule names in the syntactic block match token kinds.
- A shared corpus of accepted and rejected filters must get the same verdict from the recognizer and from `filter.Parse`. The corpus is a table in the test and includes every example in the docs. Since the parser is now syntax-only, "accepted" means syntactically valid, so no semantic rule can make the two disagree.
- The test fails with the filter on which the two disagree.

The lexical block is documentation only. Lexer tests cover it.

*Alternative:* keep goyacc notation in the docs. This was rejected because readers expect EBNF, and goyacc's left recursion and `t`-prefixed token names are parser implementation details.

*Alternative:* generate the EBNF from `grammar.y`. This was rejected because a mechanical translation keeps the left recursion, so the result reads badly.

### Lexer brace mode
`{` switches the lexer into expression mode until the matching `}`. In this mode, `+ - * / ( )` are operator tokens, numbers are unsigned, `::` is a token, and identifiers are units or keywords. Outside braces, scanning is unchanged, so `age > -3` still lexes `-3` as a number. A nested `{` is an error.

### Whole-day comparisons in the driver
`buildPredicate` reads a `LiteralExpr` value from `Plan.Value`. For a `ValueDate` on a `Timestamp` field, the driver takes `S = Value.Time` (midnight of `D` in the request zone) and `E = S.AddDate(0, 0, 1)`, which is DST-correct. It then rewrites the operator as in the `contract-sql-driver` table. `=` and `!=` compile to a range test and its `COALESCE(NOT …, TRUE)` negation, matching how `EXCLUDES` already handles NULL. Fixed string dates on timestamp fields keep their current behaviour; only relative values get day semantics, so existing filters are unaffected.

### Where the reference time comes from
- `easylist.Request` gains `Now time.Time` (instant plus zone) and `WeekStart *time.Weekday`, with nil meaning Monday. A pointer is needed because `time.Sunday` is the zero value. `Plan` carries only evaluated values, not the clock inputs.
- `CompiledList.Plan` never reads the clock. It fails if the filter contains a time keyword and `Now` is zero, because silently using the system clock would make planning nondeterministic.
- `easylist.Runtime` gains `Clock func() time.Time` (nil means `time.Now`, which is server-local) and `WeekStart *time.Weekday`. `Runtime.Fetch` fills missing `Now` and `WeekStart` before planning. Tests inject `Clock`.
- `easylistgql.ListInput.Timezone *int`: when set, the resolver sets `req.Now = rt.Now().In(time.FixedZone("", m*60))`. A shared `easylist.UTCOffset(minutes) (*time.Location, error)` validates the range -720…840 for both GraphQL and exports.

### Exports record AsOf and Timezone
`<P>ExportInput` has an optional `asOf: Time`, which uses the plugin's timestamp scalar (`Scalars.Time`). The SDL writer marks that scalar as used whenever it writes an export input, so it is always declared. The resolver uses `in.AsOf` when it is set, and otherwise `rt.Now()`. A client can therefore export "as of" the moment it loaded the list.

`ExportRequest` gains `AsOf time.Time` with tag `json:"asOf,omitzero"` and `Timezone *int` with tag `json:"timezone,omitempty"`. The export resolver stores the chosen moment as `AsOf = t.UTC().Round(0)`, which drops the monotonic reading so the JSON round trip is equal. `ValidateExport` builds `Request.Now = AsOf.In(zone)` when `AsOf` is set, so the mutation and the worker plan identically. The worker (`easylistexport.Run`) fills a zero `Now` from `rt.Now()` in the request zone, and `WeekStart` from the runtime, before planning.

*Alternative:* resolve the filter to fixed text at request time. This was rejected because it loses whole-day semantics and changes the filter the user wrote.

## Risks / Trade-offs

- [New `LiteralKind` breaks exhaustive switches in third-party drivers] → Documented in the README and the AST doc comment. Drivers already need a default branch, which `LiteralNull` established, and they get evaluated values from `Plan.Value` without having to walk the tree.
- [Type errors move from parse time to planning time] → Both happen before any query runs and both carry byte positions, so GraphQL clients see the same kind of positioned error.
- [Fixed offsets ignore DST in the client's zone] → Clients send their current offset with each request, which is correct for "now". It may be off by an hour only for date boundaries on the far side of a DST change from now, which is acceptable for this change.
- [Values that used to be syntax errors now parse] → Only on the value side, and only for the listed keywords or braces, so no previously valid filter changes meaning.
- [Large amounts overflow time arithmetic] → The evaluator caps each amount at 10,000,000 and checks the year range 1–9999 after each step.
