# Design

## Context

See proposal.md for why. The relevant parts of the code today:

- `easylist-filter` lexes to tokens and parses them with a goyacc grammar
  (`grammar.y`, generated into `parser_gen.go`). `predicateNode` holds one
  `Literal`, and `Literal` is a struct with a kind, text, position, and an
  optional expression tree that only `LiteralExpr` uses.
- `parseState` tracks every `(` and `)` it hands to the parser so that it can
  report an unclosed group or a stray `)`. List parentheses will be the same
  tokens.
- `easylist-core` reads literals only to find `LiteralExpr` values to
  evaluate. It keys evaluated values by predicate.
- `easylist-sql-goqu` compiles each predicate in `buildPredicate`. Array
  lists come from `arrayValues`, which splits a string literal on commas.
  `BETWEEN` splits its string and forces the bound kind from the field type:
  numbers for numeric fields, strings for dates. Negated forms (`!=`, text
  and array `EXCLUDES`, whole-day `!=`) are written so that NULL fields match,
  and the field's SQL is emitted once.

## Goals / Non-Goals

**Goals:**
- One list syntax in the grammar, shared by `IN`, `CONTAINS`/`EXCLUDES`, and
  `BETWEEN`, with list size and NULL items checked by the grammar where it can.
- Existing filters, apart from fields named `in` or `not`, parse to the same AST and compile
  to the same SQL as before.
- Per-item error positions in every driver error about a list.

**Non-Goals:**
- Relative values in lists. They need the planner to key evaluated values per
  literal instead of per predicate. That is a separate change.
- A general `NOT` prefix such as `NOT (a = 1)`. `NOT` is valid only before
  `IN`.
- Lists on scalar text `CONTAINS`/`EXCLUDES`. The driver rejects them, so a
  later change can give them a meaning without breaking anything.

## Decisions

### A list is a literal kind, not a new predicate shape

`LiteralList` joins `LiteralKind`, and `Literal` gains `Items() []Literal`,
which is nil for every other kind. The list's text is its source spelling and
its position is the `(`, like `LiteralExpr` with its braces. `Items()` returns
a copy, so the AST stays immutable.

Alternatives considered:
- *`Predicate.Values() []Literal`*: changes the `Predicate` interface for
  every consumer, and leaves `Value()` with no good meaning for lists.
- *A separate `ListPredicate` interface*: consumers would need a type switch
  in front of their current predicate handling, and the `CONTAINS` and
  `BETWEEN` forms would be split across two shapes.

The literal kind follows the precedent of `LiteralExpr`. Consumers that
switch on `LiteralKind` are already documented as having to handle or reject
kinds they don't know. The planner keeps working unchanged because it looks
only for `LiteralExpr`.

### Two operators, appended to the enum

`OpIn` and `OpNotIn` are added after `OpNeq`, so existing `PredicateOp` values
keep their numbers. They follow the existing pairs (`OpEq`/`OpNeq`,
`OpContains`/`OpExcludes`) rather than adding a negation flag that only one
operator would use. `NOT IN`'s operator position is the `NOT`, which is where
the operator starts in the source.

### Three list rules in the grammar

```
in_list    : '(' in_items ')'        items are plain_value | tNULL
value_list : '(' plain_items ')'     items are plain_value
range      : '(' plain_value ',' plain_value ')'
```

`IN`/`NOT IN` use `in_list`, `CONTAINS`/`EXCLUDES` take `plain_value` or
`value_list`, and `BETWEEN` takes `tSTRING` or `range`. The grammar rejects
NULL items outside `IN`, as well as wrong `BETWEEN` arity, so these are
syntax errors at the offending token and never reach a driver. The
parenthesized forms come after an operator and a grouping `(` only starts
a `primary`, so the grammar gains no shift/reduce conflicts. goyacc must
still report zero conflicts after the change.

The alternative was one generic list rule, with the parser actions checking
NULL items and arity. That gives worse error positions and pushes rules into
action code that the EBNF doc test cannot see.

### Lexer: `,`, `IN`, and `NOT`

`,` becomes `tokComma` in both modes. Inside braces nothing accepts it, so
`{1, 2}` fails at the comma, the same as any other stray symbol. `IN` and
`NOT` join the `keywords` table, and therefore the reserved words. The
parser's existing `(` bookkeeping covers list parentheses unchanged, so
`status IN ('a'` reports the unclosed `(` at position 11.

### SQL for IN and NOT IN

Let `L` be the non-NULL items bound as `$1, …` and `n` whether any item is
NULL. Every form emits the field's SQL once:

```
                 n = false                        n = true
  IN      (L)    x IN (L)                         COALESCE(x IN (L), TRUE)
  NOT IN  (L)    COALESCE(x NOT IN (L), TRUE)     COALESCE(x NOT IN (L), FALSE)
  IN      ()     -                                x IS NULL
  NOT IN  ()     -                                x IS NOT NULL
```

Because `L` never contains NULL, `x IN (L)` and `x NOT IN (L)` are NULL only
when `x` is NULL. `COALESCE` then supplies the answer the semantics require
for a NULL field. `x IN ($1, $2)` lets PostgreSQL infer each argument's type
from the column, as scalar comparisons already do. `x = ANY($1)` with one
array argument would need an explicit array type, which breaks for enums.
Numeric items keep their `?::numeric` binding inside the list.

Expanding to `x = $1 OR x = $2` / `x IS DISTINCT FROM $1 AND …` was
considered and rejected, because it repeats the author's SQL once per item.

### Driver conversion of list items

A shared helper converts each non-NULL item with `predicateArgument`, using
the item's own kind and position. `IN` and `NOT IN` use it with the field's
type. `arrayValues` uses it for a `LiteralList` with the element type, and
skips the split and trim. `BETWEEN` uses it for a `range`. The string form of
`BETWEEN` keeps forcing the bound kind, so existing filters compile the same.
For a scalar text field, `buildPredicate` checks for a list on
`CONTAINS`/`EXCLUDES` before the `ILIKE` path, and returns an error naming the
field at the list's position.

`IN` and `NOT IN` on an array field fall into the existing "only
`CONTAINS`/`EXCLUDES`/`= NULL`/`!= NULL`" check and are rejected as
unsupported operators.

## Risks / Trade-offs

- [`IN` and `NOT` become reserved, so filters on fields named `in` or `not`
  stop parsing] → The proposal marks this breaking. The README and
  query-language docs list the new reserved words, and the fix is to
  double-quote the name.
- [Consumers outside this repo that switch on `LiteralKind` or `PredicateOp`
  without a default branch silently ignore lists] → The doc comments on
  `LiteralList`, `OpIn`, and `OpNotIn` say to handle or reject them, as
  `LiteralExpr` does. The modules are unpublished, so the only in-repo
  consumers are the planner and sql-goqu.
- [The two `BETWEEN` and array forms behave slightly differently: the string
  forms trim and force kinds, the list forms don't] → The specs give a
  scenario for each difference, and the docs show the two forms side by side.
- [A very long list can exceed PostgreSQL's 65,535 bound parameters] → No
  limit is added. Such a filter fails with a database error, which is the
  same outcome as a long `OR` chain today.

## Migration Plan

No data migration is needed. Filters stored by callers keep working, except
those that use an unquoted field named `in` or `not`, which must be quoted.
Rolling back restores the old grammar, and the only filters that would then
stop working are ones that use the new syntax.
