# Proposal

## Why

A UI that builds filter text quotes every value, so it sends `age > '30'` for an integer field. Every driver rejects that today because the contract says a number field needs an unquoted number. The rule is also inconsistent: the older quoted forms `age BETWEEN '30,40'` and `ids CONTAINS '7, 8'` already read quoted text as integers, while `age > '30'` and `age BETWEEN ('30', '40')` fail.

## What Changes

- An integer, float, or numeric field accepts a string literal whose text is a valid unquoted number for that field, and converts it exactly as if it were unquoted: `age > '30'` compiles like `age > 30`.
- The same holds for each item of a value list (`IN`, `NOT IN`, `BETWEEN (…)`, and `CONTAINS`/`EXCLUDES` lists on number array fields), so `age BETWEEN ('30', '40')` matches like `age BETWEEN (30, 40)`.
- The value is still bound as a typed number, so comparisons stay numeric, never textual.
- A quoted value is not trimmed, and anything an unquoted number would not accept is still rejected with the same error text: `'30.0'` and `' 30'` on an integer field, `'1e3'` or `'NaN'` on a float or numeric field.
- No other type changes: booleans still need `TRUE` or `FALSE`, and text, uuid, enum, date, and timestamp still need a string.
- Filters that used to fail now compile. No filter that compiled before changes meaning.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `contract-sql-driver`: "Literal conversion per type" lets integer, float, and numeric fields accept a quoted number; "List item conversion" drops "a quoted number is a string" and its rejecting scenario becomes an accepting one.

## Impact

- **easylist-sqlpg**: the shared literal conversion that every PostgreSQL driver calls; its unit tests that currently expect `age = '30'`, `age IN (1, '30')`, and `age BETWEEN ('30', '40')` to fail; the conformance case "quoted number list item"; the README section on literals.
- **easylist-core**: README only, where the types section describes how filter values are converted.
- **easylist-sql-bob, easylist-sql-ent, easylist-sql-goqu, easylist-sql-gorm, easylist-sql-jet, easylist-sql-sqlbuilder**: no code change. They inherit the behavior through easylist-sqlpg and are checked by the shared conformance suite.
- **easylist-filter**, **easylist-gqlgen**, **easylist-export**: untouched. The parser still reports `'30'` as a string literal.
