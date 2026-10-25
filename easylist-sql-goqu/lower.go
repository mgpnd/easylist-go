package sqlgoqu

import (
	"fmt"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	filter "github.com/mgpnd/easylist-go/easylist-filter"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
)

// lowerFilter walks the public syntax tree in postorder without recursion.
// Visiting left children first preserves validation and prepared argument order.
func lowerFilter(p *easylist.Plan, root filter.Expr) (exp.Expression, error) {
	if root == nil {
		return nil, fmt.Errorf("empty filter syntax tree")
	}
	type frame struct {
		node    filter.Expr
		visited bool
	}
	frames := []frame{{node: root}}
	var expressions []exp.Expression
	for len(frames) > 0 {
		current := frames[len(frames)-1]
		frames = frames[:len(frames)-1]
		switch node := current.node.(type) {
		case filter.Predicate:
			predicate, err := buildPredicate(p, node)
			if err != nil {
				return nil, err
			}
			expressions = append(expressions, predicate)
		case filter.Logical:
			if node.Operator() != filter.And && node.Operator() != filter.Or {
				return nil, fmt.Errorf("invalid connector at position %d", node.OperatorPos())
			}
			if !current.visited {
				frames = append(frames, frame{node: node, visited: true}, frame{node: node.Right()}, frame{node: node.Left()})
				continue
			}
			if len(expressions) < 2 {
				return nil, fmt.Errorf("incomplete expression at position %d", node.OperatorPos())
			}
			right, left := expressions[len(expressions)-1], expressions[len(expressions)-2]
			expressions = expressions[:len(expressions)-2]
			if node.Operator() == filter.And {
				expressions = append(expressions, goqu.And(left, right))
			} else {
				expressions = append(expressions, goqu.Or(left, right))
			}
		default:
			return nil, fmt.Errorf("invalid filter syntax tree")
		}
	}
	if len(expressions) != 1 {
		return nil, fmt.Errorf("incomplete filter syntax tree")
	}
	return expressions[0], nil
}
