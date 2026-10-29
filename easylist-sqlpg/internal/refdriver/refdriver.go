// Package refdriver is the reference PostgreSQL driver of easylist-sqlpg's
// tests. It renders a lowered Query through Fragment alone, numbering
// placeholders $1, $2, ..., so its output is the SQL every node's canonical
// template gives, in the layout easylist-sql-goqu produces. Only tests import
// it.
package refdriver

import (
	"fmt"
	"strconv"
	"strings"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	"github.com/mgpnd/easylist-go/easylist-sqlpg"
)

// Driver renders lowered plans. It holds no state, so one value is safe for
// concurrent use.
type Driver struct{}

var _ easylist.Driver = Driver{}

// Compile lowers p and renders the query with bound arguments.
func (Driver) Compile(p *easylist.Plan) (string, []any, error) {
	q, err := sqlpg.Lower(p)
	if err != nil {
		return "", nil, err
	}
	sql, args := Render(q)
	return sql, args, nil
}

// Render writes q as one statement. And and Or are parenthesized as in their
// fragments, Expr slots are copied without scanning, and each Arg becomes the
// next $n.
func Render(q *sqlpg.Query) (string, []any) {
	var b strings.Builder
	args := []any{}
	b.WriteString("SELECT ")
	if q.Count {
		b.WriteString("COUNT(*)")
	}
	for i, c := range q.Select {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(c.Expr.SQL + ` AS "` + strings.ReplaceAll(c.Alias, `"`, `""`) + `"`)
	}
	b.WriteString(" FROM " + q.From.SQL)
	if q.Where != nil {
		b.WriteString(" WHERE ")
		f := q.Where.Fragment()
		next := 0
		for _, r := range f.SQL {
			if r != '?' {
				b.WriteRune(r)
				continue
			}
			switch s := f.Slots[next].(type) {
			case sqlpg.Expr:
				b.WriteString(s.SQL)
			case sqlpg.Arg:
				args = append(args, s.Value)
				b.WriteString("$" + strconv.Itoa(len(args)))
			default:
				panic(fmt.Sprintf("refdriver: slot of type %T", s))
			}
			next++
		}
	}
	for i, o := range q.Order {
		if i == 0 {
			b.WriteString(" ORDER BY ")
		} else {
			b.WriteString(", ")
		}
		if o.Desc {
			b.WriteString(o.Expr.SQL + " DESC")
		} else {
			b.WriteString(o.Expr.SQL + " ASC")
		}
	}
	if q.Page != nil {
		n := len(args)
		b.WriteString(" LIMIT $" + strconv.Itoa(n+1) + " OFFSET $" + strconv.Itoa(n+2))
		args = append(args, q.Page.Limit, q.Page.Offset)
	}
	return b.String(), args
}
