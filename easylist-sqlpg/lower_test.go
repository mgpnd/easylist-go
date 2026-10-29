package sqlpg_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	filter "github.com/mgpnd/easylist-go/easylist-filter"
	"github.com/mgpnd/easylist-go/easylist-sqlpg"
)

// The errors are the exact texts easylist-sql-goqu returned for the same
// inputs in its TestBuildPredicateRejects* tests before the lowering moved
// here. Drivers return them unchanged, so they must stay byte-identical.
func TestLowerErrors(t *testing.T) {
	cases := []struct{ filter, want string }{
		// Literal conversion.
		{"age = 9223372036854775808", "invalid integer value \"9223372036854775808\" for field \"age\" at position 7"},
		{"age = -9223372036854775809", "invalid integer value \"-9223372036854775809\" for field \"age\" at position 7"},
		{"age = 1.5", "invalid integer value \"1.5\" for field \"age\" at position 7"},
		{"age = '30'", "invalid integer value \"30\" for field \"age\" at position 7"},
		{"day = '2025-02-29'", "invalid date value \"2025-02-29\" for field \"day\" at position 7"},
		{"day = '2024-2-29'", "invalid date value \"2024-2-29\" for field \"day\" at position 7"},
		{"day = '2024-02-29T00:00:00Z'", "invalid date value \"2024-02-29T00:00:00Z\" for field \"day\" at position 7"},
		{"day = 20240229", "invalid date value \"20240229\" for field \"day\" at position 7"},
		{"at > '2026-10-25T10:00:00'", "invalid timestamp value \"2026-10-25T10:00:00\" for field \"at\" at position 6: timestamps need an offset such as Z or +02:00"},
		{"at = '2026-10-25'", "invalid timestamp value \"2026-10-25\" for field \"at\" at position 6"},
		{"at = 1", "invalid timestamp value \"1\" for field \"at\" at position 6"},
		{"uid = 'not-a-uuid'", "invalid uuid value \"not-a-uuid\" for field \"uid\" at position 7"},
		{"uid = '0b6c4c1e3d8e4f439d5a2a8a0d1d2c3b'", "invalid uuid value \"0b6c4c1e3d8e4f439d5a2a8a0d1d2c3b\" for field \"uid\" at position 7"},
		{"uid = 1", "invalid uuid value \"1\" for field \"uid\" at position 7"},
		{"status = 'archived'", "invalid enum value \"archived\" for field \"status\" at position 10: not a declared value"},
		{"status = 'Active'", "invalid enum value \"Active\" for field \"status\" at position 10: not a declared value"},
		{"status = 1", "invalid enum value \"1\" for field \"status\" at position 10"},
		{"age BETWEEN '1,2,3'", "BETWEEN requires two quoted comma-separated bounds at position 13"},
		{"age BETWEEN '1,'", "invalid integer value \"\" for field \"age\" at position 13"},
		{"age BETWEEN ',2'", "invalid integer value \"\" for field \"age\" at position 13"},
		{"age BETWEEN '1'", "BETWEEN requires two quoted comma-separated bounds at position 13"},
		{"age BETWEEN '1,2.5'", "invalid integer value \"2.5\" for field \"age\" at position 13"},
		{"at BETWEEN '2026-01-01T00:00:00Z,2026-02-01'", "invalid timestamp value \"2026-02-01\" for field \"at\" at position 12"},
		{"score BETWEEN 'NaN,2'", "invalid float value \"NaN\" for field \"score\" at position 15"},
		{"price BETWEEN '1,2e3'", "invalid numeric value \"2e3\" for field \"price\" at position 15"},
		{"score = " + strings.Repeat("9", 400), "invalid float value \"" + strings.Repeat("9", 400) + "\" for field \"score\" at position 9"},
		{"name = 1", "invalid text value \"1\" for field \"name\" at position 8"},
		{"active = 'true'", "invalid boolean value \"true\" for field \"active\" at position 10"},
		{"active = 1", "invalid boolean value \"1\" for field \"active\" at position 10"},
		{"age != 1.5", "invalid integer value \"1.5\" for field \"age\" at position 8"},
		{"name != 1", "invalid text value \"1\" for field \"name\" at position 9"},
		{"name EXCLUDES 1", "invalid text value \"1\" for field \"name\" at position 15"},
		{"active != 'true'", "invalid boolean value \"true\" for field \"active\" at position 11"},
		{"day != '2025-02-29'", "invalid date value \"2025-02-29\" for field \"day\" at position 8"},
		// Field lookup, on trees the planner has not checked.
		{"absent = 1", "unknown field \"absent\" at position 1"},
		{"\"Name\" = 'a'", "unknown field \"Name\" at position 1"},
		// Operators per type.
		{"active > TRUE", "unsupported operator \">\" for field \"active\" at position 8"},
		{"active < TRUE", "unsupported operator \"<\" for field \"active\" at position 8"},
		{"active BETWEEN 'true,false'", "unsupported operator \"BETWEEN\" for field \"active\" at position 8"},
		{"name > 'a'", "unsupported operator \">\" for field \"name\" at position 6"},
		{"name BETWEEN 'a,b'", "unsupported operator \"BETWEEN\" for field \"name\" at position 6"},
		{"age CONTAINS 1", "unsupported operator \"CONTAINS\" for field \"age\" at position 5"},
		{"age EXCLUDES 1", "unsupported operator \"EXCLUDES\" for field \"age\" at position 5"},
		{"price EXCLUDES 1", "unsupported operator \"EXCLUDES\" for field \"price\" at position 7"},
		{"active EXCLUDES TRUE", "unsupported operator \"EXCLUDES\" for field \"active\" at position 8"},
		{"uid EXCLUDES 'a'", "unsupported operator \"EXCLUDES\" for field \"uid\" at position 5"},
		{"status EXCLUDES 'act'", "unsupported operator \"EXCLUDES\" for field \"status\" at position 8"},
		{"day EXCLUDES '2024-01-01'", "unsupported operator \"EXCLUDES\" for field \"day\" at position 5"},
		{"uid > 'a'", "unsupported operator \">\" for field \"uid\" at position 5"},
		{"uid CONTAINS 'a'", "unsupported operator \"CONTAINS\" for field \"uid\" at position 5"},
		{"status >= 'active'", "unsupported operator \">=\" for field \"status\" at position 8"},
		{"status CONTAINS 'act'", "unsupported operator \"CONTAINS\" for field \"status\" at position 8"},
		{"day CONTAINS '2024-01-01'", "unsupported operator \"CONTAINS\" for field \"day\" at position 5"},
		{"at CONTAINS '2024'", "unsupported operator \"CONTAINS\" for field \"at\" at position 4"},
		{"age CONTAINS (1, 2)", "unsupported operator \"CONTAINS\" for field \"age\" at position 5"},
		{"status EXCLUDES ('active')", "unsupported operator \"EXCLUDES\" for field \"status\" at position 8"},
		{"active BETWEEN (TRUE, FALSE)", "unsupported operator \"BETWEEN\" for field \"active\" at position 8"},
		{"name BETWEEN ('a', 'b')", "unsupported operator \"BETWEEN\" for field \"name\" at position 6"},
		// Array operators.
		{"tags = 'go'", "unsupported operator \"=\" for field \"tags\" at position 6"},
		{"tags != 'go'", "unsupported operator \"!=\" for field \"tags\" at position 6"},
		{"ids > 3", "unsupported operator \">\" for field \"ids\" at position 5"},
		{"ids <= 3", "unsupported operator \"<=\" for field \"ids\" at position 5"},
		{"ids BETWEEN '1,2'", "unsupported operator \"BETWEEN\" for field \"ids\" at position 5"},
		{"days >= '2024-01-01'", "unsupported operator \">=\" for field \"days\" at position 6"},
		{"statuses = 'ok'", "unsupported operator \"=\" for field \"statuses\" at position 10"},
		{"flags = TRUE", "unsupported operator \"=\" for field \"flags\" at position 7"},
		{"labels != 'x'", "unsupported operator \"!=\" for field \"labels\" at position 8"},
		{"tags IN ('go')", "unsupported operator \"IN\" for field \"tags\" at position 6"},
		{"ids NOT IN (1, NULL)", "unsupported operator \"NOT IN\" for field \"ids\" at position 5"},
		{"statuses IN (NULL)", "unsupported operator \"IN\" for field \"statuses\" at position 10"},
		// Array values.
		{"tags CONTAINS 'a,,b'", "empty item in the value list for field \"tags\" at position 15"},
		{"tags CONTAINS ''", "empty item in the value list for field \"tags\" at position 15"},
		{"tags EXCLUDES 'a, '", "empty item in the value list for field \"tags\" at position 15"},
		{"ids CONTAINS '7,x'", "invalid integer value \"x\" for field \"ids\" at position 14"},
		{"ids CONTAINS '7,8.5'", "invalid integer value \"8.5\" for field \"ids\" at position 14"},
		{"ids CONTAINS 1.5", "invalid integer value \"1.5\" for field \"ids\" at position 14"},
		{"statuses EXCLUDES 'skipped,archived'", "invalid enum value \"archived\" for field \"statuses\" at position 19: not a declared value"},
		{"statuses CONTAINS 'OK'", "invalid enum value \"OK\" for field \"statuses\" at position 19: not a declared value"},
		{"tags CONTAINS 1", "invalid text value \"1\" for field \"tags\" at position 15"},
		{"flags CONTAINS 'yes'", "invalid boolean value \"yes\" for field \"flags\" at position 16"},
		{"prices CONTAINS '1,2e3'", "invalid numeric value \"2e3\" for field \"prices\" at position 17"},
		{"scores CONTAINS 'NaN'", "invalid float value \"NaN\" for field \"scores\" at position 17"},
		{"days CONTAINS '2025-02-29'", "invalid date value \"2025-02-29\" for field \"days\" at position 15"},
		{"seen CONTAINS '2026-01-01T00:00:00'", "invalid timestamp value \"2026-01-01T00:00:00\" for field \"seen\" at position 15: timestamps need an offset such as Z or +02:00"},
		{"groups CONTAINS 'a'", "invalid uuid value \"a\" for field \"groups\" at position 17"},
		// List values.
		{"status IN ('active', 'archived')", "invalid enum value \"archived\" for field \"status\" at position 22: not a declared value"},
		{"status NOT IN ('Active')", "invalid enum value \"Active\" for field \"status\" at position 16: not a declared value"},
		{"age IN (1, '30')", "invalid integer value \"30\" for field \"age\" at position 12"},
		{"age NOT IN (1.5)", "invalid integer value \"1.5\" for field \"age\" at position 13"},
		{"active IN ('true')", "invalid boolean value \"true\" for field \"active\" at position 12"},
		{"name IN (1)", "invalid text value \"1\" for field \"name\" at position 10"},
		{"uid IN ('not-a-uuid')", "invalid uuid value \"not-a-uuid\" for field \"uid\" at position 9"},
		{"day IN ('2024-02-29', ' 2025-01-01')", "invalid date value \" 2025-01-01\" for field \"day\" at position 23"},
		{"at IN ('2026-01-01T00:00:00')", "invalid timestamp value \"2026-01-01T00:00:00\" for field \"at\" at position 8: timestamps need an offset such as Z or +02:00"},
		{"age BETWEEN ('30', '40')", "invalid integer value \"30\" for field \"age\" at position 14"},
		{"age BETWEEN (30, 40.5)", "invalid integer value \"40.5\" for field \"age\" at position 18"},
		{"day BETWEEN (' 2024-02-29', '2025-01-01')", "invalid date value \" 2024-02-29\" for field \"day\" at position 14"},
		{"at BETWEEN ('2026-01-01T00:00:00Z', '2026-02-01')", "invalid timestamp value \"2026-02-01\" for field \"at\" at position 37"},
		{"ids CONTAINS (7, 'x')", "invalid integer value \"x\" for field \"ids\" at position 18"},
		{"ids CONTAINS (7, 8.5)", "invalid integer value \"8.5\" for field \"ids\" at position 18"},
		{"statuses EXCLUDES ('skipped', 'archived')", "invalid enum value \"archived\" for field \"statuses\" at position 31: not a declared value"},
		{"tags CONTAINS (1)", "invalid text value \"1\" for field \"tags\" at position 16"},
		{"flags CONTAINS ('true')", "invalid boolean value \"true\" for field \"flags\" at position 17"},
		{"name CONTAINS ('a', 'b')", "operator \"CONTAINS\" on text field \"name\" does not accept a value list at position 15"},
		{"name EXCLUDES ('a')", "operator \"EXCLUDES\" on text field \"name\" does not accept a value list at position 15"},
		{"full CONTAINS ('Lee')", "operator \"CONTAINS\" on text field \"full\" does not accept a value list at position 15"},
	}
	for _, tc := range cases {
		t.Run(tc.filter, func(t *testing.T) {
			q, err := sqlpg.Lower(withFilter(predicatePlan(t), parsePredicate(t, tc.filter)))
			if q != nil || err == nil || err.Error() != tc.want {
				t.Fatalf("got %#v %v\nwant error %s", q, err, tc.want)
			}
		})
	}
}

// Trees the parser never produces fail with an error rather than a panic or
// invalid SQL.
func TestLowerRejectsRewrittenTrees(t *testing.T) {
	cases := []struct {
		filter string
		op     filter.PredicateOp
		want   string
	}{
		{"age = NULL", filter.OpGt, "unsupported operator \">\" for field \"age\" at position 5"},
		{"age = NULL", filter.OpLte, "unsupported operator \"<=\" for field \"age\" at position 5"},
		{"age = NULL", filter.OpBetween, "unsupported operator \"BETWEEN\" for field \"age\" at position 5"},
		{"name = NULL", filter.OpContains, "unsupported operator \"CONTAINS\" for field \"name\" at position 6"},
		{"name = NULL", filter.OpExcludes, "unsupported operator \"EXCLUDES\" for field \"name\" at position 6"},
		{"age = 1", filter.OpIn, "operator \"IN\" requires a value list for field \"age\" at position 7"},
		{"name = 'a'", filter.OpNotIn, "operator \"NOT IN\" requires a value list for field \"name\" at position 8"},
		{"age IN (1, 2)", filter.OpEq, "invalid integer value \"(1, 2)\" for field \"age\" at position 8"},
		{"age IN (1, 2)", filter.OpGt, "invalid integer value \"(1, 2)\" for field \"age\" at position 8"},
		{"age IN (1)", filter.OpBetween, "BETWEEN requires a list of two bounds at position 8"},
		{"age IN (1, 2, 3)", filter.OpBetween, "BETWEEN requires a list of two bounds at position 8"},
		{"age IN (1, NULL)", filter.OpBetween, "NULL item in the value list for field \"age\" at position 12"},
		{"tags IN ('a', NULL)", filter.OpContains, "NULL item in the value list for field \"tags\" at position 15"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s as %d", tc.filter, tc.op), func(t *testing.T) {
			node := rewrittenOperator{parsePredicate(t, tc.filter), tc.op}
			q, err := sqlpg.Lower(withFilter(predicatePlan(t), node))
			if q != nil || err == nil || err.Error() != tc.want {
				t.Fatalf("got %#v %v\nwant error %s", q, err, tc.want)
			}
		})
	}
}

// rewrittenConnector rewrites a parsed connector.
type rewrittenConnector struct {
	filter.Logical
	op filter.LogicalOp
}

func (r rewrittenConnector) Operator() filter.LogicalOp { return r.op }

// opaque is a filter expression that is neither a predicate nor a connector.
type opaque struct{ filter.Expr }

func TestLowerRejectsInvalidTrees(t *testing.T) {
	parsed, err := filter.Parse("age = 1 AND age = 2")
	if err != nil {
		t.Fatal(err)
	}
	logical := parsed.(filter.Logical)
	for _, tc := range []struct {
		name string
		tree filter.Expr
		want string
	}{
		{"invalid connector", rewrittenConnector{logical, 9}, "invalid connector at position 9"},
		{"unknown node", opaque{parsed}, "invalid filter syntax tree"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q, err := sqlpg.Lower(withFilter(predicatePlan(t), tc.tree))
			if q != nil || err == nil || err.Error() != tc.want {
				t.Fatalf("got %#v %v\nwant error %s", q, err, tc.want)
			}
		})
	}
}

// The plan-level errors of easylist-sql-goqu's TestCompileErrors and
// TestCompileExpressionErrorPosition.
func TestLowerPlanErrors(t *testing.T) {
	withQty := tasksList
	withQty.Fields = append(append([]easylist.Field(nil), tasksList.Fields...), easylist.Field{Name: "qty", Type: easylist.Integer})
	cases := []struct {
		name string
		plan func(t *testing.T) *easylist.Plan
		want string
	}{
		{"operator", func(t *testing.T) *easylist.Plan {
			return plan(t, usersList, easylist.KindPage, easylist.Request{}, "name > 'a'")
		}, "unsupported operator \">\" for field \"name\" at position 6"},
		{"no fields", func(*testing.T) *easylist.Plan {
			return &easylist.Plan{Kind: easylist.KindPage, Table: "t", Alias: "t"}
		}, "plan for list \"\" selects no fields"},
		{"invalid kind", func(*testing.T) *easylist.Plan {
			return &easylist.Plan{Kind: 9, Table: "t", Alias: "t"}
		}, "invalid plan kind 9"},
		{"evaluated number not an integer", func(t *testing.T) *easylist.Plan {
			return plan(t, withQty, easylist.KindPage, easylist.Request{}, "qty > {10 / 4}")
		}, "invalid integer value \"2.5\" for field \"qty\" at position 7"},
		{"evaluated number on predicateList", func(t *testing.T) *easylist.Plan {
			return plan(t, predicateList, easylist.KindPage, easylist.Request{Now: nowUTC}, "age > {10 / 4}")
		}, "invalid integer value \"2.5\" for field \"age\" at position 7"},
		{"missing evaluated value", func(t *testing.T) *easylist.Plan {
			// A predicate the plan did not evaluate, such as one from another parse.
			p := plan(t, predicateList, easylist.KindPage, easylist.Request{Now: nowUTC}, "at > NOW")
			return withFilter(p, parsePredicate(t, "at > NOW"))
		}, "expression NOW for field \"at\" has no evaluated value at position 6"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q, err := sqlpg.Lower(tc.plan(t))
			if q != nil || err == nil || err.Error() != tc.want {
				t.Fatalf("got %#v %v\nwant error %s", q, err, tc.want)
			}
		})
	}
}

// Kind and field combinations the planner rejects still fail cleanly. They
// cannot reach Lower through a plan, so the test calls the internal function,
// as easylist-sql-goqu's TestExprPredicateDefensive did.
func TestExprPredicateDefensive(t *testing.T) {
	p := predicatePlan(t)
	date := easylist.Value{Kind: easylist.ValueDate, Time: tm(2026, 10, 27, 0, 0, utc)}
	stamp := easylist.Value{Kind: easylist.ValueTimestamp, Time: nowUTC}
	number := easylist.Value{Kind: easylist.ValueNumber, Number: "1"}
	cases := []struct {
		field string
		op    filter.PredicateOp
		v     easylist.Value
		want  string
	}{
		{"day", filter.OpEq, stamp, "cannot compare date field \"day\" with a timestamp value at position 42"},
		{"age", filter.OpEq, date, "cannot compare integer field \"age\" with a date value at position 42"},
		{"price", filter.OpGt, stamp, "cannot compare numeric field \"price\" with a timestamp value at position 42"},
		{"at", filter.OpEq, number, "cannot compare timestamp field \"at\" with a number value at position 42"},
		{"name", filter.OpEq, number, "cannot compare text field \"name\" with a number value at position 42"},
		{"active", filter.OpEq, number, "cannot compare boolean field \"active\" with a number value at position 42"},
		{"uid", filter.OpEq, date, "cannot compare uuid field \"uid\" with a date value at position 42"},
		{"status", filter.OpEq, stamp, "cannot compare enum field \"status\" with a timestamp value at position 42"},
		{"age", filter.OpEq, easylist.Value{}, "cannot compare integer field \"age\" with a invalid value at position 42"},
		{"age", filter.OpEq, easylist.Value{Kind: easylist.ValueNumber, Number: "x"}, "invalid integer value \"x\" for field \"age\" at position 42"},
		{"ids", filter.OpEq, number, "array field \"ids\" does not accept an expression value at position 42"},
		{"days", filter.OpContains, date, "array field \"days\" does not accept an expression value at position 42"},
		{"day", filter.OpBetween, date, "operator \"BETWEEN\" does not accept an expression value at position 42"},
		{"name", filter.OpContains, number, "operator \"CONTAINS\" does not accept an expression value at position 42"},
		{"name", filter.OpExcludes, number, "operator \"EXCLUDES\" does not accept an expression value at position 42"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			f, ok := p.Lookup(tc.field)
			if !ok {
				t.Fatalf("no field %s", tc.field)
			}
			cond, err := sqlpg.ExprPredicate(f, sqlpg.FieldExpression(p, f, true), tc.op, tc.v, 42)
			if cond != nil || err == nil || err.Error() != tc.want {
				t.Fatalf("got %#v %v\nwant error %s", cond, err, tc.want)
			}
		})
	}
}

// Args lists the filter arguments left to right with their Go types, then
// limit and offset as int64.
func TestLowerArgs(t *testing.T) {
	scoped := usersList
	scoped.Scope = "u.meta ? 'vip'"
	goods := easylist.List{
		Name: "goods", Table: "goods", Alias: "g", PrimaryKey: "id",
		Fields: []easylist.Field{
			{Name: "id", Type: easylist.Integer},
			{Name: "price", Type: easylist.Numeric},
			{Name: "born", Type: easylist.Date},
			{Name: "active", Type: easylist.Boolean},
		},
	}
	cases := []struct {
		name   string
		def    easylist.List
		kind   easylist.Kind
		req    easylist.Request
		filter string
		args   []any
	}{
		{"placeholder order", usersList, easylist.KindPage, easylist.Request{Limit: intp(10), Offset: intp(0)}, "name = 'Ann' AND age < 30", []any{"Ann", int64(30), int64(10), int64(0)}},
		{"same arguments", goods, easylist.KindPage, easylist.Request{Limit: intp(10), Offset: intp(20)}, "price > 9.50 AND born = '2000-01-02' AND active = TRUE",
			[]any{"9.50", time.Date(2000, 1, 2, 0, 0, 0, 0, time.UTC), true, int64(10), int64(20)}},
		{"IN order", usersList, easylist.KindPage, easylist.Request{Limit: intp(10), Offset: intp(0)}, "status IN ('a', NULL, 'b') AND age < 30", []any{"a", "b", int64(30), int64(10), int64(0)}},
		{"NULL forms bind nothing", usersList, easylist.KindPage, easylist.Request{Limit: intp(10), Offset: intp(0)}, "country = NULL AND name != NULL AND status IN (NULL) AND name NOT IN (NULL)", []any{int64(10), int64(0)}},
		{"grouping", usersList, easylist.KindExport, easylist.Request{}, "(name = 'a' OR age > 2) AND (status = 'c' OR country EXCLUDES 'd')", []any{"a", int64(2), "c", "%d%"}},
		{"scope binds nothing", scoped, easylist.KindCount, easylist.Request{}, "name = 'Ann'", []any{"Ann"}},
		{"array values", arraysList, easylist.KindCount, easylist.Request{}, "statuses CONTAINS 'ok,passed' OR tags EXCLUDES ('a', 'b')", []any{"ok", "passed", "a", "b"}},
		{"no filter", usersList, easylist.KindPage, easylist.Request{Limit: intp(50), Offset: intp(0)}, "", []any{int64(50), int64(0)}},
		{"whole day", tasksList, easylist.KindCount, easylist.Request{Now: nowP2}, "due = TODAY AND created_at = TODAY",
			[]any{tm(2026, 10, 27, 0, 0, utc), tm(2026, 10, 27, 0, 0, plus2), tm(2026, 10, 28, 0, 0, plus2)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q, err := sqlpg.Lower(plan(t, tc.def, tc.kind, tc.req, tc.filter))
			if err != nil {
				t.Fatal(err)
			}
			if got := q.Args(); !reflect.DeepEqual(got, tc.args) {
				t.Errorf("got %#v\nwant %#v", got, tc.args)
			}
		})
	}
}

func TestLowerQuery(t *testing.T) {
	scoped := usersList
	scoped.Scope = "u.deleted_at IS NULL"
	t.Run("page", func(t *testing.T) {
		req := easylist.Request{Fields: []string{"name", "country"}, Sort: []easylist.Sort{{Field: "last_order", Direction: easylist.Desc}}, Limit: intp(5), Offset: intp(0)}
		q, err := sqlpg.Lower(plan(t, scoped, easylist.KindPage, req, "name = 'a' OR country = NULL"))
		if err != nil {
			t.Fatal(err)
		}
		want := &sqlpg.Query{
			From: sqlpg.Expr{SQL: "users AS u LEFT JOIN countries c ON c.id = u.country_id LEFT JOIN LATERAL (SELECT created_at FROM orders o WHERE o.user_id = u.id ORDER BY created_at DESC LIMIT 1) lo ON TRUE"},
			Select: []sqlpg.Column{
				{Expr: sqlpg.Expr{SQL: `u."id"`}, Alias: "id"},
				{Expr: sqlpg.Expr{SQL: `u."name"`}, Alias: "name"},
				{Expr: sqlpg.Expr{SQL: "c.name"}, Alias: "country"},
			},
			Where: sqlpg.And{
				Left: sqlpg.Raw{SQL: "u.deleted_at IS NULL"},
				Right: sqlpg.Or{
					Left:  sqlpg.Compare{Col: sqlpg.Expr{SQL: `u."name"`}, Op: sqlpg.OpEq, Arg: sqlpg.Arg{Value: "a"}},
					Right: sqlpg.IsNull{Col: sqlpg.Expr{SQL: "(c.name)"}},
				},
			},
			Order: []sqlpg.Ordering{{Expr: sqlpg.Expr{SQL: "lo.created_at"}, Desc: true}, {Expr: sqlpg.Expr{SQL: `u."id"`}}},
			Page:  &sqlpg.Page{Limit: 5, Offset: 0},
		}
		if !reflect.DeepEqual(q, want) {
			t.Errorf("got\n%#v\nwant\n%#v", q, want)
		}
	})
	t.Run("export arrays", func(t *testing.T) {
		q, err := sqlpg.Lower(plan(t, arraysList, easylist.KindExport, easylist.Request{Fields: []string{"tags", "labels"}}, ""))
		if err != nil {
			t.Fatal(err)
		}
		want := &sqlpg.Query{
			From: sqlpg.Expr{SQL: "runs AS u LEFT JOIN LATERAL (SELECT array_agg(name ORDER BY name) AS names FROM labels WHERE run_id = u.id) l ON TRUE"},
			Select: []sqlpg.Column{
				{Expr: sqlpg.Expr{SQL: `u."id"`}, Alias: "id"},
				{Expr: sqlpg.Expr{SQL: `to_json(u."tags")::text`}, Alias: "tags"},
				{Expr: sqlpg.Expr{SQL: "to_json(l.names)::text"}, Alias: "labels"},
			},
			Order: []sqlpg.Ordering{{Expr: sqlpg.Expr{SQL: `u."id"`}}},
		}
		if !reflect.DeepEqual(q, want) {
			t.Errorf("got\n%#v\nwant\n%#v", q, want)
		}
	})
	t.Run("count", func(t *testing.T) {
		req := easylist.Request{Fields: []string{"country"}, Sort: []easylist.Sort{{Field: "last_order"}}, Limit: intp(10), Offset: intp(20)}
		q, err := sqlpg.Lower(plan(t, usersList, easylist.KindCount, req, "country = 'FR'"))
		if err != nil {
			t.Fatal(err)
		}
		want := &sqlpg.Query{
			Count: true,
			From:  sqlpg.Expr{SQL: "users AS u LEFT JOIN countries c ON c.id = u.country_id"},
			Where: sqlpg.Compare{Col: sqlpg.Expr{SQL: "(c.name)"}, Op: sqlpg.OpEq, Arg: sqlpg.Arg{Value: "FR"}},
		}
		if !reflect.DeepEqual(q, want) {
			t.Errorf("got\n%#v\nwant\n%#v", q, want)
		}
	})
}

// Each filter lowers to the node a driver maps, with its arguments typed as
// the contract requires.
func TestLowerConds(t *testing.T) {
	col := func(name string) sqlpg.Expr { return sqlpg.Expr{SQL: `t."` + name + `"`} }
	s, e := tm(2026, 10, 26, 22, 0, utc).In(plus2), tm(2026, 10, 27, 22, 0, utc).In(plus2)
	cases := []struct {
		filter string
		want   sqlpg.Cond
	}{
		{"price = 1.10", sqlpg.Compare{Col: col("price"), Op: sqlpg.OpEq, Arg: sqlpg.Arg{Value: "1.10", Cast: sqlpg.CastNumeric}}},
		{"active != TRUE", sqlpg.Compare{Col: col("active"), Op: sqlpg.OpDistinct, Arg: sqlpg.Arg{Value: true}}},
		{"uid = '0B6C4C1E-3D8E-4F43-9D5A-2A8A0D1D2C3B'", sqlpg.Compare{Col: col("uid"), Op: sqlpg.OpEq, Arg: sqlpg.Arg{Value: "0b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b"}}},
		{"full = NULL", sqlpg.IsNull{Col: sqlpg.Expr{SQL: "(t.first || ' ' || t.last)"}}},
		{"status IN (NULL)", sqlpg.IsNull{Col: col("status")}},
		{"status NOT IN (NULL, NULL)", sqlpg.IsNull{Col: col("status"), Not: true}},
		{"status IN ('active')", sqlpg.In{Col: col("status"), Args: []sqlpg.Arg{{Value: "active"}}, Null: sqlpg.NullKeep}},
		{"status IN ('active', NULL)", sqlpg.In{Col: col("status"), Args: []sqlpg.Arg{{Value: "active"}}, Null: sqlpg.NullMatch}},
		{"status NOT IN ('active')", sqlpg.In{Col: col("status"), Args: []sqlpg.Arg{{Value: "active"}}, Not: true, Null: sqlpg.NullMatch}},
		{"status NOT IN (NULL, 'active')", sqlpg.In{Col: col("status"), Args: []sqlpg.Arg{{Value: "active"}}, Not: true, Null: sqlpg.NullNoMatch}},
		{"age BETWEEN ' 40, 30 '", sqlpg.Between{Col: col("age"), Low: sqlpg.Arg{Value: int64(40)}, High: sqlpg.Arg{Value: int64(30)}}},
		{"name EXCLUDES '50%_off'", sqlpg.ILike{Col: col("name"), Pattern: sqlpg.Arg{Value: `%50\%\_off%`}, Not: true}},
		{"prices CONTAINS '1, 2.5'", sqlpg.ArrayHas{Col: col("prices"), Values: []sqlpg.Arg{{Value: "1", Cast: sqlpg.CastNumeric}, {Value: "2.5", Cast: sqlpg.CastNumeric}}}},
		{"labels EXCLUDES 'x'", sqlpg.ArrayHas{Col: sqlpg.Expr{SQL: "(l.names)"}, Values: []sqlpg.Arg{{Value: "x"}}, None: true}},
		{"at = TODAY", sqlpg.DayRange{Col: col("at"), Start: sqlpg.Arg{Value: s}, End: sqlpg.Arg{Value: e}}},
		{"at != TODAY", sqlpg.DayRange{Col: col("at"), Start: sqlpg.Arg{Value: s}, End: sqlpg.Arg{Value: e}, Not: true}},
		{"at > TODAY", sqlpg.Compare{Col: col("at"), Op: sqlpg.OpGte, Arg: sqlpg.Arg{Value: e}}},
		{"at <= TODAY", sqlpg.Compare{Col: col("at"), Op: sqlpg.OpLt, Arg: sqlpg.Arg{Value: e}}},
	}
	for _, tc := range cases {
		t.Run(tc.filter, func(t *testing.T) {
			got := lowerCount(t, tc.filter, nowP2).Where
			if fmt.Sprintf("%#v", got) != fmt.Sprintf("%#v", tc.want) {
				t.Errorf("got\n%#v\nwant\n%#v", got, tc.want)
			}
		})
	}
}

func TestLowerIsDeterministic(t *testing.T) {
	p := plan(t, usersList, easylist.KindPage, easylist.Request{Fields: []string{"name", "country", "last_order"}}, "name = 'a' OR age BETWEEN '1,2'")
	q1, err1 := sqlpg.Lower(p)
	q2, err2 := sqlpg.Lower(p)
	if err1 != nil || err2 != nil || !reflect.DeepEqual(q1, q2) {
		t.Fatalf("results differ: %#v %v / %#v %v", q1, err1, q2, err2)
	}
}
