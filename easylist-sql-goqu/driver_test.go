package sqlgoqu

import (
	"fmt"
	"reflect"
	"sync"
	"testing"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	filter "github.com/mgpnd/easylist-go/easylist-filter"
)

var usersList = easylist.List{
	Name: "users", Table: "users", Alias: "u", PrimaryKey: "id",
	Fields: []easylist.Field{
		{Name: "id", Type: easylist.Integer},
		{Name: "name", Type: easylist.Text},
		{Name: "age", Type: easylist.Integer, Nullable: true},
		{Name: "country", Type: easylist.Text, Nullable: true, SQL: "c.name", Joins: []string{"country"}},
		{Name: "last_order", Type: easylist.Timestamp, Nullable: true, SQL: "lo.created_at", Joins: []string{"last_order"}},
	},
	Joins: []easylist.Join{
		{Key: "country", SQL: "LEFT JOIN countries c ON c.id = u.country_id"},
		{Key: "last_order", SQL: "LEFT JOIN LATERAL (SELECT created_at FROM orders o WHERE o.user_id = u.id ORDER BY created_at DESC LIMIT 1) lo ON TRUE"},
	},
}

func compile(t *testing.T, def easylist.List, kind easylist.Kind, req easylist.Request, filterText string) (string, []any) {
	t.Helper()
	sql, args, err := compileErr(t, def, kind, req, filterText)
	if err != nil {
		t.Fatal(err)
	}
	return sql, args
}

func compileErr(t *testing.T, def easylist.List, kind easylist.Kind, req easylist.Request, filterText string) (string, []any, error) {
	t.Helper()
	p := plan(t, def, kind, req, filterText)
	return New().Compile(p)
}

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

func intp(n int) *int { return &n }

func TestCompilePage(t *testing.T) {
	scoped := usersList
	scoped.Scope = "u.deleted_at IS NULL"
	cases := []struct {
		name   string
		def    easylist.List
		req    easylist.Request
		filter string
		sql    string
		args   []any
	}{
		{
			name: "base and joined fields",
			def:  usersList, req: easylist.Request{Fields: []string{"name", "country"}},
			sql:  `SELECT u."id" AS "id", u."name" AS "name", c.name AS "country" FROM users AS u LEFT JOIN countries c ON c.id = u.country_id ORDER BY u."id" ASC LIMIT $1 OFFSET $2`,
			args: []any{int64(100), int64(0)},
		},
		{
			name: "placeholder order",
			def:  usersList, req: easylist.Request{Fields: []string{"name"}, Limit: intp(10), Offset: intp(0)},
			filter: "name = 'Ann' AND age < 30",
			sql:    `SELECT u."id" AS "id", u."name" AS "name" FROM users AS u WHERE (u."name" = $1 AND u."age" < $2) ORDER BY u."id" ASC LIMIT $3 OFFSET $4`,
			args:   []any{"Ann", int64(30), int64(10), int64(0)},
		},
		{
			name: "limit and offset",
			def:  usersList, req: easylist.Request{Limit: intp(50), Offset: intp(100)},
			sql:  `SELECT u."id" AS "id" FROM users AS u ORDER BY u."id" ASC LIMIT $1 OFFSET $2`,
			args: []any{int64(50), int64(100)},
		},
		{
			name: "scope ANDed with an OR filter",
			def:  scoped, req: easylist.Request{},
			filter: "name = 'a' OR name = 'b'",
			sql:    `SELECT u."id" AS "id" FROM users AS u WHERE ((u.deleted_at IS NULL) AND (u."name" = $1 OR u."name" = $2)) ORDER BY u."id" ASC LIMIT $3 OFFSET $4`,
			args:   []any{"a", "b", int64(100), int64(0)},
		},
		{
			name: "scope without filter",
			def:  scoped, req: easylist.Request{},
			sql:  `SELECT u."id" AS "id" FROM users AS u WHERE (u.deleted_at IS NULL) ORDER BY u."id" ASC LIMIT $1 OFFSET $2`,
			args: []any{int64(100), int64(0)},
		},
		{
			name: "grouping kept",
			def:  usersList, req: easylist.Request{},
			filter: "(name = 'a' OR name = 'b') AND age > 1",
			sql:    `SELECT u."id" AS "id" FROM users AS u WHERE ((u."name" = $1 OR u."name" = $2) AND u."age" > $3) ORDER BY u."id" ASC LIMIT $4 OFFSET $5`,
			args:   []any{"a", "b", int64(1), int64(100), int64(0)},
		},
		{
			name: "sort join and tiebreaker",
			def:  usersList, req: easylist.Request{Sort: []easylist.Sort{{Field: "last_order", Direction: easylist.Desc}}},
			sql:  `SELECT u."id" AS "id" FROM users AS u LEFT JOIN LATERAL (SELECT created_at FROM orders o WHERE o.user_id = u.id ORDER BY created_at DESC LIMIT 1) lo ON TRUE ORDER BY lo.created_at DESC, u."id" ASC LIMIT $1 OFFSET $2`,
			args: []any{int64(100), int64(0)},
		},
		{
			name: "filter on joined field",
			def:  usersList, req: easylist.Request{},
			filter: "country != 'FR'",
			sql:    `SELECT u."id" AS "id" FROM users AS u LEFT JOIN countries c ON c.id = u.country_id WHERE (c.name) <> $1 ORDER BY u."id" ASC LIMIT $2 OFFSET $3`,
			args:   []any{"FR", int64(100), int64(0)},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sql, args := compile(t, tc.def, easylist.KindPage, tc.req, tc.filter)
			if sql != tc.sql || !reflect.DeepEqual(args, tc.args) {
				t.Errorf("got\n%s\n%#v\nwant\n%s\n%#v", sql, args, tc.sql, tc.args)
			}
		})
	}
}

func TestCompileExport(t *testing.T) {
	sql, args := compile(t, usersList, easylist.KindExport, easylist.Request{Fields: []string{"name"}}, "age >= 18")
	want := `SELECT u."id" AS "id", u."name" AS "name" FROM users AS u WHERE u."age" >= $1 ORDER BY u."id" ASC`
	if sql != want || !reflect.DeepEqual(args, []any{int64(18)}) {
		t.Errorf("got\n%s\n%#v", sql, args)
	}
}

func TestCompileCount(t *testing.T) {
	scoped := usersList
	scoped.Scope = "u.deleted_at IS NULL"
	cases := []struct {
		name   string
		def    easylist.List
		req    easylist.Request
		filter string
		sql    string
		args   []any
	}{
		{
			name: "only filter joins",
			def:  usersList,
			req: easylist.Request{
				Fields: []string{"country"}, Sort: []easylist.Sort{{Field: "last_order"}},
				Limit: intp(10), Offset: intp(20),
			},
			filter: "country = 'FR'",
			sql:    `SELECT COUNT(*) FROM users AS u LEFT JOIN countries c ON c.id = u.country_id WHERE (c.name) = $1`,
			args:   []any{"FR"},
		},
		{
			name:   "no filter joins",
			def:    scoped,
			req:    easylist.Request{Fields: []string{"country"}, Sort: []easylist.Sort{{Field: "last_order"}}},
			filter: "name = 'a'",
			sql:    `SELECT COUNT(*) FROM users AS u WHERE ((u.deleted_at IS NULL) AND u."name" = $1)`,
			args:   []any{"a"},
		},
		{
			name: "no filter",
			def:  usersList,
			sql:  `SELECT COUNT(*) FROM users AS u`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sql, args := compile(t, tc.def, easylist.KindCount, tc.req, tc.filter)
			if sql != tc.sql || len(args) != len(tc.args) || (len(args) > 0 && !reflect.DeepEqual(args, tc.args)) {
				t.Errorf("got\n%s\n%#v\nwant\n%s\n%#v", sql, args, tc.sql, tc.args)
			}
		})
	}
}

func TestCompileErrors(t *testing.T) {
	_, _, err := compileErr(t, usersList, easylist.KindPage, easylist.Request{}, "name > 'a'")
	if err == nil {
		t.Fatal("expected an operator error")
	}
	if _, _, err := New().Compile(&easylist.Plan{Kind: easylist.KindPage, Table: "t", Alias: "t"}); err == nil {
		t.Fatal("expected an error for a plan without fields")
	}
}

func TestCompileIsDeterministic(t *testing.T) {
	p := plan(t, usersList, easylist.KindPage, easylist.Request{Fields: []string{"name", "country", "last_order"}}, "name = 'a' OR age BETWEEN '1,2'")
	sql1, args1, err1 := New().Compile(p)
	sql2, args2, err2 := New().Compile(p)
	if err1 != nil || err2 != nil || sql1 != sql2 || !reflect.DeepEqual(args1, args2) {
		t.Fatalf("results differ: %q %v / %q %v", sql1, err1, sql2, err2)
	}
}

func TestCompileConcurrently(t *testing.T) {
	d := New()
	var plans []*easylist.Plan
	var want []string
	for i := range 20 {
		p := plan(t, usersList, easylist.KindPage, easylist.Request{Fields: []string{"name", "country"}, Limit: intp(i + 1)}, fmt.Sprintf("age > %d OR name = 'n%d'", i, i))
		sql, args, err := d.Compile(p)
		if err != nil {
			t.Fatal(err)
		}
		plans = append(plans, p)
		want = append(want, fmt.Sprint(sql, args))
	}
	var wg sync.WaitGroup
	errs := make(chan string, 200)
	for g := range 200 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			i := g % len(plans)
			sql, args, err := d.Compile(plans[i])
			if got := fmt.Sprint(sql, args); err != nil || got != want[i] {
				errs <- fmt.Sprintf("plan %d: %s %v", i, got, err)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}
