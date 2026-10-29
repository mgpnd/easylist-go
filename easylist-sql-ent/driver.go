// Package sqlent is an easylist driver that compiles query plans into
// prepared PostgreSQL with ent's SQL builder, without generated code.
package sqlent

import (
	"fmt"
	"strconv"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	"github.com/mgpnd/easylist-go/easylist-sqlpg"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
)

// Driver compiles easylist plans into PostgreSQL. It holds no state and needs
// no ent client, so one value is safe for concurrent use.
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
	// Each compilation builds its own selector, so compilations share nothing.
	s := sql.Dialect(dialect.Postgres).Select().FromExpr(sql.Raw(q.From.SQL))

	if q.Count {
		s.Select(sql.Count("*"))
	} else {
		for _, c := range q.Select {
			if err := checkAlias(c.Alias); err != nil {
				return "", nil, fmt.Errorf("list %q: %w", p.List, err)
			}
			// ent writes a raw expression as is, without parentheses.
			s.AppendSelectExprAs(sql.Raw(c.Expr.SQL), c.Alias)
		}
		for _, o := range q.Order {
			if o.Desc {
				s.OrderExpr(sql.Raw(o.Expr.SQL + " DESC"))
			} else {
				s.OrderExpr(sql.Raw(o.Expr.SQL + " ASC"))
			}
		}
	}

	if q.Where != nil {
		where, err := condition(q.Where)
		if err != nil {
			return "", nil, fmt.Errorf("list %q: %w", p.List, err)
		}
		s.Where(where)
	}

	query, args := s.Query()
	if err := s.Err(); err != nil {
		return "", nil, fmt.Errorf("list %q: ent: %w", p.List, err)
	}
	if args == nil {
		args = []any{}
	}
	if q.Page != nil {
		// ent writes Limit and Offset as numbers; binding both keeps request
		// values out of the SQL text and the query shape fixed.
		n := len(args)
		query += " LIMIT $" + strconv.Itoa(n+1) + " OFFSET $" + strconv.Itoa(n+2)
		args = append(args, q.Page.Limit, q.Page.Offset)
	}
	return query, args, nil
}

// checkAlias fails unless ent writes alias as a quoted identifier. ent writes
// an alias as is when it looks like SQL already, such as one containing a
// quote or parentheses. Field names never do.
func checkAlias(alias string) error {
	quoted := sql.Dialect(dialect.Postgres).String(func(b *sql.Builder) { b.Ident(alias) })
	if quoted != `"`+alias+`"` {
		return fmt.Errorf("unsupported column alias %q", alias)
	}
	return nil
}
