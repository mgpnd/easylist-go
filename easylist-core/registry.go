package easylist

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

const (
	defaultPageSize = 100
	maxPageSize     = 10000
)

var (
	listNamePattern  = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	fieldNamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)
	graphQLName      = regexp.MustCompile(`^[_A-Za-z][_0-9A-Za-z]*$`)
)

// Registry holds validated, immutable lists. It is safe for concurrent use.
type Registry struct {
	lists map[string]*CompiledList
	order []*CompiledList
}

// CompiledList is a validated list with precomputed lookup and join data.
type CompiledList struct {
	def       List
	fields    map[string]*Field
	joinIndex map[string]int      // key to position in the topological order
	joinOrder []Join              // all joins, dependencies first
	joinsOf   map[string][]string // SQL field to the join keys it needs, closed over DependsOn
}

// NewRegistry validates lists and compiles them. Errors name the list and the
// offending field, join, or setting.
func NewRegistry(lists ...List) (*Registry, error) {
	r := &Registry{lists: make(map[string]*CompiledList, len(lists))}
	prefixes := map[string]string{}
	typeNames := map[string]string{}
	for _, l := range lists {
		if _, dup := r.lists[l.Name]; dup {
			return nil, fmt.Errorf("duplicate list name %q", l.Name)
		}
		c, err := compileList(l)
		if err != nil {
			return nil, err
		}
		prefix := c.TypePrefix()
		if other, dup := prefixes[prefix]; dup {
			return nil, fmt.Errorf("lists %q and %q both use the GraphQL type prefix %q", other, l.Name, prefix)
		}
		prefixes[prefix] = l.Name
		for _, name := range c.typeNames() {
			if owner, dup := typeNames[name]; dup {
				return nil, fmt.Errorf("list %q: GraphQL type %q is also declared by %s", l.Name, name, owner)
			}
			typeNames[name] = fmt.Sprintf("list %q", l.Name)
		}
		r.lists[l.Name] = c
		r.order = append(r.order, c)
	}
	return r, nil
}

// List returns the compiled list with the given name.
func (r *Registry) List(name string) (*CompiledList, bool) {
	c, ok := r.lists[name]
	return c, ok
}

// Lists returns every list in registration order.
func (r *Registry) Lists() []*CompiledList { return slices.Clone(r.order) }

// Name returns the list name.
func (c *CompiledList) Name() string { return c.def.Name }

// Table returns the base table.
func (c *CompiledList) Table() string { return c.def.Table }

// Alias returns the base table alias.
func (c *CompiledList) Alias() string { return c.def.Alias }

// PrimaryKey returns the primary key field name.
func (c *CompiledList) PrimaryKey() string { return c.def.PrimaryKey }

// Scope returns the static base scope, possibly empty.
func (c *CompiledList) Scope() string { return c.def.Scope }

// DefaultPageSize returns the resolved default page size.
func (c *CompiledList) DefaultPageSize() int { return c.def.DefaultPageSize }

// MaxPageSize returns the resolved maximum page size.
func (c *CompiledList) MaxPageSize() int { return c.def.MaxPageSize }

// Fields returns every field in declaration order, with titles defaulted.
func (c *CompiledList) Fields() []Field { return slices.Clone(c.def.Fields) }

// Field returns the field with the given name.
func (c *CompiledList) Field(name string) (Field, bool) {
	f, ok := c.fields[name]
	if !ok {
		return Field{}, false
	}
	return *f, true
}

// JoinOrder returns every join, each after the joins it depends on, with ties
// broken by declaration order.
func (c *CompiledList) JoinOrder() []Join { return slices.Clone(c.joinOrder) }

// TypePrefix returns the PascalCase form of the list name used to prefix its
// GraphQL type names, for example OrderItems for order_items.
func (c *CompiledList) TypePrefix() string { return PascalCase(c.def.Name) }

// typeNames lists every GraphQL type name generated for the list.
func (c *CompiledList) typeNames() []string {
	p := c.TypePrefix()
	names := []string{p + "Row", p + "Page", p + "SortField", p + "Sort", p + "ListInput", p + "ExportField", p + "ExportInput"}
	for _, f := range c.def.Fields {
		if f.Type == Enum {
			names = append(names, p+PascalCase(f.Name))
		}
	}
	return names
}

// PascalCase joins the underscore-separated words of name, capitalizing each.
func PascalCase(name string) string {
	var b strings.Builder
	for _, word := range strings.Split(name, "_") {
		if word != "" {
			b.WriteString(strings.ToUpper(word[:1]) + word[1:])
		}
	}
	return b.String()
}

// EnumName maps a field name or enum value to its GraphQL enum value name:
// upper case, with every character other than an ASCII letter or digit
// replaced by an underscore.
func EnumName(value string) string {
	b := []byte(strings.ToUpper(value))
	for i, ch := range b {
		if !(ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9') {
			b[i] = '_'
		}
	}
	return string(b)
}

func compileList(l List) (*CompiledList, error) {
	fail := func(format string, args ...any) error {
		return fmt.Errorf("list %q: "+format, append([]any{l.Name}, args...)...)
	}
	if !listNamePattern.MatchString(l.Name) {
		return nil, fmt.Errorf("invalid list name %q: must match %s", l.Name, listNamePattern)
	}
	if strings.TrimSpace(l.Table) == "" {
		return nil, fail("table is required")
	}
	if strings.TrimSpace(l.Alias) == "" {
		return nil, fail("alias is required")
	}
	if len(l.Fields) == 0 {
		return nil, fail("at least one SQL field is required")
	}

	if l.MaxPageSize == 0 {
		l.MaxPageSize = maxPageSize
	}
	if l.MaxPageSize < 1 {
		return nil, fail("maximum page size %d must be at least 1", l.MaxPageSize)
	}
	if l.DefaultPageSize == 0 {
		l.DefaultPageSize = min(defaultPageSize, l.MaxPageSize)
	}
	if l.DefaultPageSize < 1 || l.DefaultPageSize > l.MaxPageSize {
		return nil, fail("default page size %d must be between 1 and the maximum page size %d", l.DefaultPageSize, l.MaxPageSize)
	}

	c := &CompiledList{
		fields:    make(map[string]*Field, len(l.Fields)),
		joinIndex: make(map[string]int, len(l.Joins)),
		joinsOf:   map[string][]string{},
	}
	joins := make(map[string]Join, len(l.Joins))
	for _, j := range l.Joins {
		if !fieldNamePattern.MatchString(j.Key) {
			return nil, fail("invalid join key %q: must match %s", j.Key, fieldNamePattern)
		}
		if _, dup := joins[j.Key]; dup {
			return nil, fail("duplicate join key %q", j.Key)
		}
		if strings.TrimSpace(j.SQL) == "" {
			return nil, fail("join %q has no SQL", j.Key)
		}
		j.DependsOn = slices.Clone(j.DependsOn)
		joins[j.Key] = j
	}
	for _, j := range l.Joins {
		for _, dep := range j.DependsOn {
			if _, ok := joins[dep]; !ok {
				return nil, fail("join %q depends on unknown join %q", j.Key, dep)
			}
		}
	}
	order, err := topoSort(l.Joins)
	if err != nil {
		return nil, fail("%v", err)
	}
	for i, j := range order {
		c.joinIndex[j.Key] = i
		c.joinOrder = append(c.joinOrder, joins[j.Key])
	}

	fields := make([]Field, len(l.Fields))
	sqlFields := 0
	for i, f := range l.Fields {
		if !fieldNamePattern.MatchString(f.Name) {
			return nil, fail("invalid field name %q: must match %s", f.Name, fieldNamePattern)
		}
		if _, dup := c.fields[f.Name]; dup {
			return nil, fail("duplicate field name %q", f.Name)
		}
		if !f.Type.valid() {
			return nil, fail("field %q has no valid type", f.Name)
		}
		if err := checkEnum(f); err != nil {
			return nil, fail("%v", err)
		}
		if len(f.Values) > 0 && f.Type != Enum {
			return nil, fail("field %q declares values but is not an enum", f.Name)
		}
		if f.Title == "" {
			f.Title = f.Name
		}
		f.Values = slices.Clone(f.Values)
		f.Joins = slices.Clone(f.Joins)
		if f.calc != nil {
			if f.SQL != "" || len(f.Joins) > 0 {
				return nil, fail("calculated field %q cannot declare SQL or joins", f.Name)
			}
			if f.calc.goType != f.Type.goType() {
				return nil, fail("calculated field %q of type %s must return %s, not %s", f.Name, f.Type, f.Type.goType(), f.calc.goType)
			}
		} else {
			sqlFields++
			for _, key := range f.Joins {
				if _, ok := joins[key]; !ok {
					return nil, fail("field %q references unknown join %q", f.Name, key)
				}
			}
			c.joinsOf[f.Name] = c.closeJoins(joins, f.Joins)
		}
		fields[i] = f
		c.fields[f.Name] = &fields[i]
	}
	if sqlFields == 0 {
		return nil, fail("at least one SQL field is required")
	}
	for _, f := range fields {
		if f.calc == nil {
			continue
		}
		for _, req := range f.calc.requires {
			dep, ok := c.fields[req]
			if !ok {
				return nil, fail("calculated field %q requires unknown field %q", f.Name, req)
			}
			if dep.calc != nil {
				return nil, fail("calculated field %q requires calculated field %q; only SQL fields may be required", f.Name, req)
			}
		}
	}

	if l.PrimaryKey == "" {
		return nil, fail("primary key is required")
	}
	pk, ok := c.fields[l.PrimaryKey]
	if !ok || pk.calc != nil {
		return nil, fail("primary key %q is not an SQL field", l.PrimaryKey)
	}
	if pk.Nullable {
		return nil, fail("primary key %q must not be nullable", l.PrimaryKey)
	}

	l.Fields = fields
	l.Joins = slices.Clone(l.Joins)
	c.def = l
	return c, nil
}

func checkEnum(f Field) error {
	if f.Type != Enum {
		return nil
	}
	if len(f.Values) == 0 {
		return fmt.Errorf("enum field %q declares no values", f.Name)
	}
	seen := map[string]string{}
	for _, v := range f.Values {
		name := EnumName(v)
		if !graphQLName.MatchString(name) || name == "TRUE" || name == "FALSE" || name == "NULL" {
			return fmt.Errorf("enum field %q value %q maps to invalid GraphQL enum value %q", f.Name, v, name)
		}
		if prev, dup := seen[name]; dup {
			if prev == v {
				return fmt.Errorf("enum field %q declares value %q twice", f.Name, v)
			}
			return fmt.Errorf("enum field %q values %q and %q both map to GraphQL enum value %q", f.Name, prev, v, name)
		}
		seen[name] = v
	}
	return nil
}

// closeJoins returns keys and everything they transitively depend on, in
// topological order.
func (c *CompiledList) closeJoins(joins map[string]Join, keys []string) []string {
	need := map[string]bool{}
	stack := slices.Clone(keys)
	for len(stack) > 0 {
		key := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if need[key] {
			continue
		}
		need[key] = true
		stack = append(stack, joins[key].DependsOn...)
	}
	return c.sortJoins(need)
}

func (c *CompiledList) sortJoins(need map[string]bool) []string {
	out := make([]string, 0, len(need))
	for key := range need {
		out = append(out, key)
	}
	slices.SortFunc(out, func(a, b string) int { return c.joinIndex[a] - c.joinIndex[b] })
	return out
}

// topoSort orders joins so dependencies come first. Among joins whose
// dependencies are placed, the earliest declared goes next.
func topoSort(joins []Join) ([]Join, error) {
	index := make(map[string]int, len(joins))
	for i, j := range joins {
		index[j.Key] = i
	}
	placed := make([]bool, len(joins))
	out := make([]Join, 0, len(joins))
	for len(out) < len(joins) {
		next := -1
		for i, j := range joins {
			if placed[i] {
				continue
			}
			ready := true
			for _, dep := range j.DependsOn {
				if !placed[index[dep]] {
					ready = false
					break
				}
			}
			if ready {
				next = i
				break
			}
		}
		if next < 0 {
			return nil, fmt.Errorf("join dependency cycle: %s", findCycle(joins, index, placed))
		}
		placed[next] = true
		out = append(out, joins[next])
	}
	return out, nil
}

// findCycle walks dependencies among unplaced joins until a key repeats. Every
// unplaced join has an unplaced dependency, so the walk always finds one.
func findCycle(joins []Join, index map[string]int, placed []bool) string {
	start := 0
	for placed[start] {
		start++
	}
	var path []string
	seen := map[string]int{}
	key := joins[start].Key
	for {
		if at, ok := seen[key]; ok {
			return strings.Join(append(path[at:], key), " -> ")
		}
		seen[key] = len(path)
		path = append(path, key)
		for _, dep := range joins[index[key]].DependsOn {
			if !placed[index[dep]] {
				key = dep
				break
			}
		}
	}
}
