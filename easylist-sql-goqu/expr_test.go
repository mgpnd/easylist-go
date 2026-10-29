package sqlgoqu

import (
	"reflect"
	"strings"
	"testing"
	"time"
	_ "time/tzdata" // Europe/London without a system tz database

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	filter "github.com/mgpnd/easylist-go/easylist-filter"

	"github.com/doug-martin/goqu/v9"
)

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

// planExpr plans one predicate of predicateList at now.
func planExpr(t *testing.T, input string, now time.Time) (*easylist.Plan, filter.Predicate) {
	t.Helper()
	pred := parsePredicate(t, input)
	reg, err := easylist.NewRegistry(predicateList)
	if err != nil {
		t.Fatal(err)
	}
	l, _ := reg.List("t")
	p, err := l.Plan(easylist.KindPage, easylist.Request{Filter: pred, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	return p, pred
}

// renderExpr plans and compiles one predicate and returns its WHERE clause.
func renderExpr(t *testing.T, input string, now time.Time) (string, []any) {
	t.Helper()
	p, pred := planExpr(t, input, now)
	expr, err := lowerPredicate(p, pred)
	if err != nil {
		t.Fatal(err)
	}
	sql, args, err := goqu.Dialect("postgres").From(goqu.T("t")).Where(expr).Prepared(true).ToSQL()
	if err != nil {
		t.Fatal(err)
	}
	_, clause, _ := strings.Cut(sql, " WHERE ")
	return clause, args
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

// noDBClock fails when SQL reads the database clock or time zone.
func noDBClock(t *testing.T, sql string) {
	t.Helper()
	lower := strings.ToLower(sql)
	for _, fn := range []string{"now(", "current_date", "current_time", "localtime", "clock_timestamp", "statement_timestamp", "transaction_timestamp", "time zone", "date_trunc", "interval"} {
		if strings.Contains(lower, fn) {
			t.Errorf("SQL uses %s: %s", fn, sql)
		}
	}
}

func TestExprValues(t *testing.T) {
	london := londonZone(t)
	cases := []struct {
		name, filter string
		now          time.Time
		clause       string
		args         []any
	}{
		// Relative time comparison: date result on a date field.
		{"date field", "day > {TODAY + 5::DAYS}", nowUTC, `t."day" > $1`, []any{tm(2026, 11, 1, 0, 0, utc)}},
		{"date field in UTC+02:00", "day > {TODAY + 5::DAYS}", nowP2, `t."day" > $1`, []any{tm(2026, 11, 1, 0, 0, utc)}},
		// 23:30Z is already Wednesday in UTC+02:00.
		{"date field after midnight in UTC+02:00", "day = TODAY", tm(2026, 10, 27, 23, 30, utc).In(plus2), `t."day" = $1`, []any{tm(2026, 10, 28, 0, 0, utc)}},
		{"date field != TODAY", "day != TODAY", nowUTC, `t."day" IS DISTINCT FROM $1`, []any{tm(2026, 10, 27, 0, 0, utc)}},
		{"date field START_OF_MONTH", "day >= start_of_month", nowP2, `t."day" >= $1`, []any{tm(2026, 10, 1, 0, 0, utc)}},
		{"date field in London DST change", "day <= TODAY", tm(2026, 10, 25, 12, 0, london), `t."day" <= $1`, []any{tm(2026, 10, 25, 0, 0, utc)}},
		// Relative time comparison: timestamp result on a timestamp field.
		{"timestamp field", "at > {NOW - 30::MINUTES}", nowUTC, `t."at" > $1`, []any{tm(2026, 10, 27, 9, 30, utc)}},
		{"timestamp field =", "at = NOW", nowP2, `t."at" = $1`, []any{tm(2026, 10, 27, 8, 0, utc)}},
		{"timestamp field !=", "at != {NOW + 1::DAYS}", nowP2, `t."at" IS DISTINCT FROM $1`, []any{tm(2026, 10, 28, 8, 0, utc)}},
		{"timestamp field >=", "at >= {TODAY + 2::HOURS}", nowP2, `t."at" >= $1`, []any{tm(2026, 10, 27, 0, 0, utc)}},
		{"timestamp field <", "at < {NOW + (1::DAYS + 2::HOURS)}", nowUTC, `t."at" < $1`, []any{tm(2026, 10, 28, 12, 0, utc)}},
		{"timestamp field <=", "at <= {NOW - 90::MINUTES}", nowUTC, `t."at" <= $1`, []any{tm(2026, 10, 27, 8, 30, utc)}},
		// Whole-day comparison on timestamp fields, UTC.
		{"whole last day included", "at <= END_OF_WEEK", nowUTC, `t."at" < $1`, []any{tm(2026, 11, 2, 0, 0, utc)}},
		// Whole-day comparison on timestamp fields, UTC+02:00.
		{"> day in UTC+02:00", "at > TODAY", nowP2, `t."at" >= $1`, []any{tm(2026, 10, 27, 22, 0, utc)}},
		{">= day in UTC+02:00", "at >= TODAY", nowP2, `t."at" >= $1`, []any{tm(2026, 10, 26, 22, 0, utc)}},
		{"< day in UTC+02:00", "at < TODAY", nowP2, `t."at" < $1`, []any{tm(2026, 10, 26, 22, 0, utc)}},
		{"<= day in UTC+02:00", "at <= TODAY", nowP2, `t."at" < $1`, []any{tm(2026, 10, 27, 22, 0, utc)}},
		{"= day in UTC+02:00", "at = TODAY", nowP2, `(t."at" >= $1 AND t."at" < $2)`, []any{tm(2026, 10, 26, 22, 0, utc), tm(2026, 10, 27, 22, 0, utc)}},
		{"!= day in UTC+02:00", "at != TODAY", nowP2, `COALESCE(NOT (t."at" >= $1 AND t."at" < $2), TRUE)`, []any{tm(2026, 10, 26, 22, 0, utc), tm(2026, 10, 27, 22, 0, utc)}},
		{"date arithmetic on a timestamp field", "at >= {TODAY - 1::BUSINESS_DAYS}", nowP2, `t."at" >= $1`, []any{tm(2026, 10, 25, 22, 0, utc)}},
		// Europe/London leaves BST on 2026-10-25, a 25-hour day.
		{"= DST day", "at = TODAY", tm(2026, 10, 25, 12, 0, london), `(t."at" >= $1 AND t."at" < $2)`, []any{tm(2026, 10, 24, 23, 0, utc), tm(2026, 10, 26, 0, 0, utc)}},
		{"> DST day", "at > TODAY", tm(2026, 10, 25, 12, 0, london), `t."at" >= $1`, []any{tm(2026, 10, 26, 0, 0, utc)}},
		{"< DST day", "at < TODAY", tm(2026, 10, 25, 12, 0, london), `t."at" < $1`, []any{tm(2026, 10, 24, 23, 0, utc)}},
		{"<= day before DST change", "at <= {TODAY - 1::DAYS}", tm(2026, 10, 25, 12, 0, london), `t."at" < $1`, []any{tm(2026, 10, 24, 23, 0, utc)}},
		// Numbers convert like number literals of the same text.
		{"integer", "age > {2 * 3}", nowUTC, `t."age" > $1`, []any{int64(6)}},
		{"negative integer", "age = {-(1 + 2)}", nowUTC, `t."age" = $1`, []any{int64(-3)}},
		{"float", "score <= {10 / 4}", nowUTC, `t."score" <= $1`, []any{2.5}},
		{"numeric", "price > {10 / 4}", nowUTC, `t."price" > $1::numeric`, []any{"2.5"}},
		{"numeric exact", "price != {0.1 + 0.2}", nowUTC, `t."price" IS DISTINCT FROM $1::numeric`, []any{"0.3"}},
		{"numeric negative", "price < {1 - 1.10}", nowUTC, `t."price" < $1::numeric`, []any{"-0.1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clause, args := renderExpr(t, tc.filter, tc.now)
			if clause != tc.clause || !sameArgs(args, tc.args) {
				t.Fatalf("got %q %v; want %q %v", clause, args, tc.clause, tc.args)
			}
			noDBClock(t, clause)
		})
	}
}

func TestExprWholeDayRows(t *testing.T) {
	// created_at <= END_OF_WEEK on Tuesday 2026-10-27 in UTC, Monday week start.
	_, args := renderExpr(t, "at <= END_OF_WEEK", nowUTC)
	end := args[0].(time.Time)
	if last := time.Date(2026, 11, 1, 23, 59, 59, 0, utc); !last.Before(end) {
		t.Errorf("%v does not match < %v", last, end)
	}
	if next := tm(2026, 11, 2, 0, 0, utc); next.Before(end) {
		t.Errorf("%v matches < %v", next, end)
	}
}

func TestExprDSTDayIs25Hours(t *testing.T) {
	_, args := renderExpr(t, "at = TODAY", tm(2026, 10, 25, 9, 0, londonZone(t)))
	s, e := args[0].(time.Time), args[1].(time.Time)
	if e.Sub(s) != 25*time.Hour {
		t.Fatalf("day is %v long: [%v, %v)", e.Sub(s), s, e)
	}
}

func TestExprDateFieldBindsLikeLiteral(t *testing.T) {
	_, fromExpr := renderExpr(t, "day = TODAY", tm(2026, 10, 27, 23, 0, plus2))
	_, fromLiteral := renderPredicate(t, "day = '2026-10-27'")
	if !reflect.DeepEqual(fromExpr, fromLiteral) {
		t.Fatalf("TODAY binds %#v, the literal binds %#v", fromExpr, fromLiteral)
	}
}

func TestExprMicrosecondTruncation(t *testing.T) {
	now := time.Date(2026, 10, 27, 10, 0, 0, 123456700, utc)
	clause, args := renderExpr(t, "at > {NOW - 30::MINUTES}", now)
	want := time.Date(2026, 10, 27, 9, 30, 0, 123456000, utc)
	if clause != `t."at" > $1` || len(args) != 1 || !args[0].(time.Time).Equal(want) {
		t.Fatalf("got %q %v; want argument %v", clause, args, want)
	}
	// Day boundaries are truncated too; midnight already is.
	_, args = renderExpr(t, "at = TODAY", now)
	for _, a := range args {
		if a.(time.Time).Nanosecond()%1000 != 0 {
			t.Errorf("boundary %v not truncated", a)
		}
	}
}

func TestExprRejectsNonIntegerResult(t *testing.T) {
	p, pred := planExpr(t, "age > {10 / 4}", nowUTC)
	_, err := lowerPredicate(p, pred)
	if err == nil || !strings.Contains(err.Error(), `"2.5"`) || !strings.Contains(err.Error(), "position 7") {
		t.Fatalf("want an error naming 2.5 at position 7, got %v", err)
	}
}

func TestExprMissingValue(t *testing.T) {
	// A predicate the plan did not evaluate, such as one from another parse.
	p, _ := planExpr(t, "at > NOW", nowUTC)
	_, err := lowerPredicate(p, parsePredicate(t, "at > NOW"))
	if err == nil || !strings.Contains(err.Error(), "no evaluated value") || !strings.Contains(err.Error(), "position 6") {
		t.Fatalf("want a positioned error, got %v", err)
	}
}

func TestExprRecompilingMovesTheWindow(t *testing.T) {
	reg, err := easylist.NewRegistry(predicateList)
	if err != nil {
		t.Fatal(err)
	}
	l, _ := reg.List("t")
	for _, tc := range []struct {
		filter string
		want   [2][]any
	}{
		{"day = TODAY", [2][]any{{tm(2026, 10, 27, 0, 0, utc)}, {tm(2026, 10, 28, 0, 0, utc)}}},
		{"at = TODAY", [2][]any{{tm(2026, 10, 27, 0, 0, utc), tm(2026, 10, 28, 0, 0, utc)}, {tm(2026, 10, 28, 0, 0, utc), tm(2026, 10, 29, 0, 0, utc)}}},
	} {
		t.Run(tc.filter, func(t *testing.T) {
			expr, err := filter.Parse(tc.filter) // parsed once, planned twice
			if err != nil {
				t.Fatal(err)
			}
			var sqls [2]string
			for i, now := range []time.Time{nowUTC, nowUTC.AddDate(0, 0, 1)} {
				p, err := l.Plan(easylist.KindCount, easylist.Request{Filter: expr, Now: now})
				if err != nil {
					t.Fatal(err)
				}
				sql, args, err := New().Compile(p)
				if err != nil {
					t.Fatal(err)
				}
				if !sameArgs(args, tc.want[i]) {
					t.Errorf("reference %v: args %v, want %v", now, args, tc.want[i])
				}
				noDBClock(t, sql)
				sqls[i] = sql
			}
			if sqls[0] != sqls[1] {
				t.Errorf("SQL changed: %q / %q", sqls[0], sqls[1])
			}
		})
	}
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

func TestCompileExpressions(t *testing.T) {
	// Thursday 2026-10-29 15:00 in UTC+02:00. START_OF_WEEK is Monday
	// 2026-10-26 00:00+02:00; five business days after Thursday is Thursday
	// 2026-11-05.
	now := time.Date(2026, 10, 29, 15, 0, 0, 0, plus2)
	cases := []struct {
		name, filter, sql string
		args              []any
	}{
		{
			name:   "keyword, business days, and number",
			filter: "created_at >= START_OF_WEEK AND due < {TODAY + 5::BUSINESS_DAYS} AND price > {10 / 4}",
			sql:    `SELECT u."id" AS "id" FROM tasks AS u WHERE ((u."created_at" >= $1 AND u."due" < $2) AND u."price" > $3::numeric) ORDER BY u."id" ASC LIMIT $4 OFFSET $5`,
			args:   []any{tm(2026, 10, 25, 22, 0, utc), tm(2026, 11, 5, 0, 0, utc), "2.5", int64(100), int64(0)},
		},
		{
			name:   "whole-day range inside OR",
			filter: "closed_at != TODAY OR (created_at = {TODAY - 1::DAYS} AND price <= {2 * 3})",
			sql:    `SELECT u."id" AS "id" FROM tasks AS u LEFT JOIN closings c ON c.task_id = u.id WHERE (COALESCE(NOT ((c.closed_at) >= $1 AND (c.closed_at) < $2), TRUE) OR ((u."created_at" >= $3 AND u."created_at" < $4) AND u."price" <= $5::numeric)) ORDER BY u."id" ASC LIMIT $6 OFFSET $7`,
			args: []any{
				tm(2026, 10, 28, 22, 0, utc), tm(2026, 10, 29, 22, 0, utc),
				tm(2026, 10, 27, 22, 0, utc), tm(2026, 10, 28, 22, 0, utc),
				"6", int64(100), int64(0),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sql, args := compile(t, tasksList, easylist.KindPage, easylist.Request{Now: now}, tc.filter)
			if sql != tc.sql || !sameArgs(args, tc.args) {
				t.Errorf("got\n%s\n%v\nwant\n%s\n%v", sql, args, tc.sql, tc.args)
			}
			noDBClock(t, sql)
		})
	}
}

func TestCompileExpressionErrorPosition(t *testing.T) {
	def := tasksList
	def.Fields = append(append([]easylist.Field(nil), def.Fields...), easylist.Field{Name: "qty", Type: easylist.Integer})
	_, _, err := compileErr(t, def, easylist.KindPage, easylist.Request{}, "qty > {10 / 4}")
	if err == nil || !strings.Contains(err.Error(), "position 7") {
		t.Fatalf("want an error at position 7, got %v", err)
	}
	sql, args := compile(t, def, easylist.KindCount, easylist.Request{}, "qty > {2 * 3}")
	if sql != `SELECT COUNT(*) FROM tasks AS u WHERE u."qty" > $1` || !reflect.DeepEqual(args, []any{int64(6)}) {
		t.Fatalf("got %q %#v", sql, args)
	}
}
