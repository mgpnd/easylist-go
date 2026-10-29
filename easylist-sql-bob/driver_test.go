package sqlbob

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
		{Name: "name", Type: easylist.Text, Nullable: true},
		{Name: "score", Type: easylist.Float},
		{Name: "price", Type: easylist.Numeric},
		{Name: "day", Type: easylist.Date},
		{Name: "at", Type: easylist.Timestamp, Nullable: true},
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

// lines is bob's statement layout: each clause on its own line, the select
// list on the line after SELECT, and a final line break.
func lines(clauses ...string) string {
	return strings.Join(clauses, "\n") + "\n"
}

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
			sql: lines(`SELECT `,
				`u."id" AS "id", u."name" AS "name", c.name AS "country"`,
				`FROM users AS u LEFT JOIN countries c ON c.id = u.country_id`,
				`ORDER BY u."id" ASC`,
				`LIMIT $1`,
				`OFFSET $2`),
			args: []any{int64(100), int64(0)},
		},
		{
			name: "placeholder order",
			def:  usersList, req: easylist.Request{Fields: []string{"name"}, Limit: intp(10), Offset: intp(0)},
			filter: "name = 'Ann' AND age < 30",
			sql: lines(`SELECT `,
				`u."id" AS "id", u."name" AS "name"`,
				`FROM users AS u`,
				`WHERE ((u."name" = $1) AND (u."age" < $2))`,
				`ORDER BY u."id" ASC`,
				`LIMIT $3`,
				`OFFSET $4`),
			args: []any{"Ann", int64(30), int64(10), int64(0)},
		},
		{
			name: "limit and offset",
			def:  usersList, req: easylist.Request{Limit: intp(50), Offset: intp(100)},
			sql:  lines(`SELECT `, `u."id" AS "id"`, `FROM users AS u`, `ORDER BY u."id" ASC`, `LIMIT $1`, `OFFSET $2`),
			args: []any{int64(50), int64(100)},
		},
		{
			name: "offset zero is bound",
			def:  usersList, req: easylist.Request{Limit: intp(50), Offset: intp(0)},
			filter: "age > 1",
			sql:    lines(`SELECT `, `u."id" AS "id"`, `FROM users AS u`, `WHERE (u."age" > $1)`, `ORDER BY u."id" ASC`, `LIMIT $2`, `OFFSET $3`),
			args:   []any{int64(1), int64(50), int64(0)},
		},
		{
			name: "scope ANDed with an OR filter",
			def:  scoped, req: easylist.Request{},
			filter: "name = 'a' OR name = 'b'",
			sql: lines(`SELECT `, `u."id" AS "id"`, `FROM users AS u`,
				`WHERE ((u.deleted_at IS NULL) AND ((u."name" = $1) OR (u."name" = $2)))`,
				`ORDER BY u."id" ASC`, `LIMIT $3`, `OFFSET $4`),
			args: []any{"a", "b", int64(100), int64(0)},
		},
		{
			name: "scope without filter",
			def:  scoped, req: easylist.Request{},
			sql:  lines(`SELECT `, `u."id" AS "id"`, `FROM users AS u`, `WHERE (u.deleted_at IS NULL)`, `ORDER BY u."id" ASC`, `LIMIT $1`, `OFFSET $2`),
			args: []any{int64(100), int64(0)},
		},
		{
			name: "grouping kept",
			def:  usersList, req: easylist.Request{},
			filter: "(name = 'a' OR name = 'b') AND age > 1",
			sql: lines(`SELECT `, `u."id" AS "id"`, `FROM users AS u`,
				`WHERE (((u."name" = $1) OR (u."name" = $2)) AND (u."age" > $3))`,
				`ORDER BY u."id" ASC`, `LIMIT $4`, `OFFSET $5`),
			args: []any{"a", "b", int64(1), int64(100), int64(0)},
		},
		{
			name: "nested AND keeps its parentheses",
			def:  usersList, req: easylist.Request{},
			filter: "name = 'a' OR (age > 1 AND age < 9)",
			sql: lines(`SELECT `, `u."id" AS "id"`, `FROM users AS u`,
				`WHERE ((u."name" = $1) OR ((u."age" > $2) AND (u."age" < $3)))`,
				`ORDER BY u."id" ASC`, `LIMIT $4`, `OFFSET $5`),
			args: []any{"a", int64(1), int64(9), int64(100), int64(0)},
		},
		{
			name: "sort join and tiebreaker",
			def:  usersList, req: easylist.Request{Sort: []easylist.Sort{{Field: "last_order", Direction: easylist.Desc}}},
			sql: lines(`SELECT `, `u."id" AS "id"`,
				`FROM users AS u LEFT JOIN LATERAL (SELECT created_at FROM orders o WHERE o.user_id = u.id ORDER BY created_at DESC LIMIT 1) lo ON TRUE`,
				`ORDER BY lo.created_at DESC, u."id" ASC`, `LIMIT $1`, `OFFSET $2`),
			args: []any{int64(100), int64(0)},
		},
		{
			name: "filter on joined field",
			def:  usersList, req: easylist.Request{},
			filter: "country != 'FR'",
			sql: lines(`SELECT `, `u."id" AS "id"`, `FROM users AS u LEFT JOIN countries c ON c.id = u.country_id`,
				`WHERE ((c.name) IS DISTINCT FROM $1)`, `ORDER BY u."id" ASC`, `LIMIT $2`, `OFFSET $3`),
			args: []any{"FR", int64(100), int64(0)},
		},
		{
			name: "NULL comparisons",
			def:  usersList, req: easylist.Request{Limit: intp(10), Offset: intp(0)},
			filter: "country = NULL AND name != NULL",
			sql: lines(`SELECT `, `u."id" AS "id"`, `FROM users AS u LEFT JOIN countries c ON c.id = u.country_id`,
				`WHERE (((c.name) IS NULL) AND (u."name" IS NOT NULL))`, `ORDER BY u."id" ASC`, `LIMIT $1`, `OFFSET $2`),
			args: []any{int64(10), int64(0)},
		},
		{
			name: "EXCLUDES with NULL test",
			def:  usersList, req: easylist.Request{},
			filter: "country = NULL OR country EXCLUDES 'fr'",
			sql: lines(`SELECT `, `u."id" AS "id"`, `FROM users AS u LEFT JOIN countries c ON c.id = u.country_id`,
				`WHERE (((c.name) IS NULL) OR COALESCE((c.name) NOT ILIKE $1 ESCAPE E'\\', TRUE))`,
				`ORDER BY u."id" ASC`, `LIMIT $2`, `OFFSET $3`),
			args: []any{"%fr%", int64(100), int64(0)},
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

// Every node type, written natively or through its fragment. Native
// operators are parenthesized by bob; fragments are written as they are.
func TestCompileNodes(t *testing.T) {
	now := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	day := func(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }
	scoped := typesList
	scoped.Scope = "t.deleted_at IS NULL OR t.kept"
	cases := []struct {
		name   string
		def    easylist.List
		filter string
		where  string
		args   []any
	}{
		{"scope is a parenthesized raw leaf", scoped, "",
			`(t.deleted_at IS NULL OR t.kept)`, nil},
		{"= native", typesList, "name = 'a'",
			`(t."name" = $1)`, []any{"a"}},
		{"!= native", typesList, "name != 'a'",
			`(t."name" IS DISTINCT FROM $1)`, []any{"a"}},
		{"> >= < <= native", typesList, "id > 1 AND id >= 2 AND score < 3.5 AND day <= '2000-01-02'",
			`((((t."id" > $1) AND (t."id" >= $2)) AND (t."score" < $3)) AND (t."day" <= $4))`,
			[]any{int64(1), int64(2), 3.5, time.Date(2000, 1, 2, 0, 0, 0, 0, time.UTC)}},
		{"boolean native", typesList, "active = TRUE AND active != FALSE",
			`((t."active" = $1) AND (t."active" IS DISTINCT FROM $2))`, []any{true, false}},
		{"numeric comparison is a fragment", typesList, "price = 1.5 AND price != 2",
			`(t."price" = $1::numeric AND t."price" IS DISTINCT FROM $2::numeric)`, []any{"1.5", "2"}},
		{"IS NULL and IS NOT NULL native", typesList, "name = NULL OR at != NULL",
			`((t."name" IS NULL) OR (t."at" IS NOT NULL))`, nil},
		{"IN native", typesList, "status IN ('active', 'blocked')",
			`(t."status" IN ($1, $2))`, []any{"active", "blocked"}},
		{"IN with a NULL item is a fragment", typesList, "name IN ('a', NULL)",
			`COALESCE(t."name" IN ($1), TRUE)`, []any{"a"}},
		{"NOT IN is a fragment", typesList, "name NOT IN ('a', 'b')",
			`COALESCE(t."name" NOT IN ($1, $2), TRUE)`, []any{"a", "b"}},
		{"NOT IN with a NULL item is a fragment", typesList, "name NOT IN ('a', NULL)",
			`COALESCE(t."name" NOT IN ($1), FALSE)`, []any{"a"}},
		{"IN with only NULL", typesList, "name IN (NULL) AND at NOT IN (NULL)",
			`((t."name" IS NULL) AND (t."at" IS NOT NULL))`, nil},
		{"numeric IN is a fragment", typesList, "price IN (1.5, 2.50)",
			`t."price" IN ($1::numeric, $2::numeric)`, []any{"1.5", "2.50"}},
		{"BETWEEN native", typesList, "id BETWEEN '1,2'",
			`(t."id" BETWEEN $1 AND $2)`, []any{int64(1), int64(2)}},
		{"numeric BETWEEN is a fragment", typesList, "price BETWEEN '1,2.5'",
			`t."price" BETWEEN $1::numeric AND $2::numeric`, []any{"1", "2.5"}},
		{"CONTAINS and EXCLUDES are fragments", typesList, "name CONTAINS '50%_off' AND name EXCLUDES 'x'",
			`(t."name" ILIKE $1 ESCAPE E'\\' AND COALESCE(t."name" NOT ILIKE $2 ESCAPE E'\\', TRUE))`,
			[]any{`%50\%\_off%`, "%x%"}},
		{"whole-day range is a fragment", typesList, "at = TODAY",
			`(t."at" >= $1 AND t."at" < $2)`, []any{day(27), day(28)}},
		{"whole-day negation is a fragment", typesList, "at != TODAY",
			`COALESCE(NOT (t."at" >= $1 AND t."at" < $2), TRUE)`, []any{day(27), day(28)}},
		{"array CONTAINS is a fragment", arraysList, "statuses CONTAINS 'ok,passed'",
			`($1 = ANY(u."statuses") AND $2 = ANY(u."statuses"))`, []any{"ok", "passed"}},
		{"array EXCLUDES is a fragment", arraysList, "tags EXCLUDES 'go'",
			`COALESCE(NOT ($1 = ANY(u."tags")), TRUE)`, []any{"go"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sql, args := compile(t, tc.def, easylist.KindCount, easylist.Request{Now: now}, tc.filter)
			where := strings.TrimSuffix(sql[strings.Index(sql, "\nWHERE ")+len("\nWHERE "):], "\n")
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
		Scope: `u.meta ? 'vip' AND u.meta ?| array['a', 'b'] AND u.note <> '\?'`,
		Fields: []easylist.Field{
			{Name: "id", Type: easylist.Integer},
			{Name: "code", Type: easylist.Text, SQL: `regexp_replace(u.code, '^#\$', '')`},
			{Name: "tag", Type: easylist.Text, Nullable: true, SQL: "(t.meta->>'k?')", Joins: []string{"tags"}},
			{Name: "flag", Type: easylist.Boolean, SQL: "u.meta ? 'x:y'"},
		},
		Joins: []easylist.Join{{Key: "tags", SQL: "LEFT JOIN tags t ON (t.meta ? 'k') AND t.user_id = u.id AND t.code <> '$x'"}},
	}
	sql, args := compile(t, def, easylist.KindPage, easylist.Request{
		Fields: []string{"code", "tag", "flag"},
		Sort:   []easylist.Sort{{Field: "tag"}},
		Limit:  intp(10), Offset: intp(0),
	}, "code = 'x' AND (tag EXCLUDES 'q' OR flag = TRUE)")
	want := lines(`SELECT `,
		`u."id" AS "id", regexp_replace(u.code, '^#\$', '') AS "code", (t.meta->>'k?') AS "tag", u.meta ? 'x:y' AS "flag"`,
		`FROM users AS u LEFT JOIN tags t ON (t.meta ? 'k') AND t.user_id = u.id AND t.code <> '$x'`,
		`WHERE ((u.meta ? 'vip' AND u.meta ?| array['a', 'b'] AND u.note <> '\?') AND (((regexp_replace(u.code, '^#\$', '')) = $1) AND (COALESCE(((t.meta->>'k?')) NOT ILIKE $2 ESCAPE E'\\', TRUE) OR ((u.meta ? 'x:y') = $3))))`,
		`ORDER BY (t.meta->>'k?') ASC, u."id" ASC`,
		`LIMIT $4`,
		`OFFSET $5`)
	if sql != want || !reflect.DeepEqual(args, []any{"x", "%q%", true, int64(10), int64(0)}) {
		t.Errorf("got\n%s\n%#v\nwant\n%s", sql, args, want)
	}
}

// A ? inside a join stays verbatim, and the first filter value is $1, in
// page, export, and count queries.
func TestCompileJoinWithQuestionMark(t *testing.T) {
	def := easylist.List{
		Name: "users", Table: "users", Alias: "u", PrimaryKey: "id",
		Fields: []easylist.Field{
			{Name: "id", Type: easylist.Integer},
			{Name: "name", Type: easylist.Text},
			{Name: "beta", Type: easylist.Boolean, Nullable: true, SQL: "p.enabled", Joins: []string{"prefs"}},
		},
		Joins: []easylist.Join{{Key: "prefs", SQL: "LEFT JOIN prefs p ON p.data ? 'beta'"}},
	}
	for _, kind := range []easylist.Kind{easylist.KindPage, easylist.KindExport, easylist.KindCount} {
		sql, args := compile(t, def, kind, easylist.Request{Fields: []string{"beta"}}, "name = 'Ann' AND beta = TRUE")
		if !strings.Contains(sql, "\nFROM users AS u LEFT JOIN prefs p ON p.data ? 'beta'\n") ||
			!strings.Contains(sql, `WHERE ((u."name" = $1) AND ((p.enabled) = $2))`) || args[0] != "Ann" || args[1] != true {
			t.Errorf("kind %v: got\n%s\n%#v", kind, sql, args)
		}
		placeholders(t, sql, len(args))
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
			sql:  lines(`SELECT `, `u."id" AS "id", to_json(u."tags")::text AS "tags"`, `FROM runs AS u`, `ORDER BY u."id" ASC`, `LIMIT $1`, `OFFSET $2`),
			args: []any{int64(100), int64(0)},
		},
		{
			name: "author SQL array",
			kind: easylist.KindExport, req: easylist.Request{Fields: []string{"labels"}},
			sql: lines(`SELECT `, `u."id" AS "id", to_json(l.names)::text AS "labels"`,
				`FROM runs AS u LEFT JOIN LATERAL (SELECT array_agg(name ORDER BY name) AS names FROM labels WHERE run_id = u.id) l ON TRUE`,
				`ORDER BY u."id" ASC`),
			args: []any{},
		},
		{
			name: "CONTAINS list arguments",
			kind: easylist.KindPage, req: easylist.Request{Fields: []string{"statuses"}, Limit: intp(10), Offset: intp(0)},
			filter: "statuses CONTAINS 'ok,passed'",
			sql: lines(`SELECT `, `u."id" AS "id", to_json(u."statuses")::text AS "statuses"`, `FROM runs AS u`,
				`WHERE ($1 = ANY(u."statuses") AND $2 = ANY(u."statuses"))`, `ORDER BY u."id" ASC`, `LIMIT $3`, `OFFSET $4`),
			args: []any{"ok", "passed", int64(10), int64(0)},
		},
		{
			name: "EXCLUDES list inside OR",
			kind: easylist.KindPage, req: easylist.Request{},
			filter: "statuses EXCLUDES 'skipped,failed' OR tags CONTAINS 'a'",
			sql: lines(`SELECT `, `u."id" AS "id"`, `FROM runs AS u`,
				`WHERE (COALESCE(NOT ($1 = ANY(u."statuses") OR $2 = ANY(u."statuses")), TRUE) OR ($3 = ANY(u."tags")))`,
				`ORDER BY u."id" ASC`, `LIMIT $4`, `OFFSET $5`),
			args: []any{"skipped", "failed", "a", int64(100), int64(0)},
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

// Export queries have no LIMIT or OFFSET.
func TestCompileExport(t *testing.T) {
	sql, args := compile(t, usersList, easylist.KindExport, easylist.Request{Fields: []string{"name", "country"}}, "age >= 18")
	want := lines(`SELECT `,
		`u."id" AS "id", u."name" AS "name", c.name AS "country"`,
		`FROM users AS u LEFT JOIN countries c ON c.id = u.country_id`,
		`WHERE (u."age" >= $1)`,
		`ORDER BY u."id" ASC`)
	if sql != want || !reflect.DeepEqual(args, []any{int64(18)}) {
		t.Errorf("got\n%s\n%#v", sql, args)
	}
}

// Count queries select COUNT(*) and have no ORDER BY, LIMIT, or OFFSET.
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
			sql:    lines(`SELECT `, `COUNT(*)`, `FROM users AS u LEFT JOIN countries c ON c.id = u.country_id`, `WHERE ((c.name) = $1)`),
			args:   []any{"FR"},
		},
		{
			name:   "scope and filter",
			def:    scoped,
			req:    easylist.Request{Fields: []string{"country"}, Sort: []easylist.Sort{{Field: "last_order"}}},
			filter: "name = 'a'",
			sql:    lines(`SELECT `, `COUNT(*)`, `FROM users AS u`, `WHERE ((u.deleted_at IS NULL) AND (u."name" = $1))`),
			args:   []any{"a"},
		},
		{
			name: "no filter",
			def:  usersList,
			sql:  lines(`SELECT `, `COUNT(*)`, `FROM users AS u`),
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

// LIMIT and OFFSET are bound after the filter arguments, never written into
// the SQL text.
func TestCompileBoundPagination(t *testing.T) {
	sql, args := compile(t, usersList, easylist.KindPage, easylist.Request{Limit: intp(25), Offset: intp(75)}, "age > 1")
	if want := lines(`SELECT `, `u."id" AS "id"`, `FROM users AS u`, `WHERE (u."age" > $1)`, `ORDER BY u."id" ASC`, `LIMIT $2`, `OFFSET $3`); sql != want {
		t.Errorf("got\n%s\nwant\n%s", sql, want)
	}
	if strings.Contains(sql, "25") || strings.Contains(sql, "75") {
		t.Errorf("limit or offset written into the SQL: %s", sql)
	}
	if n := len(args); n < 2 || args[n-2] != int64(25) || args[n-1] != int64(75) {
		t.Errorf("args = %#v", args)
	}
	placeholders(t, sql, len(args))
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

// Every compilation builds its own bob query and expressions, so
// compilations share no builder, which -race checks.
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
		p := plan(t, usersList, kind, req, fmt.Sprintf("age > %d OR name = 'n%d' OR status IN ('s%d', 'x') OR country EXCLUDES 'c%d'", i, i, i, i))
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
