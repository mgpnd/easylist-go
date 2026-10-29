// Package sqlgorm is an easylist driver that compiles query plans into
// prepared PostgreSQL with gorm's statement builder.
package sqlgorm

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	"github.com/mgpnd/easylist-go/easylist-sqlpg"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Driver compiles easylist plans into PostgreSQL. It holds no state and needs
// no gorm handle, so one value is safe for concurrent use.
type Driver struct{}

// New returns a PostgreSQL driver.
func New() *Driver { return &Driver{} }

var _ easylist.Driver = (*Driver)(nil)

// Compile returns one prepared query with $n placeholders and its arguments.
// Arguments follow the filter left to right, then limit and offset. Author
// SQL is emitted verbatim; request values are always bound. Every validation
// error comes from sqlpg.Lower.
func (*Driver) Compile(p *easylist.Plan) (string, []any, error) {
	q, err := sqlpg.Lower(p)
	if err != nil {
		return "", nil, err
	}

	// A new session gets a fresh statement, so concurrent compilations share
	// only the handle's read-only configuration.
	tx := db.Session(&gorm.Session{NewDB: true})
	clauses := make([]clause.Expression, 0, 3)

	if q.Count {
		clauses = append(clauses, clause.Select{Expression: clause.Expr{SQL: "COUNT(*)"}})
	} else {
		cols := make([]clause.Expression, len(q.Select))
		for i, c := range q.Select {
			cols[i] = clause.Expr{SQL: "? AS ?", Vars: []any{leaf(c.Expr), clause.Column{Name: c.Alias}}}
		}
		clauses = append(clauses, clause.Select{Expression: clause.CommaExpression{Exprs: cols}})
		if len(q.Order) > 0 {
			order := make([]clause.Expression, len(q.Order))
			for i, o := range q.Order {
				if o.Desc {
					order[i] = clause.Expr{SQL: "? DESC", Vars: []any{leaf(o.Expr)}}
				} else {
					order[i] = clause.Expr{SQL: "? ASC", Vars: []any{leaf(o.Expr)}}
				}
			}
			clauses = append(clauses, clause.OrderBy{Expression: clause.CommaExpression{Exprs: order}})
		}
	}

	if q.Where != nil {
		where, err := condition(q.Where)
		if err != nil {
			return "", nil, fmt.Errorf("list %q: %w", p.List, err)
		}
		clauses = append(clauses, clause.Where{Exprs: []clause.Expression{where}})
	}

	tx = tx.Clauses(clauses...)
	// The FROM text is a zero-var expression, which gorm writes verbatim.
	tx.Statement.TableExpr = &clause.Expr{SQL: q.From.SQL}
	tx = tx.Find(&[]map[string]any{})
	if tx.Error != nil {
		return "", nil, fmt.Errorf("list %q: gorm: %w", p.List, tx.Error)
	}

	query := tx.Statement.SQL.String()
	args := append([]any{}, tx.Statement.Vars...)
	if q.Page != nil {
		// gorm drops OFFSET 0; binding both keeps the query shape fixed.
		n := len(args)
		query += " LIMIT $" + strconv.Itoa(n+1) + " OFFSET $" + strconv.Itoa(n+2)
		args = append(args, q.Page.Limit, q.Page.Offset)
	}
	return query, args, nil
}

// leaf is author or generated SQL as a gorm expression without vars, which
// gorm writes verbatim without scanning it for ?. Passed as a var of another
// expression, it is built in place rather than bound.
func leaf(e sqlpg.Expr) clause.Expr { return clause.Expr{SQL: e.SQL} }

// condition maps a lowered condition onto gorm. And and Or become gorm's
// connectors, which parenthesize both sides as their fragments do; at the top
// of WHERE, gorm writes an And without its outer parentheses. Every other
// node is rendered through its fragment, so the SQL is the canonical
// template; gorm's native predicates quote their column, which would break
// author SQL.
func condition(c sqlpg.Cond) (clause.Expression, error) {
	switch c := c.(type) {
	case sqlpg.And:
		left, right, err := conditions(c.Left, c.Right)
		if err != nil {
			return nil, err
		}
		return clause.And(left, right), nil
	case sqlpg.Or:
		left, right, err := conditions(c.Left, c.Right)
		if err != nil {
			return nil, err
		}
		return clause.Or(left, right), nil
	}
	return fragment(c.Fragment())
}

// conditions maps the two sides of a connector, left first.
func conditions(left, right sqlpg.Cond) (clause.Expression, clause.Expression, error) {
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

// fragment renders a sqlpg fragment as one gorm expression. An Expr slot
// becomes a leaf, which gorm builds verbatim. An Arg slot passes its value,
// which gorm binds as the next $n; a numeric cast already follows the slot in
// the template. gorm expands a slice after "(" and appends surplus vars as
// extra arguments, so every slot must be a scalar and the template must have
// exactly one ? per slot.
func fragment(f sqlpg.Fragment) (clause.Expression, error) {
	if n := strings.Count(f.SQL, "?"); n != len(f.Slots) {
		return nil, fmt.Errorf("condition template has %d slots for %d values", n, len(f.Slots))
	}
	vars := make([]any, len(f.Slots))
	for i, slot := range f.Slots {
		switch s := slot.(type) {
		case sqlpg.Expr:
			vars[i] = leaf(s)
		case sqlpg.Arg:
			if k := reflect.ValueOf(s.Value).Kind(); k == reflect.Slice || k == reflect.Array {
				return nil, fmt.Errorf("unsupported condition value of type %T", s.Value)
			}
			vars[i] = s.Value
		default:
			return nil, fmt.Errorf("unsupported condition slot of type %T", slot)
		}
	}
	return template{clause.Expr{SQL: f.SQL, Vars: vars}}, nil
}

// template is a fragment's clause.Expr. gorm's connectors wrap a plain
// clause.Expr whose text contains " AND " or " OR " in another pair of
// parentheses; a fragment is already self-contained, so the distinct type
// keeps its SQL exactly as written.
type template struct{ clause.Expr }
