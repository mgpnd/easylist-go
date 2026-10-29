package sqljet

import (
	"database/sql"
	"reflect"
	"testing"
	"time"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	"github.com/mgpnd/easylist-go/easylist-sqlpg"

	"github.com/go-jet/jet/v2/postgres"
)

func lower(t *testing.T, p *easylist.Plan) *sqlpg.Query {
	t.Helper()
	q, err := sqlpg.Lower(p)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

// render writes a condition as the WHERE clause of a statement over a plain
// table, with jet's own serialization.
func render(t *testing.T, c postgres.BoolExpression) (string, []any) {
	t.Helper()
	query, args, err := serialize(postgres.SELECT(postgres.Raw("1")).FROM(postgres.NewTable("", "x", "")).WHERE(c))
	if err != nil {
		t.Fatal(err)
	}
	return where(query), args
}

// A fragment's text is written as Tokens, an Expr as a Raw jet does not scan,
// and each Arg as a Raw of one named argument, bound as the next $n. The
// characters jet's SQL builder treats specially, its own named-argument
// syntax among them, stay as written.
func TestFragment(t *testing.T) {
	cases := []struct {
		name string
		f    sqlpg.Fragment
		sql  string
		args []any
	}{
		{"compare", sqlpg.Fragment{SQL: "? = ?", Slots: []any{sqlpg.Expr{SQL: "u.x"}, sqlpg.Arg{Value: "a"}}},
			`(u.x) = ($1)`, []any{"a"}},
		{"numeric cast follows the placeholder", sqlpg.Fragment{SQL: "? IN (?::numeric, ?::numeric)", Slots: []any{sqlpg.Expr{SQL: "u.p"}, sqlpg.Arg{Value: "1", Cast: sqlpg.CastNumeric}, sqlpg.Arg{Value: "2", Cast: sqlpg.CastNumeric}}},
			`(u.p) IN (($1::numeric), ($2::numeric))`, []any{"1", "2"}},
		{"escape string", sqlpg.Fragment{SQL: `? ILIKE ? ESCAPE E'\\'`, Slots: []any{sqlpg.Expr{SQL: "u.n"}, sqlpg.Arg{Value: "%a%"}}},
			`(u.n) ILIKE ($1) ESCAPE E'\\'`, []any{"%a%"}},
		{"named-argument text in author SQL", sqlpg.Fragment{SQL: "? = ?", Slots: []any{sqlpg.Expr{SQL: "f('#v', '#1', '#10', $$x$$, ?, :a)"}, sqlpg.Arg{Value: "#v"}}},
			`(f('#v', '#1', '#10', $$x$$, ?, :a)) = ($1)`, []any{"#v"}},
		{"template without text", sqlpg.Fragment{SQL: "?", Slots: []any{sqlpg.Expr{SQL: "u.ok"}}},
			`(u.ok)`, []any{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := fragment(tc.f)
			if err != nil {
				t.Fatal(err)
			}
			sql, args := render(t, c)
			if args == nil {
				args = []any{}
			}
			if sql != tc.sql || !reflect.DeepEqual(args, tc.args) {
				t.Errorf("got\n%s\n%#v\nwant\n%s\n%#v", sql, args, tc.sql, tc.args)
			}
		})
	}
}

// fragment binds every Arg slot as one $n and refuses values that would not
// be one plain argument, and templates whose slots or casts do not match.
func TestFragmentRejectsValues(t *testing.T) {
	col := sqlpg.Expr{SQL: "u.x"}
	for name, f := range map[string]sqlpg.Fragment{
		"nil":            {SQL: "? = ?", Slots: []any{col, sqlpg.Arg{}}},
		"nil pointer":    {SQL: "? = ?", Slots: []any{col, sqlpg.Arg{Value: (*string)(nil)}}},
		"slice":          {SQL: "? = ?", Slots: []any{col, sqlpg.Arg{Value: []string{"a"}}}},
		"bytes":          {SQL: "? = ?", Slots: []any{col, sqlpg.Arg{Value: []byte("a")}}},
		"array":          {SQL: "? = ?", Slots: []any{col, sqlpg.Arg{Value: [1]int64{1}}}},
		"named arg":      {SQL: "? = ?", Slots: []any{col, sqlpg.Arg{Value: sql.Named("a", 1)}}},
		"named arg ptr":  {SQL: "? = ?", Slots: []any{col, sqlpg.Arg{Value: &sql.NamedArg{Name: "a"}}}},
		"jet raw":        {SQL: "? = ?", Slots: []any{col, sqlpg.Arg{Value: postgres.Raw("1")}}},
		"jet literal":    {SQL: "? = ?", Slots: []any{col, sqlpg.Arg{Value: postgres.String("a")}}},
		"jet token":      {SQL: "? = ?", Slots: []any{col, sqlpg.Arg{Value: postgres.Token("1")}}},
		"jet statement":  {SQL: "? = ?", Slots: []any{col, sqlpg.Arg{Value: postgres.SELECT(postgres.Raw("1"))}}},
		"unknown slot":   {SQL: "? = ?", Slots: []any{col, "x"}},
		"cast missing":   {SQL: "? = ?", Slots: []any{col, sqlpg.Arg{Value: "1", Cast: sqlpg.CastNumeric}}},
		"too few slots":  {SQL: "? = ? AND ?", Slots: []any{col, sqlpg.Arg{Value: "a"}}},
		"too many slots": {SQL: "? = 1", Slots: []any{col, sqlpg.Arg{Value: "a"}}},
	} {
		if _, err := fragment(f); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

// Values of every type sqlpg binds are plain arguments.
func TestCheckValueAccepts(t *testing.T) {
	for _, v := range []any{"a", "", int64(0), 1.5, true, false, time.Date(2000, 1, 2, 0, 0, 0, 0, time.UTC)} {
		if err := checkValue(v); err != nil {
			t.Errorf("checkValue(%#v) = %v", v, err)
		}
	}
}

// jet writes an alias between double quotes as is, so an alias with a double
// quote is refused rather than emitted broken.
func TestCheckAlias(t *testing.T) {
	for alias, ok := range map[string]bool{
		"name": true, "_x1": true, "all_items": true, "ALL": true, "a.b": true,
		`a"b`: false, "": false,
	} {
		if err := checkAlias(alias); (err == nil) != ok {
			t.Errorf("checkAlias(%q) = %v", alias, err)
		}
	}
}

// jet writes an alias quoted and with its dots, and the sentinel table
// unquoted and without an alias.
func TestJetProjectionAndTable(t *testing.T) {
	query, args, err := serialize(postgres.SELECT(postgres.Raw("x").AS("a.b")).FROM(postgres.NewTable("", sentinelBase+"_x", "")))
	if err != nil || query != "SELECT (x) AS \"a.b\"\nFROM easylist_from_x" || len(args) != 0 {
		t.Errorf("got %q %#v %v", query, args, err)
	}
}

// chooseSentinel looks at every piece of author SQL of the query.
func TestChooseSentinel(t *testing.T) {
	expr := func(s string) sqlpg.Expr { return sqlpg.Expr{SQL: s} }
	base := func() *sqlpg.Query {
		return &sqlpg.Query{From: expr("users AS u"), Select: []sqlpg.Column{{Expr: expr("u.id"), Alias: "id"}}}
	}
	cases := map[string]func(q *sqlpg.Query){
		"from":   func(q *sqlpg.Query) { q.From = expr("easylist_from AS u") },
		"select": func(q *sqlpg.Query) { q.Select[0].Expr = expr("u.easylist_from") },
		"alias":  func(q *sqlpg.Query) { q.Select[0].Alias = "easylist_from" },
		"order":  func(q *sqlpg.Query) { q.Order = []sqlpg.Ordering{{Expr: expr("easylist_from")}} },
		"scope":  func(q *sqlpg.Query) { q.Where = sqlpg.Raw{SQL: "easylist_from"} },
		"filter column": func(q *sqlpg.Query) {
			q.Where = sqlpg.And{Left: sqlpg.Raw{SQL: "TRUE"}, Right: sqlpg.IsNull{Col: expr("x.easylist_from")}}
		},
	}
	if s := chooseSentinel(base()); s != "easylist_from" {
		t.Errorf("no author sentinel: %q", s)
	}
	for name, change := range cases {
		q := base()
		change(q)
		if s := chooseSentinel(q); s != "easylist_from_x" {
			t.Errorf("%s: %q", name, s)
		}
	}
	// A value equal to the sentinel is bound, not written, so it does not
	// count.
	q := base()
	q.Where = sqlpg.Compare{Col: expr("u.x"), Op: sqlpg.OpEq, Arg: sqlpg.Arg{Value: "easylist_from"}}
	if s := chooseSentinel(q); s != "easylist_from" {
		t.Errorf("value: %q", s)
	}
}
