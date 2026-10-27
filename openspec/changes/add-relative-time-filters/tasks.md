# Tasks

## 1. easylist-filter

- [ ] 1.1 Add a lexer brace mode in `lexer.go`: `{` and `}` tokens, and inside braces the operator tokens `+ - * /`, parentheses, `::`, unsigned numbers, and identifiers. A nested `{` is an error. Outside braces scanning is unchanged. Verify with `TestLexFilterTokens` cases for `{TODAY+5::DAYS}`, `{today + 5::days}`, `{-1.50 * 2}`, an unclosed `{`, a nested `{`, and `age > -3` still lexing a signed number
- [ ] 1.2 Add `LiteralExpr`, `Literal.Expr()`, and the sealed expression node interfaces (`NumberNode`, `KeywordNode`, `DurationNode`, `BinaryNode`, `NegationNode`, with `TimeKeyword`, `TimeUnit`, and `ArithOp` enums) in `ast.go`. Nodes are immutable, carry positions, and hold no evaluation logic. Verify `go build ./...`
- [ ] 1.3 Extend `grammar.y`: the `'{' sum '}'` value with precedence for `+ - * /`, unary minus and parentheses; bare keyword values via `tIDENT` on the value side restricted to `= != > >= < <=` and mapped through a keyword table; `tNUMBER '::' tIDENT` durations whose action rejects decimal amounts and unknown units. Actions only build nodes. Map the new token kinds in `yyCodes` and `tokenDisplay` in `parser.go`. Verify `goyacc` reports no conflicts and `TestYYCodesCoverAllTokenKinds` passes
- [ ] 1.4 Run `go generate ./...` in `easylist-filter`, commit the regenerated `parser_gen.go`, and verify a second `go generate ./...` leaves it unchanged and `TestGeneratedCodeExportsNothing` passes
- [ ] 1.5 Add parser tests for every `filter-parsing` and `contract-filter-ast` scenario: tree shapes and positions (precedence, parentheses, negation, no folding), keyword spellings as field names, unknown words, keywords with `CONTAINS`/`BETWEEN`, arithmetic outside braces, nested braces, decimal amounts, unknown units, and ill-typed expressions that still parse. Pin new error messages in `TestParseErrorMessages`, and verify `go test ./...` passes
- [ ] 1.6 Document keywords, braces, units, and the tree in `docs/query-language.md` (grammar block included) and `docs/parser.md`, stating that evaluation and type rules belong to the planner; verify `grammar_doc_test.go` passes

## 2. easylist-core

- [ ] 2.1 Add `Now time.Time` and `WeekStart *time.Weekday` to `Request`. Add a `Value` type (kind number, date, or timestamp; exact decimal text or `time.Time`) and `Plan.Value(filter.Predicate) (Value, bool)` for looking up evaluated expression values. Verify `go build ./...`
- [ ] 2.2 Add the expression evaluator in a new `eval.go`: static typing (number, duration as an ordered list of unit amounts, date, timestamp), number arithmetic with `math/big.Rat` and exact decimal text, duration concatenation, negation, scaling and exact division, the amount limit, time arithmetic, keyword meaning with week start, calendar and elapsed durations, business days, and the year range. Every error carries the offending node's position. Cover every `core-query-planning` evaluation scenario with table tests, including a DST zone
- [ ] 2.3 In `CompiledList.Plan`, walk the filter alongside `filterFields`, check each expression's result type against its field (number on integer/float/numeric; date on date/timestamp; timestamp on timestamp; arrays rejected), fail on a missing reference time, and store the results on the plan. Cover the field-compatibility, missing-reference-time, and default-week-start scenarios
- [ ] 2.4 Add `Clock func() time.Time` and `WeekStart *time.Weekday` to `Runtime`, plus a `Now()` method (clock or `time.Now`). `Fetch` fills a missing `Now` and `WeekStart` before planning. Cover the `core-list-execution` scenarios with a fake driver that records the plan
- [ ] 2.5 Add `UTCOffset(minutes int) (*time.Location, error)`, which validates -720…840. Add `AsOf time.Time` (`json:"asOf,omitzero"`) and `Timezone *int` (`json:"timezone,omitempty"`) to `ExportRequest`. `ValidateExport` validates `Timezone` and, when `AsOf` is set, sets `Request.Now = AsOf.In(zone)`. Verify the JSON round-trip test with and without the new fields, decoding old JSON, and the offset range error
- [ ] 2.6 Update `easylist-core/README.md`: expression values and their typing, the reference time and week start on `Request` and `Runtime`, `Plan.Value`, and `AsOf`/`Timezone` on export requests. Verify `go test ./...` passes in `easylist-core`

## 3. easylist-sql-goqu

- [ ] 3.1 In `buildPredicate`, take the value of a `LiteralExpr` from `Plan.Value`, never evaluating it. Number results go through the existing number conversion, date results on date fields bind like `YYYY-MM-DD` literals, and timestamp results bind truncated to microseconds
- [ ] 3.2 Implement whole-day comparisons for date results on timestamp fields: operator rewriting for `> >= < <=`, `(? >= ? AND ? < ?)` for `=`, and `COALESCE(NOT (…), TRUE)` for `!=`, with `E = S.AddDate(0, 0, 1)` in the result's zone. Cover every `contract-sql-driver` "Relative time comparison" and "Whole-day comparison on timestamp fields" scenario and every `sql-goqu-compilation` scenario with SQL and argument golden tests, including UTC+02:00 boundaries and a DST zone
- [ ] 3.3 Add a driver-level golden test for `created_at >= START_OF_WEEK AND due < {TODAY + 5::BUSINESS_DAYS} AND price > {10 / 4}`, planned with a fixed `Request.Now`, checking argument order. Update the README's PostgreSQL semantics section with expression values and whole-day comparisons. Verify `go test ./...` passes

## 4. easylist-gqlgen

- [ ] 4.1 Add `timezone: Int` with an SDL description to `<P>ListInput` and `<P>ExportInput` in `sdl.go`. Add `asOf: Time` (the plugin's timestamp scalar, always declared once an export input exists) to `<P>ExportInput`. Add `Timezone *int` to `ListInput` and `ExportInput` and `AsOf *time.Time` to `ExportInput`, and map them in the generated glue (`codegen.go`). Update the SDL golden tests, including a list without timestamp fields
- [ ] 4.2 In `ResolveList`, validate `Timezone` with `easylist.UTCOffset` and set `req.Now = rt.Now().In(zone)`. In `Export`, set `AsOf` to `in.AsOf` or, when that is nil, `rt.Now()`, stored as `.UTC().Round(0)`, and set `Timezone`. Cover the `gqlgen-list-resolution` scenarios with an injected `Runtime.Clock`
- [ ] 4.3 Run the generator for `internal/fixture`, commit the regenerated fixture files, add a fixture test that filters with `TODAY` and `timezone`, and update the README. Verify `go test ./...` passes

## 5. easylist-export

- [ ] 5.1 In `Run`, fill a zero `Now` with `rt.Now()` in the request's zone (server local when `Timezone` is unset) after `ValidateExport`, and fill `WeekStart` from the runtime. Cover the `export-file-generation` scenarios (a job run a day later uses `AsOf`; an old request uses the clock) and update the README

## 6. Workspace integration

- [ ] 6.1 Run `gofmt -l`, `go vet ./...`, and `go test -race ./...` in every module listed in `go.work`, and verify all pass
