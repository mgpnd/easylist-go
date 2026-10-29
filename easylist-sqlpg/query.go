// Package sqlpg lowers easylist plans into a typed description of one
// PostgreSQL query, for the authors of easylist-sql-* drivers. Lower makes
// every decision that depends on the plan: field lookup, operator checks,
// literal and list conversion, evaluated values, whole-day ranges, and the
// NULL-safe forms of !=, EXCLUDES, IN, and NOT IN. A driver only maps the
// returned Query onto its query library. Applications never import this
// package; they use a driver.
package sqlpg

// Query is a lowered plan: one PostgreSQL page, export, or count query.
type Query struct {
	// Count marks a count query: SELECT COUNT(*) with no Select, Order, or Page.
	Count bool
	// From is the base table, AS, the alias, and the planned joins in order:
	// "<table> AS <alias> <join SQL>...".
	From Expr
	// Select lists the selected fields in plan order. Page and export only.
	Select []Column
	// Where is the scope, as a Raw node ANDed in front of the filter. It is nil
	// when the plan has neither.
	Where Cond
	// Order lists the ORDER BY entries. Page and export only.
	Order []Ordering
	// Page holds the limit and offset of a page query, bound after the filter
	// arguments, including an offset of 0. It is nil for export and count.
	Page *Page
}

// Column is one selected field. Array fields are already wrapped in
// to_json(...)::text, so they reach the runtime as JSON text or NULL.
type Column struct {
	Expr  Expr
	Alias string // The field name; drivers write it as a quoted identifier.
}

// Ordering is one ORDER BY entry, ascending unless Desc is set.
type Ordering struct {
	Expr Expr
	Desc bool
}

// Page is the limit and offset of a page query.
type Page struct {
	Limit, Offset int64
}

// Expr is author SQL, or SQL generated around it, such as a quoted base
// column. Drivers write it verbatim through a construct that does not scan it,
// because it may contain ?, $, #, or : characters that a library would
// otherwise rewrite.
type Expr struct {
	SQL string
}

// Arg is a bound value. Cast is empty or CastNumeric; a Fragment already
// writes the cast after the value's slot.
type Arg struct {
	Value any
	Cast  string
}

// CastNumeric is the only cast a bound value can have. Numeric literals are
// bound as exact decimal strings, which PostgreSQL would otherwise compare as
// text.
const CastNumeric = "numeric"

// Args returns the arguments a driver must bind, with their Go types: the
// filter values in the order the filter is read, left to right, then the
// limit and offset of a page query as int64.
func (q *Query) Args() []any {
	args := []any{}
	if q.Where != nil {
		for _, slot := range q.Where.Fragment().Slots {
			if a, ok := slot.(Arg); ok {
				args = append(args, a.Value)
			}
		}
	}
	if q.Page != nil {
		args = append(args, q.Page.Limit, q.Page.Offset)
	}
	return args
}
