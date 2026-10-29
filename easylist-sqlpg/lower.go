package sqlpg

import (
	"fmt"
	"strings"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	filter "github.com/mgpnd/easylist-go/easylist-filter"
)

// Lower validates p and lowers it into a Query. Every validation error a
// PostgreSQL driver reports comes from here, so all drivers fail with the
// same text. Lower never evaluates expressions; it uses the values the
// planner stored on p. It is safe for concurrent use.
func Lower(p *easylist.Plan) (*Query, error) {
	from := p.Table + " AS " + p.Alias
	for _, j := range p.Joins {
		from += " " + j.SQL
	}
	q := &Query{From: Expr{SQL: from}}

	if p.Scope != "" {
		q.Where = Raw{SQL: p.Scope}
	}
	if p.Filter != nil {
		cond, err := lowerFilter(p, p.Filter)
		if err != nil {
			return nil, err
		}
		if q.Where != nil {
			q.Where = And{Left: q.Where, Right: cond}
		} else {
			q.Where = cond
		}
	}

	switch p.Kind {
	case easylist.KindCount:
		q.Count = true
	case easylist.KindPage, easylist.KindExport:
		if len(p.Fields) == 0 {
			return nil, fmt.Errorf("plan for list %q selects no fields", p.List)
		}
		q.Select = make([]Column, len(p.Fields))
		for i, f := range p.Fields {
			expr := fieldExpression(p, f, false)
			if f.Array {
				// Arrays reach the runtime as JSON text; to_json keeps NULL arrays NULL.
				expr.SQL = "to_json(" + expr.SQL + ")::text"
			}
			q.Select[i] = Column{Expr: expr, Alias: f.Name}
		}
		q.Order = make([]Ordering, len(p.Order))
		for i, o := range p.Order {
			q.Order[i] = Ordering{Expr: fieldExpression(p, o.Field, false), Desc: o.Desc}
		}
		if p.Kind == easylist.KindPage {
			q.Page = &Page{Limit: int64(p.Limit), Offset: int64(p.Offset)}
		}
	default:
		return nil, fmt.Errorf("invalid plan kind %d", p.Kind)
	}
	return q, nil
}

// fieldExpression returns a field's author SQL, or the quoted column of the
// base table alias. Field names match ^[a-z_][a-z0-9_]*$, so quoting them
// needs no escaping. Inside WHERE, author SQL is parenthesized so operators
// inside it cannot bind to the comparison.
func fieldExpression(p *easylist.Plan, f easylist.PlanField, parenthesize bool) Expr {
	if f.SQL == "" {
		return Expr{SQL: p.Alias + `."` + strings.ReplaceAll(f.Name, `"`, `""`) + `"`}
	}
	if parenthesize {
		return Expr{SQL: "(" + f.SQL + ")"}
	}
	return Expr{SQL: f.SQL}
}

// lowerFilter walks the public syntax tree in postorder without recursion.
// Visiting left children first preserves validation and argument order.
func lowerFilter(p *easylist.Plan, root filter.Expr) (Cond, error) {
	if root == nil {
		return nil, fmt.Errorf("empty filter syntax tree")
	}
	type frame struct {
		node    filter.Expr
		visited bool
	}
	frames := []frame{{node: root}}
	var conds []Cond
	for len(frames) > 0 {
		current := frames[len(frames)-1]
		frames = frames[:len(frames)-1]
		switch node := current.node.(type) {
		case filter.Predicate:
			cond, err := buildPredicate(p, node)
			if err != nil {
				return nil, err
			}
			conds = append(conds, cond)
		case filter.Logical:
			if node.Operator() != filter.And && node.Operator() != filter.Or {
				return nil, fmt.Errorf("invalid connector at position %d", node.OperatorPos())
			}
			if !current.visited {
				frames = append(frames, frame{node: node, visited: true}, frame{node: node.Right()}, frame{node: node.Left()})
				continue
			}
			if len(conds) < 2 {
				return nil, fmt.Errorf("incomplete expression at position %d", node.OperatorPos())
			}
			right, left := conds[len(conds)-1], conds[len(conds)-2]
			conds = conds[:len(conds)-2]
			if node.Operator() == filter.And {
				conds = append(conds, And{Left: left, Right: right})
			} else {
				conds = append(conds, Or{Left: left, Right: right})
			}
		default:
			return nil, fmt.Errorf("invalid filter syntax tree")
		}
	}
	if len(conds) != 1 {
		return nil, fmt.Errorf("incomplete filter syntax tree")
	}
	return conds[0], nil
}
