# Proposal

## Why

Filters cannot test whether a field is NULL: `NULL` is read as a field name, so `country = NULL` is a syntax error. Negative filters also drop NULL rows, because `!=` compiles to SQL `<>`. That surprises users, who expect `country != 'France'` to return rows with no country. `EXCLUDES` already parses, but every driver rejects it, so users cannot exclude a substring from a text field.

## What Changes

- Add a `NULL` keyword to the filter language. It is accepted only as the value of `=` and `!=`: `x = NULL` matches rows where `x` is NULL, and `x != NULL` matches rows where it is not. Any other operator with `NULL` is a syntax error.
- **BREAKING**: `NULL` becomes a reserved word, like `TRUE` or `AND`. A column named `null` can no longer be written unquoted and must be written as `"null"`.
- Add a `LiteralNull` literal kind to the public filter AST. A NULL predicate is an `OpEq` or `OpNeq` predicate whose value has that kind.
- **BREAKING**: `!=` matches rows whose field is NULL, so it returns exactly the rows `=` does not. `country != 'France'` now returns rows where `country` is NULL. The goqu driver compiles `!=` to `IS DISTINCT FROM` instead of `<>`.
- Support `EXCLUDES` on text fields. `x EXCLUDES 'a'` matches rows where `x` does not contain `a` case-insensitively, and rows where `x` is NULL.
- Every field type accepts `= NULL` and `!= NULL`, whether or not the field is declared nullable.

Array fields, and lists of values for `CONTAINS` and `EXCLUDES`, are out of scope. They follow in a separate change that builds on this one.

Modules touched:
- `easylist-filter`: the `NULL` keyword, grammar, AST literal kind, and docs.
- `easylist-core`: README only (operator table and NULL wording). No code changes.
- `easylist-sql-goqu`: compiling NULL comparisons, `!=`, and text `EXCLUDES`, plus the README.

`easylist-gqlgen` and `easylist-export` are not touched. They pass filter text through and pick up the new behaviour automatically.

## Capabilities

### New Capabilities
- `filter-parsing`: the filter language syntax that `easylist-filter` accepts and rejects, starting with the `NULL` keyword.
- `contract-filter-ast`: how parsed filters are represented in the public syntax tree that drivers consume, starting with NULL predicates.

### Modified Capabilities
- `contract-sql-driver`: operators per type gain `= NULL`, `!= NULL`, and text `EXCLUDES`. NULL literals are exempt from literal conversion. Comparison semantics change so that `!=` and `EXCLUDES` match NULL rows.
- `sql-goqu-compilation`: `!=` compiles to `IS DISTINCT FROM` instead of `<>`. NULL comparisons compile to `IS NULL` and `IS NOT NULL` with no bound argument. Text `EXCLUDES` compiles to a NULL-safe negated `ILIKE`.

## Impact

- **Public API (`easylist-filter`)**: a new `LiteralNull` constant. Code that switches on `LiteralKind` sees a new kind; any third-party driver must handle it or reject it.
- **Query results**: every `!=` filter compiled by `easylist-sql-goqu` returns more rows when the field is nullable and holds NULL. GraphQL and export clients see this without changing anything.
- **Generated code**: `grammar.y` changes, so `parser_gen.go` is regenerated.
- **Dependencies**: none added.
