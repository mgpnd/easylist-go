package sqljet

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"

	"github.com/mgpnd/easylist-go/easylist-sqlpg"

	"github.com/go-jet/jet/v2/postgres"
)

// condition maps a lowered condition onto jet. And and Or become jet's AND
// and OR, which parenthesize both sides, so the tree's grouping is kept; at
// the top of WHERE, jet writes the outermost one without parentheses. Every
// other node is rendered through its fragment. jet's native operators need
// typed operands, and its typed literals add casts such as $1::text, so none
// is used.
func condition(c sqlpg.Cond) (postgres.BoolExpression, error) {
	switch c := c.(type) {
	case sqlpg.And:
		left, right, err := conditions(c.Left, c.Right)
		if err != nil {
			return nil, err
		}
		return left.AND(right), nil
	case sqlpg.Or:
		left, right, err := conditions(c.Left, c.Right)
		if err != nil {
			return nil, err
		}
		return left.OR(right), nil
	}
	return fragment(c.Fragment())
}

// conditions maps the two sides of a connector, left first.
func conditions(left, right sqlpg.Cond) (postgres.BoolExpression, postgres.BoolExpression, error) {
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

// fragment renders a sqlpg fragment as one jet CustomExpression. The
// template text between slots becomes Tokens, which jet writes as is. An
// Expr slot is a Raw without named arguments, which jet writes verbatim,
// without looking for argument names in it. An Arg slot is a Raw of one
// named argument, "#v", whose text is only that name and the value's cast,
// so jet replaces exactly the name with the next $n and binds the value;
// author SQL never shares a Raw with an argument name, which jet would
// replace inside it. jet writes each Raw in parentheses and may add a space
// between parts, which changes no meaning.
func fragment(f sqlpg.Fragment) (postgres.BoolExpression, error) {
	text := strings.Split(f.SQL, "?")
	if len(text) != len(f.Slots)+1 {
		return nil, fmt.Errorf("condition template has %d slots for %d values", len(text)-1, len(f.Slots))
	}
	parts := make([]postgres.Serializer, 0, 2*len(f.Slots)+1)
	if text[0] != "" {
		parts = append(parts, postgres.Token(text[0]))
	}
	for i, slot := range f.Slots {
		next := text[i+1]
		switch s := slot.(type) {
		case sqlpg.Expr:
			parts = append(parts, postgres.Raw(s.SQL))
		case sqlpg.Arg:
			if err := checkValue(s.Value); err != nil {
				return nil, err
			}
			raw := argName
			if s.Cast != "" {
				// The cast moves into the Raw, so that it follows the
				// placeholder directly rather than jet's parentheses.
				cast := "::" + s.Cast
				if !strings.HasPrefix(next, cast) {
					return nil, fmt.Errorf("condition template does not cast slot %d to %s: %s", i+1, s.Cast, f.SQL)
				}
				raw, next = raw+cast, next[len(cast):]
			}
			parts = append(parts, postgres.Raw(raw, postgres.RawArgs{argName: s.Value}))
		default:
			return nil, fmt.Errorf("unsupported condition slot of type %T", slot)
		}
		if next != "" {
			parts = append(parts, postgres.Token(next))
		}
	}
	return postgres.BoolExp(postgres.CustomExpression(parts...)), nil
}

// argName is the one named argument of each value's Raw, and its whole text.
const argName = "#v"

// jetPath is the module path of jet, whose values describe SQL rather than
// being arguments.
const jetPath = "github.com/go-jet/jet/"

// checkValue fails for a value that is not one plain argument. jet appends a
// named argument's value to the arguments as is, so the guard is about what
// the value means there: nil or a nil pointer would bind NULL, a slice or
// array, []byte included, would bind as one array or bytea argument, a
// sql.NamedArg would be read by database/sql as a named parameter, and a jet
// value describes SQL, which an argument would not write. sqlpg produces
// none of them.
func checkValue(v any) error {
	switch v.(type) {
	case nil:
		return fmt.Errorf("unsupported NULL condition value")
	case postgres.Serializer, sql.NamedArg, *sql.NamedArg:
		return fmt.Errorf("unsupported condition value of type %T", v)
	}
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Pointer && rv.IsNil() {
		return fmt.Errorf("unsupported NULL condition value of type %T", v)
	}
	t := reflect.TypeOf(v)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if strings.HasPrefix(t.PkgPath(), jetPath) {
		return fmt.Errorf("unsupported condition value of type %T", v)
	}
	if k := t.Kind(); k == reflect.Slice || k == reflect.Array {
		return fmt.Errorf("unsupported condition value of type %T", v)
	}
	return nil
}
