# Design

## Context

See proposal.md (Why) for the motivation and specs/ for the required behaviour. Today:

- `easylist-filter` lexes `NULL` as an identifier. The grammar's `value` rule accepts only `tSTRING`, `tNUMBER`, and `tBOOL`, so `x = NULL` fails with "unexpected field".
- `easylist-core` passes the filter tree through to the plan. `filterFields` checks field names only, never operators or literal kinds, so core needs no code change.
- In `easylist-sql-goqu`, `buildPredicate` compiles `!=` to `<>`. It rejects `EXCLUDES` for every type through the final operator check, and converts every literal with `predicateArgument`, which would reject a NULL literal as an invalid value.

## Goals / Non-Goals

**Goals:**
- Represent NULL comparisons faithfully in the AST, keeping source positions so errors and future consumers can point at the keyword.
- Keep the filter language schema-free: decide in the parser what is syntactically meaningless, such as `age > NULL`, and leave type questions to drivers.
- Keep each predicate's SQL a single self-contained expression, so `AND`/`OR` grouping and argument order are unaffected.

**Non-Goals:**
- Array fields and comma-separated value lists for `CONTAINS` and `EXCLUDES`. A follow-up change covers them.
- A general `NOT` operator, or `IS NULL` / `IS NOT NULL` spellings in the filter language.
- Changing how `>`, `<`, `BETWEEN`, or `CONTAINS` treat NULL. They still never match NULL rows.

## Decisions

### NULL is a literal kind, not a new operator
`x = NULL` parses to a predicate with operator `OpEq` and a `Literal` of the new kind `LiteralNull`. The literal's text is the keyword's source spelling and its position is the keyword's position. `x != NULL` uses `OpNeq` in the same way.

*Alternative:* new operators `OpIsNull` and `OpIsNotNull` with an empty value. This was rejected because the tree would no longer mirror the source: the predicate would have an operator the user never typed and a value with no position. `LiteralNull` is also a purely additive public change. A consumer that switches on `LiteralKind` sees an unknown kind and can reject it, whereas a consumer that switches on operators would silently mis-handle new ones.

### The grammar restricts where NULL may appear
`NULL` gets its own token, `tNULL`, rather than joining the `value` rule. Two new `predicate` alternatives accept it:

```yacc
predicate : tIDENT ordinary_op value
          | tIDENT tBETWEEN tSTRING
          | tIDENT '=' tNULL
          | tIDENT tNEQ tNULL ;
```

As a result, `age > NULL`, `name CONTAINS NULL`, `age BETWEEN NULL`, and `NULL = 'x'` are syntax errors at the `NULL` token's position. This follows the existing precedent: `BETWEEN` already enforces its string argument in the grammar.

*Alternative:* add `tNULL` to `value` and reject other operators in an action or in drivers. This was rejected because it moves a schema-independent rule out of the parser and makes every driver repeat it.

The lexer adds `"NULL": tokNull` to `keywords`, which is case-insensitive like the other keywords. `tokNull` goes at the end of `tokenKind` so existing values and `yyCodes` indexes stay put. `tokenDisplay` maps `tNULL` to `NULL` so error messages read naturally. A double-quoted `"NULL"` already lexes as a quoted identifier, so it needs no change.

goyacc's default reductions may leave `NULL` out of some "expected ..." lists, for example after `=` followed by an invalid token. The error messages stay correct, just less complete. The tests should pin whatever goyacc produces rather than add a rewrite rule like the one for `AND`.

### SQL compilation in easylist-sql-goqu
`buildPredicate` handles a NULL literal first, before any type-based dispatch, because a NULL comparison is valid for every type and binds nothing:

| Filter | SQL | Arguments |
|---|---|---|
| `x = NULL` | `(col) IS NULL` | none |
| `x != NULL` | `(col) IS NOT NULL` | none |
| `x != v` | `(col) IS DISTINCT FROM ?` | `v` converted as today |
| `x EXCLUDES 'a'` (text) | `COALESCE((col) NOT ILIKE ? ESCAPE E'\\', TRUE)` | `%a%`, escaped as for `CONTAINS` |

As a safeguard, a NULL literal on any operator other than `=` or `!=` is rejected with the usual "unsupported operator" error. The parser cannot produce one, but the AST interface does not forbid it.

**`IS DISTINCT FROM` for `!=`:** this is the PostgreSQL operator that means exactly "not `=`, with NULL as an ordinary value". It works with the existing bound arguments, including `?::numeric`, booleans, and `time.Time`. The note in `predicate.go` about avoiding goqu's `Neq` stays relevant: plain `goqu.L` keeps the boolean argument bound.

**`COALESCE(... NOT ILIKE ..., TRUE)` for `EXCLUDES`:** the field's author SQL appears once.

*Alternative:* `(col IS NULL OR col NOT ILIKE ?)` repeats the author SQL, which may be an expensive expression or subquery, and needs care with parentheses. The two forms give the same results.

### Non-nullable fields accept NULL comparisons
`name = NULL` compiles to `IS NULL` even when `name` is not declared `Nullable`. `Nullable` is a promise checked when values are read; it is not a filter rule. Rejecting the filter would make it fail whenever a list definition changes a field's nullability.

### easylist-core is documentation only
The README's type table gains `EXCLUDES` for `Text` and notes that every type accepts `= NULL` and `!= NULL`. The sentence "Filters cannot compare with NULL" is replaced.

## Risks / Trade-offs

- **[Existing `!=` filters return more rows]** → The proposal marks this as BREAKING, and the `easylist-sql-goqu` README documents it. Users who want the old result add `AND x != NULL`, which the "Keeping the old behaviour" scenario covers.
- **[A column named `null` must now be quoted]** → This follows the same rule as `and`, `or`, `true`, and the other keywords. It is documented in `query-language.md`.
- **[Third-party drivers meet `LiteralNull`]** → No third-party drivers are known, since the modules are unpublished. A driver that converts literals by kind fails with an "invalid value" error rather than producing wrong SQL.
- **[`IS DISTINCT FROM` and `NOT ILIKE` rarely use indexes]** → `<>` and `NOT ILIKE` had the same limitation, so performance does not change materially.

## Migration Plan

There is no data or schema migration. Ship all modules together, since they build in one workspace. Callers relying on `!=` skipping NULL rows add `AND field != NULL`. Rollback means reverting the change; filters that use `NULL` then become syntax errors again.
