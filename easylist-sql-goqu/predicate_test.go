package sqlgoqu

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	filter "github.com/mgpnd/easylist-go/easylist-filter"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/doug-martin/goqu/v9/exp"
)

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

func predicatePlan(t *testing.T) *easylist.Plan {
	t.Helper()
	reg, err := easylist.NewRegistry(predicateList)
	if err != nil {
		t.Fatal(err)
	}
	l, _ := reg.List("t")
	p, err := l.Plan(easylist.KindPage, easylist.Request{})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func renderPredicate(t *testing.T, filter string) (string, []any) {
	t.Helper()
	expr, err := buildPredicate(predicatePlan(t), parsePredicate(t, filter))
	if err != nil {
		t.Fatal(err)
	}
	sql, args, err := goqu.Dialect("postgres").From(goqu.T("t")).Where(expr).Prepared(true).ToSQL()
	if err != nil {
		t.Fatal(err)
	}
	return sql, args
}

func TestBuildPredicateTypesAndOperators(t *testing.T) {
	day := time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	atEnd := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	offset := time.Date(2026, 10, 25, 10, 0, 0, 500_000_000, time.FixedZone("", 2*3600))
	cases := []struct {
		filter, clause string
		args           []any
	}{
		{"age < 30", `t."age" < $1`, []any{int64(30)}},
		{"age = -9223372036854775808", `t."age" = $1`, []any{int64(-9223372036854775808)}},
		{"age > 1", `t."age" > $1`, []any{int64(1)}},
		{"age >= 2", `t."age" >= $1`, []any{int64(2)}},
		{"age <= 3", `t."age" <= $1`, []any{int64(3)}},
		{"score = -1.25", `t."score" = $1`, []any{float64(-1.25)}},
		{"score > 2.5", `t."score" > $1`, []any{float64(2.5)}},
		{"price = 12345678901234567890.123456789", `t."price" = $1::numeric`, []any{"12345678901234567890.123456789"}},
		{"price <= -0.01", `t."price" <= $1::numeric`, []any{"-0.01"}},
		{"day = '2024-02-29'", `t."day" = $1`, []any{day}},
		{"day >= '2024-02-29'", `t."day" >= $1`, []any{day}},
		{"at > '2026-01-01T00:00:00Z'", `t."at" > $1`, []any{at}},
		{"at <= '2026-10-25T10:00:00.5+02:00'", `t."at" <= $1`, []any{offset}},
		{"name = 'O''Reilly'", `t."name" = $1`, []any{"O'Reilly"}},
		{"active = TRUE", `t."active" = $1`, []any{true}},
		{"active = false", `t."active" = $1`, []any{false}},
		{"uid = '0B6C4C1E-3D8E-4F43-9D5A-2A8A0D1D2C3B'", `t."uid" = $1`, []any{"0b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b"}},
		{"uid != '0b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b'", `t."uid" IS DISTINCT FROM $1`, []any{"0b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b"}},
		{"status = 'blocked'", `t."status" = $1`, []any{"blocked"}},
		{"status != 'active'", `t."status" IS DISTINCT FROM $1`, []any{"active"}},
		{"age != -9223372036854775808", `t."age" IS DISTINCT FROM $1`, []any{int64(-9223372036854775808)}},
		{"score != -1.25", `t."score" IS DISTINCT FROM $1`, []any{float64(-1.25)}},
		{"price != 12345678901234567890.123456789", `t."price" IS DISTINCT FROM $1::numeric`, []any{"12345678901234567890.123456789"}},
		{"day != '2024-02-29'", `t."day" IS DISTINCT FROM $1`, []any{day}},
		{"at != '2026-01-01T00:00:00Z'", `t."at" IS DISTINCT FROM $1`, []any{at}},
		{"name != 'O''Reilly'", `t."name" IS DISTINCT FROM $1`, []any{"O'Reilly"}},
		{"active != TRUE", `t."active" IS DISTINCT FROM $1`, []any{true}},
		{"active!=false", `t."active" IS DISTINCT FROM $1`, []any{false}},
		{"name CONTAINS 'needle'", `t."name" ILIKE $1 ESCAPE E'\\'`, []any{"%needle%"}},
		{"name EXCLUDES 'needle'", `COALESCE(t."name" NOT ILIKE $1 ESCAPE E'\\', TRUE)`, []any{"%needle%"}},
		{"full EXCLUDES 'Lee'", `COALESCE((t.first || ' ' || t.last) NOT ILIKE $1 ESCAPE E'\\', TRUE)`, []any{"%Lee%"}},
		{"full != 'Ann Lee'", `(t.first || ' ' || t.last) IS DISTINCT FROM $1`, []any{"Ann Lee"}},
		{"full = 'Ann Lee'", `(t.first || ' ' || t.last) = $1`, []any{"Ann Lee"}},
		{"age BETWEEN ' 40, 30 '", `t."age" BETWEEN $1 AND $2`, []any{int64(40), int64(30)}},
		{"score BETWEEN '-1.5,2.5'", `t."score" BETWEEN $1 AND $2`, []any{float64(-1.5), float64(2.5)}},
		{"price BETWEEN '1.01,2.02'", `t."price" BETWEEN $1::numeric AND $2::numeric`, []any{"1.01", "2.02"}},
		{"day BETWEEN '2024-02-29,2025-01-01'", `t."day" BETWEEN $1 AND $2`, []any{day, end}},
		{"at BETWEEN '2026-01-01T00:00:00Z, 2026-02-01T00:00:00Z'", `t."at" BETWEEN $1 AND $2`, []any{at, atEnd}},
		{"age BETWEEN (40, 30)", `t."age" BETWEEN $1 AND $2`, []any{int64(40), int64(30)}},
		{"score BETWEEN (-1.5, 2.5)", `t."score" BETWEEN $1 AND $2`, []any{float64(-1.5), float64(2.5)}},
		{"price BETWEEN (1.01, 2.02)", `t."price" BETWEEN $1::numeric AND $2::numeric`, []any{"1.01", "2.02"}},
		{"day BETWEEN ('2024-02-29', '2025-01-01')", `t."day" BETWEEN $1 AND $2`, []any{day, end}},
		{"at BETWEEN ('2026-01-01T00:00:00Z', '2026-02-01T00:00:00Z')", `t."at" BETWEEN $1 AND $2`, []any{at, atEnd}},
		{"active IN (TRUE)", `t."active" IN ($1)`, []any{true}},
		{"uid NOT IN ('7f1c0d2e-0000-4000-8000-000000000000')", `COALESCE(t."uid" NOT IN ($1), TRUE)`, []any{"7f1c0d2e-0000-4000-8000-000000000000"}},
		{"status IN ('active', NULL)", `COALESCE(t."status" IN ($1), TRUE)`, []any{"active"}},
		{"status NOT IN ('blocked', NULL, 'blocked')", `COALESCE(t."status" NOT IN ($1, $2), FALSE)`, []any{"blocked", "blocked"}},
	}
	for _, tc := range cases {
		t.Run(tc.filter, func(t *testing.T) {
			sql, args := renderPredicate(t, tc.filter)
			if !strings.Contains(sql, tc.clause) || !reflect.DeepEqual(args, tc.args) {
				t.Fatalf("got %q %#v; want clause %q args %#v", sql, args, tc.clause, tc.args)
			}
		})
	}
}

// NULL comparisons compile for every type, although no field here is declared
// nullable, and bind no argument. The enum's declared values are not consulted.
func TestBuildPredicateNull(t *testing.T) {
	for _, field := range []string{"uid", "name", "age", "score", "price", "day", "at", "active", "status"} {
		for _, tc := range []struct{ op, clause string }{{"=", "IS NULL"}, {"!=", "IS NOT NULL"}} {
			input := field + " " + tc.op + " NULL"
			t.Run(input, func(t *testing.T) {
				sql, args := renderPredicate(t, input)
				if want := `WHERE t."` + field + `" ` + tc.clause; !strings.HasSuffix(sql, want) || len(args) != 0 {
					t.Fatalf("got %q %#v; want suffix %q and no args", sql, args, want)
				}
			})
		}
	}
	sql, args := renderPredicate(t, "full = null")
	if !strings.HasSuffix(sql, `WHERE (t.first || ' ' || t.last) IS NULL`) || len(args) != 0 {
		t.Fatalf("author SQL: %q %#v", sql, args)
	}
}

// rewrittenOperator rewrites a parsed predicate's operator, to build trees the
// parser itself never produces, such as a NULL value after >.
type rewrittenOperator struct {
	filter.Predicate
	op filter.PredicateOp
}

func (r rewrittenOperator) Operator() filter.PredicateOp { return r.op }

func TestBuildPredicateRejectsNullWithOtherOperators(t *testing.T) {
	for _, tc := range []struct {
		field string
		op    filter.PredicateOp
		text  string
	}{
		{"age", filter.OpGt, ">"}, {"age", filter.OpLte, "<="}, {"age", filter.OpBetween, "BETWEEN"},
		{"name", filter.OpContains, "CONTAINS"}, {"name", filter.OpExcludes, "EXCLUDES"},
	} {
		t.Run(tc.field+" "+tc.text, func(t *testing.T) {
			node := rewrittenOperator{parsePredicate(t, tc.field+" = NULL"), tc.op}
			expr, err := buildPredicate(predicatePlan(t), node)
			pos := fmt.Sprintf("position %d", len(tc.field)+2)
			if expr != nil || err == nil {
				t.Fatalf("want an error, got %#v", expr)
			}
			for _, want := range []string{"unsupported operator", `"` + tc.text + `"`, `"` + tc.field + `"`, pos} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %s", err, want)
				}
			}
		})
	}
}

func TestBuildPredicateEscaping(t *testing.T) {
	sql, args := renderPredicate(t, `name CONTAINS 'a%_\b'`)
	if !strings.Contains(sql, `ILIKE $1 ESCAPE E'\\'`) || !reflect.DeepEqual(args, []any{`%a\%\_\\b%`}) {
		t.Fatalf("literal wildcard pattern: %q %#v", sql, args)
	}
	sql, args = renderPredicate(t, `name CONTAINS '50%_off'`)
	if !reflect.DeepEqual(args, []any{`%50\%\_off%`}) {
		t.Fatalf("CONTAINS scenario: %q %#v", sql, args)
	}
	sql, args = renderPredicate(t, `name EXCLUDES '50%_off'`)
	if !strings.Contains(sql, `COALESCE(t."name" NOT ILIKE $1 ESCAPE E'\\', TRUE)`) || !reflect.DeepEqual(args, []any{`%50\%\_off%`}) {
		t.Fatalf("EXCLUDES scenario: %q %#v", sql, args)
	}
	sql, args = renderPredicate(t, `name = 'x''; DROP TABLE users; --'`)
	if strings.Contains(sql, "DROP") || !reflect.DeepEqual(args, []any{"x'; DROP TABLE users; --"}) {
		t.Fatalf("literal spliced into SQL: %q %#v", sql, args)
	}
}

func TestBuildPredicateRejectsBadValues(t *testing.T) {
	cases := []struct{ filter, want string }{
		{"age = 9223372036854775808", "position 7"},
		{"age = -9223372036854775809", "position 7"},
		{"age = 1.5", "position 7"},
		{"age = '30'", "position 7"},
		{"day = '2025-02-29'", "position 7"},
		{"day = '2024-2-29'", "position 7"},
		{"day = '2024-02-29T00:00:00Z'", "position 7"},
		{"day = 20240229", "position 7"},
		{"at > '2026-10-25T10:00:00'", "offset"},
		{"at > '2026-10-25T10:00:00'", "position 6"},
		{"at = '2026-10-25'", "position 6"},
		{"at = 1", "position 6"},
		{"uid = 'not-a-uuid'", "position 7"},
		{"uid = '0b6c4c1e3d8e4f439d5a2a8a0d1d2c3b'", "position 7"},
		{"uid = 1", "position 7"},
		{"status = 'archived'", `"archived"`},
		{"status = 'archived'", "position 10"},
		{"status = 'Active'", "position 10"},
		{"status = 1", "position 10"},
		{"age BETWEEN '1,2,3'", "position 13"},
		{"age BETWEEN '1,'", "position 13"},
		{"age BETWEEN ',2'", "position 13"},
		{"age BETWEEN '1'", "position 13"},
		{"age BETWEEN '1,2.5'", "position 13"},
		{"at BETWEEN '2026-01-01T00:00:00Z,2026-02-01'", "position 12"},
		{"score BETWEEN 'NaN,2'", "position 15"},
		{"price BETWEEN '1,2e3'", "position 15"},
		{"score = " + strings.Repeat("9", 400), "position 9"},
		{"name = 1", "position 8"},
		{"active = 'true'", "position 10"},
		{"active = 1", "position 10"},
		{"age != 1.5", "position 8"},
		{"name != 1", "position 9"},
		{"name EXCLUDES 1", "position 15"},
		{"active != 'true'", "position 11"},
		{"day != '2025-02-29'", "position 8"},
		{"absent = 1", "position 1"},
		{`"Name" = 'a'`, "position 1"},
	}
	for _, tc := range cases {
		t.Run(tc.filter, func(t *testing.T) {
			expr, err := buildPredicate(predicatePlan(t), parsePredicate(t, tc.filter))
			if expr != nil || err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want nil expression and error containing %q; got %#v %v", tc.want, expr, err)
			}
		})
	}
}

func TestBuildPredicateRejectsOperators(t *testing.T) {
	cases := []struct{ filter, op, field, position string }{
		{"active > TRUE", ">", "active", "position 8"},
		{"active < TRUE", "<", "active", "position 8"},
		{"active BETWEEN 'true,false'", "BETWEEN", "active", "position 8"},
		{"name > 'a'", ">", "name", "position 6"},
		{"name BETWEEN 'a,b'", "BETWEEN", "name", "position 6"},
		{"age CONTAINS 1", "CONTAINS", "age", "position 5"},
		{"age EXCLUDES 1", "EXCLUDES", "age", "position 5"},
		{"price EXCLUDES 1", "EXCLUDES", "price", "position 7"},
		{"active EXCLUDES TRUE", "EXCLUDES", "active", "position 8"},
		{"uid EXCLUDES 'a'", "EXCLUDES", "uid", "position 5"},
		{"status EXCLUDES 'act'", "EXCLUDES", "status", "position 8"},
		{"day EXCLUDES '2024-01-01'", "EXCLUDES", "day", "position 5"},
		{"uid > 'a'", ">", "uid", "position 5"},
		{"uid CONTAINS 'a'", "CONTAINS", "uid", "position 5"},
		{"status >= 'active'", ">=", "status", "position 8"},
		{"status CONTAINS 'act'", "CONTAINS", "status", "position 8"},
		{"day CONTAINS '2024-01-01'", "CONTAINS", "day", "position 5"},
		{"at CONTAINS '2024'", "CONTAINS", "at", "position 4"},
		{"age CONTAINS (1, 2)", "CONTAINS", "age", "position 5"},
		{"status EXCLUDES ('active')", "EXCLUDES", "status", "position 8"},
		{"active BETWEEN (TRUE, FALSE)", "BETWEEN", "active", "position 8"},
		{"name BETWEEN ('a', 'b')", "BETWEEN", "name", "position 6"},
	}
	for _, tc := range cases {
		t.Run(tc.filter, func(t *testing.T) {
			_, err := buildPredicate(predicatePlan(t), parsePredicate(t, tc.filter))
			if err == nil {
				t.Fatal("expected an error")
			}
			for _, want := range []string{`"` + tc.op + `"`, `"` + tc.field + `"`, tc.position} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %s", err, want)
				}
			}
		})
	}
}

// Ensure buildPredicate consumes the public AST predicate contract.
var _ func(*easylist.Plan, filter.Predicate) (exp.Expression, error) = buildPredicate

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

func TestBuildPredicateArrays(t *testing.T) {
	day := time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		filter, clause string
		args           []any
	}{
		{"tags CONTAINS 'go'", `WHERE ($1 = ANY(t."tags"))`, []any{"go"}},
		{"tags CONTAINS 'a,b'", `WHERE ($1 = ANY(t."tags") AND $2 = ANY(t."tags"))`, []any{"a", "b"}},
		{"tags CONTAINS ' Go , rust '", `WHERE ($1 = ANY(t."tags") AND $2 = ANY(t."tags"))`, []any{"Go", "rust"}},
		{"tags EXCLUDES 'go'", `WHERE COALESCE(NOT ($1 = ANY(t."tags")), TRUE)`, []any{"go"}},
		{"ids CONTAINS '7, 8'", `WHERE ($1 = ANY(t."ids") AND $2 = ANY(t."ids"))`, []any{int64(7), int64(8)}},
		{"ids CONTAINS 7", `WHERE ($1 = ANY(t."ids"))`, []any{int64(7)}},
		{"ids EXCLUDES -3", `WHERE COALESCE(NOT ($1 = ANY(t."ids")), TRUE)`, []any{int64(-3)}},
		{"scores CONTAINS '1.5,-2'", `WHERE ($1 = ANY(t."scores") AND $2 = ANY(t."scores"))`, []any{1.5, float64(-2)}},
		{"scores CONTAINS 2.5", `WHERE ($1 = ANY(t."scores"))`, []any{2.5}},
		{"prices CONTAINS '10.50, 12345678901234567890.1'", `WHERE ($1::numeric = ANY(t."prices") AND $2::numeric = ANY(t."prices"))`, []any{"10.50", "12345678901234567890.1"}},
		{"prices EXCLUDES 1", `WHERE COALESCE(NOT ($1::numeric = ANY(t."prices")), TRUE)`, []any{"1"}},
		{"flags CONTAINS 'TRUE, false'", `WHERE ($1 = ANY(t."flags") AND $2 = ANY(t."flags"))`, []any{true, false}},
		{"flags EXCLUDES true", `WHERE COALESCE(NOT ($1 = ANY(t."flags")), TRUE)`, []any{true}},
		{"days CONTAINS '2024-02-29'", `WHERE ($1 = ANY(t."days"))`, []any{day}},
		{"seen EXCLUDES '2026-01-01T00:00:00Z'", `WHERE COALESCE(NOT ($1 = ANY(t."seen")), TRUE)`, []any{at}},
		{"groups CONTAINS '0B6C4C1E-3D8E-4F43-9D5A-2A8A0D1D2C3B'", `WHERE ($1 = ANY(t."groups"))`, []any{"0b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b"}},
		{"statuses EXCLUDES 'skipped,failed'", `WHERE COALESCE(NOT ($1 = ANY(t."statuses") OR $2 = ANY(t."statuses")), TRUE)`, []any{"skipped", "failed"}},
		{"statuses CONTAINS 'ok'", `WHERE ($1 = ANY(t."statuses"))`, []any{"ok"}},
		{"labels CONTAINS 'x'", `WHERE ($1 = ANY((l.names)))`, []any{"x"}},
		{"tags = NULL", `WHERE t."tags" IS NULL`, []any{}},
		{"statuses != NULL", `WHERE t."statuses" IS NOT NULL`, []any{}},
		{"labels = NULL", `WHERE (l.names) IS NULL`, []any{}},
		// List values give one value per item, never split or trimmed.
		{"tags CONTAINS ('a,b', ' c')", `WHERE ($1 = ANY(t."tags") AND $2 = ANY(t."tags"))`, []any{"a,b", " c"}},
		{"tags EXCLUDES ('go')", `WHERE COALESCE(NOT ($1 = ANY(t."tags")), TRUE)`, []any{"go"}},
		{"ids CONTAINS (7, -8)", `WHERE ($1 = ANY(t."ids") AND $2 = ANY(t."ids"))`, []any{int64(7), int64(-8)}},
		{"scores EXCLUDES (1.5)", `WHERE COALESCE(NOT ($1 = ANY(t."scores")), TRUE)`, []any{1.5}},
		{"prices CONTAINS (10.50, 1)", `WHERE ($1::numeric = ANY(t."prices") AND $2::numeric = ANY(t."prices"))`, []any{"10.50", "1"}},
		{"flags CONTAINS (TRUE, false)", `WHERE ($1 = ANY(t."flags") AND $2 = ANY(t."flags"))`, []any{true, false}},
		{"days CONTAINS ('2024-02-29')", `WHERE ($1 = ANY(t."days"))`, []any{day}},
		{"seen EXCLUDES ('2026-01-01T00:00:00Z')", `WHERE COALESCE(NOT ($1 = ANY(t."seen")), TRUE)`, []any{at}},
		{"groups CONTAINS ('0B6C4C1E-3D8E-4F43-9D5A-2A8A0D1D2C3B')", `WHERE ($1 = ANY(t."groups"))`, []any{"0b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b"}},
		{"statuses EXCLUDES ('skipped', 'failed')", `WHERE COALESCE(NOT ($1 = ANY(t."statuses") OR $2 = ANY(t."statuses")), TRUE)`, []any{"skipped", "failed"}},
		{"labels CONTAINS ('x')", `WHERE ($1 = ANY((l.names)))`, []any{"x"}},
		// Scalar text fields keep the substring search and never split lists.
		{"name CONTAINS 'a,b'", `WHERE t."name" ILIKE $1 ESCAPE E'\\'`, []any{"%a,b%"}},
		{"name EXCLUDES 'a,b'", `WHERE COALESCE(t."name" NOT ILIKE $1 ESCAPE E'\\', TRUE)`, []any{"%a,b%"}},
	}
	for _, tc := range cases {
		t.Run(tc.filter, func(t *testing.T) {
			sql, args := renderPredicate(t, tc.filter)
			if !strings.HasSuffix(sql, tc.clause) || len(args) != len(tc.args) || (len(args) > 0 && !reflect.DeepEqual(args, tc.args)) {
				t.Fatalf("got %q %#v; want suffix %q args %#v", sql, args, tc.clause, tc.args)
			}
			// Membership compares with the column's elements directly, so no
			// argument or column is cast to a named array type.
			if strings.Contains(sql, "[]") || strings.Contains(sql, "ARRAY") {
				t.Errorf("array predicate casts to an array type: %q", sql)
			}
		})
	}
}

func TestBuildPredicateRejectsArrayOperators(t *testing.T) {
	cases := []struct{ filter, op, field, position string }{
		{"tags = 'go'", "=", "tags", "position 6"},
		{"tags != 'go'", "!=", "tags", "position 6"},
		{"ids > 3", ">", "ids", "position 5"},
		{"ids <= 3", "<=", "ids", "position 5"},
		{"ids BETWEEN '1,2'", "BETWEEN", "ids", "position 5"},
		{"days >= '2024-01-01'", ">=", "days", "position 6"},
		{"statuses = 'ok'", "=", "statuses", "position 10"},
		{"flags = TRUE", "=", "flags", "position 7"},
		{"labels != 'x'", "!=", "labels", "position 8"},
		{"tags IN ('go')", "IN", "tags", "position 6"},
		{"ids NOT IN (1, NULL)", "NOT IN", "ids", "position 5"},
		{"statuses IN (NULL)", "IN", "statuses", "position 10"},
	}
	for _, tc := range cases {
		t.Run(tc.filter, func(t *testing.T) {
			_, err := buildPredicate(predicatePlan(t), parsePredicate(t, tc.filter))
			if err == nil {
				t.Fatal("expected an error")
			}
			for _, want := range []string{"unsupported operator", `"` + tc.op + `"`, `"` + tc.field + `"`, tc.position} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %s", err, want)
				}
			}
		})
	}
}

func TestBuildPredicateRejectsArrayValues(t *testing.T) {
	cases := []struct {
		filter string
		want   []string
	}{
		{"tags CONTAINS 'a,,b'", []string{"empty item", `"tags"`, "position 15"}},
		{"tags CONTAINS ''", []string{"empty item", "position 15"}},
		{"tags EXCLUDES 'a, '", []string{"empty item", "position 15"}},
		{"ids CONTAINS '7,x'", []string{`"x"`, `"ids"`, "position 14"}},
		{"ids CONTAINS '7,8.5'", []string{`"8.5"`, "position 14"}},
		{"ids CONTAINS 1.5", []string{`"1.5"`, "position 14"}},
		{"statuses EXCLUDES 'skipped,archived'", []string{`"archived"`, "not a declared value", "position 19"}},
		{"statuses CONTAINS 'OK'", []string{`"OK"`, "position 19"}},
		{"tags CONTAINS 1", []string{`"tags"`, "position 15"}},
		{"flags CONTAINS 'yes'", []string{`"yes"`, "position 16"}},
		{"prices CONTAINS '1,2e3'", []string{`"2e3"`, "position 17"}},
		{"scores CONTAINS 'NaN'", []string{`"NaN"`, "position 17"}},
		{"days CONTAINS '2025-02-29'", []string{`"2025-02-29"`, "position 15"}},
		{"seen CONTAINS '2026-01-01T00:00:00'", []string{"offset", "position 15"}},
		{"groups CONTAINS 'a'", []string{`"a"`, "position 17"}},
	}
	for _, tc := range cases {
		t.Run(tc.filter, func(t *testing.T) {
			expr, err := buildPredicate(predicatePlan(t), parsePredicate(t, tc.filter))
			if expr != nil || err == nil {
				t.Fatalf("want an error, got %#v", expr)
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %s", err, want)
				}
			}
		})
	}
}

// IN and NOT IN compile to the four forms of design.md on every scalar type.
// Only non-NULL items are bound, in list order, and the field's SQL is emitted
// once.
func TestBuildPredicateIn(t *testing.T) {
	day := time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	types := []struct {
		field, column, items, list string
		args                       []any
	}{
		{"name", `t."name"`, "'a', 'O''Reilly'", "$1, $2", []any{"a", "O'Reilly"}},
		{"full", `(t.first || ' ' || t.last)`, "'Ann Lee'", "$1", []any{"Ann Lee"}},
		{"age", `t."age"`, "30, -1", "$1, $2", []any{int64(30), int64(-1)}},
		{"score", `t."score"`, "1.5, 2", "$1, $2", []any{1.5, float64(2)}},
		{"price", `t."price"`, "1.01, 12345678901234567890.1", "$1::numeric, $2::numeric", []any{"1.01", "12345678901234567890.1"}},
		{"active", `t."active"`, "TRUE, false", "$1, $2", []any{true, false}},
		{"day", `t."day"`, "'2024-02-29', '2025-01-01'", "$1, $2", []any{day, end}},
		{"at", `t."at"`, "'2026-01-01T00:00:00Z'", "$1", []any{at}},
		{"uid", `t."uid"`, "'0B6C4C1E-3D8E-4F43-9D5A-2A8A0D1D2C3B'", "$1", []any{"0b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b"}},
		{"status", `t."status"`, "'active', 'blocked'", "$1, $2", []any{"active", "blocked"}},
	}
	forms := []struct{ op, null, clause string }{
		{"IN", "", "%s IN (%s)"},
		{"IN", "NULL, ", "COALESCE(%s IN (%s), TRUE)"},
		{"NOT IN", "", "COALESCE(%s NOT IN (%s), TRUE)"},
		{"NOT IN", "null, ", "COALESCE(%s NOT IN (%s), FALSE)"},
	}
	for _, typ := range types {
		for _, form := range forms {
			input := typ.field + " " + form.op + " (" + form.null + typ.items + ")"
			t.Run(input, func(t *testing.T) {
				sql, args := renderPredicate(t, input)
				want := "WHERE " + fmt.Sprintf(form.clause, typ.column, typ.list)
				if !strings.HasSuffix(sql, want) || !reflect.DeepEqual(args, typ.args) || strings.Count(sql, typ.column) != 1 {
					t.Fatalf("got %q %#v; want suffix %q args %#v and the field once", sql, args, want, typ.args)
				}
			})
		}
		// A list of only NULL items binds nothing, and the enum's declared
		// values are not consulted.
		for _, tc := range []struct{ input, clause string }{
			{typ.field + " IN (NULL)", "IS NULL"},
			{typ.field + " NOT IN (NULL, null)", "IS NOT NULL"},
		} {
			t.Run(tc.input, func(t *testing.T) {
				sql, args := renderPredicate(t, tc.input)
				if want := "WHERE " + typ.column + " " + tc.clause; !strings.HasSuffix(sql, want) || len(args) != 0 {
					t.Fatalf("got %q %#v; want suffix %q and no args", sql, args, want)
				}
			})
		}
	}
}

// A list value compiles like the quoted form it can replace.
func TestBuildPredicateListFormsMatchStringForms(t *testing.T) {
	for _, tc := range []struct{ list, quoted string }{
		{"age BETWEEN (30, 40)", "age BETWEEN '30,40'"},
		{"price BETWEEN (1.01, 2.02)", "price BETWEEN ' 1.01 , 2.02'"},
		{"day BETWEEN ('2024-02-29', '2025-01-01')", "day BETWEEN '2024-02-29, 2025-01-01'"},
		{"statuses CONTAINS ('ok', 'skipped')", "statuses CONTAINS 'ok,skipped'"},
		{"ids EXCLUDES (7, 8)", "ids EXCLUDES '7, 8'"},
	} {
		t.Run(tc.list, func(t *testing.T) {
			listSQL, listArgs := renderPredicate(t, tc.list)
			quotedSQL, quotedArgs := renderPredicate(t, tc.quoted)
			if listSQL != quotedSQL || !reflect.DeepEqual(listArgs, quotedArgs) {
				t.Fatalf("got %q %#v; want %q %#v", listSQL, listArgs, quotedSQL, quotedArgs)
			}
		})
	}
}

// List errors report the offending item's position, or the list's position
// when the list as a whole is rejected.
func TestBuildPredicateRejectsListValues(t *testing.T) {
	cases := []struct {
		filter string
		want   []string
	}{
		{"status IN ('active', 'archived')", []string{`"archived"`, "not a declared value", `"status"`, "position 22"}},
		{"status NOT IN ('Active')", []string{`"Active"`, "position 16"}},
		{"age IN (1, '30')", []string{`"30"`, `"age"`, "position 12"}},
		{"age NOT IN (1.5)", []string{`"1.5"`, "position 13"}},
		{"active IN ('true')", []string{`"true"`, "position 12"}},
		{"name IN (1)", []string{`"1"`, "position 10"}},
		{"uid IN ('not-a-uuid')", []string{`"not-a-uuid"`, "position 9"}},
		{"day IN ('2024-02-29', ' 2025-01-01')", []string{`" 2025-01-01"`, "position 23"}},
		{"at IN ('2026-01-01T00:00:00')", []string{"offset", "position 8"}},
		{"age BETWEEN ('30', '40')", []string{`"30"`, `"age"`, "position 14"}},
		{"age BETWEEN (30, 40.5)", []string{`"40.5"`, "position 18"}},
		{"day BETWEEN (' 2024-02-29', '2025-01-01')", []string{`" 2024-02-29"`, "position 14"}},
		{"at BETWEEN ('2026-01-01T00:00:00Z', '2026-02-01')", []string{`"2026-02-01"`, "position 37"}},
		{"ids CONTAINS (7, 'x')", []string{`"x"`, `"ids"`, "position 18"}},
		{"ids CONTAINS (7, 8.5)", []string{`"8.5"`, "position 18"}},
		{"statuses EXCLUDES ('skipped', 'archived')", []string{`"archived"`, "not a declared value", "position 31"}},
		{"tags CONTAINS (1)", []string{`"tags"`, "position 16"}},
		{"flags CONTAINS ('true')", []string{`"true"`, "position 17"}},
		{"name CONTAINS ('a', 'b')", []string{"value list", `"name"`, "position 15"}},
		{"name EXCLUDES ('a')", []string{"value list", `"EXCLUDES"`, `"name"`, "position 15"}},
		{"full CONTAINS ('Lee')", []string{"value list", `"full"`, "position 15"}},
	}
	for _, tc := range cases {
		t.Run(tc.filter, func(t *testing.T) {
			expr, err := buildPredicate(predicatePlan(t), parsePredicate(t, tc.filter))
			if expr != nil || err == nil {
				t.Fatalf("want an error, got %#v", expr)
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %s", err, want)
				}
			}
		})
	}
}

// Trees the parser never produces fail with an error rather than a panic or
// invalid SQL.
func TestBuildPredicateRejectsRewrittenLists(t *testing.T) {
	cases := []struct {
		filter string
		op     filter.PredicateOp
		want   []string
	}{
		{"age = 1", filter.OpIn, []string{`"IN"`, "value list", `"age"`, "position 7"}},
		{"name = 'a'", filter.OpNotIn, []string{`"NOT IN"`, "value list", `"name"`, "position 8"}},
		{"age IN (1, 2)", filter.OpEq, []string{`"(1, 2)"`, "position 8"}},
		{"age IN (1, 2)", filter.OpGt, []string{`"(1, 2)"`, "position 8"}},
		{"age IN (1)", filter.OpBetween, []string{"two bounds", "position 8"}},
		{"age IN (1, 2, 3)", filter.OpBetween, []string{"two bounds", "position 8"}},
		{"age IN (1, NULL)", filter.OpBetween, []string{"NULL item", `"age"`, "position 12"}},
		{"tags IN ('a', NULL)", filter.OpContains, []string{"NULL item", `"tags"`, "position 15"}},
	}
	for _, tc := range cases {
		t.Run(tc.filter+" as "+operatorText[tc.op], func(t *testing.T) {
			node := rewrittenOperator{parsePredicate(t, tc.filter), tc.op}
			expr, err := buildPredicate(predicatePlan(t), node)
			if expr != nil || err == nil {
				t.Fatalf("want an error, got %#v", expr)
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %s", err, want)
				}
			}
		})
	}
}
