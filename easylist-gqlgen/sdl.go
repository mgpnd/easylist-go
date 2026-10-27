package easylistgql

import (
	"fmt"
	"strings"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
)

// Scalars names the GraphQL scalars the plugin declares. Empty names default
// to Int64, Decimal, Date, UUID, and Time. Rename one when the application
// schema already declares a scalar with the default name.
type Scalars struct {
	Int64, Decimal, Date, UUID, Time string
}

func (s Scalars) withDefaults() Scalars {
	set := func(v *string, def string) {
		if *v == "" {
			*v = def
		}
	}
	set(&s.Int64, "Int64")
	set(&s.Decimal, "Decimal")
	set(&s.Date, "Date")
	set(&s.UUID, "UUID")
	set(&s.Time, "Time")
	return s
}

// scalarFor returns the GraphQL scalar of a non-enum type, and whether the
// plugin must declare it.
func (s Scalars) scalarFor(t easylist.Type) (name string, custom bool) {
	switch t {
	case easylist.Text:
		return "String", false
	case easylist.Integer:
		return s.Int64, true
	case easylist.Float:
		return "Float", false
	case easylist.Numeric:
		return s.Decimal, true
	case easylist.Boolean:
		return "Boolean", false
	case easylist.Date:
		return s.Date, true
	case easylist.Timestamp:
		return s.Time, true
	case easylist.UUID:
		return s.UUID, true
	}
	panic(fmt.Sprintf("easylistgql: no scalar for type %s", t))
}

// graphQLType returns a field's GraphQL type, without the outer non-null
// marker. Arrays are lists of non-null elements.
func graphQLType(l *easylist.CompiledList, f easylist.Field, s Scalars) string {
	if f.Array {
		return "[" + elementType(l, f, s) + "!]"
	}
	return elementType(l, f, s)
}

// elementType returns the GraphQL type of a field's values, or of its
// elements when it is an array: a scalar, or the field's generated enum.
func elementType(l *easylist.CompiledList, f easylist.Field, s Scalars) string {
	if f.Type == easylist.Enum {
		return l.TypePrefix() + easylist.PascalCase(f.Name)
	}
	name, _ := s.scalarFor(f.Type)
	return name
}

// sortable reports whether a field can be a sort entry.
func sortable(f easylist.Field) bool { return !f.Calculated() && !f.Array }

// listSDL returns the types generated for one list.
func listSDL(l *easylist.CompiledList, s Scalars) string {
	p := l.TypePrefix()
	var b strings.Builder
	fields := l.Fields()

	fmt.Fprintf(&b, "type %sRow {\n", p)
	for _, f := range fields {
		bang := "!"
		if f.Nullable {
			bang = ""
		}
		fmt.Fprintf(&b, "  %s: %s%s\n", f.Name, graphQLType(l, f, s), bang)
	}
	b.WriteString("}\n\n")

	fmt.Fprintf(&b, "type %sPage {\n  items: [%sRow!]!\n  totalCount: Int!\n}\n\n", p, p)

	for _, f := range fields {
		if f.Type != easylist.Enum {
			continue
		}
		fmt.Fprintf(&b, "enum %s {\n", elementType(l, f, s))
		for _, v := range f.Values {
			fmt.Fprintf(&b, "  %s\n", easylist.EnumName(v))
		}
		b.WriteString("}\n\n")
	}

	fmt.Fprintf(&b, "enum %sSortField {\n", p)
	for _, f := range fields {
		if sortable(f) {
			fmt.Fprintf(&b, "  %s\n", easylist.EnumName(f.Name))
		}
	}
	b.WriteString("}\n\n")

	fmt.Fprintf(&b, "input %sSort {\n  field: %sSortField!\n  direction: SortDirection! = ASC\n}\n\n", p, p)
	fmt.Fprintf(&b, "input %sListInput {\n  filter: String\n  sort: [%sSort!]\n  limit: Int\n  offset: Int\n%s}\n\n", p, p, timezoneSDL)

	fmt.Fprintf(&b, "enum %sExportField {\n", p)
	for _, f := range fields {
		fmt.Fprintf(&b, "  %s\n", easylist.EnumName(f.Name))
	}
	b.WriteString("}\n\n")

	fmt.Fprintf(&b, "input %sExportInput {\n  fields: [%sExportField!]!\n  filter: String\n  sort: [%sSort!]\n  format: ExportFormat!\n%s", p, p, p, timezoneSDL)
	fmt.Fprintf(&b, asOfSDL+"}\n", s.Time)
	return b.String()
}

// timezoneSDL is the documented timezone field of list and export inputs.
const timezoneSDL = `  """
  The client's UTC offset in minutes, positive east of UTC (UTC+02:00 is 120),
  between -720 and 840. Relative time values such as TODAY are evaluated in
  this zone. When omitted, the server's local time zone is used.
  """
  timezone: Int
`

// asOfSDL is the documented asOf field of export inputs, a format string
// taking the timestamp scalar's name.
const asOfSDL = `  """
  The moment at which relative time values such as TODAY are evaluated. When
  omitted, the server's current time is used.
  """
  asOf: %s
`

// usedScalars returns the custom scalars some list field uses, plus the
// timestamp scalar whenever there is a list, because every export input has
// asOf.
func usedScalars(lists []*easylist.CompiledList, s Scalars) map[string]bool {
	used := map[string]bool{}
	if len(lists) > 0 {
		used[s.Time] = true
	}
	for _, l := range lists {
		for _, f := range l.Fields() {
			if f.Type == easylist.Enum {
				continue
			}
			if name, custom := s.scalarFor(f.Type); custom {
				used[name] = true
			}
		}
	}
	return used
}

// sharedSDL returns the types declared once for all lists: SortDirection,
// ExportFormat, ExportJob, and every custom scalar some list uses.
func sharedSDL(lists []*easylist.CompiledList, s Scalars) string {
	used := usedScalars(lists, s)
	var b strings.Builder
	b.WriteString("enum SortDirection {\n  ASC\n  DESC\n}\n\n")
	b.WriteString("enum ExportFormat {\n  CSV\n  XLSX\n}\n\n")
	b.WriteString("type ExportJob {\n  id: ID!\n}\n")
	for _, name := range []string{s.Int64, s.Decimal, s.Date, s.UUID, s.Time} {
		if used[name] {
			fmt.Fprintf(&b, "\nscalar %s\n", name)
		}
	}
	return b.String()
}
