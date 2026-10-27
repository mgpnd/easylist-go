# Filter query language

`filter.Parse` parses a filter into an immutable public AST without a schema
or SQL dependencies. Inspect predicates through `filter.Predicate` and
connectors through `filter.Logical`; parentheses are represented by tree shape.
The parser only checks syntax. Whether a field exists, which operators it
allows, and whether a value suits its type are decided later, against a
schema; see [Evaluation and types](#evaluation-and-types).

Some filters:

```filter
name = 'Artem' AND age < 30
country = NULL OR country EXCLUDES 'fr'
(status = 'active' OR status = 'blocked') AND "Order" >= 10
age BETWEEN '30,40'
created_at >= START_OF_WEEK AND created_at <= END_OF_WEEK
due_date > {today + 5::days}
due < {TODAY + 5::BUSINESS_DAYS}
seen_at > {NOW - 90::MINUTES}
due = {TODAY + 2 * 7::DAYS}
t > {NOW + (1::DAYS + 2::HOURS)}
price > {100 * 3}
delta > {-1.50 * 2}
```

## Grammar

The grammar is written in EBNF (ISO/IEC 14977): `=` defines a rule, `,`
separates the items of a sequence, `|` separates alternatives, `[ … ]` is
optional, `{ … }` repeats zero or more times, `( … )` groups, `(* … *)` is a
comment, and `;` ends a rule. Rule names may contain spaces.

The syntactic grammar below works on the tokens that the lexer produces, not
on characters. A quoted terminal such as `"AND"`, `"TODAY"`, or `"{"` matches
a keyword, word, or symbol with that spelling, ignoring case; it never matches
a quoted identifier or a string. `identifier`, `string`, `number`, `integer`,
and `boolean` are tokens defined in the [lexical grammar](#tokens).
`grammar_doc_test.go` checks that this block and the parser accept and reject
the same filters.

```ebnf
filter              = or expression ;
or expression       = and expression, { "OR", and expression } ;
and expression      = primary, { "AND", primary } ;
primary             = predicate | "(", or expression, ")" ;
predicate           = identifier, comparison operator, comparison value
                    | identifier, set operator, plain value
                    | identifier, "BETWEEN", string
                    | identifier, ( "=" | "!=" ), "NULL" ;
comparison operator = "=" | "!=" | ">" | ">=" | "<" | "<=" ;
set operator        = "CONTAINS" | "EXCLUDES" ;
plain value         = string | number | boolean ;
comparison value    = plain value | time keyword | "{", sum, "}" ;

(* Brace expressions. *)
sum                 = term, { ( "+" | "-" ), term } ;
term                = factor, { ( "*" | "/" ), factor } ;
factor              = "-", factor | atom ;
atom                = number | duration | time keyword | "(", sum, ")" ;
duration            = integer, "::", time unit ;
time keyword        = "NOW" | "TODAY" | "START_OF_WEEK" | "END_OF_WEEK"
                    | "START_OF_MONTH" | "END_OF_MONTH"
                    | "START_OF_YEAR" | "END_OF_YEAR" ;
time unit           = "DAYS" | "BUSINESS_DAYS" | "HOURS" | "MINUTES" ;
```

`AND` binds more tightly than `OR`, and both group from left to right.
For example, `a = 1 OR b = 2 AND c = 3` means
`a = 1 OR (b = 2 AND c = 3)`. Parentheses override precedence.

## Tokens

The lexical grammar defines the tokens at the byte level. It is
documentation only; the lexer tests cover it.

```ebnf
(* Between tokens, the lexer skips ASCII spaces, tabs, carriage returns,
   line feeds, form feeds, and vertical tabs. *)
identifier        = word | quoted identifier ;
word              = word start, { word start | digit | "$" } ;
word start        = letter | "_" | ? any byte from 128 to 255 ? ;
quoted identifier = '"', { ? any byte except '"' and NUL ? | '""' }, '"' ;
string            = "'", { ? any byte except "'" and NUL ? | "''" }, "'" ;
number            = [ "+" | "-" ], integer, [ ".", integer ] ;
integer           = digit, { digit } ;
boolean           = "TRUE" | "FALSE" ;
letter            = ? an ASCII letter, A to Z or a to z ? ;
digit             = "0" | "1" | "2" | "3" | "4" | "5" | "6" | "7" | "8" | "9" ;
symbol            = "=" | "!=" | ">" | ">=" | "<" | "<=" | "(" | ")"
                  | "{" | "}" | "+" | "-" | "*" | "/" | "::" ;
```

- **Identifiers.** Unquoted names fold ASCII uppercase letters to lowercase;
  quoted names keep their spelling, and a doubled double quote stands for one
  double quote. Identifiers keep their full length; the parser applies no
  database identifier limit. `"AND" = 'a''b'` compares a column literally
  named `AND` with `a'b`.
- **Keywords.** Outside braces, a word spelled `AND`, `OR`, `CONTAINS`,
  `EXCLUDES`, `BETWEEN`, `TRUE`, `FALSE`, or `NULL`, in any case, is that
  keyword, not an identifier. These are reserved words, so a column named after
  one, such as `and`, `true`, or `null`, must be double-quoted: `"null" = 'x'`.
  The relative time keywords and units are *not* reserved: they are ordinary
  words that the grammar recognises only on the value side and inside braces.
- **Strings** use single quotes, with a doubled single quote for an embedded
  quote.
- **Numbers.** Outside braces, `+` and `-` are not tokens of their own: they
  start a signed number, so `age > -3` compares with minus three. Inside
  braces, `+` and `-` are always operators and numbers are unsigned, so
  `{TODAY+5::DAYS}` and `{TODAY + 5::DAYS}` mean the same. An `integer` in the
  syntactic grammar is a number token with no sign and no decimal point. A
  number must not run straight into a word or another `.`, so `1abc` and
  `1.2.3` are errors.
- **Braces.** `{` switches the lexer into brace mode until the next `}`.
  Inside braces every word is an identifier, even `AND` or `TRUE`, and a second
  `{` is an error: braces do not nest.
- **Symbols.** `!=`, `>=`, `<=`, and `::` are single tokens: `!` or `:` on its
  own, or separated from the next character by whitespace, is an error.

Error positions are 1-based byte offsets, including in UTF-8 input. Missing
input at the end is reported at `len(filter)+1`.

## Predicates

`=`, `!=`, `>`, `>=`, `<`, and `<=` take a string, a number, a Boolean, a
relative time keyword, or a brace expression. `!=` accepts the same values as
`=`, and the parser produces `OpNeq` for it. `CONTAINS` and `EXCLUDES` take a
string, a number, or a Boolean.

`NULL` is allowed only as the value of `=` and `!=`: `country = NULL` and
`country != NULL` test whether a field is NULL or not NULL. The predicate keeps
its `OpEq` or `OpNeq` operator, and its value is a `LiteralNull` literal whose
text is the keyword's source spelling and whose position is the keyword's.
Anywhere else `NULL` is a syntax error at its position, for example
`age > NULL`, `name CONTAINS NULL`, `age BETWEEN NULL`, or `NULL = 'x'`.
`'NULL'` in single quotes is an ordinary string.

`BETWEEN` takes one quoted string containing exactly one comma and two
nonempty trimmed bounds, for example `age BETWEEN '30,40'`. It does not
accept SQL-style `BETWEEN 30 AND 40`, and it does not accept relative values.
The conversion of `BETWEEN` bounds is a semantic check that a compiler such as
[easylist-sql-goqu](../../easylist-sql-goqu) performs against the selected
table's schema.

## Relative time values

The value of a comparison can be one of the relative time keywords, in any
case:

| Keyword          | Meaning                                    |
| ---------------- | ------------------------------------------ |
| `NOW`            | the reference time, a timestamp            |
| `TODAY`          | the date of the reference time             |
| `START_OF_WEEK`  | the first day of `TODAY`'s week            |
| `END_OF_WEEK`    | six days after `START_OF_WEEK`             |
| `START_OF_MONTH` | the first day of `TODAY`'s month           |
| `END_OF_MONTH`   | the last day of `TODAY`'s month            |
| `START_OF_YEAR`  | January 1 of `TODAY`'s year                |
| `END_OF_YEAR`    | December 31 of `TODAY`'s year              |

A keyword is recognised only on the value side, so `today = 'x' AND now > 3`
compares the fields `today` and `now`. Any other unquoted word on the value
side is an error naming it: `due = tomorrow` fails at position 7. A quoted
identifier on the value side is an error too.

Arithmetic needs braces: `{TODAY + 5::DAYS}`, `{NOW - 90::MINUTES}`,
`{TODAY + 2 * 7::DAYS}`, `{100 * 3}`. Inside braces an expression is built
from unsigned numbers, time keywords, durations, the binary operators `+`,
`-`, `*`, and `/`, unary `-`, and parentheses. `::` binds most tightly, then
unary `-`, then `*` and `/`, then `+` and `-`; binary operators of equal
precedence group from left to right. So `{TODAY + 2 * 7::DAYS}` adds
`2 * (7::DAYS)` to `TODAY`, and `{-5::DAYS * 2}` is `(-(5::DAYS)) * 2`.
Outside braces, arithmetic is an error at the first operator:
`due > TODAY + 5::DAYS` fails at the `+`.

A duration is an unsigned integer, `::`, and one of the units `DAYS`,
`BUSINESS_DAYS`, `HOURS`, or `MINUTES`, in any case. A decimal amount such as
`1.5::HOURS` is an error at the amount, and an unknown unit such as
`3::WEEKS` is an error at the unit. The parser does not limit the amount's
size. Durations of different units can be combined:
`{NOW + (1::DAYS + 2::HOURS)}`.

Relative values are allowed only with `=`, `!=`, `>`, `>=`, `<`, and `<=`.
`name CONTAINS TODAY`, `due BETWEEN TODAY`, and `name CONTAINS {1}` are errors
at the keyword or the `{`.

## The expression tree

A bare keyword or a brace expression becomes a literal of kind `LiteralExpr`.
Its text is the value's source spelling, braces included, its position is the
keyword's or the `{`'s, and `Literal.Expr()` returns the root of its tree.
`Literal.Expr()` is nil for every other kind. The nodes are:

| Interface      | Node                                    | `Pos()`              |
| -------------- | --------------------------------------- | -------------------- |
| `NumberNode`   | an unsigned number; `Text()` is its spelling | the number      |
| `KeywordNode`  | `Keyword()` is a `TimeKeyword`; `Text()` its spelling | the keyword |
| `DurationNode` | `Amount()` digits, `Unit()` a `TimeUnit`, `UnitPos()` | the amount |
| `BinaryNode`   | `Operator()` an `ArithOp`, `Left()`, `Right()` | the operator  |
| `NegationNode` | `Operand()`                             | the `-`              |

The tree mirrors the source. Parentheses only shape it, and nothing is
simplified, reordered, or evaluated: `{100 * 3}` stays a multiplication of
`100` and `3`. For `due = {TODAY + 2 * 7::DAYS}` the tree is:

```
+ @14
├── TODAY @8
└── * @18
    ├── 2 @16
    └── 7::DAYS @20 (unit @23)
```

Because the parser does not interpret the tree, expressions that are
syntactically valid but cannot be evaluated, such as `{NOW - TODAY}`,
`{1 / 0}`, or `{5::DAYS}`, parse successfully.

## Evaluation and types

Evaluating expressions and checking their types is not the parser's job. The
`easylist-core` planner does both, against the field's type and the request's
reference time, time zone, and first day of the week (Monday by default), and
hands the results to the driver. Planning errors also carry byte positions.
In summary:

- Every expression has a type: number, duration, date, or timestamp. Number
  literals are numbers, `NOW` is a timestamp, and the other keywords are
  dates. A whole expression must be a number or a time; a bare duration such
  as `{5::DAYS}` is an error.
- Number arithmetic is exact. Division by zero, or a quotient with no finite
  decimal form such as `{1 / 3}`, is an error at the `/`.
- A duration is an ordered list of unit amounts. Durations can be added and
  subtracted, negated, multiplied by an integer, and divided by an integer
  when every amount divides exactly. Each amount is at most 10,000,000.
- A date or timestamp plus or minus a duration, or a duration plus a date or
  timestamp, is a time. It stays a date only if every amount is in `DAYS` or
  `BUSINESS_DAYS`; otherwise it is a timestamp, so `{TODAY + 9::HOURS}` is a
  timestamp. Every other operation on times or durations, such as
  `{NOW - TODAY}`, is an error at the operator.
- Durations are applied left to right as written. `DAYS` moves by calendar
  days in the request's time zone, keeping the wall-clock time; `HOURS` and
  `MINUTES` move by elapsed time. `BUSINESS_DAYS` counts Monday to Friday,
  with no holidays, and `0::BUSINESS_DAYS` moves a Saturday or Sunday to the
  next Monday. Results must stay within the years 1 to 9999.
- A number result suits integer, float, and numeric fields; a date result
  suits date and timestamp fields; a timestamp result suits timestamp fields
  only. Array fields reject expression values.
- A date compared with a timestamp field covers the whole day in the request's
  time zone, so `created_at <= END_OF_WEEK` includes all of the last day.

## The AST

The AST retains decoded strings, exact numeric spelling, literal kinds,
expression trees, and source byte positions, so a compiler can report errors
at the original literal. The AST is immutable and can be reused concurrently.
