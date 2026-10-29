package sqlbob

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"

	"github.com/mgpnd/easylist-go/easylist-sqlpg"

	"github.com/stephenafamo/bob"
	"github.com/stephenafamo/bob/dialect/psql"
	"github.com/stephenafamo/bob/expr"
)

// condition maps a lowered condition onto bob. And and Or become psql.And
// and psql.Or, which parenthesize every connector, so the tree's grouping is
// kept. Compare, IsNull, Between, and In with SQL's own NULL result map to
// bob's native operators on the column leaf when no value is cast; bob
// parenthesizes each of them and binds the same values in the same order as
// the fragment. Every other node is rendered through its fragment.
func condition(c sqlpg.Cond) (bob.Expression, error) {
	switch c := c.(type) {
	case sqlpg.And:
		left, right, err := conditions(c.Left, c.Right)
		if err != nil {
			return nil, err
		}
		return psql.And(left, right), nil
	case sqlpg.Or:
		left, right, err := conditions(c.Left, c.Right)
		if err != nil {
			return nil, err
		}
		return psql.Or(left, right), nil
	case sqlpg.Compare:
		if c.Arg.Cast == "" {
			if e, ok, err := compare(c); ok || err != nil {
				return e, err
			}
		}
	case sqlpg.IsNull:
		if c.Not {
			return leaf(c.Col).IsNotNull(), nil
		}
		return leaf(c.Col).IsNull(), nil
	case sqlpg.In:
		if len(c.Args) == 0 {
			return condition(sqlpg.IsNull{Col: c.Col, Not: c.Not})
		}
		if c.Null == sqlpg.NullKeep && uncast(c.Args...) {
			vals, err := values(c.Args...)
			if err != nil {
				return nil, err
			}
			// One psql.Arg over every value writes "$n, $m"; In and NotIn
			// put it in parentheses.
			if c.Not {
				return leaf(c.Col).NotIn(psql.Arg(vals...)), nil
			}
			return leaf(c.Col).In(psql.Arg(vals...)), nil
		}
	case sqlpg.Between:
		if uncast(c.Low, c.High) {
			vals, err := values(c.Low, c.High)
			if err != nil {
				return nil, err
			}
			return leaf(c.Col).Between(psql.Arg(vals[0]), psql.Arg(vals[1])), nil
		}
	}
	return fragment(c.Fragment())
}

// conditions maps the two sides of a connector, left first.
func conditions(left, right sqlpg.Cond) (bob.Expression, bob.Expression, error) {
	l, err := condition(left)
	if err != nil {
		return nil, nil, err
	}
	r, err := condition(right)
	if err != nil {
		return nil, nil, err
	}
	return l, r, nil
}

// compare maps an uncast comparison to bob's operator of the same spelling.
// It reports false for an operator bob has no method for.
func compare(c sqlpg.Compare) (bob.Expression, bool, error) {
	vals, err := values(c.Arg)
	if err != nil {
		return nil, false, err
	}
	col, arg := leaf(c.Col), psql.Arg(vals[0])
	switch c.Op {
	case sqlpg.OpEq:
		return col.EQ(arg), true, nil
	case sqlpg.OpDistinct:
		return col.IsDistinctFrom(arg), true, nil
	case sqlpg.OpGt:
		return col.GT(arg), true, nil
	case sqlpg.OpGte:
		return col.GTE(arg), true, nil
	case sqlpg.OpLt:
		return col.LT(arg), true, nil
	case sqlpg.OpLte:
		return col.LTE(arg), true, nil
	}
	return nil, false, nil
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

// fragment renders a sqlpg fragment with psql.Raw, which numbers each ? of
// the template as the next $n. The template holds no author SQL: every Expr
// slot is an expr.Raw argument, which bob writes in place of its ? verbatim,
// without scanning it, and every Arg slot is a psql.Arg, which bob binds as
// one $n; a numeric cast already follows the slot in the template. bob reads
// \? as an escaped question mark and writes surplus ? as NULL arguments, so
// the template must have exactly one ? per slot and no \?.
func fragment(f sqlpg.Fragment) (bob.Expression, error) {
	if strings.Contains(f.SQL, `\?`) {
		return nil, fmt.Errorf("condition template contains an escaped placeholder: %s", f.SQL)
	}
	if n := strings.Count(f.SQL, "?"); n != len(f.Slots) {
		return nil, fmt.Errorf("condition template has %d slots for %d values", n, len(f.Slots))
	}
	args := make([]any, len(f.Slots))
	for i, slot := range f.Slots {
		switch s := slot.(type) {
		case sqlpg.Expr:
			args[i] = expr.Raw(s.SQL)
		case sqlpg.Arg:
			if err := checkValue(s.Value); err != nil {
				return nil, err
			}
			args[i] = psql.Arg(s.Value)
		default:
			return nil, fmt.Errorf("unsupported condition slot of type %T", slot)
		}
	}
	return psql.Raw(f.SQL, args...), nil
}

// checkValue fails for a value that is not one plain argument. psql.Arg
// binds any value as one $n, but nil would bind NULL, a bob expression or a
// named argument would reach database/sql as an object bob is meant to
// write, and a slice would bind as one array argument; sqlpg produces none
// of them.
func checkValue(v any) error {
	switch v.(type) {
	case nil:
		return fmt.Errorf("unsupported NULL condition value")
	case bob.Expression, sql.NamedArg, *sql.NamedArg:
		return fmt.Errorf("unsupported condition value of type %T", v)
	}
	if k := reflect.ValueOf(v).Kind(); k == reflect.Slice || k == reflect.Array {
		return fmt.Errorf("unsupported condition value of type %T", v)
	}
	return nil
}
