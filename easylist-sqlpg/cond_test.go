package sqlpg

import (
	"reflect"
	"testing"
	"time"
)

// Each node's fragment is the template easylist-sql-goqu used before the
// lowering moved here, with slots in template order.
func TestFragments(t *testing.T) {
	col := Expr{SQL: `t."x"`}
	sql := Expr{SQL: "(t.a || t.b)"}
	a, b := Arg{Value: "a"}, Arg{Value: "b"}
	n1, n2 := Arg{Value: "1.5", Cast: CastNumeric}, Arg{Value: "2", Cast: CastNumeric}
	s := Arg{Value: time.Date(2026, 10, 26, 22, 0, 0, 0, time.UTC)}
	e := Arg{Value: time.Date(2026, 10, 27, 22, 0, 0, 0, time.UTC)}
	cases := []struct {
		name  string
		cond  Cond
		sql   string
		slots []any
	}{
		{"raw scope", Raw{SQL: "u.meta ? 'vip' OR u.x"}, "(?)", []any{Expr{SQL: "u.meta ? 'vip' OR u.x"}}},
		{"=", Compare{Col: col, Op: OpEq, Arg: a}, "? = ?", []any{col, a}},
		{"!=", Compare{Col: sql, Op: OpDistinct, Arg: a}, "? IS DISTINCT FROM ?", []any{sql, a}},
		{">", Compare{Col: col, Op: OpGt, Arg: a}, "? > ?", []any{col, a}},
		{">=", Compare{Col: col, Op: OpGte, Arg: a}, "? >= ?", []any{col, a}},
		{"<", Compare{Col: col, Op: OpLt, Arg: a}, "? < ?", []any{col, a}},
		{"<=", Compare{Col: col, Op: OpLte, Arg: a}, "? <= ?", []any{col, a}},
		{"numeric =", Compare{Col: col, Op: OpEq, Arg: n1}, "? = ?::numeric", []any{col, n1}},
		{"numeric !=", Compare{Col: col, Op: OpDistinct, Arg: n1}, "? IS DISTINCT FROM ?::numeric", []any{col, n1}},
		{"IS NULL", IsNull{Col: sql}, "? IS NULL", []any{sql}},
		{"IS NOT NULL", IsNull{Col: col, Not: true}, "? IS NOT NULL", []any{col}},
		{"IN", In{Col: col, Args: []Arg{a, b}}, "? IN (?, ?)", []any{col, a, b}},
		{"IN with NULL", In{Col: col, Args: []Arg{a}, Null: NullMatch}, "COALESCE(? IN (?), TRUE)", []any{col, a}},
		{"NOT IN", In{Col: col, Args: []Arg{a, b}, Not: true, Null: NullMatch}, "COALESCE(? NOT IN (?, ?), TRUE)", []any{col, a, b}},
		{"NOT IN with NULL", In{Col: col, Args: []Arg{a, b}, Not: true, Null: NullNoMatch}, "COALESCE(? NOT IN (?, ?), FALSE)", []any{col, a, b}},
		{"IN numeric", In{Col: col, Args: []Arg{n1, n2}}, "? IN (?::numeric, ?::numeric)", []any{col, n1, n2}},
		{"IN only NULL", In{Col: col, Null: NullMatch}, "? IS NULL", []any{col}},
		{"NOT IN only NULL", In{Col: col, Not: true, Null: NullNoMatch}, "? IS NOT NULL", []any{col}},
		{"BETWEEN", Between{Col: col, Low: a, High: b}, "? BETWEEN ? AND ?", []any{col, a, b}},
		{"BETWEEN numeric", Between{Col: col, Low: n1, High: n2}, "? BETWEEN ?::numeric AND ?::numeric", []any{col, n1, n2}},
		{"CONTAINS", ILike{Col: col, Pattern: Arg{Value: "%a%"}}, `? ILIKE ? ESCAPE E'\\'`, []any{col, Arg{Value: "%a%"}}},
		{"EXCLUDES", ILike{Col: sql, Pattern: Arg{Value: "%a%"}, Not: true}, `COALESCE(? NOT ILIKE ? ESCAPE E'\\', TRUE)`, []any{sql, Arg{Value: "%a%"}}},
		{"array CONTAINS one", ArrayHas{Col: col, Values: []Arg{a}}, "(? = ANY(?))", []any{a, col}},
		{"array CONTAINS", ArrayHas{Col: col, Values: []Arg{a, b}}, "(? = ANY(?) AND ? = ANY(?))", []any{a, col, b, col}},
		{"array CONTAINS numeric", ArrayHas{Col: col, Values: []Arg{n1, n2}}, "(?::numeric = ANY(?) AND ?::numeric = ANY(?))", []any{n1, col, n2, col}},
		{"array EXCLUDES one", ArrayHas{Col: col, Values: []Arg{a}, None: true}, "COALESCE(NOT (? = ANY(?)), TRUE)", []any{a, col}},
		{"array EXCLUDES", ArrayHas{Col: col, Values: []Arg{a, b}, None: true}, "COALESCE(NOT (? = ANY(?) OR ? = ANY(?)), TRUE)", []any{a, col, b, col}},
		{"whole day", DayRange{Col: col, Start: s, End: e}, "(? >= ? AND ? < ?)", []any{col, s, col, e}},
		{"not whole day", DayRange{Col: sql, Start: s, End: e, Not: true}, "COALESCE(NOT (? >= ? AND ? < ?), TRUE)", []any{sql, s, sql, e}},
		{
			"AND", And{Left: Raw{SQL: "u.deleted_at IS NULL"}, Right: Compare{Col: col, Op: OpEq, Arg: a}},
			"((?) AND ? = ?)", []any{Expr{SQL: "u.deleted_at IS NULL"}, col, a},
		},
		{
			"OR inside AND", And{Left: Or{Left: Compare{Col: col, Op: OpEq, Arg: a}, Right: IsNull{Col: col}}, Right: In{Col: col, Args: []Arg{b}}},
			"((? = ? OR ? IS NULL) AND ? IN (?))", []any{col, a, col, col, b},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := tc.cond.Fragment()
			if f.SQL != tc.sql || !reflect.DeepEqual(f.Slots, tc.slots) {
				t.Errorf("got %q %#v\nwant %q %#v", f.SQL, f.Slots, tc.sql, tc.slots)
			}
		})
	}
}

func TestArgsOrder(t *testing.T) {
	q := &Query{
		Where: And{
			Left:  Raw{SQL: "u.x ? 'a'"},
			Right: Or{Left: ArrayHas{Col: Expr{SQL: "t"}, Values: []Arg{{Value: "a"}, {Value: "b"}}}, Right: Compare{Col: Expr{SQL: "n"}, Op: OpGt, Arg: Arg{Value: "2", Cast: CastNumeric}}},
		},
		Page: &Page{Limit: 10, Offset: 0},
	}
	if got, want := q.Args(), []any{"a", "b", "2", int64(10), int64(0)}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
	if got := (&Query{Count: true}).Args(); got == nil || len(got) != 0 {
		t.Errorf("no arguments: got %#v", got)
	}
}
