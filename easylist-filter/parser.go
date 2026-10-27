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
	s := &parseState{tokens: tokens, eofPos: len(input) + 1}
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
	tokNull: tNULL,
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
		s.err = fmt.Errorf("%s at position %d", describeSyntaxError(msg), pos)
	}
}

// tokenDisplay maps generated token names to user-facing names. Names not
// listed, such as quoted char tokens like '(', print as they are.
var tokenDisplay = map[string]string{
	"$end": "end of input", "tIDENT": "field", "tSTRING": "string", "tNUMBER": "number",
	"tBOOL": "boolean", "tNEQ": "'!='", "tGTE": "'>='", "tLTE": "'<='",
	"tAND": "AND", "tOR": "OR", "tBETWEEN": "BETWEEN",
	"tCONTAINS": "CONTAINS", "tEXCLUDES": "EXCLUDES", "tNULL": "NULL",
}

// describeSyntaxError rewrites goyacc's verbose message, such as
// "syntax error: unexpected $end, expecting tIDENT or '('", into
// "unexpected end of input, expected field or '('".
func describeSyntaxError(msg string) string {
	msg = strings.TrimPrefix(msg, "syntax error: ")
	msg = strings.TrimPrefix(msg, "unexpected ")
	got, want, _ := strings.Cut(msg, ", expecting ")
	out := "unexpected " + displayToken(got)
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
	for i, n := range names {
		names[i] = displayToken(n)
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

func displayToken(name string) string {
	if d, ok := tokenDisplay[name]; ok {
		return d
	}
	return name
}
