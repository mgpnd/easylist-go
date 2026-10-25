package filter

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestYaccRulesStripsActions(t *testing.T) {
	src := "%{\npackage x\n%}\n%token A\n%%\n\nr\n\t: A { s := \"}\"; _ = '{' }\n\t| r A\n\t;\n%%\n"
	if got, want := yaccRules(src), []string{"r : A | r A"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestGrammarDocMatchesYacc(t *testing.T) {
	y, err := os.ReadFile("grammar.y")
	if err != nil {
		t.Fatal(err)
	}
	md, err := os.ReadFile("docs/query-language.md")
	if err != nil {
		t.Fatal(err)
	}
	got, want := docGrammarRules(string(md)), yaccRules(string(y))
	if len(want) == 0 || !reflect.DeepEqual(got, want) {
		t.Fatalf("docs/query-language.md grammar is out of sync with grammar.y\ndoc:  %q\nyacc: %q", got, want)
	}
}

// yaccRules returns the normalized rules of a yacc source's rules section,
// with Go action blocks removed.
func yaccRules(src string) []string {
	var section []string
	inRules := false
	for _, line := range strings.Split(src, "\n") {
		if line == "%%" {
			if inRules {
				break
			}
			inRules = true
			continue
		}
		if inRules {
			section = append(section, line)
		}
	}
	return splitRules(stripActions(strings.Join(section, "\n")))
}

// docGrammarRules returns the normalized rules of the first yacc-fenced block
// in a Markdown document.
func docGrammarRules(md string) []string {
	_, rest, ok := strings.Cut(md, "```yacc\n")
	if !ok {
		return nil
	}
	body, _, _ := strings.Cut(rest, "```")
	return splitRules(body)
}

// stripActions removes brace-delimited blocks, ignoring braces inside Go
// string and rune literals.
func stripActions(s string) string {
	var out strings.Builder
	depth := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case depth > 0 && (c == '"' || c == '\'' || c == '`'):
			i = skipLiteral(s, i)
		case c == '{':
			depth++
		case c == '}':
			depth--
		case depth == 0:
			out.WriteByte(c)
		}
	}
	return out.String()
}

// skipLiteral returns the index of the closing quote of the literal that
// opens at s[i].
func skipLiteral(s string, i int) int {
	quote := s[i]
	for i++; i < len(s) && s[i] != quote; i++ {
		if s[i] == '\\' && quote != '`' {
			i++
		}
	}
	return i
}

// splitRules splits on ';' and collapses whitespace, dropping empty entries.
func splitRules(s string) []string {
	var rules []string
	for _, r := range strings.Split(s, ";") {
		if r = strings.Join(strings.Fields(r), " "); r != "" {
			rules = append(rules, r)
		}
	}
	return rules
}
