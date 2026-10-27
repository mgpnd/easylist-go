package filter

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// This file checks that the syntactic EBNF grammar in docs/query-language.md
// and Parse accept and reject the same filters. A small reader parses the
// EBNF block, a backtracking recognizer matches lexFilter's tokens against
// its filter rule, and both verdicts are compared on a shared corpus.

// docLexicalTokens matches the lexical names that the syntactic block uses
// to the tokens that lexFilter produces.
var docLexicalTokens = map[string]func(token) bool{
	"identifier": func(t token) bool { return t.kind == tokIdent },
	"string":     func(t token) bool { return t.kind == tokString },
	"number":     func(t token) bool { return t.kind == tokNumber },
	"boolean":    func(t token) bool { return t.kind == tokBool },
	// An integer is a number token with no sign and no decimal point.
	"integer": func(t token) bool {
		return t.kind == tokNumber && strings.Trim(t.text, "0123456789") == ""
	},
}

// docTerminalMatches reports whether a quoted terminal matches a token: a
// keyword, word, or symbol spelled the same, ignoring ASCII case. Strings and
// quoted identifiers never match a terminal.
func docTerminalMatches(t token, terminal string) bool {
	if t.kind == tokString || t.quoted {
		return false
	}
	return foldASCIILower(t.text) == foldASCIILower(terminal)
}

// docCorpusAccepted lists filters both must accept, in addition to every
// line of the docs' filter blocks and the expression tree test inputs.
var docCorpusAccepted = []string{
	// Existing parser test inputs.
	`a = 1 OR b = 2 AND c = 3`, `"é" = 'a''b'`, `n != 'x'`, `NaMe = +001.20`, `a > -2`, `a >= 2`,
	`a < 2`, `a <= 2`, `a CONTAINS ''`, `a EXCLUDES FaLsE`, `a BETWEEN 'unsplit'`, `a != -2`,
	`a != 'x'`, `a!=TRUE`, `missing = '1'`, `country = NULL`, `country != null`, `country = Null`,
	`country=NULL`, `country!=NULL`, `"NULL" = 'x'`, `country = 'NULL'`,
	`country = NULL OR country EXCLUDES 'fr'`, `a = 1 OR b = 2 OR c = 3`, `a = 1 AND b = 2 AND c = 3`,
	`(a = 1 OR b = 2) AND c = 3`, `((a = 1))`, `(a = 1 OR b != 'x') AND c BETWEEN '1,2'`,
	strings.Repeat("(", 50) + "a = 1" + strings.Repeat(")", 50),
	strings.Repeat("a = 1 OR ", 50) + "a = 1",
	`"` + strings.Repeat("a", 62) + `é" = 1`, strings.Repeat("A", 64) + ` = 1`,
	`"AND" = 'a''b'`, `"null" = 'x'`, `a CONTAINS 1`, `a EXCLUDES TRUE`,
	// filter-parsing scenarios and other relative values.
	`t > {NOW - TODAY}`, `price > {1 / 0}`, `due > {5::DAYS}`, `created_at >= START_OF_WEEK`,
	`due = today`, `due = Today`, `due = TODAY`, `today = 'x' AND now > 3`,
	`due_date > {today + 5::days}`, `due = {TODAY + 2 * 7::DAYS}`, `due = {TODAY+5::DAYS}`,
	`due = {TODAY + 5::DAYS}`, `t > {NOW - 30::minutes}`, `price > 300`, `price > {100 * 3}`,
	`delta > {-1.50 * 2}`, `t > {NOW + (1::DAYS + 2::HOURS)}`, `d = END_OF_WEEK`,
	`d != START_OF_MONTH`, `d > END_OF_MONTH`, `d < start_of_year`, `d <= End_Of_Year`, `t > now`,
	`n = {1}`, `n != {1}`, `n >= {1}`, `n < {1}`, `n <= {1}`, `START_OF_WEEK = TODAY`,
	`days = {1::days}`, `"today" = now`, `(a = {1} OR b = TODAY) AND c < {NOW - 1::HOURS}`,
	`a = {-(TODAY + 1::DAYS) * 2}`, `a = {0::BUSINESS_DAYS}`, `age > -3 AND x = {1} AND y < +2`,
	`a = {NOW + 10000001::MINUTES}`, `a = {1 / (2 - 2)}`, `a = {10 / 4 - 0.1}`,
	`a = {(2::DAYS + 3::HOURS) / 2}`, `a = {TODAY + 9::HOURS}`,
}

// docCorpusRejected lists filters both must reject, in addition to the
// syntax error and error message test inputs.
var docCorpusRejected = []string{
	// Examples from the docs.
	`age BETWEEN 30 AND 40`, `due = tomorrow`, `due > TODAY + 5::DAYS`, `name CONTAINS TODAY`,
	`due BETWEEN TODAY`, `name CONTAINS {1}`, `t > {NOW - 1.5::HOURS}`, `t > {NOW - 3::WEEKS}`,
	`due > {{TODAY}}`, `due > {}`, `due > {TODAY + 5::DAYS`, `"é" = @`, `1abc = 1`, `a = 1.2.3`,
	// Lexer errors and other edge cases.
	`a = {1:2}`, `a = 1 : 2`, `n ! 2`, `a = "today"`, `a = {"today"}`, `a = {5::"DAYS"}`,
	`a = {5::days::days}`, `a = {+1}`, `a = {1 +- }`, `a = {TODAY 5::DAYS}`, `a = { }`,
	`a = {TODAY} {NOW}`, `a = TODAY NOW`, `a = (TODAY)`, `a = -TODAY`, `a = TODAY - 1`,
	`a = {1}AND b = 2 c`, `{1} = a`, `TODAY`, `a = {START_OF_DAY}`, `a = {1::HOUR}`, `a BETWEEN '1,2' AND`,
	`a = {1.5::DAYS}`, `a = {-1::}`, `a = {::DAYS}`, `a CONTAINS NOW`, `a EXCLUDES {NOW}`,
	`a = {now()}`, `a = {x}`, `a = {1 * * 2}`, `a = {(1 + 2}`, `a = {1 + 2)}`, `a = {1}}`, `}`,
	`a = 1 * 2`, `a = 1 / 2`, `a = 5::DAYS`,
}

func TestGrammarDocMatchesParser(t *testing.T) {
	md, err := os.ReadFile("docs/query-language.md")
	if err != nil {
		t.Fatal(err)
	}
	blocks := fencedBlocks(string(md), "ebnf")
	if len(blocks) != 2 {
		t.Fatalf("docs/query-language.md has %d ebnf blocks, want a syntactic and a lexical block", len(blocks))
	}
	if strings.Contains(string(md), "```yacc") {
		t.Fatal("docs/query-language.md still has a yacc block")
	}
	g, err := parseEBNF(blocks[0])
	if err != nil {
		t.Fatalf("syntactic ebnf block: %v", err)
	}
	if g.rules["filter"] == nil {
		t.Fatal("syntactic ebnf block has no filter rule")
	}
	for _, name := range g.undefined() {
		if docLexicalTokens[name] == nil {
			t.Errorf("syntactic ebnf block uses %q, which is neither a rule nor a known token", name)
		}
		if !regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(name) + `\s*=`).MatchString(blocks[1]) {
			t.Errorf("lexical ebnf block does not define %q", name)
		}
	}

	accepted := slices.Clone(docCorpusAccepted)
	for _, block := range fencedBlocks(string(md), "filter") {
		for _, line := range strings.Split(strings.TrimSpace(block), "\n") {
			accepted = append(accepted, line)
		}
	}
	for _, tc := range expressionTreeCases {
		accepted = append(accepted, tc.input)
	}
	rejected := slices.Clone(docCorpusRejected)
	for _, tc := range syntaxErrorCases {
		rejected = append(rejected, tc.input)
	}
	for _, tc := range errorMessageCases {
		rejected = append(rejected, tc.input)
	}

	for _, want := range []struct {
		filters []string
		accept  bool
	}{{accepted, true}, {rejected, false}} {
		for _, input := range want.filters {
			_, err := Parse(input)
			parser := err == nil
			doc := false
			if tokens, err := lexFilter(input); err == nil {
				doc = g.recognize("filter", tokens, docLexicalTokens, docTerminalMatches)
			}
			if parser != want.accept || doc != want.accept {
				t.Errorf("filter %q: parser accepts %v (%v), documented EBNF accepts %v; corpus expects %v",
					input, parser, err, doc, want.accept)
			}
		}
	}
}

// fencedBlocks returns the bodies of the Markdown code blocks fenced with
// ```lang, in order.
func fencedBlocks(md, lang string) []string {
	var blocks []string
	rest := md
	for {
		_, after, ok := strings.Cut(rest, "```"+lang+"\n")
		if !ok {
			return blocks
		}
		body, tail, _ := strings.Cut(after, "```")
		blocks = append(blocks, body)
		rest = tail
	}
}

// ebnfNode is an EBNF expression.
type ebnfNode struct {
	kind  ebnfKind
	items []*ebnfNode // sequence and alternatives; one item for the others
	text  string      // terminal text or rule name
}

type ebnfKind uint8

const (
	ebnfSeq ebnfKind = iota
	ebnfAlt
	ebnfOpt
	ebnfRep
	ebnfTerminal
	ebnfRef
)

// ebnfGrammar is a parsed EBNF block.
type ebnfGrammar struct {
	rules map[string]*ebnfNode
	order []string
}

// parseEBNF reads ISO/IEC 14977 rules: name = definitions ; with sequence
// ',', alternation '|', '[ ]', '{ }', '( )', quoted terminals, and (* *)
// comments. Rule names are letters, digits, '_', and single inner spaces.
// It does not support exceptions, repetition counts, or special sequences.
func parseEBNF(src string) (ebnfGrammar, error) {
	p := &ebnfParser{src: src}
	g := ebnfGrammar{rules: map[string]*ebnfNode{}}
	for {
		if err := p.skip(); err != nil {
			return g, err
		}
		if p.i == len(p.src) {
			return g, nil
		}
		name, err := p.name()
		if err != nil {
			return g, err
		}
		if _, dup := g.rules[name]; dup {
			return g, fmt.Errorf("rule %q defined twice", name)
		}
		if err := p.expect('='); err != nil {
			return g, err
		}
		body, err := p.alternatives()
		if err != nil {
			return g, err
		}
		if err := p.expect(';'); err != nil {
			return g, err
		}
		g.rules[name] = body
		g.order = append(g.order, name)
	}
}

type ebnfParser struct {
	src string
	i   int
}

// skip skips whitespace and comments.
func (p *ebnfParser) skip() error {
	for p.i < len(p.src) {
		switch {
		case strings.HasPrefix(p.src[p.i:], "(*"):
			end := strings.Index(p.src[p.i+2:], "*)")
			if end < 0 {
				return fmt.Errorf("unterminated comment at offset %d", p.i)
			}
			p.i += 2 + end + 2
		case strings.IndexByte(" \t\r\n", p.src[p.i]) >= 0:
			p.i++
		default:
			return nil
		}
	}
	return nil
}

// peek skips whitespace and comments and returns the next byte, or 0.
func (p *ebnfParser) peek() (byte, error) {
	if err := p.skip(); err != nil {
		return 0, err
	}
	if p.i == len(p.src) {
		return 0, nil
	}
	return p.src[p.i], nil
}

func (p *ebnfParser) expect(ch byte) error {
	got, err := p.peek()
	if err != nil {
		return err
	}
	if got != ch {
		return fmt.Errorf("expected %q at offset %d", ch, p.i)
	}
	p.i++
	return nil
}

func isEBNFLetter(ch byte) bool { return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' }

func isEBNFNameByte(ch byte) bool { return isEBNFLetter(ch) || isFilterDigit(ch) || ch == '_' }

// name reads a rule name, joining its words with single spaces.
func (p *ebnfParser) name() (string, error) {
	if ch, err := p.peek(); err != nil {
		return "", err
	} else if !isEBNFLetter(ch) {
		return "", fmt.Errorf("expected a rule name at offset %d", p.i)
	}
	var words []string
	for {
		start := p.i
		for p.i < len(p.src) && isEBNFNameByte(p.src[p.i]) {
			p.i++
		}
		words = append(words, p.src[start:p.i])
		// Another word follows only after plain spaces.
		j := p.i
		for j < len(p.src) && strings.IndexByte(" \t\r\n", p.src[j]) >= 0 {
			j++
		}
		if j == p.i || j == len(p.src) || !isEBNFLetter(p.src[j]) {
			return strings.Join(words, " "), nil
		}
		p.i = j
	}
}

func (p *ebnfParser) alternatives() (*ebnfNode, error) {
	n := &ebnfNode{kind: ebnfAlt}
	for {
		seq, err := p.sequence()
		if err != nil {
			return nil, err
		}
		n.items = append(n.items, seq)
		if ch, err := p.peek(); err != nil {
			return nil, err
		} else if ch != '|' {
			return n, nil
		}
		p.i++
	}
}

func (p *ebnfParser) sequence() (*ebnfNode, error) {
	n := &ebnfNode{kind: ebnfSeq}
	for {
		item, err := p.primary()
		if err != nil {
			return nil, err
		}
		n.items = append(n.items, item)
		if ch, err := p.peek(); err != nil {
			return nil, err
		} else if ch != ',' {
			return n, nil
		}
		p.i++
	}
}

func (p *ebnfParser) primary() (*ebnfNode, error) {
	ch, err := p.peek()
	if err != nil {
		return nil, err
	}
	closers := map[byte]struct {
		close byte
		kind  ebnfKind
	}{'[': {']', ebnfOpt}, '{': {'}', ebnfRep}, '(': {')', ebnfAlt}}
	switch {
	case closers[ch].close != 0:
		p.i++
		inner, err := p.alternatives()
		if err != nil {
			return nil, err
		}
		if err := p.expect(closers[ch].close); err != nil {
			return nil, err
		}
		if ch == '(' {
			return inner, nil
		}
		return &ebnfNode{kind: closers[ch].kind, items: []*ebnfNode{inner}}, nil
	case ch == '\'' || ch == '"':
		end := strings.IndexByte(p.src[p.i+1:], ch)
		if end <= 0 {
			return nil, fmt.Errorf("unterminated or empty terminal at offset %d", p.i)
		}
		n := &ebnfNode{kind: ebnfTerminal, text: p.src[p.i+1 : p.i+1+end]}
		p.i += end + 2
		return n, nil
	case isEBNFLetter(ch):
		name, err := p.name()
		return &ebnfNode{kind: ebnfRef, text: name}, err
	}
	return nil, fmt.Errorf("unexpected %q at offset %d", ch, p.i)
}

// undefined returns the names that rules refer to but do not define, sorted.
func (g ebnfGrammar) undefined() []string {
	var names []string
	var walk func(*ebnfNode)
	walk = func(n *ebnfNode) {
		if n.kind == ebnfRef && g.rules[n.text] == nil && !slices.Contains(names, n.text) {
			names = append(names, n.text)
		}
		for _, item := range n.items {
			walk(item)
		}
	}
	for _, name := range g.order {
		walk(g.rules[name])
	}
	slices.Sort(names)
	return names
}

// recognize reports whether tokens, all of them, derive from rule start.
// Names that are not rules are matched one token at a time by tokenKinds,
// and quoted terminals by terminal. It explores every alternative, so it
// does not depend on their order. The grammar must not be left recursive.
func (g ebnfGrammar) recognize(start string, tokens []token, tokenKinds map[string]func(token) bool, terminal func(token, string) bool) bool {
	r := &ebnfRecognizer{g: g, tokens: tokens, kinds: tokenKinds, terminal: terminal, memo: map[ebnfMemoKey][]int{}}
	return slices.Contains(r.match(&ebnfNode{kind: ebnfRef, text: start}, 0), len(tokens))
}

type ebnfMemoKey struct {
	rule string
	pos  int
}

type ebnfRecognizer struct {
	g        ebnfGrammar
	tokens   []token
	kinds    map[string]func(token) bool
	terminal func(token, string) bool
	memo     map[ebnfMemoKey][]int
}

// match returns every position at which n, starting at pos, can end.
func (r *ebnfRecognizer) match(n *ebnfNode, pos int) []int {
	switch n.kind {
	case ebnfTerminal:
		if pos < len(r.tokens) && r.terminal(r.tokens[pos], n.text) {
			return []int{pos + 1}
		}
		return nil
	case ebnfRef:
		rule := r.g.rules[n.text]
		if rule == nil {
			if pos < len(r.tokens) && r.kinds[n.text] != nil && r.kinds[n.text](r.tokens[pos]) {
				return []int{pos + 1}
			}
			return nil
		}
		key := ebnfMemoKey{n.text, pos}
		if ends, ok := r.memo[key]; ok {
			return ends
		}
		ends := r.match(rule, pos)
		r.memo[key] = ends
		return ends
	case ebnfSeq:
		ends := []int{pos}
		for _, item := range n.items {
			var next []int
			for _, p := range ends {
				next = mergeEnds(next, r.match(item, p))
			}
			if ends = next; len(ends) == 0 {
				return nil
			}
		}
		return ends
	case ebnfAlt:
		var ends []int
		for _, item := range n.items {
			ends = mergeEnds(ends, r.match(item, pos))
		}
		return ends
	case ebnfOpt:
		return mergeEnds([]int{pos}, r.match(n.items[0], pos))
	case ebnfRep:
		ends, frontier := []int{pos}, []int{pos}
		for len(frontier) > 0 {
			var next []int
			for _, p := range frontier {
				for _, e := range r.match(n.items[0], p) {
					if !slices.Contains(ends, e) {
						ends = mergeEnds(ends, []int{e})
						next = append(next, e)
					}
				}
			}
			frontier = next
		}
		return ends
	}
	panic("unknown ebnf node")
}

// mergeEnds returns the sorted union of two sorted position lists.
func mergeEnds(a, b []int) []int {
	out := slices.Concat(a, b)
	slices.Sort(out)
	return slices.Compact(out)
}

// The reader and recognizer are checked on a toy grammar over filter tokens.
func TestEBNFReaderAndRecognizer(t *testing.T) {
	g, err := parseEBNF(`
		(* A toy grammar. *)
		list      = item, { "AND", item } ;
		item      = "(", [ list ], ")" | word or number | 'TRUE' ;
		word or number = identifier | number ;
		greedy    = { identifier }, identifier, "OR" ; (* needs backtracking *)
		choice    = ( identifier | identifier, identifier ), "=" ;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(g.order, "; "); got != "list; item; word or number; greedy; choice" {
		t.Fatalf("rules: %s", got)
	}
	if got := g.undefined(); !slices.Equal(got, []string{"identifier", "number"}) {
		t.Fatalf("undefined: %q", got)
	}
	for _, tc := range []struct {
		rule, input string
		want        bool
	}{
		{"list", `()`, true}, {"list", `(a)`, true}, {"list", `(a AND 1) and ((true))`, true},
		{"list", `a AND`, false}, {"list", `(a`, false}, {"list", ``, false}, {"list", `'TRUE'`, false},
		{"list", `"a"`, true}, {"list", `"TRUE"`, true}, {"list", `TRUE TRUE`, false},
		{"greedy", `a b c OR`, true}, {"greedy", `a OR`, true}, {"greedy", `OR`, false}, {"greedy", `a b`, false},
		{"choice", `a =`, true}, {"choice", `a b =`, true}, {"choice", `a b c =`, false},
	} {
		tokens, err := lexFilter(tc.input)
		if err != nil {
			t.Fatal(err)
		}
		if got := g.recognize(tc.rule, tokens, docLexicalTokens, docTerminalMatches); got != tc.want {
			t.Errorf("%s %q: got %v, want %v", tc.rule, tc.input, got, tc.want)
		}
	}

	for _, src := range []string{
		`a = b`, `a = b ;; `, `a = (b ;`, `a = [ b ;`, `a = { b ] ;`, `a = "b ;`, `a = "" ;`,
		`a = b | ;`, `a = b, ;`, `(* open`, `= b ;`, `a = b ; a = c ;`, `a b = c - d ;`, `a = ? x ? ;`,
	} {
		if _, err := parseEBNF(src); err == nil {
			t.Errorf("parseEBNF(%q) succeeded, want an error", src)
		}
	}
}
