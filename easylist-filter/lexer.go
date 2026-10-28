package filter

import (
	"fmt"
	"strings"
)

type tokenKind uint8

const (
	tokIdent tokenKind = iota
	tokString
	tokNumber
	tokBool
	tokEq
	tokGt
	tokGte
	tokLt
	tokLte
	tokContains
	tokExcludes
	tokBetween
	tokAnd
	tokOr
	tokLParen
	tokRParen
	tokNeq
	tokNull
	tokLBrace // '{', which starts brace mode
	tokRBrace // '}', which ends brace mode
	tokPlus   // '+', inside braces only; outside it starts a number
	tokMinus  // '-', inside braces only; outside it starts a number
	tokStar   // '*'
	tokSlash  // '/'
	tokDColon // '::'
	tokComma  // ','
	tokIn
	tokNot

	numTokenKinds
)

type token struct {
	kind   tokenKind
	text   string
	pos    int // 1-based byte offset in the filter source.
	quoted bool
}

var keywords = map[string]tokenKind{
	"TRUE": tokBool, "FALSE": tokBool,
	"CONTAINS": tokContains, "EXCLUDES": tokExcludes, "BETWEEN": tokBetween,
	"AND": tokAnd, "OR": tokOr, "NULL": tokNull, "IN": tokIn, "NOT": tokNot,
}

// lexer scans filter syntax without interpreting predicates or value types.
// It works on bytes, so positions are byte offsets even for UTF-8 input.
//
// A '{' switches it into brace mode until the next '}'. In brace mode,
// + and - are operator tokens, numbers are unsigned, and every word is an
// identifier, never a keyword such as AND. Outside braces, + and - start a
// signed number. '*', '/', '::', and ',' are tokens in both modes, so that the
// parser, not the lexer, rejects them where they do not belong at their own
// position.
type lexer struct {
	src   string
	i     int  // next byte to read
	brace bool // inside { }
}

// lexFilter returns all tokens, or no tokens if any byte is invalid.
func lexFilter(src string) ([]token, error) {
	l := &lexer{src: src}
	var tokens []token
	for {
		l.skipWhile(isFilterSpace)
		if l.done() {
			return tokens, nil
		}
		tok, err := l.scanToken()
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, tok)
	}
}

func (l *lexer) scanToken() (token, error) {
	start := l.i
	ch := l.src[start]
	switch {
	case ch == '\'' || ch == '"':
		return l.scanQuoted()
	case isFilterIdentStart(ch):
		return l.scanIdentifier(), nil
	case isFilterDigit(ch) || !l.brace && (ch == '+' || ch == '-'):
		return l.scanNumber()
	}

	l.i++
	var kind tokenKind
	switch ch {
	case '{':
		if l.brace {
			return token{}, fmt.Errorf("nested '{' at position %d", start+1)
		}
		l.brace = true
		kind = tokLBrace
	case '}':
		l.brace = false
		kind = tokRBrace
	case '+': // only reached in brace mode
		kind = tokPlus
	case '-': // only reached in brace mode
		kind = tokMinus
	case '*':
		kind = tokStar
	case '/':
		kind = tokSlash
	case ':':
		if !l.accept(':') {
			return token{}, fmt.Errorf("expected ':' after ':' at position %d", start+1)
		}
		kind = tokDColon
	case ',':
		kind = tokComma
	case '=':
		kind = tokEq
	case '(':
		kind = tokLParen
	case ')':
		kind = tokRParen
	case '>':
		kind = tokGt
		if l.accept('=') {
			kind = tokGte
		}
	case '<':
		kind = tokLt
		if l.accept('=') {
			kind = tokLte
		}
	case '!':
		if !l.accept('=') {
			return token{}, fmt.Errorf("expected '=' after '!' at position %d", start+1)
		}
		kind = tokNeq
	default:
		return token{}, fmt.Errorf("unexpected character %q at position %d", ch, start+1)
	}
	return token{kind: kind, text: l.src[start:l.i], pos: start + 1}, nil
}

// scanQuoted reads a single-quoted value or a double-quoted identifier.
// Repeating the delimiter inside the token decodes to one literal quote.
func (l *lexer) scanQuoted() (token, error) {
	start := l.i
	quote := l.src[start]
	l.i++

	var text strings.Builder
	for !l.done() {
		ch := l.src[l.i]
		l.i++
		switch {
		case ch == quote && !l.accept(quote):
			tok := token{kind: tokString, text: text.String(), pos: start + 1}
			if quote == '"' {
				tok.kind, tok.quoted = tokIdent, true
			}
			return tok, nil
		case ch == 0:
			return token{}, fmt.Errorf("unexpected NUL at position %d", l.i)
		}
		text.WriteByte(ch)
	}
	return token{}, fmt.Errorf("unterminated quote at position %d", start+1)
}

// scanIdentifier reads a keyword or an unquoted identifier. Keywords keep
// their source spelling; identifiers are folded like PostgreSQL does. Folding
// keeps the length, so the source spelling is src[pos-1:pos-1+len(text)].
// In brace mode every word is an identifier.
func (l *lexer) scanIdentifier() token {
	start := l.i
	l.skipWhile(isFilterIdentByte)
	word := l.src[start:l.i]
	if kind, ok := keywords[strings.ToUpper(word)]; ok && !l.brace {
		return token{kind: kind, text: word, pos: start + 1}
	}
	return token{kind: tokIdent, text: foldASCIILower(word), pos: start + 1}
}

// scanNumber reads an integer or decimal, optionally signed outside braces. A
// name byte or dot right after it is an error, so "1abc" or "1.2.3" cannot
// split into tokens.
func (l *lexer) scanNumber() (token, error) {
	start := l.i
	if !l.brace && !l.accept('+') {
		l.accept('-')
	}
	if !l.skipWhile(isFilterDigit) {
		return token{}, fmt.Errorf("invalid number at position %d", start+1)
	}
	if l.accept('.') && !l.skipWhile(isFilterDigit) {
		return token{}, fmt.Errorf("invalid decimal at position %d", l.i+1)
	}
	if l.peekIs(isFilterIdentByte) || l.peekIs(func(ch byte) bool { return ch == '.' }) {
		return token{}, fmt.Errorf("invalid number at position %d", l.i+1)
	}
	return token{kind: tokNumber, text: l.src[start:l.i], pos: start + 1}, nil
}

func (l *lexer) done() bool { return l.i >= len(l.src) }

func (l *lexer) peekIs(pred func(byte) bool) bool { return !l.done() && pred(l.src[l.i]) }

// accept consumes ch if it is the next byte.
func (l *lexer) accept(ch byte) bool {
	if l.done() || l.src[l.i] != ch {
		return false
	}
	l.i++
	return true
}

// skipWhile consumes bytes matching pred and reports whether it consumed any.
func (l *lexer) skipWhile(pred func(byte) bool) bool {
	start := l.i
	for l.peekIs(pred) {
		l.i++
	}
	return l.i > start
}

// foldASCIILower lowercases ASCII letters only. Unlike strings.ToLower, it
// leaves non-ASCII bytes untouched, even when they are not valid UTF-8.
func foldASCIILower(s string) string {
	b := []byte(s)
	for i, ch := range b {
		if 'A' <= ch && ch <= 'Z' {
			b[i] = ch + 'a' - 'A'
		}
	}
	return string(b)
}

func isFilterSpace(ch byte) bool { return strings.IndexByte(" \t\r\n\f\v", ch) >= 0 }

func isFilterIdentStart(ch byte) bool {
	return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch == '_' || ch >= 128
}

func isFilterIdentByte(ch byte) bool { return isFilterIdentStart(ch) || isFilterDigit(ch) || ch == '$' }

func isFilterDigit(ch byte) bool { return ch >= '0' && ch <= '9' }
