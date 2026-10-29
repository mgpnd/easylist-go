package sqlgoqu

import (
	"github.com/mgpnd/easylist-go/easylist-sqlpg"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
)

// condition maps a lowered condition onto goqu. And and Or become goqu's
// connectors, which parenthesize both sides as their fragments do. Every
// other node is rendered through its fragment, so the SQL is exactly the
// canonical template; goqu has no native construct with the same output.
func condition(c sqlpg.Cond) (exp.Expression, error) {
	switch c := c.(type) {
	case sqlpg.And:
		left, right, err := conditions(c.Left, c.Right)
		if err != nil {
			return nil, err
		}
		return goqu.And(left, right), nil
	case sqlpg.Or:
		left, right, err := conditions(c.Left, c.Right)
		if err != nil {
			return nil, err
		}
		return goqu.Or(left, right), nil
	}
	return fragment(c.Fragment())
}

// conditions maps the two sides of a connector, left first.
func conditions(left, right sqlpg.Cond) (exp.Expression, exp.Expression, error) {
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
