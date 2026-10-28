%{
package filter
%}

%union {
	tok  token
	expr Expr
	op   opAt
	lit  Literal
	lits []Literal
	node ExprNode
}

%token <tok> tIDENT tSTRING tNUMBER tBOOL tNEQ tGTE tLTE tCONTAINS tEXCLUDES tBETWEEN tAND tOR tNULL tDCOLON tIN tNOT '=' '>' '<' '(' ')' '{' '}' '+' '-' '*' '/' ','

%type <expr> or_expr and_expr primary predicate
%type <op> comparison_op set_op
%type <lit> plain_value comparison_value in_item in_list value_list range
%type <lits> in_items plain_items
%type <node> sum term factor atom

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
	: tIDENT comparison_op comparison_value { $$ = &predicateNode{field: $1, operator: $2.op, operatorPos: $2.pos, value: $3} }
	| tIDENT set_op plain_value             { $$ = &predicateNode{field: $1, operator: $2.op, operatorPos: $2.pos, value: $3} }
	| tIDENT tBETWEEN tSTRING               { $$ = &predicateNode{field: $1, operator: OpBetween, operatorPos: $2.pos, value: Literal{kind: LiteralString, text: $3.text, pos: $3.pos}} }
	| tIDENT '=' tNULL                      { $$ = &predicateNode{field: $1, operator: OpEq, operatorPos: $2.pos, value: Literal{kind: LiteralNull, text: $3.text, pos: $3.pos}} }
	| tIDENT tNEQ tNULL                     { $$ = &predicateNode{field: $1, operator: OpNeq, operatorPos: $2.pos, value: Literal{kind: LiteralNull, text: $3.text, pos: $3.pos}} }
	| tIDENT tIN in_list                    { $$ = &predicateNode{field: $1, operator: OpIn, operatorPos: $2.pos, value: $3} }
	| tIDENT tNOT tIN in_list               { $$ = &predicateNode{field: $1, operator: OpNotIn, operatorPos: $2.pos, value: $4} }
	| tIDENT set_op value_list              { $$ = &predicateNode{field: $1, operator: $2.op, operatorPos: $2.pos, value: $3} }
	| tIDENT tBETWEEN range                 { $$ = &predicateNode{field: $1, operator: OpBetween, operatorPos: $2.pos, value: $3} }
	;

comparison_op
	: '='  { $$ = opAt{OpEq, $1.pos} }
	| tNEQ { $$ = opAt{OpNeq, $1.pos} }
	| '>'  { $$ = opAt{OpGt, $1.pos} }
	| tGTE { $$ = opAt{OpGte, $1.pos} }
	| '<'  { $$ = opAt{OpLt, $1.pos} }
	| tLTE { $$ = opAt{OpLte, $1.pos} }
	;

set_op
	: tCONTAINS { $$ = opAt{OpContains, $1.pos} }
	| tEXCLUDES { $$ = opAt{OpExcludes, $1.pos} }
	;

plain_value
	: tSTRING { $$ = Literal{kind: LiteralString, text: $1.text, pos: $1.pos} }
	| tNUMBER { $$ = Literal{kind: LiteralNumber, text: $1.text, pos: $1.pos} }
	| tBOOL   { $$ = Literal{kind: LiteralBool, text: $1.text, pos: $1.pos} }
	;

in_list
	: '(' in_items ')' { $$ = yylex.(*parseState).list($1, $2, $3) }
	;

in_items
	: in_item              { $$ = []Literal{$1} }
	| in_items ',' in_item { $$ = append($1, $3) }
	;

in_item
	: plain_value
	| tNULL { $$ = Literal{kind: LiteralNull, text: $1.text, pos: $1.pos} }
	;

value_list
	: '(' plain_items ')' { $$ = yylex.(*parseState).list($1, $2, $3) }
	;

plain_items
	: plain_value                 { $$ = []Literal{$1} }
	| plain_items ',' plain_value { $$ = append($1, $3) }
	;

range
	: '(' plain_value ',' plain_value ')' { $$ = yylex.(*parseState).list($1, []Literal{$2, $4}, $5) }
	;

comparison_value
	: plain_value
	| tIDENT      { s := yylex.(*parseState); $$ = Literal{kind: LiteralExpr, text: s.spelling($1), pos: $1.pos, expr: s.keyword($1)} }
	| '{' sum '}' { s := yylex.(*parseState); $$ = Literal{kind: LiteralExpr, text: s.src[$1.pos-1 : $3.pos], pos: $1.pos, expr: $2} }
	;

sum
	: term
	| sum '+' term { $$ = &binaryNode{operator: ArithAdd, pos: $2.pos, left: $1, right: $3} }
	| sum '-' term { $$ = &binaryNode{operator: ArithSub, pos: $2.pos, left: $1, right: $3} }
	;

term
	: factor
	| term '*' factor { $$ = &binaryNode{operator: ArithMul, pos: $2.pos, left: $1, right: $3} }
	| term '/' factor { $$ = &binaryNode{operator: ArithDiv, pos: $2.pos, left: $1, right: $3} }
	;

factor
	: '-' factor { $$ = &negationNode{pos: $1.pos, operand: $2} }
	| atom
	;

atom
	: tNUMBER                { $$ = &numberNode{text: $1.text, pos: $1.pos} }
	| tNUMBER tDCOLON tIDENT { $$ = yylex.(*parseState).duration($1, $3) }
	| tIDENT                 { $$ = yylex.(*parseState).keyword($1) }
	| '(' sum ')'            { $$ = $2 }
	;

%%
