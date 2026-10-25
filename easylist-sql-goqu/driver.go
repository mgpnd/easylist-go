// Package sqlgoqu is an easylist driver that compiles query plans into
// prepared PostgreSQL with goqu.
package sqlgoqu

import (
	"fmt"
	"strconv"
	"strings"

	easylist "github.com/mgpnd/easylist-go/easylist-core"

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
// SQL is emitted verbatim; request values are always bound.
func (*Driver) Compile(p *easylist.Plan) (string, []any, error) {
	from := p.Table + " AS " + p.Alias
	for _, j := range p.Joins {
		from += " " + j.SQL
	}
	ds := goqu.Dialect("postgres").From(goqu.L(from)).Prepared(true)

	var where []exp.Expression
	if p.Scope != "" {
		where = append(where, goqu.L("("+p.Scope+")"))
	}
	if p.Filter != nil {
		expr, err := lowerFilter(p, p.Filter)
		if err != nil {
			return "", nil, err
		}
		where = append(where, expr)
	}
	if len(where) > 0 {
		ds = ds.Where(goqu.And(where...))
	}

	switch p.Kind {
	case easylist.KindCount:
		ds = ds.Select(goqu.COUNT(goqu.Star()))
	case easylist.KindPage, easylist.KindExport:
		if len(p.Fields) == 0 {
			return "", nil, fmt.Errorf("plan for list %q selects no fields", p.List)
		}
		cols := make([]any, len(p.Fields))
		for i, f := range p.Fields {
			cols[i] = fieldExpression(p, f, false).As(goqu.C(f.Name))
		}
		ds = ds.Select(cols...)
		order := make([]exp.OrderedExpression, len(p.Order))
		for i, o := range p.Order {
			if o.Desc {
				order[i] = fieldExpression(p, o.Field, false).Desc()
			} else {
				order[i] = fieldExpression(p, o.Field, false).Asc()
			}
		}
		ds = ds.Order(order...)
	default:
		return "", nil, fmt.Errorf("invalid plan kind %d", p.Kind)
	}

	query, args, err := ds.ToSQL()
	if err != nil {
		return "", nil, err
	}
	if p.Kind == easylist.KindPage {
		// goqu drops OFFSET 0; binding both keeps the query shape fixed.
		n := len(args)
		query += " LIMIT $" + strconv.Itoa(n+1) + " OFFSET $" + strconv.Itoa(n+2)
		args = append(args, int64(p.Limit), int64(p.Offset))
	}
	return query, args, nil
}

// fieldExpression returns a field's author SQL, or the quoted column of the
// base table alias. Field names match ^[a-z_][a-z0-9_]*$, so quoting them
// needs no escaping.
func fieldExpression(p *easylist.Plan, f easylist.PlanField, parenthesize bool) exp.LiteralExpression {
	if f.SQL == "" {
		return goqu.L(p.Alias + `."` + strings.ReplaceAll(f.Name, `"`, `""`) + `"`)
	}
	if parenthesize {
		return goqu.L("(" + f.SQL + ")")
	}
	return goqu.L(f.SQL)
}
