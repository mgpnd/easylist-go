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

// predicateList has one field per type, plus a field read through author SQL.
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

// nullOperator rewrites a parsed NULL predicate's operator, which the parser
// itself never allows.
type nullOperator struct {
	filter.Predicate
	op filter.PredicateOp
}

func (n nullOperator) Operator() filter.PredicateOp { return n.op }

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
			node := nullOperator{parsePredicate(t, tc.field+" = NULL"), tc.op}
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
