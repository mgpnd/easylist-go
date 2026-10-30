package filter

import (
	"fmt"
	"go/ast"
	"go/parser"
	gotoken "go/token"
	"strings"
	"sync"
	"testing"
)

// These tests catch incorrect precedence, association, normalization, literal
// decoding, and source positions through the public inspection API.
func TestParsePublicAST(t *testing.T) {
	root, err := Parse(`a = 1 OR b = 2 AND c = 3`)
	if err != nil {
		t.Fatal(err)
	}
	top, ok := root.(Logical)
	if !ok || top.Operator() != Or || top.OperatorPos() != 7 {
		t.Fatalf("root: %#v", root)
	}
	left, ok := top.Left().(Predicate)
	if !ok || left.Field() != "a" || left.Value().Text() != "1" || left.Value().Kind() != LiteralNumber {
		t.Fatalf("left: %#v", top.Left())
	}
	right, ok := top.Right().(Logical)
	if !ok || right.Operator() != And {
		t.Fatalf("right: %#v", top.Right())
	}
	for _, child := range []struct {
		expr         Expr
		field, value string
	}{{right.Left(), "b", "2"}, {right.Right(), "c", "3"}} {
		p, ok := child.expr.(Predicate)
		if !ok || p.Field() != child.field || p.Value().Text() != child.value {
			t.Fatalf("child: %#v", child.expr)
		}
	}
	root, err = Parse(`"é" = 'a''b'`)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := root.(Predicate)
	if !ok || p.Field() != "é" || !p.FieldQuoted() || p.FieldPos() != 1 || p.Operator() != OpEq || p.OperatorPos() != 6 || p.Value().Pos() != 8 || p.Value().Kind() != LiteralString || p.Value().Text() != "a'b" {
		t.Fatalf("quoted predicate: %#v", root)
	}
	root, err = Parse(`n != 'x'`)
	if err != nil {
		t.Fatal(err)
	}
	p, ok = root.(Predicate)
	if !ok || p.Operator() != OpNeq || p.OperatorPos() != 3 || p.Value().Pos() != 6 {
		t.Fatalf("not-equal predicate: %#v", root)
	}
}

func TestParseOperatorsAndLiterals(t *testing.T) {
	for _, tc := range []struct {
		input string
		op    PredicateOp
		kind  LiteralKind
		text  string
	}{
		{`NaMe = +001.20`, OpEq, LiteralNumber, "+001.20"},
		{`a > -2`, OpGt, LiteralNumber, "-2"},
		{`a >= 2`, OpGte, LiteralNumber, "2"},
		{`a < 2`, OpLt, LiteralNumber, "2"},
		{`a <= 2`, OpLte, LiteralNumber, "2"},
		{`a CONTAINS ''`, OpContains, LiteralString, ""},
		{`a EXCLUDES FaLsE`, OpExcludes, LiteralBool, "FaLsE"},
		{`a BETWEEN 'unsplit'`, OpBetween, LiteralString, "unsplit"},
		{`a != -2`, OpNeq, LiteralNumber, "-2"},
		{`a != 'x'`, OpNeq, LiteralString, "x"},
		{`a!=TRUE`, OpNeq, LiteralBool, "TRUE"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			root, err := Parse(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			p, ok := root.(Predicate)
			if !ok || p.Operator() != tc.op || p.Value().Kind() != tc.kind || p.Value().Text() != tc.text || p.FieldQuoted() {
				t.Fatalf("predicate: %#v", root)
			}
			if tc.input == `NaMe = +001.20` && p.Field() != "name" {
				t.Fatalf("field: %q", p.Field())
			}
		})
	}
	// Parsing does not require schema validation or split BETWEEN bounds.
	if root, err := Parse(`missing = '1'`); err != nil || root == nil {
		t.Fatalf("schema-independent parse: %#v %v", root, err)
	}
}

// These tests cover NULL comparisons: the keyword is a value only for = and !=,
// in any case, and a quoted "NULL" stays a field name.
func TestParseNull(t *testing.T) {
	for _, tc := range []struct {
		input string
		op    PredicateOp
		opPos int
		text  string
		pos   int
	}{
		{`country = NULL`, OpEq, 9, "NULL", 11},
		{`country != null`, OpNeq, 9, "null", 12},
		{`country = Null`, OpEq, 9, "Null", 11},
		{`country=NULL`, OpEq, 8, "NULL", 9},
		{`country!=NULL`, OpNeq, 8, "NULL", 10},
	} {
		t.Run(tc.input, func(t *testing.T) {
			root, err := Parse(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			p, ok := root.(Predicate)
			if !ok || p.Field() != "country" || p.FieldQuoted() || p.FieldPos() != 1 ||
				p.Operator() != tc.op || p.OperatorPos() != tc.opPos ||
				p.Value().Kind() != LiteralNull || p.Value().Text() != tc.text || p.Value().Pos() != tc.pos {
				t.Fatalf("predicate: %#v", root)
			}
		})
	}

	root, err := Parse(`"NULL" = 'x'`)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := root.(Predicate)
	if !ok || p.Field() != "NULL" || !p.FieldQuoted() || p.Operator() != OpEq || p.Value().Kind() != LiteralString || p.Value().Text() != "x" {
		t.Fatalf("quoted NULL field: %#v", root)
	}

	root, err = Parse(`country = 'NULL'`)
	if err != nil {
		t.Fatal(err)
	}
	p, ok = root.(Predicate)
	if !ok || p.Value().Kind() != LiteralString || p.Value().Text() != "NULL" || p.Value().Pos() != 11 {
		t.Fatalf("NULL string: %#v", root)
	}

	root, err = Parse(`country = NULL OR country EXCLUDES 'fr'`)
	if err != nil {
		t.Fatal(err)
	}
	top, ok := root.(Logical)
	if !ok || top.Operator() != Or || top.OperatorPos() != 16 {
		t.Fatalf("root: %#v", root)
	}
	left, ok := top.Left().(Predicate)
	if !ok || left.Field() != "country" || left.Operator() != OpEq || left.Value().Kind() != LiteralNull {
		t.Fatalf("left: %#v", top.Left())
	}
	right, ok := top.Right().(Predicate)
	if !ok || right.Field() != "country" || right.Operator() != OpExcludes || right.Value().Kind() != LiteralString || right.Value().Text() != "fr" {
		t.Fatalf("right: %#v", top.Right())
	}
}

// dumpLiteral renders a literal as its kind, text, and position, such as
// string"a"@12, with a list's items in brackets.
func dumpLiteral(l Literal) string {
	kinds := [...]string{LiteralString: "string", LiteralNumber: "number", LiteralBool: "bool",
		LiteralNull: "null", LiteralExpr: "expr", LiteralList: "list"}
	out := fmt.Sprintf("%s%q@%d", kinds[l.Kind()], l.Text(), l.Pos())
	if items := l.Items(); items != nil {
		var parts []string
		for _, item := range items {
			parts = append(parts, dumpLiteral(item))
		}
		out += "[" + strings.Join(parts, " ") + "]"
	}
	return out
}

// These tests cover list values: IN and NOT IN lists with NULL items, list
// values after CONTAINS and EXCLUDES, BETWEEN pairs, and the source text and
// positions of lists and their items.
func TestParseLists(t *testing.T) {
	for _, tc := range []struct {
		input string
		op    PredicateOp
		opPos int
		value string
	}{
		// filter-parsing and contract-filter-ast scenarios.
		{`status IN ('success', 'pending')`, OpIn, 8, `list"('success', 'pending')"@11[string"success"@12 string"pending"@23]`},
		{`status NOT IN ('error')`, OpNotIn, 8, `list"('error')"@15[string"error"@16]`},
		{`status IN ('success', 'pending', NULL)`, OpIn, 8, `list"('success', 'pending', NULL)"@11[string"success"@12 string"pending"@23 null"NULL"@34]`},
		{`x IN ('a', 1, TRUE)`, OpIn, 3, `list"('a', 1, TRUE)"@6[string"a"@7 number"1"@12 bool"TRUE"@15]`},
		{`status IN ('a')`, OpIn, 8, `list"('a')"@11[string"a"@12]`},
		{`status in ('a')`, OpIn, 8, `list"('a')"@11[string"a"@12]`},
		{`status not In ('a')`, OpNotIn, 8, `list"('a')"@15[string"a"@16]`},
		{`status IN ('a', null)`, OpIn, 8, `list"('a', null)"@11[string"a"@12 null"null"@17]`},
		{`tags CONTAINS ('a,b', 'c')`, OpContains, 6, `list"('a,b', 'c')"@15[string"a,b"@16 string"c"@23]`},
		{`tags EXCLUDES ('x')`, OpExcludes, 6, `list"('x')"@15[string"x"@16]`},
		{`age BETWEEN (30, 40)`, OpBetween, 5, `list"(30, 40)"@13[number"30"@14 number"40"@18]`},
		{`tags CONTAINS 'a,b'`, OpContains, 6, `string"a,b"@15`},
		{`age BETWEEN '30,40'`, OpBetween, 5, `string"30,40"@13`},
		// Items are never split, trimmed, or checked against each other.
		{`tags CONTAINS (' a ', 1.50, false)`, OpContains, 6, `list"(' a ', 1.50, false)"@15[string" a "@16 number"1.50"@23 bool"false"@29]`},
		{`x NOT IN (NULL)`, OpNotIn, 3, `list"(NULL)"@10[null"NULL"@11]`},
		{`x IN (NULL, null)`, OpIn, 3, `list"(NULL, null)"@6[null"NULL"@7 null"null"@13]`},
		{`x BETWEEN ('2026-01-01', -1)`, OpBetween, 3, `list"('2026-01-01', -1)"@11[string"2026-01-01"@12 number"-1"@26]`},
		{"x IN(\t'it''s' ,'b' )", OpIn, 3, `list"(\t'it''s' ,'b' )"@5[string"it's"@7 string"b"@16]`},
		{`x NOT  in('a')`, OpNotIn, 3, `list"('a')"@10[string"a"@11]`},
	} {
		t.Run(tc.input, func(t *testing.T) {
			root, err := Parse(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			p, ok := root.(Predicate)
			if !ok || p.FieldPos() != 1 || p.Operator() != tc.op || p.OperatorPos() != tc.opPos {
				t.Fatalf("predicate: %#v", root)
			}
			if got := dumpLiteral(p.Value()); got != tc.value {
				t.Errorf("value\n got %s\nwant %s", got, tc.value)
			}
			if p.Value().Expr() != nil {
				t.Errorf("value has an expression tree")
			}
		})
	}

	// Keyword case does not matter.
	for _, pair := range [][2]string{
		{`status in ('a')`, `status IN ('a')`}, {`status not In ('a')`, `status NOT IN ('a')`},
	} {
		a, errA := Parse(pair[0])
		b, errB := Parse(pair[1])
		if errA != nil || errB != nil {
			t.Fatalf("%v %v", errA, errB)
		}
		pa, pb := a.(Predicate), b.(Predicate)
		if pa.Field() != pb.Field() || pa.Operator() != pb.Operator() || pa.OperatorPos() != pb.OperatorPos() ||
			dumpLiteral(pa.Value()) != dumpLiteral(pb.Value()) {
			t.Errorf("%s parses differently from %s", pair[0], pair[1])
		}
	}

	// A double-quoted field may be named after a reserved word.
	root, err := Parse(`"in" = 'x' AND "NOT" IN ('y')`)
	if err != nil {
		t.Fatal(err)
	}
	top := root.(Logical)
	l, r := top.Left().(Predicate), top.Right().(Predicate)
	if l.Field() != "in" || !l.FieldQuoted() || l.Operator() != OpEq || r.Field() != "NOT" || !r.FieldQuoted() || r.Operator() != OpIn {
		t.Fatalf("quoted reserved fields: %#v", root)
	}

	// Lists combine with connectors and groups like any predicate.
	root, err = Parse(`(status IN ('a') AND x NOT IN (1, 2)) OR tags CONTAINS ('z')`)
	if err != nil {
		t.Fatal(err)
	}
	top = root.(Logical)
	if top.Operator() != Or || top.Left().(Logical).Operator() != And ||
		top.Left().(Logical).Right().(Predicate).Operator() != OpNotIn || top.Right().(Predicate).Operator() != OpContains {
		t.Fatalf("grouping: %#v", root)
	}
}

// Items returns a copy, so a consumer cannot change the AST, and nil for
// literals other than lists.
func TestLiteralItems(t *testing.T) {
	root, err := Parse(`status IN ('a', 'b')`)
	if err != nil {
		t.Fatal(err)
	}
	v := root.(Predicate).Value()
	items := v.Items()
	items[0] = Literal{}
	_ = append(items[:1], Literal{})
	if got := dumpLiteral(v); got != `list"('a', 'b')"@11[string"a"@12 string"b"@17]` {
		t.Fatalf("list changed through Items: %s", got)
	}
	for _, input := range []string{`a = 'x'`, `a = 1`, `a = TRUE`, `a = NULL`, `a = TODAY`, `a = {1}`, `tags CONTAINS 'a,b'`} {
		root, err := Parse(input)
		if err != nil {
			t.Fatal(err)
		}
		if v := root.(Predicate).Value(); v.Kind() == LiteralList || v.Items() != nil {
			t.Errorf("%s: kind %d, items %v", input, v.Kind(), v.Items())
		}
	}
}

func TestParseGroupingAndAssociativity(t *testing.T) {
	for _, tc := range []struct {
		input      string
		top, child LogicalOp
	}{
		{`a = 1 OR b = 2 OR c = 3`, Or, Or},
		{`a = 1 AND b = 2 AND c = 3`, And, And},
		{`(a = 1 OR b = 2) AND c = 3`, And, Or},
	} {
		root, err := Parse(tc.input)
		if err != nil {
			t.Fatal(err)
		}
		top, ok := root.(Logical)
		if !ok || top.Operator() != tc.top {
			t.Fatalf("root: %#v", root)
		}
		left, ok := top.Left().(Logical)
		if !ok || left.Operator() != tc.child {
			t.Fatalf("left: %#v", top.Left())
		}
		if p, ok := top.Right().(Predicate); !ok || p.Field() != "c" {
			t.Fatalf("right: %#v", top.Right())
		}
	}
	if root, err := Parse(`((a = 1))`); err != nil {
		t.Fatal(err)
	} else if _, ok := root.(Predicate); !ok {
		t.Fatalf("parentheses: %#v", root)
	}
}

var syntaxErrorCases = []struct {
	input string
	pos   int
}{
	{"", 1}, {"   ", 4}, {"a", 2}, {"a =", 4}, {"a = AND b = 2", 5}, {"a 1", 3},
	{"a = 1 b = 2", 7}, {"AND a = 1", 1}, {"OR a = 1", 1}, {"a = 1 AND", 10},
	{"a = 1 OR", 9}, {"a = 1 AND OR b = 2", 11}, {"()", 2}, {"(a = 1", 1},
	{"a = 1)", 6}, {"(a = 1 OR)", 10}, {"a = 1 (b = 2)", 7}, {"(a = 1)(b = 2)", 8},
	{"age BETWEEN 1", 13}, {"a BETWEEN 1", 11}, {"a BETWEEN 1 AND 2", 11}, {"a = 1 AND (", 11},
	{"a = 1 OR (b = 2", 10}, {"\"é\" 1", 6}, {"a = 'unterminated", 5},
	{"missing = 1 AND", 16}, {"a = 1 @", 7}, {"a !=", 5}, {"a != AND b = 2", 6},
	{"a = 1 != 2", 7}, {"!= 1", 1}, {"a ! 1", 3},
	{"(a = 1))", 8}, {"(a = )", 6}, {"a = )", 5}, {"((a = 1) OR (b = 2", 13},
	{"age > NULL", 7}, {"age >= NULL", 8}, {"age < NULL", 7}, {"age <= NULL", 8},
	{"name CONTAINS NULL", 15}, {"name EXCLUDES NULL", 15}, {"age BETWEEN NULL", 13},
	{"NULL = 'x'", 1}, {"null = NULL", 1}, {"a NULL", 3}, {"a = NULL NULL", 10},
	{"a = 1 AND NULL", 11}, {"a = NULL AND", 13},
	{"name CONTAINS TODAY", 15}, {"name EXCLUDES TODAY", 15}, {"due BETWEEN TODAY", 13},
	{"name CONTAINS {TODAY}", 15}, {"due BETWEEN {TODAY}", 13}, {"due = tomorrow", 7},
	{"due > {TODAY + 5::DAYS", 23}, {"due > {}", 8}, {"due > TODAY + 5::DAYS", 13},
	{"due > TODAY +5::DAYS", 13}, {"due > {{TODAY}}", 8}, {"t > {NOW - 3::WEEKS}", 15},
	{"t > {NOW - 1.5::HOURS}", 12}, {"TODAY", 6}, {"{TODAY} = 1", 1}, {"a = {1} + 2", 9},
	{"a = {1}}", 8}, {"a = {1 2}", 8}, {"a = {1 + * 2}", 10}, {"a = {()}", 7},
	{"a = {1::DAYS::DAYS}", 13}, {"a = {DAYS::1}", 6}, {"a = {TODAY::DAYS}", 11},
	{"a = {NULL}", 6}, {"a = {TRUE}", 6}, {"a = {AND}", 6}, {"a = {1 = 2}", 8},
	// Lists. These come from the filter-parsing scenarios, then edge cases.
	{"status IN 'a'", 11}, {"status NOT = 'a'", 12}, {"status IN ()", 12}, {"status IN ('a',)", 16},
	{"a = 1, b = 2", 6}, {"age BETWEEN (30)", 16}, {"age BETWEEN (30, 40, 50)", 20},
	{"due IN (TODAY)", 9}, {"due BETWEEN ({TODAY}, {NOW})", 14}, {"in = 'x'", 1}, {"not = 1", 1},
	{"tags CONTAINS ('a', NULL)", 21}, {"age BETWEEN (NULL, 40)", 14}, {"status IN ('a'", 11},
	{"n = 1,2", 6}, {"a = {1, 2}", 7}, {"x IN ('a', {1})", 12}, {"x IN ('a', TODAY)", 12},
	{"tags CONTAINS (TODAY)", 16}, {"tags CONTAINS ()", 16}, {"tags EXCLUDES ('a',)", 20},
	{"age BETWEEN ()", 14}, {"age BETWEEN (30,)", 17}, {"x IN ('a' 'b')", 11}, {"x IN (('a'))", 7},
	{"x IN ('a'))", 11}, {"x = ('a')", 5}, {"x > ('a', 'b')", 5}, {"x IN", 5}, {"x NOT", 6},
	{"x NOT IN", 9}, {"x NOT ('a')", 7}, {"NOT x = 1", 1}, {"x IN ('a') NOT", 12},
	{"x NOT NOT IN ('a')", 7}, {"x IN NOT ('a')", 6}, {"x IN ('a'), ('b')", 11}, {"x IN (,)", 7},
	{"(x IN ('a')", 1}, {"x CONTAINS ('a', 'b'", 12}, {"x IN status", 6}, {`x IN ("a")`, 7},
}

func TestParseSyntaxErrors(t *testing.T) {
	for _, tc := range syntaxErrorCases {
		t.Run(tc.input, func(t *testing.T) {
			root, err := Parse(tc.input)
			if root != nil || err == nil || !strings.HasSuffix(err.Error(), fmt.Sprintf("position %d", tc.pos)) {
				t.Fatalf("want nil AST and position %d, got %#v %v", tc.pos, root, err)
			}
		})
	}
}

func TestParseLexerErrorWinsOverSyntaxError(t *testing.T) {
	_, err := Parse(`a a 'x`)
	if err == nil || err.Error() != "unterminated quote at position 5" {
		t.Fatalf("got %v", err)
	}
}

// The first rows mirror the error table in docs/parser.md, which is maintained
// by hand and must be updated together with them. The rest are edge cases
// around parentheses and expected-token lists.
var errorMessageCases = []struct{ input, want string }{
	{"", "unexpected end of input, expected field or '(' at position 1"},
	{"a 1", "unexpected number at position 3"},
	{"a =", "unexpected end of input, expected string, number, boolean, NULL, time keyword, or '{' at position 4"},
	{"a BETWEEN 1", "unexpected number, expected string or '(' at position 11"},
	{"(a = 1 OR)", "unexpected ')', expected field or '(' at position 10"},
	{"a = 1 b = 2", "unexpected field at position 7"},
	{"a = 1)", "unmatched ')' at position 6"},
	{"a = 1 AND (", "unmatched '(' at position 11"},
	{"((a = 1) OR (b = 2", "unmatched '(' at position 13"},
	{"a = 'x", "unterminated quote at position 5"},
	{"1abc = 1", "invalid number at position 2"},
	{"a = 1 @", "unexpected character '@' at position 7"},
	{"status IN ()", "unexpected ')', expected string, number, boolean, or NULL at position 12"},
	{"age BETWEEN (30)", "unexpected ')', expected ',' at position 16"},
	{"status IN ('a'", "unmatched '(' at position 11"},
	{"AND a = 1", "unexpected AND, expected field or '(' at position 1"},
	{"a = 1 != 2", "unexpected '!=' at position 7"},
	{"a = )", "unmatched ')' at position 5"},
	{"(a = )", "unexpected ')', expected string, number, boolean, NULL, time keyword, or '{' at position 6"},
	{"(a = 1))", "unmatched ')' at position 8"},
	{"(a = 1 b = 2)", "unexpected field, expected AND, OR, or ')' at position 8"},
	{"(a = 1 OR b = 2 c = 3)", "unexpected field, expected AND, OR, or ')' at position 17"},
	{"age > NULL", "unexpected NULL, expected string, number, boolean, time keyword, or '{' at position 7"},
	{"name CONTAINS NULL", "unexpected NULL, expected string, number, boolean, or '(' at position 15"},
	{"age BETWEEN NULL", "unexpected NULL, expected string or '(' at position 13"},
	{"NULL = 'x'", "unexpected NULL, expected field or '(' at position 1"},
	{"a = NULL NULL", "unexpected NULL at position 10"},
	{"due = tomorrow", "unknown time keyword 'tomorrow' at position 7"},
	{"due = {TOMORROW}", "unknown time keyword 'TOMORROW' at position 8"},
	{`due = "today"`, "unexpected quoted identifier at position 7"},
	{`due = {"today"}`, "unexpected quoted identifier at position 8"},
	{"name CONTAINS TODAY", "unexpected identifier, expected string, number, boolean, or '(' at position 15"},
	{"name EXCLUDES {1}", "unexpected '{', expected string, number, boolean, or '(' at position 15"},
	{"due BETWEEN TODAY", "unexpected identifier, expected string or '(' at position 13"},
	{"due BETWEEN {TODAY}", "unexpected '{', expected string or '(' at position 13"},
	{"due > {TODAY + 5::DAYS", "unexpected end of input, expected '}', '+', '-', '*', or '/' at position 23"},
	{"due > {}", "unexpected '}', expected time keyword, number, '(', or '-' at position 8"},
	{"due > {TODAY +}", "unexpected '}', expected time keyword, number, '(', or '-' at position 15"},
	{"due > {TODAY NOW}", "unexpected time keyword, expected '}', '+', '-', '*', or '/' at position 14"},
	{"due > {(TODAY}", "unexpected '}', expected ')', '+', '-', '*', or '/' at position 14"},
	{"due > {(TODAY", "unmatched '(' at position 8"},
	{"due > {TODAY)}", "unmatched ')' at position 13"},
	{"due > {'x'}", "unexpected string, expected time keyword, number, '(', or '-' at position 8"},
	{"due > {{TODAY}}", "nested '{' at position 8"},
	{"due > TODAY + 5::DAYS", "invalid number at position 13"},
	{"due > TODAY * 2", "unexpected '*' at position 13"},
	{"due > 5::DAYS", "unexpected '::' at position 8"},
	{"due > TODAY}", "unexpected '}' at position 12"},
	{"t > {NOW - 3::WEEKS}", "unknown duration unit 'WEEKS', expected DAYS, BUSINESS_DAYS, HOURS, or MINUTES at position 15"},
	{"t > {NOW - 3::5}", "unexpected number, expected unit at position 15"},
	{"t > {NOW - 3:HOURS}", "expected ':' after ':' at position 13"},
	{"t > {NOW - 1.5::HOURS}", "duration amount 1.5 is not a whole number at position 12"},
	{"status IN 'a'", "unexpected string, expected '(' at position 11"},
	{"status NOT = 'a'", "unexpected '=', expected IN at position 12"},
	{"status IN ('a',)", "unexpected ')', expected string, number, boolean, or NULL at position 16"},
	{"a = 1, b = 2", "unexpected ',' at position 6"},
	{"age BETWEEN (30, 40, 50)", "unexpected ',', expected ')' at position 20"},
	{"due IN (TODAY)", "unexpected identifier, expected string, number, boolean, or NULL at position 9"},
	{"due BETWEEN ({TODAY}, {NOW})", "unexpected '{', expected string, number, or boolean at position 14"},
	{"in = 'x'", "unexpected IN, expected field or '(' at position 1"},
	{"not = 1", "unexpected NOT, expected field or '(' at position 1"},
	{"tags CONTAINS ('a', NULL)", "unexpected NULL, expected string, number, or boolean at position 21"},
	{"age BETWEEN (NULL, 40)", "unexpected NULL, expected string, number, or boolean at position 14"},
	{"x IN ('a', TODAY)", "unexpected identifier, expected string, number, boolean, or NULL at position 12"},
	{"tags CONTAINS (TODAY)", "unexpected identifier, expected string, number, or boolean at position 16"},
	{"x IN ('a' b)", "unexpected field, expected ')' or ',' at position 11"},
	{"x IN status", "unexpected identifier, expected '(' at position 6"},
	{"x NOT", "unexpected end of input, expected IN at position 6"},
	{"x IN ('a'))", "unmatched ')' at position 11"},
	{"a = {1, 2}", "unexpected ',', expected '}', '+', '-', '*', or '/' at position 7"},
}

func TestParseErrorMessages(t *testing.T) {
	for _, tc := range errorMessageCases {
		t.Run(tc.input, func(t *testing.T) {
			_, err := Parse(tc.input)
			if err == nil || err.Error() != tc.want || strings.Contains(err.Error(), "\n") {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}

func TestYYCodesCoverAllTokenKinds(t *testing.T) {
	if len(yyCodes) != int(numTokenKinds) {
		t.Fatalf("len(yyCodes) = %d, want %d", len(yyCodes), int(numTokenKinds))
	}
	for k := tokIdent; k < numTokenKinds; k++ {
		if yyCodes[k] == 0 {
			t.Errorf("tokenKind %d has no parser code", k)
		}
	}
}

// goyacc exports a constant for every named token. The grammar names them with
// a lowercase prefix so that nothing from the generated file joins the API.
func TestGeneratedCodeExportsNothing(t *testing.T) {
	file, err := parser.ParseFile(gotoken.NewFileSet(), "parser_gen.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil && d.Name.IsExported() {
				t.Errorf("exported function %s", d.Name.Name)
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.ValueSpec:
					for _, n := range s.Names {
						if n.IsExported() {
							t.Errorf("exported identifier %s", n.Name)
						}
					}
				case *ast.TypeSpec:
					if s.Name.IsExported() {
						t.Errorf("exported type %s", s.Name.Name)
					}
				}
			}
		}
	}
}

func TestParseConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				if root, err := Parse(`(a = 1 OR b != 'x') AND c BETWEEN '1,2' AND d NOT IN ('x', NULL)`); err != nil || root == nil {
					t.Error(err)
				}
				if _, err := Parse(`a = 1 AND (`); err == nil {
					t.Error("want error")
				}
			}
		}()
	}
	wg.Wait()
}

func TestParseDepthAndIdentifiers(t *testing.T) {
	for _, input := range []string{strings.Repeat("(", 2000) + "a = 1" + strings.Repeat(")", 2000), strings.Repeat("a = 1 OR ", 1999) + "a = 1"} {
		if root, err := Parse(input); err != nil || root == nil {
			t.Fatalf("deep parse: %v", err)
		}
	}
	for _, tc := range []struct{ input, want string }{
		{`"` + strings.Repeat("a", 62) + `é" = 1`, strings.Repeat("a", 62) + "é"},
		{strings.Repeat("A", 64) + ` = 1`, strings.Repeat("a", 64)},
	} {
		root, err := Parse(tc.input)
		if err != nil {
			t.Fatal(err)
		}
		p, ok := root.(Predicate)
		if !ok || p.Field() != tc.want {
			t.Fatalf("identifier: %#v", root)
		}
	}
}

// dumpExpr renders an expression tree with positions: numbers as 2@13,
// keywords as TODAY"today"@7, durations as 7::DAYS"DAYS"@17/20 (amount and
// unit positions), binary nodes as (*@15 left right), and negations as
// (neg@9 operand).
func dumpExpr(n ExprNode) string {
	switch n := n.(type) {
	case NumberNode:
		return fmt.Sprintf("%s@%d", n.Text(), n.Pos())
	case KeywordNode:
		return fmt.Sprintf("%s%q@%d", n.Keyword(), n.Text(), n.Pos())
	case DurationNode:
		return fmt.Sprintf("%s::%s@%d/%d", n.Amount(), n.Unit(), n.Pos(), n.UnitPos())
	case BinaryNode:
		return fmt.Sprintf("(%s@%d %s %s)", n.Operator(), n.Pos(), dumpExpr(n.Left()), dumpExpr(n.Right()))
	case NegationNode:
		return fmt.Sprintf("(neg@%d %s)", n.Pos(), dumpExpr(n.Operand()))
	}
	return fmt.Sprintf("unknown %T", n)
}

// parseExprValue parses a single predicate and returns its expression value.
func parseExprValue(t *testing.T, input string) (Predicate, Literal) {
	t.Helper()
	root, err := Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := root.(Predicate)
	if !ok {
		t.Fatalf("root: %#v", root)
	}
	v := p.Value()
	if v.Kind() != LiteralExpr || v.Expr() == nil {
		t.Fatalf("value kind %d, expr %v; want an expression", v.Kind(), v.Expr())
	}
	return p, v
}

// These tests pin tree shapes and positions: precedence, association,
// parentheses, negation, and the absence of folding or type checking.
var expressionTreeCases = []struct{ input, text, tree string }{
	// filter-parsing scenarios.
	{`t > {NOW - TODAY}`, `{NOW - TODAY}`, `(-@10 NOW"NOW"@6 TODAY"TODAY"@12)`},
	{`price > {1 / 0}`, `{1 / 0}`, `(/@12 1@10 0@14)`},
	{`due > {5::DAYS}`, `{5::DAYS}`, `5::DAYS@8/11`},
	{`due_date > {today + 5::days}`, `{today + 5::days}`, `(+@19 TODAY"today"@13 5::DAYS@21/24)`},
	{`due = {TODAY + 2 * 7::DAYS}`, `{TODAY + 2 * 7::DAYS}`, `(+@14 TODAY"TODAY"@8 (*@18 2@16 7::DAYS@20/23))`},
	{`due = {TODAY+5::DAYS}`, `{TODAY+5::DAYS}`, `(+@13 TODAY"TODAY"@8 5::DAYS@14/17)`},
	{`due = {TODAY + 5::DAYS}`, `{TODAY + 5::DAYS}`, `(+@14 TODAY"TODAY"@8 5::DAYS@16/19)`},
	{`t > {NOW - 30::minutes}`, `{NOW - 30::minutes}`, `(-@10 NOW"NOW"@6 30::MINUTES@12/16)`},
	// contract-filter-ast scenarios.
	{`price > {100 * 3}`, `{100 * 3}`, `(*@14 100@10 3@16)`},
	{`delta > {-1.50 * 2}`, `{-1.50 * 2}`, `(*@16 (neg@10 1.50@11) 2@18)`},
	{`t > {NOW + (1::DAYS + 2::HOURS)}`, `{NOW + (1::DAYS + 2::HOURS)}`, `(+@10 NOW"NOW"@6 (+@21 1::DAYS@13/16 2::HOURS@23/26))`},
	// Association, parentheses, and negation.
	{`a = {1 - 2 - 3}`, `{1 - 2 - 3}`, `(-@12 (-@8 1@6 2@10) 3@14)`},
	{`a = {8 / 4 * 2}`, `{8 / 4 * 2}`, `(*@12 (/@8 8@6 4@10) 2@14)`},
	{`a = {1 - (2 - 3)}`, `{1 - (2 - 3)}`, `(-@8 1@6 (-@13 2@11 3@15))`},
	{`a = {1 + 2 * 3 - 4 / 5}`, `{1 + 2 * 3 - 4 / 5}`, `(-@16 (+@8 1@6 (*@12 2@10 3@14)) (/@20 4@18 5@22))`},
	{`a = {--1}`, `{--1}`, `(neg@6 (neg@7 1@8))`},
	{`a = {-(1 + 2)}`, `{-(1 + 2)}`, `(neg@6 (+@10 1@8 2@12))`},
	{`a = {-5::DAYS * 2}`, `{-5::DAYS * 2}`, `(*@15 (neg@6 5::DAYS@7/10) 2@17)`},
	{`a = {2 - -3}`, `{2 - -3}`, `(-@8 2@6 (neg@10 3@11))`},
	{`a = {((TODAY))}`, `{((TODAY))}`, `TODAY"TODAY"@8`},
	{`a = {007.50}`, `{007.50}`, `007.50@6`},
	// No size limit, and every keyword and unit in any case.
	{`a = {TODAY + 99999999999999999999::DAYS}`, `{TODAY + 99999999999999999999::DAYS}`, `(+@12 TODAY"TODAY"@6 99999999999999999999::DAYS@14/36)`},
	{`a = {0::Business_Days + 1::HOURS}`, `{0::Business_Days + 1::HOURS}`, `(+@23 0::BUSINESS_DAYS@6/9 1::HOURS@25/28)`},
	{`a = {start_of_week + End_Of_Week}`, `{start_of_week + End_Of_Week}`, `(+@20 START_OF_WEEK"start_of_week"@6 END_OF_WEEK"End_Of_Week"@22)`},
	{`a = {START_OF_MONTH-END_OF_MONTH}`, `{START_OF_MONTH-END_OF_MONTH}`, `(-@20 START_OF_MONTH"START_OF_MONTH"@6 END_OF_MONTH"END_OF_MONTH"@21)`},
	{`a = {start_of_year*end_of_year}`, `{start_of_year*end_of_year}`, `(*@19 START_OF_YEAR"start_of_year"@6 END_OF_YEAR"end_of_year"@20)`},
	{"a = {\tNOW\n}", "{\tNOW\n}", `NOW"NOW"@7`},
}

func TestParseExpressionTrees(t *testing.T) {
	for _, tc := range expressionTreeCases {
		t.Run(tc.input, func(t *testing.T) {
			_, v := parseExprValue(t, tc.input)
			if v.Text() != tc.text || v.Pos() != strings.Index(tc.input, "{")+1 {
				t.Errorf("text %q at %d, want %q at %d", v.Text(), v.Pos(), tc.text, strings.Index(tc.input, "{")+1)
			}
			if got := dumpExpr(v.Expr()); got != tc.tree {
				t.Errorf("tree\n got %s\nwant %s", got, tc.tree)
			}
		})
	}
}

func TestParseExpressionLiterals(t *testing.T) {
	// Brace expression literal.
	p, v := parseExprValue(t, `due > {today + 5::days}`)
	if p.Field() != "due" || p.Operator() != OpGt || v.Text() != "{today + 5::days}" || v.Pos() != 7 {
		t.Fatalf("brace literal: %q %d %q %d", p.Field(), p.Operator(), v.Text(), v.Pos())
	}
	if b, ok := v.Expr().(BinaryNode); !ok || b.Operator() != ArithAdd {
		t.Fatalf("root: %s", dumpExpr(v.Expr()))
	}

	// Bare keyword literals keep their spelling, in any case.
	for _, tc := range []struct{ input, text string }{
		{`due = TODAY`, "TODAY"}, {`due = today`, "today"}, {`due = Today`, "Today"}, {`due=TODAY`, "TODAY"},
	} {
		p, v := parseExprValue(t, tc.input)
		k, ok := v.Expr().(KeywordNode)
		pos := strings.Index(tc.input, tc.text) + 1
		if !ok || k.Keyword() != TimeToday || k.Text() != tc.text || k.Pos() != pos ||
			v.Text() != tc.text || v.Pos() != pos || p.Field() != "due" || p.Operator() != OpEq {
			t.Fatalf("%s: literal %q@%d, root %s", tc.input, v.Text(), v.Pos(), dumpExpr(v.Expr()))
		}
	}

	// Every comparison operator takes a bare keyword or a brace expression.
	for _, tc := range []struct {
		input string
		op    PredicateOp
	}{
		{`created_at >= START_OF_WEEK`, OpGte}, {`d = END_OF_WEEK`, OpEq}, {`d != START_OF_MONTH`, OpNeq},
		{`d > END_OF_MONTH`, OpGt}, {`d < start_of_year`, OpLt}, {`d <= End_Of_Year`, OpLte}, {`t > now`, OpGt},
		{`n = {1}`, OpEq}, {`n != {1}`, OpNeq}, {`n > {1}`, OpGt}, {`n >= {1}`, OpGte}, {`n < {1}`, OpLt}, {`n <= {1}`, OpLte},
	} {
		if p, _ := parseExprValue(t, tc.input); p.Operator() != tc.op {
			t.Errorf("%s: operator %d, want %d", tc.input, p.Operator(), tc.op)
		}
	}

	// Plain literals have no tree.
	root, err := Parse(`price > 300`)
	if err != nil {
		t.Fatal(err)
	}
	if v := root.(Predicate).Value(); v.Kind() != LiteralNumber || v.Expr() != nil {
		t.Fatalf("plain literal: kind %d, expr %v", v.Kind(), v.Expr())
	}
	for _, input := range []string{`a = 'TODAY'`, `a = TRUE`, `a = NULL`, `a BETWEEN '1,2'`, `a CONTAINS 'x'`, `a IN ('x')`} {
		root, err := Parse(input)
		if err != nil {
			t.Fatal(err)
		}
		if v := root.(Predicate).Value(); v.Kind() == LiteralExpr || v.Expr() != nil {
			t.Errorf("%s: kind %d, expr %v", input, v.Kind(), v.Expr())
		}
	}
}

func TestParseExpressionNodes(t *testing.T) {
	// Duration node.
	_, v := parseExprValue(t, `t > {NOW - 30::minutes}`)
	b, ok := v.Expr().(BinaryNode)
	if !ok || b.Operator() != ArithSub || b.Pos() != 10 {
		t.Fatalf("root: %s", dumpExpr(v.Expr()))
	}
	d, ok := b.Right().(DurationNode)
	if !ok || d.Amount() != "30" || d.Unit() != UnitMinutes || d.Pos() != 12 || d.UnitPos() != 16 {
		t.Fatalf("duration: %s", dumpExpr(b.Right()))
	}

	// Keyword node.
	_, v = parseExprValue(t, `due = today`)
	if k, ok := v.Expr().(KeywordNode); !ok || k.Keyword() != TimeToday || k.Text() != "today" || k.Pos() != 7 {
		t.Fatalf("keyword: %s", dumpExpr(v.Expr()))
	}

	// Binary node position and no folding.
	_, v = parseExprValue(t, `price > {100 * 3}`)
	b, ok = v.Expr().(BinaryNode)
	if !ok || b.Operator() != ArithMul || b.Pos() != 14 {
		t.Fatalf("binary: %s", dumpExpr(v.Expr()))
	}
	l, lok := b.Left().(NumberNode)
	r, rok := b.Right().(NumberNode)
	if !lok || !rok || l.Text() != "100" || r.Text() != "3" {
		t.Fatalf("operands: %s", dumpExpr(v.Expr()))
	}

	// Negation node.
	_, v = parseExprValue(t, `delta > {-1.50 * 2}`)
	b, ok = v.Expr().(BinaryNode)
	if !ok || b.Operator() != ArithMul {
		t.Fatalf("root: %s", dumpExpr(v.Expr()))
	}
	neg, ok := b.Left().(NegationNode)
	if !ok || neg.Pos() != 10 {
		t.Fatalf("negation: %s", dumpExpr(b.Left()))
	}
	if n, ok := neg.Operand().(NumberNode); !ok || n.Text() != "1.50" || n.Pos() != 11 {
		t.Fatalf("operand: %s", dumpExpr(neg.Operand()))
	}
}

// Each node type implements exactly one of the node interfaces, so a type
// switch over them is unambiguous.
func TestExprNodeInterfacesAreDisjoint(t *testing.T) {
	_, v := parseExprValue(t, `a = {-(TODAY + 1::DAYS) * 2}`)
	var nodes []ExprNode
	var walk func(ExprNode)
	walk = func(n ExprNode) {
		nodes = append(nodes, n)
		switch n := n.(type) {
		case BinaryNode:
			walk(n.Left())
			walk(n.Right())
		case NegationNode:
			walk(n.Operand())
		}
	}
	walk(v.Expr())
	if len(nodes) != 6 {
		t.Fatalf("nodes: %s", dumpExpr(v.Expr()))
	}
	for _, n := range nodes {
		count := 0
		for _, ok := range []bool{
			isNode[NumberNode](n), isNode[KeywordNode](n), isNode[DurationNode](n), isNode[BinaryNode](n), isNode[NegationNode](n),
		} {
			if ok {
				count++
			}
		}
		if count != 1 {
			t.Errorf("%T implements %d node interfaces", n, count)
		}
	}
}

func isNode[T ExprNode](n ExprNode) bool { _, ok := n.(T); return ok }

func TestParseKeywordsOnlyOnValueSide(t *testing.T) {
	root, err := Parse(`today = 'x' AND now > 3`)
	if err != nil {
		t.Fatal(err)
	}
	top := root.(Logical)
	l, r := top.Left().(Predicate), top.Right().(Predicate)
	if l.Field() != "today" || l.Value().Kind() != LiteralString || r.Field() != "now" || r.Value().Kind() != LiteralNumber {
		t.Fatalf("fields: %q %q", l.Field(), r.Field())
	}
	for _, input := range []string{`START_OF_WEEK = TODAY`, `days = {1::days}`, `"today" = now`} {
		if _, err := Parse(input); err != nil {
			t.Errorf("%s: %v", input, err)
		}
	}
	root, err = Parse(`(a = {1} OR b = TODAY) AND c < {NOW - 1::HOURS}`)
	if err != nil {
		t.Fatal(err)
	}
	if top := root.(Logical); top.Operator() != And || top.Left().(Logical).Operator() != Or {
		t.Fatalf("grouping: %#v", root)
	}
}

func TestEnumStrings(t *testing.T) {
	var got []string
	for k := TimeNow; k <= TimeEndOfYear+1; k++ {
		got = append(got, k.String())
	}
	for u := UnitDays; u <= UnitMinutes+1; u++ {
		got = append(got, u.String())
	}
	for o := ArithAdd; o <= ArithDiv+1; o++ {
		got = append(got, o.String())
	}
	for o := OpEq; o <= OpNotIn+1; o++ {
		got = append(got, o.String())
	}
	for o := And; o <= Or+1; o++ {
		got = append(got, o.String())
	}
	want := "NOW|TODAY|START_OF_WEEK|END_OF_WEEK|START_OF_MONTH|END_OF_MONTH|START_OF_YEAR|END_OF_YEAR|TimeKeyword(8)|" +
		"DAYS|BUSINESS_DAYS|HOURS|MINUTES|TimeUnit(4)|+|-|*|/|ArithOp(4)|" +
		"=|>|>=|<|<=|CONTAINS|EXCLUDES|BETWEEN|!=|IN|NOT IN|PredicateOp(11)|AND|OR|LogicalOp(2)"
	if strings.Join(got, "|") != want {
		t.Fatalf("got %s", strings.Join(got, "|"))
	}
}
