package sqlsqlbuilder

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"

	"github.com/mgpnd/easylist-go/easylist-sqlpg"

	"github.com/huandu/go-sqlbuilder"
)

// condition maps a lowered condition onto go-sqlbuilder and returns builder
// text whose references c's Args resolve when the statement is built. And and
// Or become c.And and c.Or, which parenthesize every connector, so the tree's
// grouping is kept. Compare, IsNull, Between, and In with SQL's own NULL
// result map to c's native methods when no value is cast; each writes exactly
// the node's fragment, with the same values in the same order. Every other
// node is rendered through its fragment.
//
// The native methods write their field into the statement as is when it is
// built, without reading $ in it, so the column is passed unescaped. They
// return "" for an empty field, which a connector would drop, so an empty
// column takes the fragment.
func condition(c *sqlbuilder.Cond, n sqlpg.Cond) (string, error) {
	switch n := n.(type) {
	case sqlpg.And:
		left, right, err := conditions(c, n.Left, n.Right)
		if err != nil {
			return "", err
		}
		return c.And(left, right), nil
	case sqlpg.Or:
		left, right, err := conditions(c, n.Left, n.Right)
		if err != nil {
			return "", err
		}
		return c.Or(left, right), nil
	case sqlpg.Compare:
		if n.Col.SQL != "" && n.Arg.Cast == "" {
			if s, ok, err := compare(c, n); ok || err != nil {
				return s, err
			}
		}
	case sqlpg.IsNull:
		if n.Col.SQL != "" {
			if n.Not {
				return c.IsNotNull(n.Col.SQL), nil
			}
			return c.IsNull(n.Col.SQL), nil
		}
	case sqlpg.In:
		if len(n.Args) == 0 {
			return condition(c, sqlpg.IsNull{Col: n.Col, Not: n.Not})
		}
		if n.Col.SQL != "" && n.Null == sqlpg.NullKeep && uncast(n.Args...) {
			vals, err := values(n.Args...)
			if err != nil {
				return "", err
			}
			// In and NotIn write each value as one $n, separated by ", ".
			if n.Not {
				return c.NotIn(n.Col.SQL, vals...), nil
			}
			return c.In(n.Col.SQL, vals...), nil
		}
	case sqlpg.Between:
		if n.Col.SQL != "" && uncast(n.Low, n.High) {
			vals, err := values(n.Low, n.High)
			if err != nil {
				return "", err
			}
			return c.Between(n.Col.SQL, vals[0], vals[1]), nil
		}
	}
	return fragment(c, n.Fragment())
}

// conditions maps the two sides of a connector, left first.
func conditions(c *sqlbuilder.Cond, left, right sqlpg.Cond) (string, string, error) {
	l, err := condition(c, left)
	if err != nil {
		return "", "", err
	}
	r, err := condition(c, right)
	if err != nil {
		return "", "", err
	}
	return l, r, nil
}

// compare maps an uncast comparison to go-sqlbuilder's method of the same
// operator. It reports false for an operator go-sqlbuilder has no method for.
func compare(c *sqlbuilder.Cond, n sqlpg.Compare) (string, bool, error) {
	vals, err := values(n.Arg)
	if err != nil {
		return "", false, err
	}
	col, v := n.Col.SQL, vals[0]
	switch n.Op {
	case sqlpg.OpEq:
		return c.Equal(col, v), true, nil
	case sqlpg.OpDistinct:
		// The PostgreSQL flavor writes IS DISTINCT FROM itself; other flavors
		// would emulate it.
		return c.IsDistinctFrom(col, v), true, nil
	case sqlpg.OpGt:
		return c.GreaterThan(col, v), true, nil
	case sqlpg.OpGte:
		return c.GreaterEqualThan(col, v), true, nil
	case sqlpg.OpLt:
		return c.LessThan(col, v), true, nil
	case sqlpg.OpLte:
		return c.LessEqualThan(col, v), true, nil
	}
	return "", false, nil
}

// uncast reports whether no argument has a cast.
func uncast(args ...sqlpg.Arg) bool {
	for _, a := range args {
		if a.Cast != "" {
			return false
		}
	}
	return true
}

// values returns the arguments' values, checked with checkValue.
func values(args ...sqlpg.Arg) ([]any, error) {
	vals := make([]any, len(args))
	for i, a := range args {
		if err := checkValue(a.Value); err != nil {
			return nil, err
		}
		vals[i] = a.Value
	}
	return vals, nil
}

// fragment renders a sqlpg fragment as builder text. Each ? of the template
// becomes its slot: an Expr is written as escaped text, which the builder
// writes back verbatim, and an Arg is c.Var of its value, a reference the
// builder writes as one $n; a numeric cast already follows the slot in the
// template. The template's own text is escaped too. The template must have
// exactly one ? per slot, and no digit may follow a reference, which would
// make it a reference to another argument.
func fragment(c *sqlbuilder.Cond, f sqlpg.Fragment) (string, error) {
	parts := strings.Split(f.SQL, "?")
	if len(parts)-1 != len(f.Slots) {
		return "", fmt.Errorf("condition template has %d slots for %d values", len(parts)-1, len(f.Slots))
	}
	var b strings.Builder
	afterVar := false
	write := func(s string) error {
		if afterVar && s != "" && '0' <= s[0] && s[0] <= '9' {
			return fmt.Errorf("condition template has a digit after a value slot: %s", f.SQL)
		}
		if s != "" {
			afterVar = false
		}
		b.WriteString(s)
		return nil
	}
	if err := write(text(parts[0])); err != nil {
		return "", err
	}
	for i, slot := range f.Slots {
		switch s := slot.(type) {
		case sqlpg.Expr:
			if err := write(text(s.SQL)); err != nil {
				return "", err
			}
		case sqlpg.Arg:
			if err := checkValue(s.Value); err != nil {
				return "", err
			}
			if err := write(c.Var(s.Value)); err != nil {
				return "", err
			}
			afterVar = true
		default:
			return "", fmt.Errorf("unsupported condition slot of type %T", slot)
		}
		if err := write(text(parts[i+1])); err != nil {
			return "", err
		}
	}
	return b.String(), nil
}

// libraryPath is the import path of go-sqlbuilder's package.
var libraryPath = reflect.TypeOf(sqlbuilder.Raw("")).PkgPath()

// checkValue fails for a value that is not one plain argument. go-sqlbuilder
// writes a Builder as nested SQL, its own Raw, List, Tuple, and Named values
// as SQL text, lists, or shared references, and a sql.NamedArg as @name; nil
// would bind NULL, and a slice would bind as one array argument. sqlpg
// produces none of them.
func checkValue(v any) error {
	switch v.(type) {
	case nil:
		return fmt.Errorf("unsupported NULL condition value")
	case sqlbuilder.Builder, sql.NamedArg, *sql.NamedArg:
		return fmt.Errorf("unsupported condition value of type %T", v)
	}
	t := reflect.TypeOf(v)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.PkgPath() == libraryPath {
		return fmt.Errorf("unsupported condition value of type %T", v)
	}
	if k := t.Kind(); k == reflect.Slice || k == reflect.Array {
		return fmt.Errorf("unsupported condition value of type %T", v)
	}
	return nil
}
