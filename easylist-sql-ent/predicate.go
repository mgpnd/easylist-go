package sqlent

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/mgpnd/easylist-go/easylist-sqlpg"

	"entgo.io/ent/dialect/sql"
)

// condition maps a lowered condition onto ent. And and Or become sql.And and
// sql.Or, which parenthesize a nested connector; at the top of WHERE, ent
// writes the outermost one without parentheses. Every other node is rendered
// through its fragment, so the SQL is the canonical template; ent's native
// predicates quote their column, which would break author SQL.
func condition(c sqlpg.Cond) (*sql.Predicate, error) {
	switch c := c.(type) {
	case sqlpg.And:
		left, right, err := conditions(c.Left, c.Right)
		if err != nil {
			return nil, err
		}
		return sql.And(left, right), nil
	case sqlpg.Or:
		left, right, err := conditions(c.Left, c.Right)
		if err != nil {
			return nil, err
		}
		return sql.Or(left, right), nil
	}
	return fragment(c.Fragment())
}

// conditions maps the two sides of a connector, left first.
func conditions(left, right sqlpg.Cond) (*sql.Predicate, *sql.Predicate, error) {
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

// fragment renders a sqlpg fragment as one ent predicate with a single
// builder function, which ent's connectors join without extra parentheses.
// The template text between slots and every Expr slot are written with
// WriteString, which copies them without scanning for ? or $. Every Arg slot
// goes through Arg, which binds it as the next $n; a numeric cast already
// follows the slot in the template. Arg writes nil as NULL, a Querier as SQL,
// and a ParamFormatter in its own format, so such values are rejected; ent
// would bind a slice as one argument, which is never meant either.
func fragment(f sqlpg.Fragment) (*sql.Predicate, error) {
	text := strings.Split(f.SQL, "?")
	if len(text) != len(f.Slots)+1 {
		return nil, fmt.Errorf("condition template has %d slots for %d values", len(text)-1, len(f.Slots))
	}
	for _, slot := range f.Slots {
		switch s := slot.(type) {
		case sqlpg.Expr:
		case sqlpg.Arg:
			if err := checkValue(s.Value); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("unsupported condition slot of type %T", slot)
		}
	}
	return sql.P(func(b *sql.Builder) {
		b.WriteString(text[0])
		for i, slot := range f.Slots {
			switch s := slot.(type) {
			case sqlpg.Expr:
				b.WriteString(s.SQL)
			case sqlpg.Arg:
				b.Arg(s.Value)
			}
			b.WriteString(text[i+1])
		}
	}), nil
}

// checkValue fails for a value that ent's Builder.Arg would not bind as
// exactly one $n placeholder.
func checkValue(v any) error {
	switch v.(type) {
	case nil:
		return fmt.Errorf("unsupported NULL condition value")
	case sql.Querier, sql.ParamFormatter:
		return fmt.Errorf("unsupported condition value of type %T", v)
	}
	if k := reflect.ValueOf(v).Kind(); k == reflect.Slice || k == reflect.Array {
		return fmt.Errorf("unsupported condition value of type %T", v)
	}
	return nil
}
