// Package sqlbob is an easylist driver that compiles query plans into
// prepared PostgreSQL with bob's psql query builder.
package sqlbob

import (
	"context"
	"fmt"
	"strings"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	"github.com/mgpnd/easylist-go/easylist-sqlpg"

	"github.com/stephenafamo/bob"
	"github.com/stephenafamo/bob/dialect/psql"
	"github.com/stephenafamo/bob/dialect/psql/dialect"
	"github.com/stephenafamo/bob/dialect/psql/sm"
	"github.com/stephenafamo/bob/expr"
)

// Driver compiles easylist plans into PostgreSQL. It holds no state and needs
// no bob executor, so one value is safe for concurrent use.
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
	// Each compilation builds its own query from new mods, so compilations
	// share nothing.
	mods := []bob.Mod[*dialect.SelectQuery]{sm.From(expr.Raw(q.From.SQL))}

	if q.Count {
		mods = append(mods, sm.Columns(expr.Raw("COUNT(*)")))
	} else {
		for _, c := range q.Select {
			if err := checkAlias(c.Alias); err != nil {
				return "", nil, fmt.Errorf("list %q: %w", p.List, err)
			}
			// As writes the leaf as is and the alias as a quoted identifier.
			mods = append(mods, sm.Columns(leaf(c.Expr).As(c.Alias)))
		}
		for _, o := range q.Order {
			if o.Desc {
				mods = append(mods, sm.OrderBy(expr.Raw(o.Expr.SQL)).Desc())
			} else {
				mods = append(mods, sm.OrderBy(expr.Raw(o.Expr.SQL)).Asc())
			}
		}
	}

	if q.Where != nil {
		where, err := condition(q.Where)
		if err != nil {
			return "", nil, fmt.Errorf("list %q: %w", p.List, err)
		}
		mods = append(mods, sm.Where(where))
	}

	if q.Page != nil {
		// sm.Limit and sm.Offset write a plain value into the SQL text, so
		// both get an argument expression, which bob binds after the filter
		// arguments, an offset of 0 included.
		mods = append(mods, sm.Limit(psql.Arg(q.Page.Limit)), sm.Offset(psql.Arg(q.Page.Offset)))
	}

	query, args, err := psql.Select(mods...).Build(context.Background())
	if err != nil {
		return "", nil, fmt.Errorf("list %q: bob: %w", p.List, err)
	}
	if args == nil {
		args = []any{}
	}
	return query, args, nil
}

// leaf is author or generated SQL as a bob expression. expr.Raw writes its
// text verbatim, without scanning it for ? or $.
func leaf(e sqlpg.Expr) psql.Expression {
	return dialect.NewExpression(expr.Raw(e.SQL))
}

// checkAlias fails unless bob writes alias as a quoted identifier. bob wraps
// an alias in double quotes without escaping a quote inside it. Field names
// never contain one.
func checkAlias(alias string) error {
	if alias == "" || strings.Contains(alias, `"`) {
		return fmt.Errorf("unsupported column alias %q", alias)
	}
	return nil
}
