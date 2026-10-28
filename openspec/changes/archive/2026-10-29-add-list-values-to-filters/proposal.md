# Proposal

## Why

Filtering a field against several values means chaining `OR`s today, as in
`(status = 'success' OR status = 'pending')`. Array `CONTAINS`/`EXCLUDES` and
`BETWEEN` pack their values into one quoted string, `'a,b'`. That string form
cannot express a value containing a comma, forces every value to be a string,
and can only report errors at the whole literal. A parenthesized list of
typed values solves all three, and adding `IN` gives the language its first
real list operator.

## What Changes

- New `IN` and `NOT IN` predicates take a parenthesized list of plain values
  (strings, numbers, Booleans) and `NULL`, such as
  `status IN ('success', 'pending', NULL)` or `status NOT IN ('error')`.
  `IN` matches rows that `=` would match for any item. `NOT IN` matches rows
  that `!=` would match for every item, so rows whose field is NULL match it
  unless the list contains `NULL`.
- Array `CONTAINS` and `EXCLUDES` also accept a parenthesized list, such as
  `tags CONTAINS ('a,b', 'c')`. List items are never split or trimmed, so an
  element containing a comma can be matched for the first time. The quoted
  `'a,b'` form is unchanged.
- `BETWEEN` also accepts a parenthesized pair, such as `age BETWEEN (30, 40)`.
  Each bound keeps the type it was written as and is converted like the
  value of `=`. The quoted `'30,40'` form is unchanged.
- Scalar text `CONTAINS` and `EXCLUDES` reject a list value with an error at
  the `(`.
- `IN` is rejected on array fields, like every other operator except
  `CONTAINS`, `EXCLUDES`, `= NULL`, and `!= NULL`.
- `NULL` is allowed only as an `IN`/`NOT IN` item, not in `CONTAINS`,
  `EXCLUDES`, or `BETWEEN` lists. Relative values (`TODAY`, `{…}`) are not
  allowed in any list. Relative values in lists are left for a later change.
- The public AST gains a list literal kind, which holds the items, and the
  `IN` and `NOT IN` operators.
- The lexer gains a `,` token.
- **BREAKING**: `IN` and `NOT` become reserved words. A field named `in` or
  `not` must now be double-quoted, as in `"in" = 'x'`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `filter-parsing`: accepts `IN`/`NOT IN` lists, list values after
  `CONTAINS`/`EXCLUDES`, and pairs after `BETWEEN`. Allows `NULL` as an `IN`
  item only, and reserves `IN` and `NOT`.
- `contract-filter-ast`: adds the list literal kind and the `IN`/`NOT IN`
  operators. Allows a NULL literal as an `IN`/`NOT IN` list item.
- `contract-sql-driver`: defines which field types accept `IN`/`NOT IN`, the
  matching rules for `IN` and `NOT IN` (NULL items included), list values for
  array `CONTAINS`/`EXCLUDES`, list bounds for `BETWEEN`, and rejecting lists
  on scalar text.
- `sql-goqu-compilation`: defines the PostgreSQL SQL for `IN` and `NOT IN`,
  including how NULL items and NULL fields are handled.

## Impact

- **easylist-filter**: lexer (`,` token, `IN` and `NOT` keywords),
  `grammar.y` and the regenerated `parser_gen.go`, the token table in
  `parser.go`, `ast.go` (`LiteralList`, `Literal.Items()`, `OpIn`,
  `OpNotIn`), `docs/query-language.md` (EBNF and prose), and
  `grammar_doc_test.go`.
- **easylist-core**: README (operators per type, and the array list text
  that says commas cannot be matched) and the operator comment in `plan.go`.
  No planner behaviour changes, because lists hold no expression values.
- **easylist-sql-goqu**: `predicate.go` (`IN`/`NOT IN`, array list values,
  `BETWEEN` pairs, rejecting lists on text) and README.
- **easylist-gqlgen** and **easylist-export** pass filter text through
  unchanged and need no changes.
- Consumers of the public AST that switch on `LiteralKind` or `PredicateOp`
  must handle or reject the new kind and operators.
