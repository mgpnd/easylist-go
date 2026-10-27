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

func TestParseSyntaxErrors(t *testing.T) {
	for _, tc := range []struct {
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
	} {
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
func TestParseErrorMessages(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"", "unexpected end of input, expected field or '(' at position 1"},
		{"a 1", "unexpected number at position 3"},
		{"a =", "unexpected end of input, expected string, number, or boolean at position 4"},
		{"a BETWEEN 1", "unexpected number, expected string at position 11"},
		{"(a = 1 OR)", "unexpected ')', expected field or '(' at position 10"},
		{"a = 1 b = 2", "unexpected field at position 7"},
		{"a = 1)", "unmatched ')' at position 6"},
		{"a = 1 AND (", "unmatched '(' at position 11"},
		{"((a = 1) OR (b = 2", "unmatched '(' at position 13"},
		{"a = 'x", "unterminated quote at position 5"},
		{"1abc = 1", "invalid number at position 2"},
		{"a = 1 @", "unexpected character '@' at position 7"},
		{"AND a = 1", "unexpected AND, expected field or '(' at position 1"},
		{"a = 1 != 2", "unexpected '!=' at position 7"},
		{"a = )", "unmatched ')' at position 5"},
		{"(a = )", "unexpected ')', expected string, number, or boolean at position 6"},
		{"(a = 1))", "unmatched ')' at position 8"},
		{"(a = 1 b = 2)", "unexpected field, expected AND, OR, or ')' at position 8"},
		{"(a = 1 OR b = 2 c = 3)", "unexpected field, expected AND, OR, or ')' at position 17"},
		{"age > NULL", "unexpected NULL, expected string, number, or boolean at position 7"},
		{"name CONTAINS NULL", "unexpected NULL, expected string, number, or boolean at position 15"},
		{"age BETWEEN NULL", "unexpected NULL, expected string at position 13"},
		{"NULL = 'x'", "unexpected NULL, expected field or '(' at position 1"},
		{"a = NULL NULL", "unexpected NULL at position 10"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			_, err := Parse(tc.input)
			if err == nil || err.Error() != tc.want || strings.Contains(err.Error(), "\n") {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}

func TestYYCodesCoverAllTokenKinds(t *testing.T) {
	if len(yyCodes) != int(tokNull)+1 {
		t.Fatalf("len(yyCodes) = %d, want %d", len(yyCodes), int(tokNull)+1)
	}
	for k := tokIdent; k <= tokNull; k++ {
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
				if root, err := Parse(`(a = 1 OR b != 'x') AND c BETWEEN '1,2'`); err != nil || root == nil {
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
