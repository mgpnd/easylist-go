package filter

import (
	"fmt"
	"slices"
	"strings"
)

//go:generate go run golang.org/x/tools/cmd/goyacc@v0.49.0 -o parser_gen.go -p yy -v "" grammar.y

func init() { yyErrorVerbose = true }

// Parse validates the complete filter syntax and returns an immutable tree.
// On any lexical or syntax error it returns a nil expression and an error with
// a 1-based byte position. It does not validate fields or values against a schema.
func Parse(input string) (Expr, error) {
	tokens, err := lexFilter(input)
	if err != nil {
		return nil, err
	}
	s := &parseState{src: input, tokens: tokens, eofPos: len(input) + 1}
	yyNewParser().Parse(s)
	if s.err != nil {
		return nil, s.err
	}
	return s.result, nil
}

// opAt is a predicate operator with its source position.
type opAt struct {
	op  PredicateOp
	pos int
}

// parseState feeds lexed tokens to the generated parser and records its result.
// Each Parse call owns one, so Parse is safe for concurrent use.
type parseState struct {
	src        string
	tokens     []token
	next       int
	last       token // most recent lookahead, for error positions
	atEOF      bool
	strayClose bool  // last was a ')' with no '(' open
	eofPos     int   // len(input)+1
	open       []int // positions of '(' handed to the parser and not yet closed, innermost last
	result     Expr
	err        error // first error only
}

// yyCodes maps lexer token kinds to the generated parser's token codes.
var yyCodes = [...]int{
	tokIdent: tIDENT, tokString: tSTRING, tokNumber: tNUMBER, tokBool: tBOOL,
	tokEq: '=', tokGt: '>', tokGte: tGTE, tokLt: '<', tokLte: tLTE,
	tokContains: tCONTAINS, tokExcludes: tEXCLUDES, tokBetween: tBETWEEN,
	tokAnd: tAND, tokOr: tOR, tokLParen: '(', tokRParen: ')', tokNeq: tNEQ,
	tokNull: tNULL, tokLBrace: '{', tokRBrace: '}', tokPlus: '+', tokMinus: '-',
	tokStar: '*', tokSlash: '/', tokDColon: tDCOLON, tokComma: ',', tokIn: tIN, tokNot: tNOT,
}

// Lex implements yyLexer.
func (s *parseState) Lex(lval *yySymType) int {
	if s.next == len(s.tokens) {
		s.atEOF = true
		return 0
	}
	tok := s.tokens[s.next]
	s.next++
	s.last = tok
	lval.tok = tok
	s.strayClose = false
	switch tok.kind {
	case tokLParen:
		s.open = append(s.open, tok.pos)
	case tokRParen:
		if len(s.open) > 0 {
			s.open = s.open[:len(s.open)-1]
		} else {
			s.strayClose = true
		}
	}
	return yyCodes[tok.kind]
}

// fail records an error found by a grammar action. Parsing goes on, but only
// the first error is kept, and Parse returns it.
func (s *parseState) fail(format string, args ...any) {
	if s.err == nil {
		s.err = fmt.Errorf(format, args...)
	}
}

// spelling returns an unquoted identifier's source spelling. Folding keeps
// the length, so it is the same span of the input as the folded text.
func (s *parseState) spelling(tok token) string {
	return s.src[tok.pos-1 : tok.pos-1+len(tok.text)]
}

// list builds a list literal from its parentheses and items. Its text is the
// source span from '(' to ')'.
func (s *parseState) list(lparen token, items []Literal, rparen token) Literal {
	return Literal{kind: LiteralList, text: s.src[lparen.pos-1 : rparen.pos], pos: lparen.pos, items: items}
}

// keyword builds the node for an identifier on the value side or inside
// braces, which must be an unquoted relative time keyword.
func (s *parseState) keyword(tok token) ExprNode {
	if tok.quoted {
		s.fail("unexpected quoted identifier at position %d", tok.pos)
		return nil
	}
	for k, name := range timeKeywordNames {
		if tok.text == foldASCIILower(name) {
			return &keywordNode{keyword: TimeKeyword(k), text: s.spelling(tok), pos: tok.pos}
		}
	}
	s.fail("unknown time keyword '%s' at position %d", s.spelling(tok), tok.pos)
	return nil
}

// duration builds the node for amount::unit. The amount must be an integer;
// its size is not limited here.
func (s *parseState) duration(amount, unit token) ExprNode {
	if strings.Contains(amount.text, ".") {
		s.fail("duration amount %s is not a whole number at position %d", amount.text, amount.pos)
		return nil
	}
	if unit.quoted {
		s.fail("unexpected quoted identifier at position %d", unit.pos)
		return nil
	}
	for u, name := range timeUnitNames {
		if unit.text == foldASCIILower(name) {
			return &durationNode{amount: amount.text, pos: amount.pos, unit: TimeUnit(u), unitPos: unit.pos}
		}
	}
	s.fail("unknown duration unit '%s', expected DAYS, BUSINESS_DAYS, HOURS, or MINUTES at position %d", s.spelling(unit), unit.pos)
	return nil
}

// Error implements yyLexer. Only the first error is kept. An unclosed group is
// the more useful diagnosis at end of input, and a stray ')' is reported as such.
func (s *parseState) Error(msg string) {
	if s.err != nil {
		return
	}
	switch {
	case s.atEOF && len(s.open) > 0:
		s.err = fmt.Errorf("unmatched '(' at position %d", s.open[len(s.open)-1])
	case !s.atEOF && s.strayClose:
		s.err = fmt.Errorf("unmatched ')' at position %d", s.last.pos)
	default:
		pos := s.last.pos
		if s.atEOF {
			pos = s.eofPos
		}
		s.err = fmt.Errorf("%s at position %d", describeSyntaxError(msg, s.errorContext()), pos)
	}
}

// syntaxContext says where the parser stopped, for wording its message.
type syntaxContext struct {
	ident    string // what to call an identifier token
	afterCmp string // the comparison operator just before, or ""
}

// errorContext looks at the tokens before the lookahead. Inside braces an
// identifier is a time keyword, or a unit right after '::'; right after a
// predicate operator or where a list item belongs it is an identifier;
// elsewhere it is a field.
func (s *parseState) errorContext() syntaxContext {
	look := s.next - 1 // index of the lookahead
	if s.atEOF {
		look = len(s.tokens)
	}
	ctx := syntaxContext{ident: "field"}
	if look == 0 {
		return ctx
	}
	inBrace := false
	for _, tok := range s.tokens[:look] {
		switch tok.kind {
		case tokLBrace:
			inBrace = true
		case tokRBrace:
			inBrace = false
		}
	}
	switch prev := s.tokens[look-1]; {
	case inBrace && prev.kind == tokDColon:
		ctx.ident = "unit"
	case inBrace:
		ctx.ident = "time keyword"
	case prev.kind == tokEq || prev.kind == tokNeq || prev.kind == tokGt ||
		prev.kind == tokGte || prev.kind == tokLt || prev.kind == tokLte:
		ctx.ident = "identifier"
		ctx.afterCmp = prev.text
	case isValueOperator(prev.kind) || prev.kind == tokNot || prev.kind == tokComma ||
		prev.kind == tokLParen && look >= 2 && isValueOperator(s.tokens[look-2].kind):
		ctx.ident = "identifier"
	}
	return ctx
}

// isValueOperator reports whether kind is an operator that a value list may
// follow.
func isValueOperator(kind tokenKind) bool {
	return kind == tokContains || kind == tokExcludes || kind == tokBetween || kind == tokIn
}

// tokenDisplay maps generated token names to user-facing names. Names not
// listed, such as quoted char tokens like '(', print as they are. tIDENT is
// named by syntaxContext.
var tokenDisplay = map[string]string{
	"$end": "end of input", "tSTRING": "string", "tNUMBER": "number",
	"tBOOL": "boolean", "tNEQ": "'!='", "tGTE": "'>='", "tLTE": "'<='",
	"tAND": "AND", "tOR": "OR", "tBETWEEN": "BETWEEN",
	"tCONTAINS": "CONTAINS", "tEXCLUDES": "EXCLUDES", "tNULL": "NULL", "tDCOLON": "'::'",
	"tIN": "IN", "tNOT": "NOT",
}

// describeSyntaxError rewrites goyacc's verbose message, such as
// "syntax error: unexpected $end, expecting tIDENT or '('", into
// "unexpected end of input, expected field or '('".
func describeSyntaxError(msg string, ctx syntaxContext) string {
	msg = strings.TrimPrefix(msg, "syntax error: ")
	msg = strings.TrimPrefix(msg, "unexpected ")
	got, want, _ := strings.Cut(msg, ", expecting ")
	out := "unexpected " + displayToken(got, ctx)
	// goyacc lists at most four expected tokens. A comparison operator can be
	// followed by more, so the list is written out here.
	if want == "" && ctx.afterCmp != "" {
		if ctx.afterCmp == "=" || ctx.afterCmp == "!=" {
			return out + ", expected string, number, boolean, NULL, time keyword, or '{'"
		}
		return out + ", expected string, number, boolean, time keyword, or '{'"
	}
	if want == "" {
		return out
	}
	names := strings.Split(want, " or ")
	// The LALR default reduction to or_expr makes goyacc list only the tokens
	// that can follow an or_expr, which omits AND. Every or_expr ends in an
	// and_expr, so wherever tOR may follow, tAND may too.
	if i := slices.Index(names, "tOR"); i >= 0 && !slices.Contains(names, "tAND") {
		names = slices.Insert(names, i, "tAND")
	}
	// The same holds inside braces: every sum ends in a term, so wherever '+'
	// may follow, '*' and '/' may too.
	if i := slices.Index(names, "'-'"); i >= 0 && slices.Contains(names, "'+'") && !slices.Contains(names, "'*'") {
		names = slices.Insert(names, i+1, "'*'", "'/'")
	}
	for i, n := range names {
		names[i] = displayToken(n, ctx)
	}
	switch len(names) {
	case 1:
		return out + ", expected " + names[0]
	case 2:
		return out + ", expected " + names[0] + " or " + names[1]
	}
	last := len(names) - 1
	return out + ", expected " + strings.Join(names[:last], ", ") + ", or " + names[last]
}

func displayToken(name string, ctx syntaxContext) string {
	if name == "tIDENT" {
		return ctx.ident
	}
	if d, ok := tokenDisplay[name]; ok {
		return d
	}
	return name
}
