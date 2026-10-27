# Tasks

## 1. easylist-filter

- [x] 1.1 Add the `LiteralNull` kind to `LiteralKind` in `ast.go`, documented as the `NULL` keyword with its source spelling; verify `go build ./...` succeeds in `easylist-filter`
- [x] 1.2 Add `tokNull` at the end of `tokenKind` and add `"NULL": tokNull` to `keywords` in `lexer.go`; verify with `TestLexFilterTokens` cases for `NULL`, `null`, and `Null` (keyword token with source spelling) and `"NULL"` (quoted identifier)
- [x] 1.3 In `grammar.y`, declare `tNULL` and add the `tIDENT '=' tNULL` and `tIDENT tNEQ tNULL` predicate alternatives, which build an `OpEq`/`OpNeq` predicate with a `LiteralNull` value at the keyword's position. Map `tokNull` to `tNULL` in `yyCodes` and `tNULL` to `NULL` in `tokenDisplay` in `parser.go`. Verify `goyacc` reports no conflicts and `TestYYCodesCoverAllTokenKinds` passes
- [x] 1.4 Run `go generate ./...` in `easylist-filter`, commit the regenerated `parser_gen.go`, and verify a second `go generate ./...` leaves it unchanged and `TestGeneratedCodeExportsNothing` passes
- [x] 1.5 Add parser tests for every `filter-parsing` and `contract-filter-ast` scenario: `= NULL`, `!= NULL`, keyword case, quoted `"NULL"` field, NULL inside `OR`, the value's kind, text, and position, `'NULL'` as a string, and syntax errors at the `NULL` position for `age > NULL`, `name CONTAINS NULL`, `age BETWEEN NULL`, and `NULL = 'x'`. Pin the exact error messages in `TestParseErrorMessages`, and verify `go test ./...` passes
- [x] 1.6 Update `docs/query-language.md` (grammar block, `NULL` keyword and where it is allowed, reserved word note for columns named `null`) and the predicate and error-display sections of `docs/parser.md`; verify `grammar_doc_test.go` passes with the new grammar block

## 2. easylist-core

- [x] 2.1 Update the types table and NULL paragraph in `easylist-core/README.md`: `Text` gains `EXCLUDES`, every type accepts `= NULL` and `!= NULL`, and `!=` and `EXCLUDES` match NULL rows. Replace "Filters cannot compare with NULL". Verify `go test ./...` passes in `easylist-core` (README example tests still compile) and that no other README sentence says filters cannot compare with NULL

## 3. easylist-sql-goqu

- [x] 3.1 In `buildPredicate`, handle a `LiteralNull` value before type dispatch: compile `OpEq` to `? IS NULL` and `OpNeq` to `? IS NOT NULL` with the parenthesized column and no argument, and reject other operators as unsupported. Verify with predicate tests for `= NULL` and `!= NULL` on every field type, including a non-nullable field and an enum (no declared-value check), and a test that a NULL literal on a non-equality operator is rejected
- [x] 3.2 Compile `OpNeq` with a value to `? IS DISTINCT FROM ?` and update the comment that explains avoiding goqu's `Neq`; verify the existing `!=` predicate tests expect `IS DISTINCT FROM` for text, numeric (`::numeric` cast kept), boolean (bound `true`), date, uuid, and enum
- [x] 3.3 Allow `OpExcludes` on text, compiling to `COALESCE(? NOT ILIKE ? ESCAPE E'\\', TRUE)` with the same escaped `%…%` pattern as `CONTAINS`; verify with tests for the SQL and the `%50\%\_off%` argument, and that `EXCLUDES` on non-text types is still rejected with the operator, field, and position
- [x] 3.4 Add a driver-level golden test for `country = NULL AND name != NULL` with limit 10 and offset 0, checking `IS NULL`, `IS NOT NULL`, and arguments `10`, `0` bound to `$1`, `$2`; verify `go test ./...` passes
- [x] 3.5 Update the "PostgreSQL semantics" section of `easylist-sql-goqu/README.md`: `!=` compiles to `IS DISTINCT FROM` and matches NULL, with `AND x != NULL` for the old behaviour. Document `= NULL`/`!= NULL` and text `EXCLUDES`. Verify the README no longer says `IS DISTINCT FROM` is not used and its example test passes

## 4. Workspace integration

- [x] 4.1 Run `go vet ./...` and `go test -race ./...` in every module listed in `go.work` and verify all pass, including the `easylist-gqlgen` fixture and `easylist-export` tests that parse filters
