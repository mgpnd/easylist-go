package sqlpg_test

import (
	"reflect"
	"testing"
	"time"
	_ "time/tzdata" // Europe/London without a system tz database

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	filter "github.com/mgpnd/easylist-go/easylist-filter"
	"github.com/mgpnd/easylist-go/easylist-sqlpg"
	"github.com/mgpnd/easylist-go/easylist-sqlpg/internal/refdriver"
)

// The lists below are the ones easylist-sql-goqu's tests use, so the
// expected SQL and errors here are the ones that driver produced before the
// lowering moved to this package.

var usersList = easylist.List{
	Name: "users", Table: "users", Alias: "u", PrimaryKey: "id",
	Fields: []easylist.Field{
		{Name: "id", Type: easylist.Integer},
		{Name: "name", Type: easylist.Text},
		{Name: "age", Type: easylist.Integer, Nullable: true},
		{Name: "status", Type: easylist.Text, Nullable: true},
		{Name: "country", Type: easylist.Text, Nullable: true, SQL: "c.name", Joins: []string{"country"}},
		{Name: "last_order", Type: easylist.Timestamp, Nullable: true, SQL: "lo.created_at", Joins: []string{"last_order"}},
	},
	Joins: []easylist.Join{
		{Key: "country", SQL: "LEFT JOIN countries c ON c.id = u.country_id"},
		{Key: "last_order", SQL: "LEFT JOIN LATERAL (SELECT created_at FROM orders o WHERE o.user_id = u.id ORDER BY created_at DESC LIMIT 1) lo ON TRUE"},
	},
}

// arraysList has a base-column array, an enum array, and an author-SQL array.
var arraysList = easylist.List{
	Name: "runs", Table: "runs", Alias: "u", PrimaryKey: "id",
	Fields: []easylist.Field{
		{Name: "id", Type: easylist.Integer},
		{Name: "tags", Type: easylist.Text, Array: true, Nullable: true},
		{Name: "statuses", Type: easylist.Enum, Array: true, Values: []string{"ok", "passed", "skipped", "failed"}},
		{Name: "labels", Type: easylist.Text, Array: true, SQL: "l.names", Joins: []string{"labels"}},
	},
	Joins: []easylist.Join{
		{Key: "labels", SQL: "LEFT JOIN LATERAL (SELECT array_agg(name ORDER BY name) AS names FROM labels WHERE run_id = u.id) l ON TRUE"},
	},
}

// predicateList has one scalar and one array field per type, plus fields
// read through author SQL.
var predicateList = easylist.List{
	Name: "t", Table: "t", Alias: "t", PrimaryKey: "uid",
	Fields: []easylist.Field{
		{Name: "uid", Type: easylist.UUID},
		{Name: "name", Type: easylist.Text},
		{Name: "age", Type: easylist.Integer},
		{Name: "score", Type: easylist.Float},
		{Name: "price", Type: easylist.Numeric},
		{Name: "day", Type: easylist.Date},
		{Name: "at", Type: easylist.Timestamp},
		{Name: "active", Type: easylist.Boolean},
		{Name: "status", Type: easylist.Enum, Values: []string{"active", "blocked"}},
		{Name: "full", Type: easylist.Text, SQL: "t.first || ' ' || t.last"},
		{Name: "tags", Type: easylist.Text, Array: true, Nullable: true},
		{Name: "ids", Type: easylist.Integer, Array: true},
		{Name: "scores", Type: easylist.Float, Array: true},
		{Name: "prices", Type: easylist.Numeric, Array: true},
		{Name: "flags", Type: easylist.Boolean, Array: true},
		{Name: "days", Type: easylist.Date, Array: true},
		{Name: "seen", Type: easylist.Timestamp, Array: true},
		{Name: "groups", Type: easylist.UUID, Array: true},
		{Name: "statuses", Type: easylist.Enum, Array: true, Values: []string{"ok", "skipped", "failed"}},
		{Name: "labels", Type: easylist.Text, Array: true, SQL: "l.names"},
	},
}

var tasksList = easylist.List{
	Name: "tasks", Table: "tasks", Alias: "u", PrimaryKey: "id",
	Fields: []easylist.Field{
		{Name: "id", Type: easylist.Integer},
		{Name: "created_at", Type: easylist.Timestamp},
		{Name: "due", Type: easylist.Date, Nullable: true},
		{Name: "price", Type: easylist.Numeric},
		{Name: "closed_at", Type: easylist.Timestamp, Nullable: true, SQL: "c.closed_at", Joins: []string{"closed"}},
	},
	Joins: []easylist.Join{{Key: "closed", SQL: "LEFT JOIN closings c ON c.task_id = u.id"}},
}

var (
	utc    = time.UTC
	plus2  = time.FixedZone("", 2*3600)
	nowUTC = tm(2026, 10, 27, 10, 0, utc)   // Tuesday
	nowP2  = tm(2026, 10, 27, 10, 0, plus2) // Tuesday, 08:00Z
)

// tm returns a whole minute in loc.
func tm(y int, m time.Month, d, h, min int, loc *time.Location) time.Time {
	return time.Date(y, m, d, h, min, 0, 0, loc)
}

func londonZone(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func intp(n int) *int { return &n }

func plan(t *testing.T, def easylist.List, kind easylist.Kind, req easylist.Request, filterText string) *easylist.Plan {
	t.Helper()
	reg, err := easylist.NewRegistry(def)
	if err != nil {
		t.Fatal(err)
	}
	l, _ := reg.List(def.Name)
	if filterText != "" {
		if req.Filter, err = filter.Parse(filterText); err != nil {
			t.Fatal(err)
		}
	}
	p, err := l.Plan(kind, req)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// compile renders a plan through the reference driver.
func compile(t *testing.T, def easylist.List, kind easylist.Kind, req easylist.Request, filterText string) (string, []any) {
	t.Helper()
	sql, args, err := refdriver.Driver{}.Compile(plan(t, def, kind, req, filterText))
	if err != nil {
		t.Fatal(err)
	}
	return sql, args
}

func parsePredicate(t *testing.T, input string) filter.Predicate {
	t.Helper()
	expr, err := filter.Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	predicate, ok := expr.(filter.Predicate)
	if !ok {
		t.Fatalf("want predicate, got %T", expr)
	}
	return predicate
}

// predicatePlan is a page plan of predicateList without a filter.
func predicatePlan(t *testing.T) *easylist.Plan {
	t.Helper()
	return plan(t, predicateList, easylist.KindPage, easylist.Request{}, "")
}

// withFilter returns a copy of p whose filter is f, a tree the planner has
// not validated, such as one with an unknown field or a rewritten operator.
func withFilter(p *easylist.Plan, f filter.Expr) *easylist.Plan {
	c := *p
	c.Filter = f
	return &c
}

// lowerCount lowers a count plan of predicateList filtered by input at now,
// so the query has only the predicate's arguments.
func lowerCount(t *testing.T, input string, now time.Time) *sqlpg.Query {
	t.Helper()
	q, err := sqlpg.Lower(plan(t, predicateList, easylist.KindCount, easylist.Request{Now: now}, input))
	if err != nil {
		t.Fatal(err)
	}
	return q
}

// sameArgs compares times as instants and everything else exactly.
func sameArgs(got, want []any) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if w, ok := want[i].(time.Time); ok {
			if g, ok := got[i].(time.Time); !ok || !g.Equal(w) {
				return false
			}
			continue
		}
		if !reflect.DeepEqual(got[i], want[i]) {
			return false
		}
	}
	return true
}

// rewrittenOperator rewrites a parsed predicate's operator, to build trees the
// parser itself never produces, such as a NULL value after >.
type rewrittenOperator struct {
	filter.Predicate
	op filter.PredicateOp
}

func (r rewrittenOperator) Operator() filter.PredicateOp { return r.op }
