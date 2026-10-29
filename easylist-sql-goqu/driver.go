// Package sqlgoqu is an easylist driver that compiles query plans into
// prepared PostgreSQL with goqu.
package sqlgoqu

import (
	"fmt"
	"strconv"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	"github.com/mgpnd/easylist-go/easylist-sqlpg"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/doug-martin/goqu/v9/exp"
)

// Driver compiles easylist plans into PostgreSQL. It holds no state, so one
// value is safe for concurrent use.
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
	ds := goqu.Dialect("postgres").From(goqu.L(q.From.SQL)).Prepared(true)

	if q.Where != nil {
		where, err := condition(q.Where)
		if err != nil {
			return "", nil, fmt.Errorf("list %q: %w", p.List, err)
		}
		ds = ds.Where(where)
	}

	if q.Count {
		ds = ds.Select(goqu.COUNT(goqu.Star()))
	} else {
		cols := make([]any, len(q.Select))
		for i, c := range q.Select {
			cols[i] = goqu.L(c.Expr.SQL).As(goqu.C(c.Alias))
		}
		ds = ds.Select(cols...)
		order := make([]exp.OrderedExpression, len(q.Order))
		for i, o := range q.Order {
			if o.Desc {
				order[i] = goqu.L(o.Expr.SQL).Desc()
			} else {
				order[i] = goqu.L(o.Expr.SQL).Asc()
			}
		}
		ds = ds.Order(order...)
	}

	query, args, err := ds.ToSQL()
	if err != nil {
		return "", nil, err
	}
	if q.Page != nil {
		// goqu drops OFFSET 0; binding both keeps the query shape fixed.
		n := len(args)
		query += " LIMIT $" + strconv.Itoa(n+1) + " OFFSET $" + strconv.Itoa(n+2)
		args = append(args, q.Page.Limit, q.Page.Offset)
	}
	return query, args, nil
}
