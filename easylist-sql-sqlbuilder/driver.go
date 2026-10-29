// Package sqlsqlbuilder is an easylist driver that compiles query plans into
// prepared PostgreSQL with go-sqlbuilder's PostgreSQL flavor. The package is
// named sql plus the library's name, like the other easylist SQL drivers, so
// that it does not clash with go-sqlbuilder's own package sqlbuilder.
package sqlsqlbuilder

import (
	"fmt"
	"strings"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	"github.com/mgpnd/easylist-go/easylist-sqlpg"

	"github.com/huandu/go-sqlbuilder"
)

// Driver compiles easylist plans into PostgreSQL. It holds no state and needs
// no database handle, so one value is safe for concurrent use.
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
	// Each compilation builds its own builder, whose Args hold every bound
	// value, so compilations share nothing. go-sqlbuilder reads $ in every
	// string it is given as a reference to its Args, so author SQL is always
	// passed through text, which writes it back verbatim.
	sb := sqlbuilder.PostgreSQL.NewSelectBuilder()

	if q.Count {
		sb.Select("COUNT(*)")
	} else {
		cols := make([]string, len(q.Select))
		for i, c := range q.Select {
			if err := checkAlias(c.Alias); err != nil {
				return "", nil, fmt.Errorf("list %q: %w", p.List, err)
			}
			// As writes its alias as given, so the alias is quoted here.
			cols[i] = sb.As(text(c.Expr.SQL), text(`"`+c.Alias+`"`))
		}
		sb.Select(cols...)
	}
	sb.From(text(q.From.SQL))

	if q.Where != nil {
		where, err := condition(&sb.Cond, q.Where)
		if err != nil {
			return "", nil, fmt.Errorf("list %q: %w", p.List, err)
		}
		sb.Where(where)
	}

	for _, o := range q.Order {
		if o.Desc {
			sb.OrderByDesc(text(o.Expr.SQL))
		} else {
			sb.OrderByAsc(text(o.Expr.SQL))
		}
	}

	if q.Page != nil {
		// sb.Limit and sb.Offset bind an int, and the contract binds int64,
		// so both are bound here instead. SQL writes the clause after the
		// last clause set above, which is the end of the statement, and
		// numbers both values after the filter arguments, an offset of 0
		// included.
		sb.SQL("LIMIT " + sb.Var(q.Page.Limit) + " OFFSET " + sb.Var(q.Page.Offset))
	}

	query, args := sb.BuildWithFlavor(sqlbuilder.PostgreSQL)
	if args == nil {
		args = []any{}
	}
	return query, args, nil
}

// text returns author or generated SQL as go-sqlbuilder text: Escape doubles
// each $, which the builder writes back as one $, so $1, $?, ${name}, and $$
// in the SQL are never read as references. go-sqlbuilder gives ? no meaning
// in the PostgreSQL flavor, so ? needs no escape.
func text(sql string) string {
	return sqlbuilder.Escape(sql)
}

// checkAlias fails unless the alias can be written as a quoted identifier
// without escaping. Field names never contain a double quote.
func checkAlias(alias string) error {
	if alias == "" || strings.Contains(alias, `"`) {
		return fmt.Errorf("unsupported column alias %q", alias)
	}
	return nil
}
