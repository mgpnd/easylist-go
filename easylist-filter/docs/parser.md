# How the parser works

This document describes the internals of `filter.Parse`. The language itself,
the grammar a user writes, is described in [query-language.md](query-language.md).

`Parse` runs in two stages:

```
input string ──lexFilter──▶ []token ──parseState──▶ yyNewParser().Parse ──▶ Expr
             (lexer.go)             (parser.go)     (parser_gen.go, from grammar.y)
```

The lexer turns bytes into tokens and knows nothing about grammar. The parser
turns tokens into a tree and never looks at raw bytes. Either stage can fail;
on failure `Parse` returns a nil `Expr` and an error ending in
`at position N`, where `N` is a 1-based byte offset into the input.

## Stage 1: the lexer

`lexFilter` drives a `lexer`, a cursor over the input string:

```go
type lexer struct {
	src string
	i   int // next byte to read
}
```

The scanners are built from four small helpers, so none of them repeat
bounds checks:

| Helper           | Meaning                                                     |
| ---------------- | ----------------------------------------------------------- |
| `done()`         | the cursor is at the end of input                           |
| `peekIs(pred)`   | the next byte exists and matches `pred`; consumes nothing   |
| `accept(ch)`     | consume the next byte if it is `ch`; report whether it was  |
| `skipWhile(pred)`| consume bytes matching `pred`; report whether any were      |

The main loop skips ASCII whitespace and calls `scanToken`, which dispatches
on the first byte:

| First byte              | Scanner          | Produces                                   |
| ----------------------- | ---------------- | ------------------------------------------ |
| `'` or `"`              | `scanQuoted`     | `tokString` (`'…'`) or quoted `tokIdent` (`"…"`) |
| letter, `_`, byte ≥ 128 | `scanIdentifier` | a keyword token or `tokIdent`              |
| digit, `+`, `-`         | `scanNumber`     | `tokNumber`                                |
| `= ( ) > < !`           | inline in `scanToken` | operator and paren tokens             |

Each token records its kind, its text, and the 1-based byte position of its
first byte. A few rules worth knowing:

- **Two-byte operators.** After `>` or `<`, `accept('=')` decides between
  `tokGt`/`tokGte` and `tokLt`/`tokLte`. `!` is only valid when followed by
  `=`, so `a ! 1` fails at the `!`.
- **Quotes.** A doubled delimiter is an escaped quote: `'it''s'` decodes to
  `it's`. The token text holds the decoded value, and its position points at
  the opening quote. NUL bytes are rejected even inside quotes.
- **Keywords and identifiers.** `scanIdentifier` reads the whole word, then
  looks up its uppercase form in the `keywords` map. Keywords keep their
  source spelling (`aNd` stays `aNd`). Everything else is a field name and
  goes through `foldASCIILower`, which lowercases `A`–`Z` and leaves every
  other byte alone. This matches PostgreSQL's folding of unquoted names.
  `strings.ToLower` is not used, because it would rewrite non-ASCII letters
  and invalid UTF-8.
- **Numbers.** An optional sign, digits, and an optional `.` that must be
  followed by digits. The number must not run straight into an identifier
  byte or another `.`, so `1abc` and `1.2.3` are errors rather than being
  split into misleading separate tokens.

The lexer is all-or-nothing: on the first invalid byte it returns no tokens,
only the error.

## Stage 2: the parser

The parser is generated. `grammar.y` holds the syntax rules, and
[goyacc](https://pkg.go.dev/golang.org/x/tools/cmd/goyacc) builds an LALR(1)
parser from them into `parser_gen.go`, which is committed. The `go:generate`
directive in `parser.go` runs it at a pinned version. The rules in
`grammar.y` mirror the grammar in [query-language.md](query-language.md);
`grammar_doc_test.go` fails if the two drift apart.

`parser.go` is the hand-written glue around the generated code:

- `Parse` lexes the input, wraps the tokens in a `parseState`, and runs the
  generated parser (`yyNewParser().Parse`).
- `parseState` implements goyacc's `yyLexer` interface. Its `Lex` method feeds
  the lexer's tokens to the parser one at a time, translating each token kind
  to a parser code through the `yyCodes` table, and returns `0` at the end of
  input. It also tracks the positions of open `(` for error reporting.
- The grammar's named tokens are `t`-prefixed (`tIDENT`, `tAND`, and so on).
  goyacc turns each one into a constant, and the lowercase first letter keeps
  those constants unexported, so nothing from `parser_gen.go` joins the
  package API. `TestGeneratedCodeExportsNothing` checks this.
- Reductions in `grammar.y` build the tree directly. The finished tree is
  stored on `parseState`, and `Parse` returns it.

Each `Parse` call owns its own `parseState`, so `Parse` is safe for concurrent
use.

### Precedence and associativity

Two properties fall out of the shape of the rules, with no `%left` or
`%prec` declarations:

- **Both connectors are left-associative**, because the rules are left
  recursive: `or_expr tOR and_expr` and `and_expr tAND primary`. The parser
  folds each new operand onto the tree built so far, so `a OR b OR c` becomes
  `(a OR b) OR c`.
- **AND binds tighter than OR**, because `or_expr` is built from `and_expr`.
  An `AND` chain is reduced to a single `and_expr` before the parser can see
  the next `OR`.

### Example

For `a = 1 OR b = 2 AND c = 3`, the parser reduces `a = 1` to a predicate and
then to an `and_expr` and an `or_expr`. On seeing `OR`, it shifts and reads
`b = 2`; the next token is `AND`, so it keeps going and reduces
`b = 2 AND c = 3` to one `and_expr` before combining it with the `OR`. That
produces this tree:

```
OR @7
├── a = 1
└── AND @16
    ├── b = 2
    └── c = 3
```

Parentheses don't create a node of their own. The `'(' or_expr ')'` rule
returns its inner expression, so `(a = 1 OR b = 2) AND c = 3` is just an
`AND` whose left child is an `OR`.

### Predicates

The two `predicate` rules build a `predicateNode`:

- `tIDENT ordinary_op value` covers `=`, `!=`, `>`, `>=`, `<`, `<=`,
  `CONTAINS` and `EXCLUDES`. The `ordinary_op` rules translate each token to
  the public `PredicateOp` and keep its position. The `value` rules do the
  same for `tSTRING`, `tNUMBER` and `tBOOL`.
- `tIDENT tBETWEEN tSTRING` is a separate rule, so the grammar itself enforces
  that `BETWEEN` takes a string.

The resulting `predicateNode` keeps the field token (name, quoted flag,
position), the operator and its position, and a `Literal` with the decoded
text and position. Downstream compilers use these positions to report
schema errors.

### Nesting depth

Nesting does not recurse in Go. The generated parser keeps its state on a
value stack that starts small and grows as needed, so deeply nested groups
cost memory, not call stack.

## Error reporting

Every error names the position of the token where parsing stopped, or
`len(input)+1` if the input ended too early. goyacc reports a syntax error
through `parseState.Error`, which keeps only the first one and applies three
rules in order:

1. **End of input with an open `(`:** `unmatched '(' at position N`, naming
   the innermost open `(`. This wins over the generic message. For
   `a = 1 AND (` the user is told about the `(` at position 11 rather than
   "unexpected end of input, expected field or '(' at position 12", because
   the open group is the more likely mistake.
2. **A `)` with no open `(`:** `unmatched ')' at position N`. `Lex` sets
   `strayClose` when it hands the parser a `)` while no `(` is open, and this
   rule applies when that happens before the end of input.
3. **Anything else:** goyacc's verbose message, such as
   `syntax error: unexpected $end, expecting tIDENT or '('`, reworded by
   `describeSyntaxError` to `unexpected end of input, expected field or '('`
   and ending in ` at position N`.

The rewording uses the `tokenDisplay` map, which turns generated token names
into user-facing ones: `$end` becomes "end of input", `tIDENT` becomes
"field", and `tSTRING`, `tNUMBER` and `tBOOL` become "string", "number" and
"boolean". `tNEQ`, `tGTE` and `tLTE` print as `'!='`, `'>='` and `'<='`, and
the keyword tokens print as their keyword, such as `AND` and `OR`. Quoted
character tokens like `'('` are not in the map and print as they are. A list
of expected tokens reads "expected a, b, or c".

goyacc's default reductions can leave `tAND` out of an expected list that
contains `tOR`: after `(a = 1`, the parser reduces to an `or_expr` before it
checks the next token, so goyacc lists only what may follow an `or_expr`.
Every `or_expr` ends in an `and_expr`, so wherever `OR` may follow, `AND` may
too, and `describeSyntaxError` inserts `AND` before `OR`. For example,
`(a = 1 b = 2)` reports "unexpected field, expected AND, OR, or ')'".

`parseState.Lex` records the most recent token it returned and whether it
reached the end of input, which is what the rules above use to pick a
position and a message. `open` holds the positions of `(` handed to the
parser and not yet closed, innermost last. `strayClose` is true when the
token just returned is a `)` that arrived with no `(` open; rule 2 applies
only if the error comes before the end of input.

Sample results. This table is maintained by hand: it mirrors the first rows
of `TestParseErrorMessages` and must be updated together with them.

| Input                 | Error                                       |
| --------------------- | ------------------------------------------- |
| *(empty)*             | `unexpected end of input, expected field or '(' at position 1` |
| `a 1`                 | `unexpected number at position 3`           |
| `a =`                 | `unexpected end of input, expected string, number, or boolean at position 4` |
| `a BETWEEN 1`         | `unexpected number, expected string at position 11` |
| `(a = 1 OR)`          | `unexpected ')', expected field or '(' at position 10` |
| `a = 1 b = 2`         | `unexpected field at position 7`            |
| `a = 1)`              | `unmatched ')' at position 6`               |
| `a = 1 AND (`         | `unmatched '(' at position 11`              |
| `((a = 1) OR (b = 2`  | `unmatched '(' at position 13`              |
| `a = 'x`              | `unterminated quote at position 5` (lexer)  |
| `1abc = 1`            | `invalid number at position 2` (lexer)      |
| `a = 1 @`             | `unexpected character '@' at position 7` (lexer) |

## Extending the parser

After any change to `grammar.y`, run `go generate ./...` and commit the
updated `parser_gen.go`.

- **New predicate operator:** add a token kind and its scanning in
  `lexer.go` (in the `keywords` map if it is a word), a `PredicateOp` in
  `ast.go`, a `t`-prefixed `%token` and an `ordinary_op` alternative in
  `grammar.y`, a `yyCodes` entry in `parser.go`, and a `tokenDisplay` entry
  for the token. Then run `go generate`.
- **New literal kind:** add a token kind, its scanner, a `LiteralKind`, a
  `t`-prefixed `%token` and a `value` alternative in `grammar.y`, a `yyCodes`
  entry, and a `tokenDisplay` entry.
- **New rule** (for example a prefix `NOT`): edit `grammar.y`, update the
  `yacc` block in [query-language.md](query-language.md) (the test enforces
  that it matches), then run `go generate`.

Update the tests in `parser_test.go` / `lexer_test.go` alongside any grammar
change. `TestParseDepthAndIdentifiers` covers 2,000 nested groups.
