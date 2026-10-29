package sqlbob

import (
	"context"
	"database/sql"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mgpnd/easylist-go/easylist-sqlpg"

	"github.com/stephenafamo/bob"
	"github.com/stephenafamo/bob/dialect/psql"
	"github.com/stephenafamo/bob/dialect/psql/dialect"
	"github.com/stephenafamo/bob/expr"
)

// render writes e as bob writes it inside a statement, numbering from $1.
func render(t *testing.T, e bob.Expression) (string, []any) {
	t.Helper()
	var b strings.Builder
	args, err := e.WriteSQL(context.Background(), &b, dialect.Dialect, 1)
	if err != nil {
		t.Fatal(err)
	}
	return b.String(), args
}

// Each node condition maps natively is the node's fragment in parentheses,
// with the same arguments in the same order and with the same Go types, no
// cast, and the column written once. The column is author SQL with ? and $
// in it, which both forms write verbatim.
func TestNativeMatchesFragment(t *testing.T) {
	col := sqlpg.Expr{SQL: `(u.meta->>'a?$1')`}
	at := time.Date(2026, 10, 27, 9, 30, 0, 123456000, time.UTC)
	arg := func(v any) sqlpg.Arg { return sqlpg.Arg{Value: v} }
	var nodes []sqlpg.Cond
	for _, op := range []sqlpg.Op{sqlpg.OpEq, sqlpg.OpDistinct, sqlpg.OpGt, sqlpg.OpGte, sqlpg.OpLt, sqlpg.OpLte} {
		for _, v := range []any{"a", int64(3), 1.5, true, at} {
			nodes = append(nodes, sqlpg.Compare{Col: col, Op: op, Arg: arg(v)})
		}
	}
	nodes = append(nodes,
		sqlpg.IsNull{Col: col},
		sqlpg.IsNull{Col: col, Not: true},
		sqlpg.In{Col: col, Args: []sqlpg.Arg{arg("a")}},
		sqlpg.In{Col: col, Args: []sqlpg.Arg{arg("a"), arg("b"), arg("c")}},
		sqlpg.In{Col: col, Args: []sqlpg.Arg{arg(int64(1)), arg(int64(2))}, Not: true},
		sqlpg.Between{Col: col, Low: arg(int64(1)), High: arg(int64(9))},
		sqlpg.Between{Col: col, Low: arg(at), High: arg(at.Add(time.Hour))},
	)
	for _, n := range nodes {
		e, err := condition(n)
		if err != nil {
			t.Fatal(err)
		}
		f, err := fragment(n.Fragment())
		if err != nil {
			t.Fatal(err)
		}
		native, nargs := render(t, e)
		frag, fargs := render(t, f)
		if native != "("+frag+")" || !reflect.DeepEqual(nargs, fargs) {
			t.Errorf("%#v:\nnative   %s %#v\nfragment %s %#v", n, native, nargs, frag, fargs)
		}
		for i := range nargs {
			if reflect.TypeOf(nargs[i]) != reflect.TypeOf(fargs[i]) {
				t.Errorf("%#v: arg %d is %T, fragment binds %T", n, i+1, nargs[i], fargs[i])
			}
		}
		if strings.Contains(native, "::") || strings.Count(native, col.SQL) != 1 {
			t.Errorf("%#v: cast or repeated column: %s", n, native)
		}
	}
}

// Nodes without a native form, and native-capable nodes with a cast or a
// NULL mode, are written exactly as their fragment.
func TestFragmentNodes(t *testing.T) {
	col := sqlpg.Expr{SQL: `u."x"`}
	num := func(v string) sqlpg.Arg { return sqlpg.Arg{Value: v, Cast: sqlpg.CastNumeric} }
	arg := func(v any) sqlpg.Arg { return sqlpg.Arg{Value: v} }
	at := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	for _, n := range []sqlpg.Cond{
		sqlpg.Raw{SQL: "u.a ? 'b' OR u.c"},
		sqlpg.Compare{Col: col, Op: sqlpg.OpEq, Arg: num("1.5")},
		sqlpg.Compare{Col: col, Op: sqlpg.OpDistinct, Arg: num("1.5")},
		sqlpg.In{Col: col, Args: []sqlpg.Arg{num("1"), num("2")}},
		sqlpg.In{Col: col, Args: []sqlpg.Arg{arg("a")}, Null: sqlpg.NullMatch},
		sqlpg.In{Col: col, Args: []sqlpg.Arg{arg("a")}, Not: true, Null: sqlpg.NullMatch},
		sqlpg.In{Col: col, Args: []sqlpg.Arg{arg("a")}, Not: true, Null: sqlpg.NullNoMatch},
		sqlpg.Between{Col: col, Low: num("1"), High: num("2")},
		sqlpg.Between{Col: col, Low: arg(int64(1)), High: num("2")},
		sqlpg.ILike{Col: col, Pattern: arg("%a%")},
		sqlpg.ILike{Col: col, Pattern: arg("%a%"), Not: true},
		sqlpg.ArrayHas{Col: col, Values: []sqlpg.Arg{arg("a"), arg("b")}},
		sqlpg.ArrayHas{Col: col, Values: []sqlpg.Arg{arg("a")}, None: true},
		sqlpg.DayRange{Col: col, Start: arg(at), End: arg(at.AddDate(0, 0, 1))},
		sqlpg.DayRange{Col: col, Start: arg(at), End: arg(at.AddDate(0, 0, 1)), Not: true},
	} {
		e, err := condition(n)
		if err != nil {
			t.Fatal(err)
		}
		got, args := render(t, e)
		f := n.Fragment()
		want := f.SQL
		var wantArgs []any
		for _, slot := range f.Slots {
			switch s := slot.(type) {
			case sqlpg.Expr:
				want = strings.Replace(want, "?", s.SQL, 1)
			case sqlpg.Arg:
				wantArgs = append(wantArgs, s.Value)
				want = strings.Replace(want, "?", "$"+strconv.Itoa(len(wantArgs)), 1)
			}
		}
		if got != want || !reflect.DeepEqual(args, wantArgs) {
			t.Errorf("%#v:\ngot  %s %#v\nwant %s %#v", n, got, args, want, wantArgs)
		}
	}
}

// An IN without arguments is the IsNull its fragment is.
func TestEmptyIn(t *testing.T) {
	col := sqlpg.Expr{SQL: `u."x"`}
	for not, want := range map[bool]string{false: `(u."x" IS NULL)`, true: `(u."x" IS NOT NULL)`} {
		e, err := condition(sqlpg.In{Col: col, Not: not, Null: sqlpg.NullMatch})
		if err != nil {
			t.Fatal(err)
		}
		if got, args := render(t, e); got != want || len(args) != 0 {
			t.Errorf("got %s %#v, want %s", got, args, want)
		}
	}
}

// fragment and the native operators bind every Arg slot as one $n and refuse
// values that psql.Arg would bind as NULL, as a bob expression or named
// argument, or as one array argument.
func TestRejectsValues(t *testing.T) {
	col := sqlpg.Expr{SQL: "u.x"}
	for name, v := range map[string]any{
		"nil":        nil,
		"slice":      []string{"a"},
		"bytes":      []byte("a"),
		"array":      [1]int64{1},
		"raw":        expr.Raw("1"),
		"expression": psql.Arg(1),
		"clause":     psql.Raw("?", 1),
		"named":      sql.Named("a", 1),
		"named ptr":  &sql.NamedArg{Name: "a", Value: 1},
	} {
		a := sqlpg.Arg{Value: v}
		for _, n := range []sqlpg.Cond{
			sqlpg.Compare{Col: col, Op: sqlpg.OpEq, Arg: a},
			sqlpg.In{Col: col, Args: []sqlpg.Arg{{Value: "ok"}, a}},
			sqlpg.Between{Col: col, Low: a, High: sqlpg.Arg{Value: int64(1)}},
			sqlpg.ILike{Col: col, Pattern: a},
			sqlpg.Or{Left: sqlpg.IsNull{Col: col}, Right: sqlpg.Compare{Col: col, Op: sqlpg.OpLt, Arg: a}},
		} {
			if _, err := condition(n); err == nil {
				t.Errorf("%s in %T: expected an error", name, n)
			}
		}
	}
	for name, f := range map[string]sqlpg.Fragment{
		"unknown slot":        {SQL: "? = ?", Slots: []any{col, "x"}},
		"too few slots":       {SQL: "? = ? AND ?", Slots: []any{col, sqlpg.Arg{Value: "a"}}},
		"too many slots":      {SQL: "? = 1", Slots: []any{col, sqlpg.Arg{Value: "a"}}},
		"escaped question":    {SQL: `? = '\?' AND ? = 1`, Slots: []any{col, col}},
		"expression value":    {SQL: "? = ?", Slots: []any{col, sqlpg.Arg{Value: expr.Raw("1")}}},
		"slice in a fragment": {SQL: "? = ?", Slots: []any{col, sqlpg.Arg{Value: []int64{1}}}},
	} {
		if _, err := fragment(f); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

// bob writes an alias in double quotes without escaping; field names never
// need it, and any other alias is refused rather than emitted broken.
func TestCheckAlias(t *testing.T) {
	for alias, ok := range map[string]bool{
		"name": true, "_x1": true, "all_items": true, "ALL": true,
		`a"b`: false, "": false,
	} {
		if err := checkAlias(alias); (err == nil) != ok {
			t.Errorf("checkAlias(%q) = %v", alias, err)
		}
	}
}
