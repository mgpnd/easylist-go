package querysql

import (
	"reflect"
	"strings"
	"testing"
	"time"

	filter "github.com/mgpnd/easylist-filter"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/doug-martin/goqu/v9/exp"
)

func predicateTable() tableMeta {
	return tableMeta{name: "t", columns: map[string]columnMeta{
		"name":   {name: "name", kind: kindText},
		"age":    {name: "age", kind: kindInteger},
		"score":  {name: "score", kind: kindFloat},
		"price":  {name: "price", kind: kindNumeric},
		"day":    {name: "day", kind: kindDate},
		"active": {name: "active", kind: kindBoolean},
		"tags":   {name: "tags", kind: kindText, arrayDimensions: 1},
		"ages":   {name: "ages", kind: kindInteger, arrayDimensions: 1},
		"scores": {name: "scores", kind: kindFloat, arrayDimensions: 1},
		"prices": {name: "prices", kind: kindNumeric, arrayDimensions: 1},
		"days":   {name: "days", kind: kindDate, arrayDimensions: 1},
		"flags":  {name: "flags", kind: kindBoolean, arrayDimensions: 1},
		"data":   {name: "data", kind: kindUnsupported},
		"a.b":    {name: "a.b", kind: kindText},
	}}
}

func renderPredicate(t *testing.T, filter string) (string, []any) {
	t.Helper()
	expr, err := buildPredicate(predicateTable(), parsePredicate(t, filter))
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
	cases := []struct {
		filter, clause string
		args           []any
	}{
		{"age < 30", `"t"."age" < $1`, []any{int64(30)}},
		{"age = -9223372036854775808", `"t"."age" = $1`, []any{int64(-9223372036854775808)}},
		{"age > 1", `"t"."age" > $1`, []any{int64(1)}},
		{"age >= 2", `"t"."age" >= $1`, []any{int64(2)}},
		{"age <= 3", `"t"."age" <= $1`, []any{int64(3)}},
		{"score = -1.25", `"t"."score" = $1`, []any{float64(-1.25)}},
		{"score > 2.5", `"t"."score" > $1`, []any{float64(2.5)}},
		{"price = 12345678901234567890.123456789", `"t"."price" = $1::numeric`, []any{"12345678901234567890.123456789"}},
		{"price <= -0.01", `"t"."price" <= $1::numeric`, []any{"-0.01"}},
		{"day = '2024-02-29'", `"t"."day" = $1`, []any{day}},
		{"day >= '2024-02-29'", `"t"."day" >= $1`, []any{day}},
		{"name = 'O''Reilly'", `"t"."name" = $1`, []any{"O'Reilly"}},
		{"active = TRUE", `"t"."active" = $1`, []any{true}},
		{"active = false", `"t"."active" = $1`, []any{false}},
		{"age != -9223372036854775808", `"t"."age" <> $1`, []any{int64(-9223372036854775808)}},
		{"score != -1.25", `"t"."score" <> $1`, []any{float64(-1.25)}},
		{"price != 12345678901234567890.123456789", `"t"."price" <> $1::numeric`, []any{"12345678901234567890.123456789"}},
		{"day != '2024-02-29'", `"t"."day" <> $1`, []any{day}},
		{"name != 'O''Reilly'", `"t"."name" <> $1`, []any{"O'Reilly"}},
		{"active != TRUE", `"t"."active" <> $1`, []any{true}},
		{"active!=false", `"t"."active" <> $1`, []any{false}},
		{"name CONTAINS 'needle'", `"t"."name" ILIKE $1 ESCAPE E'\\'`, []any{"%needle%"}},
		{"tags CONTAINS 'needle'", `array_position("t"."tags", $1) IS NOT NULL`, []any{"needle"}},
		{"ages CONTAINS 30", `array_position("t"."ages", $1) IS NOT NULL`, []any{int64(30)}},
		{"scores CONTAINS 1.5", `array_position("t"."scores", $1) IS NOT NULL`, []any{float64(1.5)}},
		{"prices CONTAINS 1.50", `array_position("t"."prices", $1::numeric) IS NOT NULL`, []any{"1.50"}},
		{"days CONTAINS '2024-02-29'", `array_position("t"."days", $1) IS NOT NULL`, []any{day}},
		{"flags CONTAINS TRUE", `array_position("t"."flags", $1) IS NOT NULL`, []any{true}},
		{"age BETWEEN ' 40, 30 '", `"t"."age" BETWEEN $1 AND $2`, []any{int64(40), int64(30)}},
		{"score BETWEEN '-1.5,2.5'", `"t"."score" BETWEEN $1 AND $2`, []any{float64(-1.5), float64(2.5)}},
		{"price BETWEEN '1.01,2.02'", `"t"."price" BETWEEN $1::numeric AND $2::numeric`, []any{"1.01", "2.02"}},
		{"day BETWEEN '2024-02-29,2025-01-01'", `"t"."day" BETWEEN $1 AND $2`, []any{day, end}},
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

func TestBuildPredicateEscapingAndNullArrays(t *testing.T) {
	sql, args := renderPredicate(t, `name CONTAINS 'a%_\b'`)
	if !strings.Contains(sql, `ILIKE $1 ESCAPE E'\\'`) || !reflect.DeepEqual(args, []any{`%a\%\_\\b%`}) {
		t.Fatalf("literal wildcard pattern: %q %#v", sql, args)
	}
	sql, args = renderPredicate(t, `"a.b" = 'safe'`)
	if !strings.Contains(sql, `"t"."a.b" = $1`) || !reflect.DeepEqual(args, []any{"safe"}) {
		t.Fatalf("dotted identifier split: %q %#v", sql, args)
	}
	sql, args = renderPredicate(t, "tags EXCLUDES 'needle'")
	if !strings.Contains(sql, `"t"."tags" IS NOT NULL`) || !strings.Contains(sql, `array_position("t"."tags", $1) IS NULL`) || !strings.Contains(sql, " AND ") || !reflect.DeepEqual(args, []any{"needle"}) {
		t.Fatalf("exclusion must reject null arrays and accept empty arrays: %q %#v", sql, args)
	}

	table := predicateTable()
	table.schema, table.name = "odd.schema", "odd.table"
	expr, err := buildPredicate(table, parsePredicate(t, `"a.b" = 'value'`))
	if err != nil {
		t.Fatal(err)
	}
	sql, args, err = goqu.Dialect("postgres").From(goqu.T(table.name).Schema(table.schema)).Where(expr).Prepared(true).ToSQL()
	if err != nil || !strings.Contains(sql, `"odd.schema"."odd.table"."a.b" = $1`) || !reflect.DeepEqual(args, []any{"value"}) {
		t.Fatalf("qualified identifier: %q %#v %v", sql, args, err)
	}
}

func TestBuildPredicateRejectsBadValues(t *testing.T) {
	cases := []struct{ filter, position string }{
		{"age = 9223372036854775808", "position 7"},
		{"age = -9223372036854775809", "position 7"},
		{"age = 1.5", "position 7"},
		{"age = '30'", "position 7"},
		{"day = '2025-02-29'", "position 7"},
		{"day = '2024-2-29'", "position 7"},
		{"day = '2024-02-29T00:00:00Z'", "position 7"},
		{"day = 20240229", "position 7"},
		{"age BETWEEN '1,2,3'", "position 13"},
		{"age BETWEEN '1,'", "position 13"},
		{"age BETWEEN ',2'", "position 13"},
		{"age BETWEEN '1'", "position 13"},
		{"age BETWEEN '1,2.5'", "position 13"},
		{"score BETWEEN 'NaN,2'", "position 15"},
		{"price BETWEEN '1,2e3'", "position 15"},
		{"score = " + strings.Repeat("9", 400), "position 9"},
		{"name = 1", "position 8"},
		{"active = 'true'", "position 10"},
		{"active = 1", "position 10"},
		{"age != 1.5", "position 8"},
		{"name != 1", "position 9"},
		{"active != 'true'", "position 11"},
		{"day != '2025-02-29'", "position 8"},
		{"tags CONTAINS 1", "position 15"},
		{"ages EXCLUDES '1'", "position 15"},
		{"absent = 1", "position 1"},
		{"data = 'x'", "position 1"},
		{"name > 'a'", "position 6"},
		{"active < TRUE", "position 8"},
		{"age CONTAINS 1", "position 5"},
		{"name EXCLUDES 'a'", "position 6"},
		{"name BETWEEN 'a,b'", "position 6"},
		{"tags = 'a'", "position 6"},
		{"tags != 'a'", "position 6"},
		{"data != 'x'", "position 1"},
		{"ages > 1", "position 6"},
		{"ages BETWEEN '1,2'", "position 6"},
	}
	for _, tc := range cases {
		t.Run(tc.filter, func(t *testing.T) {
			expr, err := buildPredicate(predicateTable(), parsePredicate(t, tc.filter))
			if expr != nil || err == nil || !strings.Contains(err.Error(), tc.position) {
				t.Fatalf("want nil expression and error at %s; got %#v %v", tc.position, expr, err)
			}
		})
	}
}

// Ensure buildPredicate consumes the public AST predicate contract.
var _ func(tableMeta, filter.Predicate) (exp.Expression, error) = buildPredicate

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
