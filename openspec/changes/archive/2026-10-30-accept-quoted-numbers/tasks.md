# Tasks

## 1. easylist-core

- [x] 1.1 In the Types section of `easylist-core/README.md`, say that integer, float, and numeric fields also accept a quoted number, converted exactly like an unquoted one (`age > '30'` is `age > 30`, while `'30.0'` and `' 30'` still fail on an integer field), and that booleans still need `TRUE` or `FALSE`. Verify by reading the section next to the `IN` paragraph's "Each item is converted to the field type" sentence, and checking that the two agree.

## 2. easylist-sqlpg

- [x] 2.1 In `predicateArgument` (`easylist-sqlpg/predicate.go`), accept `filter.LiteralString` next to `filter.LiteralNumber` in the `Integer`, `Float`, and `Numeric` cases, keeping the existing parse, `predicateNumber` pattern, and finiteness checks and the error text unchanged. Verify with `go test ./...` in `easylist-sqlpg` after 2.2.
- [x] 2.2 In `TestLowerErrors` (`easylist-sqlpg/lower_test.go`), remove the `age = '30'`, `age IN (1, '30')`, and `age BETWEEN ('30', '40')` cases. Add failing cases for `age = '30.0'`, `age = ' 30'`, `age = 'abc'`, `score = 'NaN'`, `price = '1e3'`, and `active = 'true'`, each with the exact error text and position. Add lowering tests showing that `age > '30'`, `score >= '-1.5'`, `price < '19.990'`, `age IN (1, '30')`, and `age BETWEEN ('30', '40')` produce the same SQL and arguments as their unquoted forms, with `int64`, `float64`, and numeric-cast string arguments. Verify with `go test -race ./...` in `easylist-sqlpg`.
- [x] 2.3 In `easylist-sqlpg/conformance/cases.go`, replace the "quoted number list item" error case with a matching case for `age BETWEEN ('30', '40')` that expects `ids(2, 3, 4, 9)` and the same arguments as "BETWEEN list bounds". Add a case for `age > '9'` with its expected ids and an `int64(9)` argument, so the live run proves the comparison is numeric. Verify with `EASYLIST_PG_DSN=… go test -race ./...` in `easylist-sqlpg`: the new cases run live and none is skipped.
- [x] 2.4 In `easylist-sqlpg/README.md`, next to the array literal bullets, say that number fields and number array items accept quoted numbers, and that `ids CONTAINS (7, 'x')` still fails at `'x'`. Verify that the text matches the behavior the tests from 2.2 and 2.3 pin down.

## 3. Driver modules

- [x] 3.1 In `easylist-sql-goqu/predicate_test.go`, remove the `age = '30'` row of `TestBuildPredicateRejectsBadValues` and the `age IN (1, '30')` and `age BETWEEN ('30', '40')` rows of `TestBuildPredicateRejectsListValues`, which still expect the old rejection. Verify with `go test -race ./...` in `easylist-sql-goqu`.
- [x] 3.2 Run the full workspace suite with a live database, `EASYLIST_PG_DSN=… go test -race $(go list -m -f '{{.Dir}}/...')`, from the repository root. Verify that easylist-sql-bob, -ent, -goqu, -gorm, -jet, and -sqlbuilder all pass the new conformance cases with no driver code change, and that no live case is skipped.
