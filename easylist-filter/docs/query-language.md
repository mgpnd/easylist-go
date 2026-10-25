# Filter query language

`filter.Parse` parses this grammar into an immutable public AST without a schema
or SQL dependencies. Inspect predicates through `filter.Predicate` and connectors
through `filter.Logical`; parentheses are represented by tree shape. The grammar
operates on lexer tokens. ASCII whitespace between tokens is ignored.
The grammar block is the rules section of `grammar.y` with its Go actions
removed, and a test keeps them identical. `|` separates alternatives and `;`
ends a rule. Names starting with `t` are lexer tokens: `tIDENT`, `tSTRING`,
`tNUMBER`, and `tBOOL` are the IDENT, STRING, NUMBER, and BOOL tokens described
below; `tNEQ`, `tGTE`, and `tLTE` are `!=`, `>=`, and `<=`; the rest are the
keywords. They are unexported so they stay out of the package API. The parser
is generated from `grammar.y`; see [parser.md](parser.md).

```yacc
filter      : or_expr ;
or_expr     : and_expr | or_expr tOR and_expr ;
and_expr    : primary | and_expr tAND primary ;
primary     : predicate | '(' or_expr ')' ;
predicate   : tIDENT ordinary_op value | tIDENT tBETWEEN tSTRING ;
ordinary_op : '=' | tNEQ | '>' | tGTE | '<' | tLTE | tCONTAINS | tEXCLUDES ;
value       : tSTRING | tNUMBER | tBOOL ;
```

`IDENT` is an unquoted identifier or a double-quoted identifier. Unquoted names
fold ASCII uppercase letters to lowercase; quoted names retain their spelling,
and doubled double quotes represent a literal double quote. Identifiers keep
their full length; the parser applies no database identifier limit. `STRING` uses
single quotes and doubled single quotes for an embedded quote. `NUMBER` has
an optional sign, one or more ASCII digits, and an optional decimal point
followed by digits. `BOOL` is `TRUE` or `FALSE`. Unquoted keywords and
Boolean tokens are case-insensitive. The lexer ignores ASCII spaces, tabs,
carriage returns, line feeds, form feeds, and vertical tabs between tokens.
`!=` is a single token: `!` on its own, or separated from `=` by
whitespace, is an error. Error positions are 1-based byte offsets, including
in UTF-8 input. Missing input at the end is reported at `len(filter)+1`.

`AND` binds more tightly than `OR`, and both group from left to right.
For example, `a = 1 OR b = 2 AND c = 3` means
`a = 1 OR (b = 2 AND c = 3)`. Parentheses override precedence.
`"AND" = 'a''b'` compares a column literally named `AND` with `a'b`.

`!=` accepts the same values as `=`. The parser produces `OpNeq` for it.

`BETWEEN` takes one quoted string containing exactly one comma and two
nonempty trimmed bounds, for example `age BETWEEN '30,40'`. It does not
accept SQL-style `BETWEEN 30 AND 40`. Column existence, allowed operators,
value types, and the conversion of `BETWEEN` bounds are semantic checks
that a compiler such as
[easylist-sql-goqu](../../easylist-sql-goqu) performs
against the selected table's schema.

The AST retains decoded strings, exact numeric spelling, literal kinds, and
source byte positions, so a compiler can report errors at the original literal.
Parsing checks only that `BETWEEN` receives a string. The AST is immutable and
can be reused concurrently.
