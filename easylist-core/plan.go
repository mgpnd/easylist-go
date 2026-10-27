package easylist

import (
	"fmt"
	"slices"
	"time"

	filter "github.com/mgpnd/easylist-go/easylist-filter"
)

// Kind tells a driver which query to produce for a plan.
type Kind uint8

const (
	KindPage   Kind = iota // Selected rows with ordering, limit, and offset.
	KindCount              // One row holding COUNT(*) of matching base rows.
	KindExport             // Selected rows with ordering and no pagination.
)

// Direction is a sort direction. The zero value sorts ascending.
type Direction string

const (
	Asc  Direction = "ASC"
	Desc Direction = "DESC"
)

// Sort is one sort entry of a request.
type Sort struct {
	Field     string    `json:"field"`
	Direction Direction `json:"direction,omitempty"`
}

// Request describes what a caller wants from a list.
type Request struct {
	// Fields are the selected SQL and calculated field names. Duplicates are
	// ignored. The primary key is always selected.
	Fields []string
	Filter filter.Expr // Nil means no filter.
	Sort   []Sort
	// Limit and Offset are validated for page and count plans, applied only to
	// page plans, and rejected for export plans.
	Limit  *int // Nil means the list's default page size.
	Offset *int // Nil means 0.
	// Now is the reference time for relative values such as TODAY, and its
	// location is the time zone they are evaluated in. Planning a filter that
	// uses a time keyword fails when Now is zero; Runtime.Fetch fills it from
	// the runtime clock.
	Now time.Time
	// WeekStart is the first day of the week for START_OF_WEEK and
	// END_OF_WEEK. Nil means Monday.
	WeekStart *time.Weekday
}

// Plan is a dialect-neutral query description handed to a Driver. Plans are
// immutable and safe to share between goroutines.
type Plan struct {
	Kind  Kind
	List  string
	Table string
	Alias string
	// Joins are in dependency order; their SQL is emitted verbatim.
	Joins []PlanJoin
	// Fields are the selected SQL fields, in list declaration order. Drivers
	// select them in this order under their names. Count plans select none.
	Fields []PlanField
	Scope  string      // Static author SQL ANDed with the filter; may be empty.
	Filter filter.Expr // May be nil. Resolve its fields with Lookup.
	Order  []PlanOrder // Empty for count plans.
	// Limit and Offset apply only when Kind is KindPage.
	Limit  int
	Offset int

	fields map[string]*Field
	values map[filter.Predicate]Value // Evaluated expression literals.
}

// PlanJoin is one join clause of a plan.
type PlanJoin struct {
	Key string
	SQL string
}

// PlanField describes an SQL field to a driver.
type PlanField struct {
	Name     string
	SQL      string // Empty means the column Name of the base table alias.
	Type     Type
	Nullable bool
	// Array marks an array of Type. Drivers select it as JSON array text, or
	// NULL, and accept only CONTAINS, EXCLUDES, = NULL, and != NULL on it.
	Array  bool
	Values []string // Enum values.
}

// PlanOrder is one ORDER BY entry.
type PlanOrder struct {
	Field PlanField
	Desc  bool
}

// Lookup resolves an SQL field of the plan's list, such as a filter field.
// Calculated and unknown fields are not found.
func (p *Plan) Lookup(name string) (PlanField, bool) {
	f, ok := p.fields[name]
	if !ok || f.calc != nil {
		return PlanField{}, false
	}
	return planField(f), true
}

// Value returns the evaluated value of pred's expression literal, such as
// TODAY or {100 * 3}. It reports false for a predicate whose value is not an
// expression or that is not part of the plan's filter.
func (p *Plan) Value(pred filter.Predicate) (Value, bool) {
	v, ok := p.values[pred]
	return v, ok
}

// Driver compiles plans into one SQL query and its bound arguments. Drivers
// must emit list-author SQL verbatim, bind every request value, and be safe
// for concurrent use.
type Driver interface {
	Compile(p *Plan) (query string, args []any, err error)
}

func planField(f *Field) PlanField {
	return PlanField{Name: f.Name, SQL: f.SQL, Type: f.Type, Nullable: f.Nullable, Array: f.Array, Values: f.Values}
}

// Plan validates req against the list and builds a plan of the given kind.
// Every kind validates the whole request, so a request that plans as a page
// also plans as a count.
// Expression values are type-checked against their fields and evaluated at
// req.Now; Plan never reads the system clock.
func (c *CompiledList) Plan(kind Kind, req Request) (*Plan, error) {
	return c.plan(kind, req, false)
}

// plan builds a plan. With typeOnly, expression values are type-checked but
// times are not evaluated, so no reference time is needed.
func (c *CompiledList) plan(kind Kind, req Request, typeOnly bool) (*Plan, error) {
	selected := map[string]bool{c.def.PrimaryKey: true}
	for _, name := range req.Fields {
		f, ok := c.fields[name]
		if !ok {
			return nil, fmt.Errorf("list %q has no field %q", c.def.Name, name)
		}
		if f.calc == nil {
			selected[name] = true
			continue
		}
		for _, dep := range f.calc.requires {
			selected[dep] = true
		}
	}

	env := exprEnv{now: req.Now, weekStart: time.Monday, typeOnly: typeOnly}
	if req.WeekStart != nil {
		if *req.WeekStart < time.Sunday || *req.WeekStart > time.Saturday {
			return nil, fmt.Errorf("invalid week start %d", *req.WeekStart)
		}
		env.weekStart = *req.WeekStart
	}
	filtered, values, err := c.filterFields(req.Filter, env)
	if err != nil {
		return nil, err
	}

	order, err := c.order(req.Sort)
	if err != nil {
		return nil, err
	}

	p := &Plan{
		Kind: kind, List: c.def.Name, Table: c.def.Table, Alias: c.def.Alias,
		Scope: c.def.Scope, Filter: req.Filter, fields: c.fields, values: values,
	}
	switch kind {
	case KindPage, KindCount:
		limit, offset := c.def.DefaultPageSize, 0
		if req.Limit != nil {
			if *req.Limit < 1 || *req.Limit > c.def.MaxPageSize {
				return nil, fmt.Errorf("limit %d must be between 1 and the maximum page size %d", *req.Limit, c.def.MaxPageSize)
			}
			limit = *req.Limit
		}
		if req.Offset != nil {
			if *req.Offset < 0 {
				return nil, fmt.Errorf("offset %d must not be negative", *req.Offset)
			}
			offset = *req.Offset
		}
		if kind == KindPage {
			p.Limit, p.Offset = limit, offset
		}
	case KindExport:
		if req.Limit != nil || req.Offset != nil {
			return nil, fmt.Errorf("export requests take no limit or offset")
		}
	default:
		return nil, fmt.Errorf("invalid plan kind %d", kind)
	}

	needJoins := map[string]bool{}
	addJoins := func(names map[string]bool) {
		for name := range names {
			for _, key := range c.joinsOf[name] {
				needJoins[key] = true
			}
		}
	}
	addJoins(filtered)
	if kind != KindCount {
		addJoins(selected)
		for _, o := range order {
			addJoins(map[string]bool{o.Field.Name: true})
		}
		for _, f := range c.def.Fields {
			if selected[f.Name] {
				p.Fields = append(p.Fields, planField(c.fields[f.Name]))
			}
		}
		p.Order = order
	}
	for _, key := range c.sortJoins(needJoins) {
		p.Joins = append(p.Joins, PlanJoin{Key: key, SQL: c.joinOrder[c.joinIndex[key]].SQL})
	}
	return p, nil
}

// filterFields validates every predicate field without recursion, evaluates
// every expression literal, and returns the set of filtered field names and
// the values by predicate.
func (c *CompiledList) filterFields(root filter.Expr, env exprEnv) (map[string]bool, map[filter.Predicate]Value, error) {
	names := map[string]bool{}
	var values map[filter.Predicate]Value
	stack := []filter.Expr{}
	if root != nil {
		stack = append(stack, root)
	}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		switch n := node.(type) {
		case filter.Predicate:
			f, ok := c.fields[n.Field()]
			if !ok {
				return nil, nil, fmt.Errorf("unknown field %q at position %d", n.Field(), n.FieldPos())
			}
			if f.calc != nil {
				return nil, nil, fmt.Errorf("cannot filter on calculated field %q at position %d", n.Field(), n.FieldPos())
			}
			names[f.Name] = true
			if n.Value().Kind() == filter.LiteralExpr {
				v, ok, err := exprValue(n, f, env)
				if err != nil {
					return nil, nil, err
				}
				if ok {
					if values == nil {
						values = map[filter.Predicate]Value{}
					}
					values[n] = v
				}
			}
		case filter.Logical:
			// Push right first so fields are checked left to right.
			stack = append(stack, n.Right(), n.Left())
		default:
			return nil, nil, fmt.Errorf("invalid filter syntax tree")
		}
	}
	return names, values, nil
}

func (c *CompiledList) order(sorts []Sort) ([]PlanOrder, error) {
	var out []PlanOrder
	seen := map[string]bool{}
	for _, s := range sorts {
		f, ok := c.fields[s.Field]
		if !ok {
			return nil, fmt.Errorf("cannot sort on unknown field %q", s.Field)
		}
		if f.calc != nil {
			return nil, fmt.Errorf("cannot sort on calculated field %q", s.Field)
		}
		if f.Array {
			return nil, fmt.Errorf("cannot sort on array field %q", s.Field)
		}
		if seen[s.Field] {
			return nil, fmt.Errorf("field %q is sorted more than once", s.Field)
		}
		seen[s.Field] = true
		var desc bool
		switch s.Direction {
		case "", Asc:
		case Desc:
			desc = true
		default:
			return nil, fmt.Errorf("invalid sort direction %q for field %q", s.Direction, s.Field)
		}
		out = append(out, PlanOrder{Field: planField(f), Desc: desc})
	}
	if !seen[c.def.PrimaryKey] {
		out = append(out, PlanOrder{Field: planField(c.fields[c.def.PrimaryKey])})
	}
	return slices.Clip(out), nil
}
