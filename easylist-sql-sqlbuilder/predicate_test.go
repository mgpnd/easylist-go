package sqlsqlbuilder

import (
	"database/sql"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mgpnd/easylist-go/easylist-sqlpg"

	"github.com/huandu/go-sqlbuilder"
)

// render builds the text that build returns as the WHERE clause of a new
// PostgreSQL select builder, numbering from $1, and returns the condition
// without the WHERE keyword.
func render(t *testing.T, build func(c *sqlbuilder.Cond) (string, error)) (string, []any) {
	t.Helper()
	sb := sqlbuilder.PostgreSQL.NewSelectBuilder()
	s, err := build(&sb.Cond)
	if err != nil {
		t.Fatal(err)
	}
	sb.Where(s)
	query, args := sb.Build()
	if !strings.HasPrefix(query, "WHERE ") {
		t.Fatalf("not a WHERE clause: %q", query)
	}
	return strings.TrimPrefix(query, "WHERE "), args
}

// reference matches the one go-sqlbuilder reference a native method returns.
var reference = regexp.MustCompile(`^\$\d+$`)

// Each node condition maps natively is written exactly as the node's
// fragment, with the same arguments in the same order and with the same Go
// types, no cast, and the column written once. The column is author SQL with
// ? and go-sqlbuilder's own $ syntax in it, which both forms write verbatim:
// the native methods write it unescaped, the fragment escaped.
func TestNativeMatchesFragment(t *testing.T) {
	col := sqlpg.Expr{SQL: `(u.meta->>'a?$1 $? ${x} $$ $')`}
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
		var token string
		native, nargs := render(t, func(c *sqlbuilder.Cond) (string, error) {
			s, err := condition(c, n)
			token = s
			return s, err
		})
		frag, fargs := render(t, func(c *sqlbuilder.Cond) (string, error) { return fragment(c, n.Fragment()) })
		if !reference.MatchString(token) {
			t.Errorf("%#v: not a native method: %q", n, token)
		}
		if native != frag || !reflect.DeepEqual(nargs, fargs) {
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
// NULL mode, are written exactly as their fragment, with every author SQL
// slot verbatim.
func TestFragmentNodes(t *testing.T) {
	col := sqlpg.Expr{SQL: `u."x$1${y}$$"`}
	num := func(v string) sqlpg.Arg { return sqlpg.Arg{Value: v, Cast: sqlpg.CastNumeric} }
	arg := func(v any) sqlpg.Arg { return sqlpg.Arg{Value: v} }
	at := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	for _, n := range []sqlpg.Cond{
		sqlpg.Raw{SQL: "u.a ? 'b' OR u.c ~ '^$'"},
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
		got, args := render(t, func(c *sqlbuilder.Cond) (string, error) { return condition(c, n) })
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
	for not, want := range map[bool]string{false: `u."x" IS NULL`, true: `u."x" IS NOT NULL`} {
		got, args := render(t, func(c *sqlbuilder.Cond) (string, error) {
			return condition(c, sqlpg.In{Col: col, Not: not, Null: sqlpg.NullMatch})
		})
		if got != want || len(args) != 0 {
			t.Errorf("got %s %#v, want %s", got, args, want)
		}
	}
}

// The native methods return "" for an empty field, which a connector drops;
// an empty column takes the fragment, so no condition disappears.
func TestEmptyColumn(t *testing.T) {
	col := sqlpg.Expr{}
	for _, n := range []sqlpg.Cond{
		sqlpg.Compare{Col: col, Op: sqlpg.OpEq, Arg: sqlpg.Arg{Value: "a"}},
		sqlpg.IsNull{Col: col},
		sqlpg.In{Col: col, Args: []sqlpg.Arg{{Value: "a"}}},
		sqlpg.Between{Col: col, Low: sqlpg.Arg{Value: int64(1)}, High: sqlpg.Arg{Value: int64(2)}},
	} {
		got, _ := render(t, func(c *sqlbuilder.Cond) (string, error) {
			return condition(c, sqlpg.And{Left: sqlpg.Raw{SQL: "u.a"}, Right: n})
		})
		f := n.Fragment()
		want := "((u.a) AND " + strings.Replace(f.SQL, "?", "", 1) + ")"
		for i := 1; strings.Contains(want, "?"); i++ {
			want = strings.Replace(want, "?", "$"+strconv.Itoa(i), 1)
		}
		if got != want {
			t.Errorf("%#v: got %s, want %s", n, got, want)
		}
	}
}

// fragment and the native methods bind every Arg slot as one $n and refuse
// values that go-sqlbuilder would write as SQL, a list, a nested builder, or
// a named argument, or bind as NULL or as one array argument.
func TestRejectsValues(t *testing.T) {
	col := sqlpg.Expr{SQL: "u.x"}
	ints := []int64{1}
	for name, v := range map[string]any{
		"nil":            nil,
		"slice":          []string{"a"},
		"slice pointer":  &ints,
		"bytes":          []byte("a"),
		"array":          [1]int64{1},
		"raw":            sqlbuilder.Raw("1"),
		"list":           sqlbuilder.List([]int{1, 2}),
		"tuple":          sqlbuilder.Tuple(1, 2),
		"builder named":  sqlbuilder.Named("a", 1),
		"builder":        sqlbuilder.Build("1"),
		"select builder": sqlbuilder.PostgreSQL.NewSelectBuilder().Select("1"),
		"flavor":         sqlbuilder.PostgreSQL,
		"named":          sql.Named("a", 1),
		"named ptr":      &sql.NamedArg{Name: "a", Value: 1},
	} {
		a := sqlpg.Arg{Value: v}
		for _, n := range []sqlpg.Cond{
			sqlpg.Compare{Col: col, Op: sqlpg.OpEq, Arg: a},
			sqlpg.In{Col: col, Args: []sqlpg.Arg{{Value: "ok"}, a}},
			sqlpg.Between{Col: col, Low: a, High: sqlpg.Arg{Value: int64(1)}},
			sqlpg.ILike{Col: col, Pattern: a},
			sqlpg.Or{Left: sqlpg.IsNull{Col: col}, Right: sqlpg.Compare{Col: col, Op: sqlpg.OpLt, Arg: a}},
		} {
			sb := sqlbuilder.PostgreSQL.NewSelectBuilder()
			if _, err := condition(&sb.Cond, n); err == nil {
				t.Errorf("%s in %T: expected an error", name, n)
			}
		}
	}
	for name, f := range map[string]sqlpg.Fragment{
		"unknown slot":        {SQL: "? = ?", Slots: []any{col, "x"}},
		"too few slots":       {SQL: "? = ? AND ?", Slots: []any{col, sqlpg.Arg{Value: "a"}}},
		"too many slots":      {SQL: "? = 1", Slots: []any{col, sqlpg.Arg{Value: "a"}}},
		"digit after a value": {SQL: "? = ?1", Slots: []any{col, sqlpg.Arg{Value: "a"}}},
		"digit after values":  {SQL: "? IN (??1)", Slots: []any{col, sqlpg.Arg{Value: "a"}, sqlpg.Arg{Value: "b"}}},
		"value then column":   {SQL: "? = ??", Slots: []any{col, sqlpg.Arg{Value: "a"}, sqlpg.Expr{SQL: "1"}}},
		"raw value":           {SQL: "? = ?", Slots: []any{col, sqlpg.Arg{Value: sqlbuilder.Raw("1")}}},
		"slice in a fragment": {SQL: "? = ?", Slots: []any{col, sqlpg.Arg{Value: []int64{1}}}},
	} {
		sb := sqlbuilder.PostgreSQL.NewSelectBuilder()
		if _, err := fragment(&sb.Cond, f); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

// The driver writes an alias in double quotes without escaping; field names
// never need it, and any other alias is refused rather than emitted broken.
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
