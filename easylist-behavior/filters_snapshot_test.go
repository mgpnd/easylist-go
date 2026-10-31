package behavior_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mgpnd/easylist-go/easylist-behavior/lists"
	easylist "github.com/mgpnd/easylist-go/easylist-core"
	filter "github.com/mgpnd/easylist-go/easylist-filter"
	sqlgorm "github.com/mgpnd/easylist-go/easylist-sql-gorm"
)

// TestFilterSnapshots runs every case of every testdata/filters/<family>.txt
// against the people list and writes the matched rows to <family>.golden
// and the generated SQL to <family>.sql.golden.
func TestFilterSnapshots(t *testing.T) {
	db := openDB(t)
	people, ok := lists.Registry.List("people")
	if !ok {
		t.Fatal(`lists.Registry has no list "people"`)
	}
	files, err := filepath.Glob(filepath.Join("testdata", "filters", "*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("testdata/filters holds no fixture")
	}
	driver := &recordingDriver{Driver: sqlgorm.New()}
	rt := &easylist.Runtime{Driver: driver, DB: easylist.SQLDB(db), Clock: clock}

	keep := map[string]bool{}
	for _, file := range files {
		family := strings.TrimSuffix(filepath.Base(file), ".txt")
		keep[family+".golden"] = true
		keep[family+".sql.golden"] = true
		t.Run(family, func(t *testing.T) {
			src, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			cases, err := parseFilterCases(file, string(src))
			if err != nil {
				t.Fatal(err)
			}
			var rows, sqls []string
			for _, c := range cases {
				driver.calls = nil
				rows = append(rows, runFilterCase(t, rt, people, c))
				if driver.calls != nil {
					sqls = append(sqls, sqlOutcome(c, driver.calls))
				}
			}
			if sqls == nil {
				sqls = []string{"# No case of " + family + ".txt compiles to SQL.\n"}
			}
			golden(t, "filters/"+family+".golden", strings.Join(rows, "\n"))
			golden(t, "filters/"+family+".sql.golden", strings.Join(sqls, "\n"))
		})
	}
	prune(t, "filters", keep, ".golden")
}

// compiled is one query a driver compiled.
type compiled struct {
	kind  easylist.Kind
	query string
	args  []any
}

// recordingDriver records the queries its driver compiles successfully.
// It is not safe for concurrent use.
type recordingDriver struct {
	easylist.Driver
	calls []compiled
}

func (d *recordingDriver) Compile(p *easylist.Plan) (string, []any, error) {
	query, args, err := d.Driver.Compile(p)
	if err == nil {
		d.calls = append(d.calls, compiled{p.Kind, query, args})
	}
	return query, args, err
}

// echo renders the comment lines and the expression of c.
func echo(c filterCase) string {
	var sb strings.Builder
	for _, line := range c.Comments {
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	sb.WriteString(c.Expr)
	sb.WriteByte('\n')
	return sb.String()
}

// runFilterCase fetches the people matching c and renders the outcome: the
// echoed case, then the row count and a table of the rows, or the error.
func runFilterCase(t *testing.T, rt *easylist.Runtime, l *easylist.CompiledList, c filterCase) string {
	out := echo(c)
	expr, err := filter.Parse(c.Expr)
	if err != nil {
		return out + "=> error: " + err.Error() + "\n"
	}
	fields := filterFields(l, expr)
	limit := l.MaxPageSize()
	res, err := rt.Fetch(context.Background(), l, easylist.Request{
		Fields: append([]string{"name"}, fields...),
		Filter: expr,
		Limit:  &limit,
	}, easylist.Want{Rows: true, Count: true})
	if err != nil {
		return out + "=> error: " + err.Error() + "\n"
	}
	if int64(len(res.Rows)) != res.Total {
		t.Errorf("%s:%d: the page holds %d rows, but the count query says %d", c.Expr, c.Line, len(res.Rows), res.Total)
	}
	noun := "rows"
	if len(res.Rows) == 1 {
		noun = "row"
	}
	out += fmt.Sprintf("=> %d %s\n", len(res.Rows), noun)
	if len(res.Rows) == 0 {
		return out
	}
	return out + rowTable(l, append([]string{"id", "name"}, fields...), res.Rows)
}

// filterFields returns the fields expr references, in order of first
// appearance, without duplicates, id, and name. It leaves out names that
// are not SQL fields of l: selecting an unknown field would replace the
// filter's own error, and calculated fields cannot be filtered on anyway.
func filterFields(l *easylist.CompiledList, expr filter.Expr) []string {
	var names []string
	var walk func(filter.Expr)
	walk = func(e filter.Expr) {
		switch e := e.(type) {
		case filter.Logical:
			walk(e.Left())
			walk(e.Right())
		case filter.Predicate:
			name := e.Field()
			f, ok := l.Field(name)
			if ok && !f.Calculated() && name != "id" && name != "name" && !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	}
	walk(expr)
	return names
}

// rowTable renders rows as an ASCII table of the given fields, with a header
// line. Numbers are right-aligned, everything else left-aligned. Each line
// starts with two spaces.
func rowTable(l *easylist.CompiledList, fields []string, rows []easylist.Row) string {
	cells := make([][]string, len(rows)+1)
	cells[0] = fields
	for i, r := range rows {
		cells[i+1] = make([]string, len(fields))
		for j, name := range fields {
			f, _ := l.Field(name)
			cells[i+1][j] = renderValue(f, r)
		}
	}
	widths := make([]int, len(fields))
	right := make([]bool, len(fields))
	for j, name := range fields {
		f, _ := l.Field(name)
		right[j] = !f.Array && (f.Type == easylist.Integer || f.Type == easylist.Float || f.Type == easylist.Numeric)
		for _, line := range cells {
			widths[j] = max(widths[j], len(line[j]))
		}
	}
	var sb strings.Builder
	for _, line := range cells {
		parts := make([]string, len(line))
		for j, cell := range line {
			if right[j] {
				parts[j] = fmt.Sprintf("%*s", widths[j], cell)
			} else {
				parts[j] = fmt.Sprintf("%-*s", widths[j], cell)
			}
		}
		sb.WriteString(strings.TrimRight("  "+strings.Join(parts, " | "), " "))
		sb.WriteByte('\n')
	}
	return sb.String()
}

// renderValue renders the value of field f in row r: NULL, text and enums
// as they are, numbers in their exact text, dates as 2006-01-02, timestamps
// as RFC 3339 in UTC, and arrays as [a, b].
func renderValue(f easylist.Field, r easylist.Row) string {
	v, ok := r.Value(f.Name)
	if !ok {
		return "NULL"
	}
	scalar := func(v any) string {
		switch v := v.(type) {
		case string:
			return v
		case int64:
			return strconv.FormatInt(v, 10)
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64)
		case bool:
			return strconv.FormatBool(v)
		case time.Time:
			if f.Type == easylist.Date {
				return v.Format(time.DateOnly)
			}
			return v.UTC().Format(time.RFC3339)
		}
		return fmt.Sprintf("<%T %v>", v, v)
	}
	var items []string
	switch v := v.(type) {
	case []string:
		items = v
	case []int64:
		items = mapSlice(v, func(x int64) string { return scalar(x) })
	case []float64:
		items = mapSlice(v, func(x float64) string { return scalar(x) })
	case []bool:
		items = mapSlice(v, func(x bool) string { return scalar(x) })
	case []time.Time:
		items = mapSlice(v, func(x time.Time) string { return scalar(x) })
	default:
		return scalar(v)
	}
	return "[" + strings.Join(items, ", ") + "]"
}

func mapSlice[T any](s []T, fn func(T) string) []string {
	out := make([]string, len(s))
	for i, x := range s {
		out[i] = fn(x)
	}
	return out
}

// sqlOutcome renders the queries compiled for c: the echoed case, then each
// query with its arguments.
func sqlOutcome(c filterCase, calls []compiled) string {
	var sb strings.Builder
	sb.WriteString(echo(c))
	for _, call := range calls {
		kind := "page"
		if call.kind == easylist.KindCount {
			kind = "count"
		}
		fmt.Fprintf(&sb, "=> %s\n", kind)
		for line := range strings.SplitSeq(call.query, "\n") {
			sb.WriteString(strings.TrimRight("  "+line, " \t"))
			sb.WriteByte('\n')
		}
		fmt.Fprintf(&sb, "  args: %s\n", renderArgs(call.args))
	}
	return sb.String()
}

// renderArgs renders bound arguments as a Go-like literal list that names
// each argument's type, with times in RFC 3339 in UTC.
func renderArgs(args []any) string {
	items := make([]string, len(args))
	for i, a := range args {
		switch a := a.(type) {
		case string:
			items[i] = strconv.Quote(a)
		case bool:
			items[i] = strconv.FormatBool(a)
		case int64:
			items[i] = "int64(" + strconv.FormatInt(a, 10) + ")"
		case int:
			items[i] = "int(" + strconv.Itoa(a) + ")"
		case float64:
			items[i] = "float64(" + strconv.FormatFloat(a, 'g', -1, 64) + ")"
		case time.Time:
			items[i] = "time(" + a.UTC().Format(time.RFC3339Nano) + ")"
		default:
			items[i] = fmt.Sprintf("%#v", a)
		}
	}
	return "[" + strings.Join(items, ", ") + "]"
}
