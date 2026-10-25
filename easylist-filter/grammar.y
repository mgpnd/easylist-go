%{
package filter
%}

%union {
	tok  token
	expr Expr
	op   opAt
	lit  Literal
}

%token <tok> tIDENT tSTRING tNUMBER tBOOL tNEQ tGTE tLTE tCONTAINS tEXCLUDES tBETWEEN tAND tOR '=' '>' '<' '(' ')'

%type <expr> or_expr and_expr primary predicate
%type <op> ordinary_op
%type <lit> value

%%

filter
	: or_expr { yylex.(*parseState).result = $1 }
	;

or_expr
	: and_expr
	| or_expr tOR and_expr { $$ = &logicalNode{operator: Or, operatorPos: $2.pos, left: $1, right: $3} }
	;

and_expr
	: primary
	| and_expr tAND primary { $$ = &logicalNode{operator: And, operatorPos: $2.pos, left: $1, right: $3} }
	;

primary
	: predicate
	| '(' or_expr ')' { $$ = $2 }
	;

predicate
	: tIDENT ordinary_op value { $$ = &predicateNode{field: $1, operator: $2.op, operatorPos: $2.pos, value: $3} }
	| tIDENT tBETWEEN tSTRING  { $$ = &predicateNode{field: $1, operator: OpBetween, operatorPos: $2.pos, value: Literal{kind: LiteralString, text: $3.text, pos: $3.pos}} }
	;

ordinary_op
	: '='       { $$ = opAt{OpEq, $1.pos} }
	| tNEQ      { $$ = opAt{OpNeq, $1.pos} }
	| '>'       { $$ = opAt{OpGt, $1.pos} }
	| tGTE      { $$ = opAt{OpGte, $1.pos} }
	| '<'       { $$ = opAt{OpLt, $1.pos} }
	| tLTE      { $$ = opAt{OpLte, $1.pos} }
	| tCONTAINS { $$ = opAt{OpContains, $1.pos} }
	| tEXCLUDES { $$ = opAt{OpExcludes, $1.pos} }
	;

value
	: tSTRING { $$ = Literal{kind: LiteralString, text: $1.text, pos: $1.pos} }
	| tNUMBER { $$ = Literal{kind: LiteralNumber, text: $1.text, pos: $1.pos} }
	| tBOOL   { $$ = Literal{kind: LiteralBool, text: $1.text, pos: $1.pos} }
	;

%%
