# Tasks

## 1. easylist-filter

- [ ] 1.1 Add a lexer brace mode in `lexer.go`: `{` and `}` tokens, and inside braces the operator tokens `+ - * /`, parentheses, `::`, unsigned numbers, and identifiers. A nested `{` is an error. Outside braces scanning is unchanged. Verify with `TestLexFilterTokens` cases for `{TODAY+5::DAYS}`, `{today + 5::days}`, `{-1.50 * 2}`, an unclosed `{`, a nested `{`, and `age > -3` still lexing a signed number
- [ ] 1.2 Add `LiteralTime`, the `TimeValue` type with `IsDate()` and `Eval(now, weekStart)`, and `Literal.Time()` in `ast.go` (keyword evaluation in a new `timevalue.go`). Cover keyword meaning, Monday and Sunday week starts, zone-dependent `TODAY`, leap-year `END_OF_MONTH`, `DAYS` across a DST change, `HOURS`/`MINUTES`, `BUSINESS_DAYS` forward, backward and zero on weekends, and the out-of-range error, with table tests
- [ ] 1.3 Add brace-expression typing and numeric folding (`math/big.Rat`, exact decimal text, rejecting non-terminating quotients and division by zero), with errors at operator positions. Cover every rule in the `filter-parsing` "Duration arithmetic types", "Time arithmetic types", and "Brace expression result" requirements and the duration amount limit and unit errors
- [ ] 1.4 Extend `grammar.y`: the `'{' expr '}'` value with precedence for `+ - * /`, unary minus and parentheses; bare keyword values via `tIDENT` on the value side restricted to `= != > >= < <=`; `n::UNIT` durations. Map the new token kinds in `yyCodes` and `tokenDisplay` in `parser.go`. Verify `goyacc` reports no conflicts and `TestYYCodesCoverAllTokenKinds` passes
- [ ] 1.5 Run `go generate ./...` in `easylist-filter`, commit the regenerated `parser_gen.go`, and verify a second `go generate ./...` leaves it unchanged and `TestGeneratedCodeExportsNothing` passes
- [ ] 1.6 Add parser tests for every `filter-parsing` and `contract-filter-ast` scenario, including keyword spellings as field names, unknown words, keywords with `CONTAINS`/`BETWEEN`, arithmetic outside braces, folded literal text and positions, and relative literal text, position and `IsDate()`. Pin new error messages in `TestParseErrorMessages`, and verify `go test ./...` passes
- [ ] 1.7 Document keywords, braces, units, typing rules, business days, and the reference-time and time-zone model in `docs/query-language.md` (grammar block included) and `docs/parser.md`; verify `grammar_doc_test.go` passes

## 2. easylist-core

- [ ] 2.1 Add `Now time.Time` and `WeekStart *time.Weekday` to `Request`, and `Now time.Time` and `WeekStart time.Weekday` to `Plan`. `CompiledList.Plan` copies them, defaults the week start to Monday, and fails with the literal's position when the filter has a `LiteralTime` and `Now` is zero (walk the tree alongside `filterFields`). Cover the `core-query-planning` scenarios with tests
- [ ] 2.2 Add `Clock func() time.Time` and `WeekStart *time.Weekday` to `Runtime`, plus a `Now()` method (clock or `time.Now`). `Fetch` fills a missing `Now` and `WeekStart` before planning. Cover the `core-list-execution` scenarios with a fake driver that records the plan
- [ ] 2.3 Add `UTCOffset(minutes int) (*time.Location, error)`, which validates -720…840. Add `AsOf time.Time` (`json:"asOf,omitzero"`) and `Timezone *int` (`json:"timezone,omitempty"`) to `ExportRequest`. `ValidateExport` validates `Timezone` and, when `AsOf` is set, sets `Request.Now = AsOf.In(zone)`. Verify the JSON round-trip test with and without the new fields, decoding old JSON, and the offset range error
- [ ] 2.4 Update `easylist-core/README.md`: relative values in the types table, the reference time and week start on `Request` and `Runtime`, and `AsOf`/`Timezone` on export requests. Verify `go test ./...` passes in `easylist-core`

## 3. easylist-sql-goqu

- [ ] 3.1 In `predicateArgument`/`buildPredicate`, evaluate `LiteralTime` values with `p.Now` and `p.WeekStart`. Date values on date fields bind like `YYYY-MM-DD` literals, timestamp values on timestamp fields bind truncated to microseconds, and timestamp values on date fields, any other field type, and array fields are rejected at the literal position. Evaluation errors are reported at the literal position
- [ ] 3.2 Implement whole-day comparisons for date values on timestamp fields: operator rewriting for `> >= < <=`, `(? >= ? AND ? < ?)` for `=`, and `COALESCE(NOT (…), TRUE)` for `!=`. Cover every `contract-sql-driver` "Relative time comparison" and "Whole-day comparison on timestamp fields" scenario and every `sql-goqu-compilation` scenario with SQL and argument golden tests, including UTC+02:00 boundaries and a DST zone
- [ ] 3.3 Add a driver-level golden test for `created_at >= START_OF_WEEK AND due < {TODAY + 5::BUSINESS_DAYS}` with a fixed `Plan.Now`, checking argument order. Update the README's PostgreSQL semantics section with relative values and whole-day comparisons. Verify `go test ./...` passes

## 4. easylist-gqlgen

- [ ] 4.1 Add `timezone: Int` with an SDL description to `<P>ListInput` and `<P>ExportInput` in `sdl.go`. Add `Timezone *int` to `ListInput` and `ExportInput`, and map it in the generated glue (`codegen.go`). Update the SDL golden tests
- [ ] 4.2 In `ResolveList`, validate `Timezone` with `easylist.UTCOffset` and set `req.Now = rt.Now().In(zone)`. In `Export`, set `AsOf = rt.Now().UTC().Round(0)` and `Timezone`. Cover the `gqlgen-list-resolution` scenarios with an injected `Runtime.Clock`
- [ ] 4.3 Run the generator for `internal/fixture`, commit the regenerated fixture files, add a fixture test that filters with `TODAY` and `timezone`, and update the README. Verify `go test ./...` passes

## 5. easylist-export

- [ ] 5.1 In `Run`, fill a zero `Now` with `rt.Now()` in the request's zone (server local when `Timezone` is unset) after `ValidateExport`, and fill `WeekStart` from the runtime. Cover the `export-file-generation` scenarios (a job run a day later uses `AsOf`; an old request uses the clock) and update the README

## 6. Workspace integration

- [ ] 6.1 Run `gofmt -l`, `go vet ./...`, and `go test -race ./...` in every module listed in `go.work`, and verify all pass
