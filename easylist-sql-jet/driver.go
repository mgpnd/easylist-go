// Package sqljet is an easylist driver that compiles query plans into
// prepared PostgreSQL with jet's postgres package, without generated code.
package sqljet

import (
	"fmt"
	"strings"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	"github.com/mgpnd/easylist-go/easylist-sqlpg"

	"github.com/go-jet/jet/v2/postgres"
)

// Driver compiles easylist plans into PostgreSQL. It holds no state and needs
// no database handle or generated table code, so one value is safe for
// concurrent use.
type Driver struct{}

// New returns a PostgreSQL driver.
func New() *Driver { return &Driver{} }

var _ easylist.Driver = (*Driver)(nil)

// Compile returns one prepared query with $n placeholders and its arguments.
// Arguments follow the filter left to right, then limit and offset. Author
// SQL is emitted verbatim; request values are always bound. Every validation
// error comes from sqlpg.Lower, except jet's own failures.
func (*Driver) Compile(p *easylist.Plan) (string, []any, error) {
	q, err := sqlpg.Lower(p)
	if err != nil {
		return "", nil, err
	}
	query, args, err := build(q)
	if err != nil {
		return "", nil, fmt.Errorf("list %q: %w", p.List, err)
	}
	return query, args, nil
}

// build maps a lowered query onto a new jet statement, so compilations
// share nothing. jet takes no verbatim FROM text, so the statement reads
// from a table named by a sentinel, which jet writes unquoted, and the one
// occurrence of the sentinel in the output is replaced with q.From.
func build(q *sqlpg.Query) (string, []any, error) {
	sentinel := chooseSentinel(q)

	var projections []postgres.Projection
	if q.Count {
		projections = append(projections, postgres.Raw("COUNT(*)"))
	} else {
		for _, c := range q.Select {
			if err := checkAlias(c.Alias); err != nil {
				return "", nil, err
			}
			// jet writes the alias between double quotes and the expression
			// in parentheses.
			projections = append(projections, postgres.Raw(c.Expr.SQL).AS(c.Alias))
		}
	}
	if len(projections) == 0 {
		return "", nil, fmt.Errorf("query selects no columns")
	}
	stmt := postgres.SELECT(projections[0], projections[1:]...).
		FROM(postgres.NewTable("", sentinel, ""))

	if q.Where != nil {
		where, err := condition(q.Where)
		if err != nil {
			return "", nil, err
		}
		stmt = stmt.WHERE(where)
	}

	if len(q.Order) > 0 {
		order := make([]postgres.OrderByClause, len(q.Order))
		for i, o := range q.Order {
			// jet writes an ORDER BY expression without parentheses.
			if o.Desc {
				order[i] = postgres.Raw(o.Expr.SQL).DESC()
			} else {
				order[i] = postgres.Raw(o.Expr.SQL).ASC()
			}
		}
		stmt = stmt.ORDER_BY(order...)
	}

	if q.Page != nil {
		// LIMIT binds its int64 as the next $n. OFFSET binds an Int literal,
		// which the postgres dialect writes as $n without a cast, and jet
		// writes the clause for any offset, 0 included.
		stmt = stmt.LIMIT(q.Page.Limit).OFFSET(q.Page.Offset)
	}

	query, args, err := serialize(stmt)
	if err != nil {
		return "", nil, err
	}
	if query, err = replaceFrom(query, sentinel, q.From.SQL); err != nil {
		return "", nil, err
	}
	if args == nil {
		args = []any{}
	}
	return query, args, nil
}

// serialize returns the statement's SQL and arguments without the ";\n"
// jet appends and the line break it starts with. jet panics on statements
// it cannot write, which serialize returns as an error.
func serialize(stmt postgres.SelectStatement) (query string, args []any, err error) {
	defer func() {
		if r := recover(); r != nil {
			query, args, err = "", nil, fmt.Errorf("jet: %v", r)
		}
	}()
	query, args = stmt.Sql()
	query, ok := strings.CutSuffix(query, ";\n")
	if !ok {
		return "", nil, fmt.Errorf("jet: statement does not end with a semicolon: %q", query)
	}
	return strings.TrimPrefix(query, "\n"), args, nil
}

// sentinelBase is the table name the statement is built over. It is a
// lowercase identifier and no PostgreSQL keyword, so jet writes it unquoted.
const sentinelBase = "easylist_from"

// chooseSentinel returns sentinelBase, with "_x" appended until it occurs in
// none of the query's author SQL: FROM, the selected expressions and their
// aliases, the ORDER BY entries, and every SQL slot of the condition, which
// includes the scope.
func chooseSentinel(q *sqlpg.Query) string {
	texts := []string{q.From.SQL}
	for _, c := range q.Select {
		texts = append(texts, c.Expr.SQL, c.Alias)
	}
	for _, o := range q.Order {
		texts = append(texts, o.Expr.SQL)
	}
	if q.Where != nil {
		for _, slot := range q.Where.Fragment().Slots {
			if e, ok := slot.(sqlpg.Expr); ok {
				texts = append(texts, e.SQL)
			}
		}
	}
	sentinel := sentinelBase
	for occurs(texts, sentinel) {
		sentinel += "_x"
	}
	return sentinel
}

// occurs reports whether s occurs in any of texts.
func occurs(texts []string, s string) bool {
	for _, t := range texts {
		if strings.Contains(t, s) {
			return true
		}
	}
	return false
}

// replaceFrom replaces the sentinel table name in query with the verbatim
// FROM text. It fails unless the sentinel occurs exactly once, which would
// mean jet wrote it differently or some SQL around it contains it too.
func replaceFrom(query, sentinel, from string) (string, error) {
	if n := strings.Count(query, sentinel); n != 1 {
		return "", fmt.Errorf("jet: FROM placeholder %q occurs %d times in the statement", sentinel, n)
	}
	return strings.Replace(query, sentinel, from, 1), nil
}

// checkAlias fails unless jet's double quotes around the alias make it one
// quoted identifier. Field names never contain a double quote.
func checkAlias(alias string) error {
	if alias == "" || strings.Contains(alias, `"`) {
		return fmt.Errorf("unsupported column alias %q", alias)
	}
	return nil
}
