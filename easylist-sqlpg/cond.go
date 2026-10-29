package sqlpg

import "strings"

// Cond is one node of a lowered WHERE condition: And, Or, Raw, Compare,
// IsNull, In, Between, ILike, ArrayHas, or DayRange. A driver maps And and Or
// onto its library's connectors and every other node either onto a native
// construct with the same semantics, no cast, and no extra argument, or onto
// its library's raw-with-arguments primitive over Fragment.
type Cond interface {
	// Fragment returns the node's canonical PostgreSQL template.
	Fragment() Fragment
}

// Fragment is a PostgreSQL template. Each ? in SQL marks the next entry of
// Slots, which is either an Expr, written verbatim, or an Arg, bound as one
// placeholder. The cast of a CastNumeric Arg follows its ? in SQL. SQL never
// contains author SQL, which is only ever a slot, so a library may scan the
// template for ? without rewriting author SQL.
type Fragment struct {
	SQL   string
	Slots []any
}

// And requires both conditions. Its fragment is "(left AND right)".
type And struct{ Left, Right Cond }

// Or requires either condition. Its fragment is "(left OR right)".
type Or struct{ Left, Right Cond }

// Raw is the list's scope: a condition in author SQL, written in parentheses.
type Raw struct{ SQL string }

// Op is a comparison operator, spelled as in PostgreSQL.
type Op string

const (
	OpEq       Op = "="
	OpDistinct Op = "IS DISTINCT FROM" // !=, which also matches NULL rows.
	OpGt       Op = ">"
	OpGte      Op = ">="
	OpLt       Op = "<"
	OpLte      Op = "<="
)

// Compare compares a column with a bound value. Booleans compare with = and
// IS DISTINCT FROM too, never IS TRUE, so every value stays bound.
type Compare struct {
	Col Expr
	Op  Op
	Arg Arg
}

// IsNull tests a column with IS NULL, or IS NOT NULL when Not is set.
type IsNull struct {
	Col Expr
	Not bool
}

// NullMode says what IN or NOT IN gives for a NULL column.
type NullMode uint8

const (
	NullKeep    NullMode = iota // SQL's NULL result, which WHERE treats as no match.
	NullMatch                   // COALESCE(..., TRUE): the row matches.
	NullNoMatch                 // COALESCE(..., FALSE): the row does not match.
)

// In tests a column with IN, or NOT IN when Not is set, over the bound
// values of the list's non-NULL items in list order. NULL never enters the
// list; Null decides NULL columns instead. The column is written once. Lower
// gives In at least one argument; with none, the fragment is an IsNull.
type In struct {
	Col  Expr
	Args []Arg
	Not  bool
	Null NullMode
}

// Between tests "column BETWEEN low AND high", inclusive.
type Between struct {
	Col       Expr
	Low, High Arg
}

// ILike is text CONTAINS, or EXCLUDES when Not is set. Pattern is already
// escaped with backslashes and wrapped in %...%. EXCLUDES matches NULL rows.
type ILike struct {
	Col     Expr
	Pattern Arg
	Not     bool
}

// ArrayHas is array CONTAINS: the array holds an element equal to each value.
// With None it is EXCLUDES: the array holds none of them, and NULL arrays
// match. Each value is compared with "= ANY", so PostgreSQL infers its type
// from the element type. Values is never empty.
type ArrayHas struct {
	Col    Expr
	Values []Arg
	None   bool
}

// DayRange tests whether a timestamp column falls in [Start, End), one whole
// day. With Not it is the NULL-safe negation, which matches NULL rows. The
// column is written twice.
type DayRange struct {
	Col        Expr
	Start, End Arg
	Not        bool
}

// slot is the template text of one bound value.
func slot(a Arg) string {
	if a.Cast != "" {
		return "?::" + a.Cast
	}
	return "?"
}

func connect(left, right Cond, connector string) Fragment {
	l, r := left.Fragment(), right.Fragment()
	slots := make([]any, 0, len(l.Slots)+len(r.Slots))
	slots = append(append(slots, l.Slots...), r.Slots...)
	return Fragment{SQL: "(" + l.SQL + " " + connector + " " + r.SQL + ")", Slots: slots}
}

func (c And) Fragment() Fragment { return connect(c.Left, c.Right, "AND") }

func (c Or) Fragment() Fragment { return connect(c.Left, c.Right, "OR") }

// Fragment parenthesizes the scope so that an OR inside it cannot escape.
func (c Raw) Fragment() Fragment {
	return Fragment{SQL: "(?)", Slots: []any{Expr{SQL: c.SQL}}}
}

func (c Compare) Fragment() Fragment {
	return Fragment{SQL: "? " + string(c.Op) + " " + slot(c.Arg), Slots: []any{c.Col, c.Arg}}
}

func (c IsNull) Fragment() Fragment {
	if c.Not {
		return Fragment{SQL: "? IS NOT NULL", Slots: []any{c.Col}}
	}
	return Fragment{SQL: "? IS NULL", Slots: []any{c.Col}}
}

func (c In) Fragment() Fragment {
	if len(c.Args) == 0 {
		// A list of only NULL items is = NULL or != NULL.
		return IsNull{Col: c.Col, Not: c.Not}.Fragment()
	}
	items := make([]string, len(c.Args))
	slots := make([]any, 0, len(c.Args)+1)
	slots = append(slots, c.Col)
	for i, a := range c.Args {
		items[i] = slot(a)
		slots = append(slots, a)
	}
	sql := "? IN (" + strings.Join(items, ", ") + ")"
	if c.Not {
		sql = "? NOT IN (" + strings.Join(items, ", ") + ")"
	}
	switch c.Null {
	case NullMatch:
		sql = "COALESCE(" + sql + ", TRUE)"
	case NullNoMatch:
		sql = "COALESCE(" + sql + ", FALSE)"
	}
	return Fragment{SQL: sql, Slots: slots}
}

func (c Between) Fragment() Fragment {
	return Fragment{SQL: "? BETWEEN " + slot(c.Low) + " AND " + slot(c.High), Slots: []any{c.Col, c.Low, c.High}}
}

func (c ILike) Fragment() Fragment {
	if c.Not {
		// NOT ILIKE is NULL for a NULL field; COALESCE makes such rows match
		// while emitting the author SQL only once.
		return Fragment{SQL: `COALESCE(? NOT ILIKE ` + slot(c.Pattern) + ` ESCAPE E'\\', TRUE)`, Slots: []any{c.Col, c.Pattern}}
	}
	return Fragment{SQL: `? ILIKE ` + slot(c.Pattern) + ` ESCAPE E'\\'`, Slots: []any{c.Col, c.Pattern}}
}

func (c ArrayHas) Fragment() Fragment {
	tests := make([]string, len(c.Values))
	slots := make([]any, 0, 2*len(c.Values))
	for i, v := range c.Values {
		tests[i] = slot(v) + " = ANY(?)"
		slots = append(slots, v, c.Col)
	}
	if c.None {
		return Fragment{SQL: "COALESCE(NOT (" + strings.Join(tests, " OR ") + "), TRUE)", Slots: slots}
	}
	return Fragment{SQL: "(" + strings.Join(tests, " AND ") + ")", Slots: slots}
}

func (c DayRange) Fragment() Fragment {
	slots := []any{c.Col, c.Start, c.Col, c.End}
	test := "? >= " + slot(c.Start) + " AND ? < " + slot(c.End)
	if c.Not {
		// The range test is NULL for a NULL field; COALESCE makes such rows
		// match, as EXCLUDES does.
		return Fragment{SQL: "COALESCE(NOT (" + test + "), TRUE)", Slots: slots}
	}
	return Fragment{SQL: "(" + test + ")", Slots: slots}
}
