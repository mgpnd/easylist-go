package behavior_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mgpnd/easylist-go/easylist-behavior/lists"
	easylist "github.com/mgpnd/easylist-go/easylist-core"
	filter "github.com/mgpnd/easylist-go/easylist-filter"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

// TestFieldTypeCoverage fails when the fixture lists lack a field type as a
// scalar or as an array, or lack a nullable, non-nullable, joined, or
// calculated field. Types are enumerated through their names, so a new type
// is picked up as soon as it has one.
func TestFieldTypeCoverage(t *testing.T) {
	type shape struct {
		typ   easylist.Type
		array bool
	}
	seen := map[shape]bool{}
	var nullable, nonNullable, joined, calculated bool
	for _, l := range lists.Registry.Lists() {
		for _, f := range l.Fields() {
			seen[shape{f.Type, f.Array}] = true
			nullable = nullable || f.Nullable
			nonNullable = nonNullable || !f.Nullable
			joined = joined || len(f.Joins) > 0
			calculated = calculated || f.Calculated()
		}
	}

	for typ := easylist.Text; typ.String() != "invalid"; typ++ {
		if !seen[shape{typ, false}] {
			t.Errorf("no fixture list declares a scalar %s field", typ)
		}
		if !seen[shape{typ, true}] {
			t.Errorf("no fixture list declares an array %s field", typ)
		}
	}
	if !nullable {
		t.Error("no fixture list declares a nullable field")
	}
	if !nonNullable {
		t.Error("no fixture list declares a non-nullable field")
	}
	if !joined {
		t.Error("no fixture list declares a joined field")
	}
	if !calculated {
		t.Error("no fixture list declares a calculated field")
	}
}

// TestFilterFeatureCoverage fails when the filter fixtures, together with the
// filter arguments of the GraphQL fixtures, leave a predicate operator,
// logical connector, time keyword, duration unit, arithmetic operator, or
// unary minus unused. Enums are enumerated through their names, so a new
// constant is picked up as soon as it has one. Filters that do not parse are
// skipped: they are error cases and use no feature.
func TestFilterFeatureCoverage(t *testing.T) {
	var exprs []string
	files, err := filepath.Glob(filepath.Join("testdata", "filters", "*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("testdata/filters holds no fixture")
	}
	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		cases, err := parseFilterCases(file, string(src))
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cases {
			exprs = append(exprs, c.Expr)
		}
	}
	gqlFilters, err := graphQLFilterArgs(filepath.Join("testdata", "queries"))
	if err != nil {
		t.Fatal(err)
	}
	exprs = append(exprs, gqlFilters...)

	var u filterUsage
	for _, text := range exprs {
		if expr, err := filter.Parse(text); err == nil {
			u.expr(expr)
		}
	}

	requireAll(t, "operator", u.predicateOps)
	requireAll(t, "connector", u.logicalOps)
	requireAll(t, "time keyword", u.keywords)
	requireAll(t, "duration unit", u.units)
	requireAll(t, "arithmetic operator", u.arithOps)
	if !u.negation {
		t.Error("no filter fixture uses unary minus")
	}
}

// filterUsage records the filter features an expression tree uses.
type filterUsage struct {
	predicateOps map[filter.PredicateOp]bool
	logicalOps   map[filter.LogicalOp]bool
	keywords     map[filter.TimeKeyword]bool
	units        map[filter.TimeUnit]bool
	arithOps     map[filter.ArithOp]bool
	negation     bool
}

func (u *filterUsage) expr(e filter.Expr) {
	switch e := e.(type) {
	case filter.Predicate:
		mark(&u.predicateOps, e.Operator())
		if n := e.Value().Expr(); n != nil {
			u.node(n)
		}
	case filter.Logical:
		mark(&u.logicalOps, e.Operator())
		u.expr(e.Left())
		u.expr(e.Right())
	default:
		panic(fmt.Sprintf("unknown filter.Expr %T", e))
	}
}

func (u *filterUsage) node(n filter.ExprNode) {
	switch n := n.(type) {
	case filter.NumberNode:
	case filter.KeywordNode:
		mark(&u.keywords, n.Keyword())
	case filter.DurationNode:
		mark(&u.units, n.Unit())
	case filter.BinaryNode:
		mark(&u.arithOps, n.Operator())
		u.node(n.Left())
		u.node(n.Right())
	case filter.NegationNode:
		u.negation = true
		u.node(n.Operand())
	default:
		panic(fmt.Sprintf("unknown filter.ExprNode %T", n))
	}
}

func mark[E comparable](m *map[E]bool, e E) {
	if *m == nil {
		*m = map[E]bool{}
	}
	(*m)[e] = true
}

// requireAll fails t for each value of E, enumerated from 0 through its
// String names, that seen lacks. The enumeration stops at the enumName
// fallback, such as "PredicateOp(11)", which marks the end of the enum.
func requireAll[E interface {
	~uint8
	fmt.Stringer
}](t *testing.T, what string, seen map[E]bool) {
	t.Helper()
	for i := 0; i <= 0xff; i++ {
		e := E(i)
		name := e.String()
		if name == fmt.Sprintf("%s(%d)", reflect.TypeFor[E]().Name(), i) {
			if i == 0 {
				t.Errorf("%s has no named value", reflect.TypeFor[E]())
			}
			return
		}
		if !seen[e] {
			t.Errorf("no filter fixture uses %s %q", what, name)
		}
	}
	t.Errorf("%s has no fallback name below 256", reflect.TypeFor[E]())
}

// graphQLFilterArgs returns the string values of every argument or input
// object field named filter, at any depth, in the GraphQL documents
// dir/*.graphql. A missing dir holds no filter.
func graphQLFilterArgs(dir string) ([]string, error) {
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.graphql"))
	if err != nil {
		return nil, err
	}
	var out []string
	var value func(name string, v *ast.Value)
	value = func(name string, v *ast.Value) {
		if v == nil {
			return
		}
		if name == "filter" && (v.Kind == ast.StringValue || v.Kind == ast.BlockValue) {
			out = append(out, v.Raw)
		}
		for _, c := range v.Children {
			value(c.Name, c.Value)
		}
	}
	args := func(list ast.ArgumentList) {
		for _, a := range list {
			value(a.Name, a.Value)
		}
	}
	directives := func(list ast.DirectiveList) {
		for _, d := range list {
			args(d.Arguments)
		}
	}
	var selections func(ast.SelectionSet)
	selections = func(set ast.SelectionSet) {
		for _, s := range set {
			switch s := s.(type) {
			case *ast.Field:
				args(s.Arguments)
				directives(s.Directives)
				selections(s.SelectionSet)
			case *ast.InlineFragment:
				directives(s.Directives)
				selections(s.SelectionSet)
			case *ast.FragmentSpread:
				directives(s.Directives)
			}
		}
	}
	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		doc, err := parser.ParseQuery(&ast.Source{Name: file, Input: string(src)})
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		for _, op := range doc.Operations {
			for _, v := range op.VariableDefinitions {
				value(v.Variable, v.DefaultValue)
			}
			directives(op.Directives)
			selections(op.SelectionSet)
		}
		for _, f := range doc.Fragments {
			directives(f.Directives)
			selections(f.SelectionSet)
		}
	}
	return out, nil
}
