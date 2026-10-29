package sqlgorm

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	filter "github.com/mgpnd/easylist-go/easylist-filter"
)

var usersList = easylist.List{
	Name: "users", Table: "users", Alias: "u", PrimaryKey: "id",
	Fields: []easylist.Field{
		{Name: "id", Type: easylist.Integer},
		{Name: "name", Type: easylist.Text},
		{Name: "age", Type: easylist.Integer, Nullable: true},
		{Name: "status", Type: easylist.Text, Nullable: true},
		{Name: "country", Type: easylist.Text, Nullable: true, SQL: "c.name", Joins: []string{"country"}},
		{Name: "last_order", Type: easylist.Timestamp, Nullable: true, SQL: "lo.created_at", Joins: []string{"last_order"}},
	},
	Joins: []easylist.Join{
		{Key: "country", SQL: "LEFT JOIN countries c ON c.id = u.country_id"},
		{Key: "last_order", SQL: "LEFT JOIN LATERAL (SELECT created_at FROM orders o WHERE o.user_id = u.id ORDER BY created_at DESC LIMIT 1) lo ON TRUE"},
	},
}

// arraysList has a base-column array, an enum array, and an author-SQL array.
var arraysList = easylist.List{
	Name: "runs", Table: "runs", Alias: "u", PrimaryKey: "id",
	Fields: []easylist.Field{
		{Name: "id", Type: easylist.Integer},
		{Name: "tags", Type: easylist.Text, Array: true, Nullable: true},
		{Name: "statuses", Type: easylist.Enum, Array: true, Values: []string{"ok", "passed", "skipped", "failed"}},
		{Name: "labels", Type: easylist.Text, Array: true, SQL: "l.names", Joins: []string{"labels"}},
	},
	Joins: []easylist.Join{
		{Key: "labels", SQL: "LEFT JOIN LATERAL (SELECT array_agg(name ORDER BY name) AS names FROM labels WHERE run_id = u.id) l ON TRUE"},
	},
}

// typesList has one scalar field per type.
var typesList = easylist.List{
	Name: "t", Table: "t", Alias: "t", PrimaryKey: "id",
	Fields: []easylist.Field{
		{Name: "id", Type: easylist.Integer},
		{Name: "uid", Type: easylist.UUID},
		{Name: "name", Type: easylist.Text},
		{Name: "score", Type: easylist.Float},
		{Name: "price", Type: easylist.Numeric},
		{Name: "day", Type: easylist.Date},
		{Name: "at", Type: easylist.Timestamp},
		{Name: "active", Type: easylist.Boolean},
		{Name: "status", Type: easylist.Enum, Values: []string{"active", "blocked"}},
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

// placeholderPattern matches one $n placeholder.
var placeholderPattern = regexp.MustCompile(`\$(\d+)`)

// placeholders fails unless sql uses exactly $1 through $n, each once, so
// each argument is bound as one value.
func placeholders(t *testing.T, sql string, n int) {
	t.Helper()
	seen := map[string]int{}
	for _, m := range placeholderPattern.FindAllStringSubmatch(sql, -1) {
		seen[m[1]]++
	}
	for i := 1; i <= n; i++ {
		if seen[strconv.Itoa(i)] != 1 {
			t.Errorf("$%d occurs %d times: %s", i, seen[strconv.Itoa(i)], sql)
		}
	}
	if len(seen) != n {
		t.Errorf("want placeholders $1 to $%d: %s", n, sql)
	}
}

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
			sql:    `SELECT u."id" AS "id", u."name" AS "name" FROM users AS u WHERE u."name" = $1 AND u."age" < $2 ORDER BY u."id" ASC LIMIT $3 OFFSET $4`,
			args:   []any{"Ann", int64(30), int64(10), int64(0)},
		},
		{
			name: "limit and offset",
			def:  usersList, req: easylist.Request{Limit: intp(50), Offset: intp(100)},
			sql:  `SELECT u."id" AS "id" FROM users AS u ORDER BY u."id" ASC LIMIT $1 OFFSET $2`,
			args: []any{int64(50), int64(100)},
		},
		{
			name: "offset zero is bound",
			def:  usersList, req: easylist.Request{Limit: intp(50), Offset: intp(0)},
			filter: "age > 1",
			sql:    `SELECT u."id" AS "id" FROM users AS u WHERE u."age" > $1 ORDER BY u."id" ASC LIMIT $2 OFFSET $3`,
			args:   []any{int64(1), int64(50), int64(0)},
		},
		{
			name: "scope ANDed with an OR filter",
			def:  scoped, req: easylist.Request{},
			filter: "name = 'a' OR name = 'b'",
			sql:    `SELECT u."id" AS "id" FROM users AS u WHERE (u.deleted_at IS NULL) AND (u."name" = $1 OR u."name" = $2) ORDER BY u."id" ASC LIMIT $3 OFFSET $4`,
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
			sql:    `SELECT u."id" AS "id" FROM users AS u WHERE (u."name" = $1 OR u."name" = $2) AND u."age" > $3 ORDER BY u."id" ASC LIMIT $4 OFFSET $5`,
			args:   []any{"a", "b", int64(1), int64(100), int64(0)},
		},
		{
			name: "nested AND keeps its parentheses",
			def:  usersList, req: easylist.Request{},
			filter: "name = 'a' OR (age > 1 AND age < 9)",
			sql:    `SELECT u."id" AS "id" FROM users AS u WHERE (u."name" = $1 OR (u."age" > $2 AND u."age" < $3)) ORDER BY u."id" ASC LIMIT $4 OFFSET $5`,
			args:   []any{"a", int64(1), int64(9), int64(100), int64(0)},
		},
		{
			name: "fragments with AND inside are not wrapped again",
			def:  usersList, req: easylist.Request{Now: time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)},
			filter: "age BETWEEN '1,2' AND (last_order != TODAY OR name = 'x')",
			sql:    `SELECT u."id" AS "id" FROM users AS u LEFT JOIN LATERAL (SELECT created_at FROM orders o WHERE o.user_id = u.id ORDER BY created_at DESC LIMIT 1) lo ON TRUE WHERE u."age" BETWEEN $1 AND $2 AND (COALESCE(NOT ((lo.created_at) >= $3 AND (lo.created_at) < $4), TRUE) OR u."name" = $5) ORDER BY u."id" ASC LIMIT $6 OFFSET $7`,
			args: []any{
				int64(1), int64(2),
				time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC),
				"x", int64(100), int64(0),
			},
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
			sql:    `SELECT u."id" AS "id" FROM users AS u LEFT JOIN countries c ON c.id = u.country_id WHERE (c.name) IS DISTINCT FROM $1 ORDER BY u."id" ASC LIMIT $2 OFFSET $3`,
			args:   []any{"FR", int64(100), int64(0)},
		},
		{
			name: "NULL comparisons",
			def:  usersList, req: easylist.Request{Limit: intp(10), Offset: intp(0)},
			filter: "country = NULL AND name != NULL",
			sql:    `SELECT u."id" AS "id" FROM users AS u LEFT JOIN countries c ON c.id = u.country_id WHERE (c.name) IS NULL AND u."name" IS NOT NULL ORDER BY u."id" ASC LIMIT $1 OFFSET $2`,
			args:   []any{int64(10), int64(0)},
		},
		{
			name: "EXCLUDES with NULL test",
			def:  usersList, req: easylist.Request{},
			filter: "country = NULL OR country EXCLUDES 'fr'",
			sql:    `SELECT u."id" AS "id" FROM users AS u LEFT JOIN countries c ON c.id = u.country_id WHERE ((c.name) IS NULL OR COALESCE((c.name) NOT ILIKE $1 ESCAPE E'\\', TRUE)) ORDER BY u."id" ASC LIMIT $2 OFFSET $3`,
			args:   []any{"%fr%", int64(100), int64(0)},
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

// gorm runs a ? that directly follows "(" through processValue, which
// expands slices, instead of binding it. No slot value is a slice, so each
// IN item is still exactly one placeholder, and a column slot after "(" is
// written verbatim.
func TestCompileInLists(t *testing.T) {
	cases := []struct {
		name   string
		def    easylist.List
		filter string
		where  string
		args   []any
	}{
		{"IN argument order", usersList, "status IN ('a', NULL, 'b') AND age < 30",
			`COALESCE(u."status" IN ($1, $2), TRUE) AND u."age" < $3`, []any{"a", "b", int64(30)}},
		{"IN without NULL", usersList, "status IN ('a', 'b')",
			`u."status" IN ($1, $2)`, []any{"a", "b"}},
		{"one item", usersList, "age IN (7)",
			`u."age" IN ($1)`, []any{int64(7)}},
		{"IN with only NULL", usersList, "status IN (NULL) AND name NOT IN (NULL)",
			`u."status" IS NULL AND u."name" IS NOT NULL`, nil},
		{"NOT IN on a joined field", usersList, "country NOT IN ('FR', 'DE')",
			`COALESCE((c.name) NOT IN ($1, $2), TRUE)`, []any{"FR", "DE"}},
		{"NOT IN with NULL", usersList, "status NOT IN ('a', NULL)",
			`COALESCE(u."status" NOT IN ($1), FALSE)`, []any{"a"}},
		{"numeric items", typesList, "price IN (1.5, 2.50)",
			`t."price" IN ($1::numeric, $2::numeric)`, []any{"1.5", "2.50"}},
		{"date items", typesList, "day IN ('2000-01-02', '2000-01-03')",
			`t."day" IN ($1, $2)`, []any{time.Date(2000, 1, 2, 0, 0, 0, 0, time.UTC), time.Date(2000, 1, 3, 0, 0, 0, 0, time.UTC)}},
		{"enum items without a cast", typesList, "status IN ('active', 'blocked')",
			`t."status" IN ($1, $2)`, []any{"active", "blocked"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sql, args := compile(t, tc.def, easylist.KindCount, easylist.Request{}, tc.filter)
			_, where, _ := strings.Cut(sql, " WHERE ")
			if tc.args == nil {
				tc.args = []any{}
			}
			if where != tc.where || !reflect.DeepEqual(args, tc.args) {
				t.Errorf("got\n%s\n%#v\nwant\n%s\n%#v", where, args, tc.where, tc.args)
			}
			placeholders(t, sql, len(tc.args))
		})
	}
}

// Every argument type binds as one $n with its Go type unchanged, and no
// placeholder other than a numeric one is cast.
func TestCompileArgumentTypes(t *testing.T) {
	at := time.Date(2026, 1, 1, 12, 30, 0, 0, time.UTC)
	sql, args := compile(t, typesList, easylist.KindPage, easylist.Request{Limit: intp(10), Offset: intp(20)},
		"uid = '7f1c0d2e-0000-4000-8000-000000000000' AND name = 'Ann' AND id = 3 AND score > 1.5 AND price > 9.50 AND day = '2000-01-02' AND at < '2026-01-01T12:30:00Z' AND active = TRUE AND status != 'blocked'")
	want := []any{
		"7f1c0d2e-0000-4000-8000-000000000000", "Ann", int64(3), 1.5, "9.50",
		time.Date(2000, 1, 2, 0, 0, 0, 0, time.UTC), at, true, "blocked",
		int64(10), int64(20),
	}
	if len(args) != len(want) {
		t.Fatalf("args = %#v", args)
	}
	for i := range want {
		if reflect.TypeOf(args[i]) != reflect.TypeOf(want[i]) || !reflect.DeepEqual(args[i], want[i]) {
			t.Errorf("arg %d = %#v, want %#v", i+1, args[i], want[i])
		}
	}
	placeholders(t, sql, len(want))
	if strings.Count(sql, "::") != 1 || !strings.Contains(sql, "$5::numeric") {
		t.Errorf("casts: %s", sql)
	}
}

// Author SQL is never scanned: ?, $, #, and : in the FROM text, a join, the
// scope, a selected and filtered field, and an ORDER BY entry are written
// verbatim, and only request values are bound.
func TestCompileAuthorSQLWithPlaceholders(t *testing.T) {
	def := easylist.List{
		Name: "users", Table: "users", Alias: "u", PrimaryKey: "id",
		Scope: "u.meta ? 'vip' AND u.meta ?| array['a', 'b']",
		Fields: []easylist.Field{
			{Name: "id", Type: easylist.Integer},
			{Name: "code", Type: easylist.Text, SQL: `regexp_replace(u.code, '^#\$', '')`},
			{Name: "tag", Type: easylist.Text, Nullable: true, SQL: "(t.meta->>'k?')", Joins: []string{"tags"}},
			{Name: "flag", Type: easylist.Boolean, SQL: "u.meta ? 'x:y'"},
		},
		Joins: []easylist.Join{{Key: "tags", SQL: "LEFT JOIN tags t ON (t.meta ? 'k') AND t.user_id = u.id"}},
	}
	sql, args := compile(t, def, easylist.KindPage, easylist.Request{
		Fields: []string{"code", "tag", "flag"},
		Sort:   []easylist.Sort{{Field: "tag"}},
		Limit:  intp(10), Offset: intp(0),
	}, "code = 'x' AND (tag EXCLUDES 'q' OR flag = TRUE)")
	want := `SELECT u."id" AS "id", regexp_replace(u.code, '^#\$', '') AS "code", (t.meta->>'k?') AS "tag", u.meta ? 'x:y' AS "flag" ` +
		`FROM users AS u LEFT JOIN tags t ON (t.meta ? 'k') AND t.user_id = u.id ` +
		`WHERE (u.meta ? 'vip' AND u.meta ?| array['a', 'b']) AND ((regexp_replace(u.code, '^#\$', '')) = $1 AND (COALESCE(((t.meta->>'k?')) NOT ILIKE $2 ESCAPE E'\\', TRUE) OR (u.meta ? 'x:y') = $3)) ` +
		`ORDER BY (t.meta->>'k?') ASC, u."id" ASC LIMIT $4 OFFSET $5`
	if sql != want || !reflect.DeepEqual(args, []any{"x", "%q%", true, int64(10), int64(0)}) {
		t.Errorf("got\n%s\n%#v\nwant\n%s", sql, args, want)
	}
}

func TestCompileArrays(t *testing.T) {
	cases := []struct {
		name   string
		kind   easylist.Kind
		req    easylist.Request
		filter string
		sql    string
		args   []any
	}{
		{
			name: "base column array",
			kind: easylist.KindPage, req: easylist.Request{Fields: []string{"tags"}},
			sql:  `SELECT u."id" AS "id", to_json(u."tags")::text AS "tags" FROM runs AS u ORDER BY u."id" ASC LIMIT $1 OFFSET $2`,
			args: []any{int64(100), int64(0)},
		},
		{
			name: "author SQL array",
			kind: easylist.KindExport, req: easylist.Request{Fields: []string{"labels"}},
			sql:  `SELECT u."id" AS "id", to_json(l.names)::text AS "labels" FROM runs AS u LEFT JOIN LATERAL (SELECT array_agg(name ORDER BY name) AS names FROM labels WHERE run_id = u.id) l ON TRUE ORDER BY u."id" ASC`,
			args: []any{},
		},
		{
			name: "CONTAINS list arguments",
			kind: easylist.KindPage, req: easylist.Request{Fields: []string{"statuses"}, Limit: intp(10), Offset: intp(0)},
			filter: "statuses CONTAINS 'ok,passed'",
			sql:    `SELECT u."id" AS "id", to_json(u."statuses")::text AS "statuses" FROM runs AS u WHERE ($1 = ANY(u."statuses") AND $2 = ANY(u."statuses")) ORDER BY u."id" ASC LIMIT $3 OFFSET $4`,
			args:   []any{"ok", "passed", int64(10), int64(0)},
		},
		{
			name: "EXCLUDES and NULL test",
			kind: easylist.KindPage, req: easylist.Request{},
			filter: "tags EXCLUDES 'go' AND labels != NULL",
			sql:    `SELECT u."id" AS "id" FROM runs AS u LEFT JOIN LATERAL (SELECT array_agg(name ORDER BY name) AS names FROM labels WHERE run_id = u.id) l ON TRUE WHERE COALESCE(NOT ($1 = ANY(u."tags")), TRUE) AND (l.names) IS NOT NULL ORDER BY u."id" ASC LIMIT $2 OFFSET $3`,
			args:   []any{"go", int64(100), int64(0)},
		},
		{
			name: "EXCLUDES list inside OR",
			kind: easylist.KindPage, req: easylist.Request{},
			filter: "statuses EXCLUDES 'skipped,failed' OR tags CONTAINS 'a'",
			sql:    `SELECT u."id" AS "id" FROM runs AS u WHERE (COALESCE(NOT ($1 = ANY(u."statuses") OR $2 = ANY(u."statuses")), TRUE) OR ($3 = ANY(u."tags"))) ORDER BY u."id" ASC LIMIT $4 OFFSET $5`,
			args:   []any{"skipped", "failed", "a", int64(100), int64(0)},
		},
		{
			name: "count with array filter",
			kind: easylist.KindCount, req: easylist.Request{Fields: []string{"tags"}},
			filter: "statuses EXCLUDES 'skipped,failed'",
			sql:    `SELECT COUNT(*) FROM runs AS u WHERE COALESCE(NOT ($1 = ANY(u."statuses") OR $2 = ANY(u."statuses")), TRUE)`,
			args:   []any{"skipped", "failed"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sql, args := compile(t, arraysList, tc.kind, tc.req, tc.filter)
			if sql != tc.sql || !reflect.DeepEqual(args, tc.args) {
				t.Errorf("got\n%s\n%#v\nwant\n%s\n%#v", sql, args, tc.sql, tc.args)
			}
		})
	}
}

func TestCompileExport(t *testing.T) {
	sql, args := compile(t, usersList, easylist.KindExport, easylist.Request{Fields: []string{"name", "country"}}, "age >= 18")
	want := `SELECT u."id" AS "id", u."name" AS "name", c.name AS "country" FROM users AS u LEFT JOIN countries c ON c.id = u.country_id WHERE u."age" >= $1 ORDER BY u."id" ASC`
	if sql != want || !reflect.DeepEqual(args, []any{int64(18)}) {
		t.Errorf("got\n%s\n%#v", sql, args)
	}
}

// Count queries have no ORDER BY, LIMIT, or OFFSET; gorm adds no ordering,
// primary key, or soft-delete condition of its own.
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
			name:   "scope and filter",
			def:    scoped,
			req:    easylist.Request{Fields: []string{"country"}, Sort: []easylist.Sort{{Field: "last_order"}}},
			filter: "name = 'a'",
			sql:    `SELECT COUNT(*) FROM users AS u WHERE (u.deleted_at IS NULL) AND u."name" = $1`,
			args:   []any{"a"},
		},
		{
			name: "no filter",
			def:  usersList,
			sql:  `SELECT COUNT(*) FROM users AS u`,
			args: []any{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sql, args := compile(t, tc.def, easylist.KindCount, tc.req, tc.filter)
			if sql != tc.sql || !reflect.DeepEqual(args, tc.args) {
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

// Compilations share the package's gorm handle but never a statement, which
// -race checks.
func TestCompileConcurrently(t *testing.T) {
	d := New()
	var plans []*easylist.Plan
	var want []string
	for i := range 20 {
		req := easylist.Request{Fields: []string{"name", "country"}, Limit: intp(i + 1)}
		kind := []easylist.Kind{easylist.KindPage, easylist.KindExport, easylist.KindCount}[i%3]
		if kind == easylist.KindExport {
			req.Limit = nil
		}
		p := plan(t, usersList, kind, req, fmt.Sprintf("age > %d OR name = 'n%d' OR status IN ('s%d', 'x')", i, i, i))
		sql, args, err := d.Compile(p)
		if err != nil {
			t.Fatal(err)
		}
		plans = append(plans, p)
		want = append(want, fmt.Sprintf("%s %#v", sql, args))
	}
	var wg sync.WaitGroup
	errs := make(chan string, 400)
	for g := range 400 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			i := g % len(plans)
			sql, args, err := d.Compile(plans[i])
			if got := fmt.Sprintf("%s %#v", sql, args); err != nil || got != want[i] {
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
