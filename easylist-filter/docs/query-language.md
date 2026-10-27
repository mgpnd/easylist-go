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
predicate   : tIDENT ordinary_op value | tIDENT tBETWEEN tSTRING
            | tIDENT '=' tNULL | tIDENT tNEQ tNULL ;
ordinary_op : '=' | tNEQ | '>' | tGTE | '<' | tLTE | tCONTAINS | tEXCLUDES ;
value       : tSTRING | tNUMBER | tBOOL ;
```

`IDENT` is an unquoted identifier or a double-quoted identifier. Unquoted names
fold ASCII uppercase letters to lowercase; quoted names retain their spelling,
and doubled double quotes represent a literal double quote. Identifiers keep
their full length; the parser applies no database identifier limit. `STRING` uses
single quotes and doubled single quotes for an embedded quote. `NUMBER` has
an optional sign, one or more ASCII digits, and an optional decimal point
followed by digits. `BOOL` is `TRUE` or `FALSE`, and `tNULL` is the `NULL`
keyword. Unquoted keywords, Boolean tokens, and `NULL` are case-insensitive. The lexer ignores ASCII spaces, tabs,
carriage returns, line feeds, form feeds, and vertical tabs between tokens.
`!=` is a single token: `!` on its own, or separated from `=` by
whitespace, is an error. Error positions are 1-based byte offsets, including
in UTF-8 input. Missing input at the end is reported at `len(filter)+1`.

`AND` binds more tightly than `OR`, and both group from left to right.
For example, `a = 1 OR b = 2 AND c = 3` means
`a = 1 OR (b = 2 AND c = 3)`. Parentheses override precedence.
`"AND" = 'a''b'` compares a column literally named `AND` with `a'b`.
Keywords are reserved words, so a column named after one, such as `and`,
`true`, or `null`, must be double-quoted: `"null" = 'x'`.

`!=` accepts the same values as `=`. The parser produces `OpNeq` for it.

`NULL` is allowed only as the value of `=` and `!=`: `country = NULL` and
`country != NULL` test whether a field is NULL or not NULL. The predicate keeps
its `OpEq` or `OpNeq` operator, and its value is a `LiteralNull` literal whose
text is the keyword's source spelling and whose position is the keyword's.
Anywhere else `NULL` is a syntax error at its position, for example
`age > NULL`, `name CONTAINS NULL`, `age BETWEEN NULL`, or `NULL = 'x'`.
`'NULL'` in single quotes is an ordinary string.

`BETWEEN` takes one quoted string containing exactly one comma and two
nonempty trimmed bounds, for example `age BETWEEN '30,40'`. It does not
accept SQL-style `BETWEEN 30 AND 40`. Column existence, allowed operators,
value types, and the conversion of `BETWEEN` bounds are semantic checks
that a compiler such as
[easylist-sql-goqu](../../easylist-sql-goqu) performs
against the selected table's schema.

The AST retains decoded strings, exact numeric spelling, literal kinds, and
source byte positions, so a compiler can report errors at the original literal.
Parsing checks only that `BETWEEN` receives a string and that `NULL`
follows `=` or `!=`. The AST is immutable and
can be reused concurrently.
