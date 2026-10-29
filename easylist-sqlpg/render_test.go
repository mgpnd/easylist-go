package sqlpg_test

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	filter "github.com/mgpnd/easylist-go/easylist-filter"
	"github.com/mgpnd/easylist-go/easylist-sqlpg/internal/refdriver"
)

// The golden SQL below is what easylist-sql-goqu produced before the lowering
// moved to this package. The reference driver renders only Fragment, so these
// tests pin every node's canonical template.

// clause compiles a count plan of predicateList and returns its WHERE clause.
func clause(t *testing.T, input string, now time.Time) (string, []any) {
	t.Helper()
	sql, args := compile(t, predicateList, easylist.KindCount, easylist.Request{Now: now}, input)
	const prefix = "SELECT COUNT(*) FROM t AS t WHERE "
	if !strings.HasPrefix(sql, prefix) {
		t.Fatalf("unexpected count query %q", sql)
	}
	return strings.TrimPrefix(sql, prefix), args
}

var placeholderCast = regexp.MustCompile(`\$[0-9]+::`)

// onlyNumericCasts fails when a placeholder in sql has a cast other than
// ::numeric, or when a ::numeric placeholder binds a non-string.
func onlyNumericCasts(t *testing.T, sql string) {
	t.Helper()
	for _, loc := range placeholderCast.FindAllStringIndex(sql, -1) {
		if !strings.HasPrefix(sql[loc[1]:], "numeric") {
			t.Errorf("placeholder with a cast other than numeric: %s", sql)
		}
	}
}

func TestRenderPage(t *testing.T) {
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
			name: "offset zero is bound",
			def:  usersList, req: easylist.Request{Limit: intp(50), Offset: intp(0)},
			sql:  `SELECT u."id" AS "id" FROM users AS u ORDER BY u."id" ASC LIMIT $1 OFFSET $2`,
			args: []any{int64(50), int64(0)},
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
			sql:    `SELECT u."id" AS "id" FROM users AS u LEFT JOIN countries c ON c.id = u.country_id WHERE (c.name) IS DISTINCT FROM $1 ORDER BY u."id" ASC LIMIT $2 OFFSET $3`,
			args:   []any{"FR", int64(100), int64(0)},
		},
		{
			name: "NULL comparisons",
			def:  usersList, req: easylist.Request{Limit: intp(10), Offset: intp(0)},
			filter: "country = NULL AND name != NULL",
			sql:    `SELECT u."id" AS "id" FROM users AS u LEFT JOIN countries c ON c.id = u.country_id WHERE ((c.name) IS NULL AND u."name" IS NOT NULL) ORDER BY u."id" ASC LIMIT $1 OFFSET $2`,
			args:   []any{int64(10), int64(0)},
		},
		{
			name: "EXCLUDES with NULL test",
			def:  usersList, req: easylist.Request{},
			filter: "country = NULL OR country EXCLUDES 'fr'",
			sql:    `SELECT u."id" AS "id" FROM users AS u LEFT JOIN countries c ON c.id = u.country_id WHERE ((c.name) IS NULL OR COALESCE((c.name) NOT ILIKE $1 ESCAPE E'\\', TRUE)) ORDER BY u."id" ASC LIMIT $2 OFFSET $3`,
			args:   []any{"%fr%", int64(100), int64(0)},
		},
		{
			name: "IN argument order",
			def:  usersList, req: easylist.Request{Limit: intp(10), Offset: intp(0)},
			filter: "status IN ('a', NULL, 'b') AND age < 30",
			sql:    `SELECT u."id" AS "id" FROM users AS u WHERE (COALESCE(u."status" IN ($1, $2), TRUE) AND u."age" < $3) ORDER BY u."id" ASC LIMIT $4 OFFSET $5`,
			args:   []any{"a", "b", int64(30), int64(10), int64(0)},
		},
		{
			name: "IN with only NULL",
			def:  usersList, req: easylist.Request{Limit: intp(10), Offset: intp(0)},
			filter: "status IN (NULL) AND name NOT IN (NULL)",
			sql:    `SELECT u."id" AS "id" FROM users AS u WHERE (u."status" IS NULL AND u."name" IS NOT NULL) ORDER BY u."id" ASC LIMIT $1 OFFSET $2`,
			args:   []any{int64(10), int64(0)},
		},
		{
			name: "NOT IN on a joined field",
			def:  usersList, req: easylist.Request{},
			filter: "country NOT IN ('FR', 'DE')",
			sql:    `SELECT u."id" AS "id" FROM users AS u LEFT JOIN countries c ON c.id = u.country_id WHERE COALESCE((c.name) NOT IN ($1, $2), TRUE) ORDER BY u."id" ASC LIMIT $3 OFFSET $4`,
			args:   []any{"FR", "DE", int64(100), int64(0)},
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

func TestRenderArrays(t *testing.T) {
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
			name: "CONTAINS list value",
			kind: easylist.KindPage, req: easylist.Request{Fields: []string{"statuses"}, Limit: intp(10), Offset: intp(0)},
			filter: "statuses CONTAINS ('ok', 'passed')",
			sql:    `SELECT u."id" AS "id", to_json(u."statuses")::text AS "statuses" FROM runs AS u WHERE ($1 = ANY(u."statuses") AND $2 = ANY(u."statuses")) ORDER BY u."id" ASC LIMIT $3 OFFSET $4`,
			args:   []any{"ok", "passed", int64(10), int64(0)},
		},
		{
			name: "EXCLUDES and NULL test",
			kind: easylist.KindPage, req: easylist.Request{},
			filter: "tags EXCLUDES 'go' AND labels != NULL",
			sql:    `SELECT u."id" AS "id" FROM runs AS u LEFT JOIN LATERAL (SELECT array_agg(name ORDER BY name) AS names FROM labels WHERE run_id = u.id) l ON TRUE WHERE (COALESCE(NOT ($1 = ANY(u."tags")), TRUE) AND (l.names) IS NOT NULL) ORDER BY u."id" ASC LIMIT $2 OFFSET $3`,
			args:   []any{"go", int64(100), int64(0)},
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

// IN binds enum items as plain arguments, so PostgreSQL infers the enum type
// from the column and the SQL names no type.
func TestRenderEnumIn(t *testing.T) {
	def := easylist.List{
		Name: "accounts", Table: "accounts", Alias: "a", PrimaryKey: "id",
		Fields: []easylist.Field{
			{Name: "id", Type: easylist.Integer},
			{Name: "status", Type: easylist.Enum, Values: []string{"active", "blocked"}},
		},
	}
	sql, args := compile(t, def, easylist.KindCount, easylist.Request{}, "status IN ('active', 'blocked')")
	want := `SELECT COUNT(*) FROM accounts AS a WHERE a."status" IN ($1, $2)`
	if sql != want || !reflect.DeepEqual(args, []any{"active", "blocked"}) {
		t.Errorf("got\n%s\n%#v\nwant\n%s", sql, args, want)
	}
	if strings.Contains(sql, "::") {
		t.Errorf("enum list has a cast: %s", sql)
	}
}

func TestRenderExportAndCount(t *testing.T) {
	scoped := usersList
	scoped.Scope = "u.deleted_at IS NULL"
	cases := []struct {
		name   string
		def    easylist.List
		kind   easylist.Kind
		req    easylist.Request
		filter string
		sql    string
		args   []any
	}{
		{
			name: "export", def: usersList, kind: easylist.KindExport,
			req: easylist.Request{Fields: []string{"name"}}, filter: "age >= 18",
			sql:  `SELECT u."id" AS "id", u."name" AS "name" FROM users AS u WHERE u."age" >= $1 ORDER BY u."id" ASC`,
			args: []any{int64(18)},
		},
		{
			name: "count with only filter joins", def: usersList, kind: easylist.KindCount,
			req: easylist.Request{
				Fields: []string{"country"}, Sort: []easylist.Sort{{Field: "last_order"}},
				Limit: intp(10), Offset: intp(20),
			},
			filter: "country = 'FR'",
			sql:    `SELECT COUNT(*) FROM users AS u LEFT JOIN countries c ON c.id = u.country_id WHERE (c.name) = $1`,
			args:   []any{"FR"},
		},
		{
			name: "count with scope", def: scoped, kind: easylist.KindCount,
			req:    easylist.Request{Fields: []string{"country"}, Sort: []easylist.Sort{{Field: "last_order"}}},
			filter: "name = 'a'",
			sql:    `SELECT COUNT(*) FROM users AS u WHERE ((u.deleted_at IS NULL) AND u."name" = $1)`,
			args:   []any{"a"},
		},
		{
			name: "count without filter", def: usersList, kind: easylist.KindCount,
			sql:  `SELECT COUNT(*) FROM users AS u`,
			args: []any{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sql, args := compile(t, tc.def, tc.kind, tc.req, tc.filter)
			if sql != tc.sql || !reflect.DeepEqual(args, tc.args) {
				t.Errorf("got\n%s\n%#v\nwant\n%s\n%#v", sql, args, tc.sql, tc.args)
			}
		})
	}
}

// Author SQL is written verbatim and binds nothing, even when it holds
// characters that query libraries treat as placeholders or named arguments.
func TestRenderAuthorSQLWithPlaceholderCharacters(t *testing.T) {
	def := easylist.List{
		Name: "users", Table: "users", Alias: "u", PrimaryKey: "id",
		Scope: "u.meta ? 'vip'",
		Fields: []easylist.Field{
			{Name: "id", Type: easylist.Integer},
			{Name: "name", Type: easylist.Text},
			{Name: "code", Type: easylist.Text, SQL: `regexp_replace(u.code, '^#\$', '')`},
			{Name: "tier", Type: easylist.Text, SQL: "m.tier", Joins: []string{"meta"}},
		},
		Joins: []easylist.Join{{Key: "meta", SQL: "LEFT JOIN LATERAL (SELECT u.meta ->> 'tier'::text AS tier, u.meta ?| array['a:b'] AS any) m ON TRUE"}},
	}
	sql, args := compile(t, def, easylist.KindPage, easylist.Request{Fields: []string{"code", "tier"}, Limit: intp(10), Offset: intp(0)}, "name = 'Ann' AND code = 'x' AND tier != ':#1'")
	want := `SELECT u."id" AS "id", regexp_replace(u.code, '^#\$', '') AS "code", m.tier AS "tier" FROM users AS u LEFT JOIN LATERAL (SELECT u.meta ->> 'tier'::text AS tier, u.meta ?| array['a:b'] AS any) m ON TRUE ` +
		`WHERE ((u.meta ? 'vip') AND ((u."name" = $1 AND (regexp_replace(u.code, '^#\$', '')) = $2) AND (m.tier) IS DISTINCT FROM $3)) ORDER BY u."id" ASC LIMIT $4 OFFSET $5`
	if sql != want || !reflect.DeepEqual(args, []any{"Ann", "x", ":#1", int64(10), int64(0)}) {
		t.Errorf("got\n%s\n%#v\nwant\n%s", sql, args, want)
	}
}

func TestRenderTypesAndOperators(t *testing.T) {
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
		{"age BETWEEN (40, 30)", `t."age" BETWEEN $1 AND $2`, []any{int64(40), int64(30)}},
		{"price BETWEEN (1.01, 2.02)", `t."price" BETWEEN $1::numeric AND $2::numeric`, []any{"1.01", "2.02"}},
		{"day BETWEEN ('2024-02-29', '2025-01-01')", `t."day" BETWEEN $1 AND $2`, []any{day, end}},
		{"active IN (TRUE)", `t."active" IN ($1)`, []any{true}},
		{"uid NOT IN ('7f1c0d2e-0000-4000-8000-000000000000')", `COALESCE(t."uid" NOT IN ($1), TRUE)`, []any{"7f1c0d2e-0000-4000-8000-000000000000"}},
		{"status IN ('active', NULL)", `COALESCE(t."status" IN ($1), TRUE)`, []any{"active"}},
		{"status NOT IN ('blocked', NULL, 'blocked')", `COALESCE(t."status" NOT IN ($1, $2), FALSE)`, []any{"blocked", "blocked"}},
		{"name CONTAINS 'a%_\\b'", `t."name" ILIKE $1 ESCAPE E'\\'`, []any{`%a\%\_\\b%`}},
		{"name EXCLUDES '50%_off'", `COALESCE(t."name" NOT ILIKE $1 ESCAPE E'\\', TRUE)`, []any{`%50\%\_off%`}},
		{"name = 'x''; DROP TABLE users; --'", `t."name" = $1`, []any{"x'; DROP TABLE users; --"}},
	}
	for _, tc := range cases {
		t.Run(tc.filter, func(t *testing.T) {
			sql, args := clause(t, tc.filter, time.Time{})
			if sql != tc.clause || !reflect.DeepEqual(args, tc.args) {
				t.Fatalf("got %q %#v; want %q %#v", sql, args, tc.clause, tc.args)
			}
			onlyNumericCasts(t, sql)
		})
	}
}

// NULL comparisons compile for every type and bind no argument.
func TestRenderNull(t *testing.T) {
	for _, field := range []string{"uid", "name", "age", "score", "price", "day", "at", "active", "status", "tags"} {
		for _, tc := range []struct{ op, clause string }{{"=", "IS NULL"}, {"!=", "IS NOT NULL"}} {
			input := field + " " + tc.op + " NULL"
			t.Run(input, func(t *testing.T) {
				sql, args := clause(t, input, time.Time{})
				if want := `t."` + field + `" ` + tc.clause; sql != want || len(args) != 0 {
					t.Fatalf("got %q %#v; want %q and no args", sql, args, want)
				}
			})
		}
	}
	sql, args := clause(t, "full = null", time.Time{})
	if sql != `(t.first || ' ' || t.last) IS NULL` || len(args) != 0 {
		t.Fatalf("author SQL: %q %#v", sql, args)
	}
}

func TestRenderArrayMembership(t *testing.T) {
	day := time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		filter, clause string
		args           []any
	}{
		{"tags CONTAINS 'go'", `($1 = ANY(t."tags"))`, []any{"go"}},
		{"tags CONTAINS ' Go , rust '", `($1 = ANY(t."tags") AND $2 = ANY(t."tags"))`, []any{"Go", "rust"}},
		{"tags EXCLUDES 'go'", `COALESCE(NOT ($1 = ANY(t."tags")), TRUE)`, []any{"go"}},
		{"ids CONTAINS '7, 8'", `($1 = ANY(t."ids") AND $2 = ANY(t."ids"))`, []any{int64(7), int64(8)}},
		{"ids CONTAINS 7", `($1 = ANY(t."ids"))`, []any{int64(7)}},
		{"scores CONTAINS '1.5,-2'", `($1 = ANY(t."scores") AND $2 = ANY(t."scores"))`, []any{1.5, float64(-2)}},
		{"prices CONTAINS '10.50, 12345678901234567890.1'", `($1::numeric = ANY(t."prices") AND $2::numeric = ANY(t."prices"))`, []any{"10.50", "12345678901234567890.1"}},
		{"prices EXCLUDES 1", `COALESCE(NOT ($1::numeric = ANY(t."prices")), TRUE)`, []any{"1"}},
		{"flags CONTAINS 'TRUE, false'", `($1 = ANY(t."flags") AND $2 = ANY(t."flags"))`, []any{true, false}},
		{"days CONTAINS '2024-02-29'", `($1 = ANY(t."days"))`, []any{day}},
		{"seen EXCLUDES '2026-01-01T00:00:00Z'", `COALESCE(NOT ($1 = ANY(t."seen")), TRUE)`, []any{at}},
		{"groups CONTAINS '0B6C4C1E-3D8E-4F43-9D5A-2A8A0D1D2C3B'", `($1 = ANY(t."groups"))`, []any{"0b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b"}},
		{"statuses EXCLUDES 'skipped,failed'", `COALESCE(NOT ($1 = ANY(t."statuses") OR $2 = ANY(t."statuses")), TRUE)`, []any{"skipped", "failed"}},
		{"labels CONTAINS 'x'", `($1 = ANY((l.names)))`, []any{"x"}},
		{"tags CONTAINS ('a,b', ' c')", `($1 = ANY(t."tags") AND $2 = ANY(t."tags"))`, []any{"a,b", " c"}},
		{"prices CONTAINS (10.50, 1)", `($1::numeric = ANY(t."prices") AND $2::numeric = ANY(t."prices"))`, []any{"10.50", "1"}},
		{"statuses EXCLUDES ('skipped', 'failed')", `COALESCE(NOT ($1 = ANY(t."statuses") OR $2 = ANY(t."statuses")), TRUE)`, []any{"skipped", "failed"}},
		{"name CONTAINS 'a,b'", `t."name" ILIKE $1 ESCAPE E'\\'`, []any{"%a,b%"}},
	}
	for _, tc := range cases {
		t.Run(tc.filter, func(t *testing.T) {
			sql, args := clause(t, tc.filter, time.Time{})
			if sql != tc.clause || !reflect.DeepEqual(args, tc.args) {
				t.Fatalf("got %q %#v; want %q args %#v", sql, args, tc.clause, tc.args)
			}
			// Membership compares with the column's elements directly, so no
			// argument or column is cast to a named array type.
			if strings.Contains(sql, "[]") || strings.Contains(sql, "ARRAY") {
				t.Errorf("array predicate casts to an array type: %q", sql)
			}
			onlyNumericCasts(t, sql)
		})
	}
}

// IN and NOT IN compile to four forms on every scalar type. Only non-NULL
// items are bound, in list order, and the field's SQL is emitted once.
func TestRenderIn(t *testing.T) {
	day := time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC)
	types := []struct {
		field, column, items, list string
		args                       []any
	}{
		{"name", `t."name"`, "'a', 'O''Reilly'", "$1, $2", []any{"a", "O'Reilly"}},
		{"full", `(t.first || ' ' || t.last)`, "'Ann Lee'", "$1", []any{"Ann Lee"}},
		{"age", `t."age"`, "30, -1", "$1, $2", []any{int64(30), int64(-1)}},
		{"score", `t."score"`, "1.5, 2", "$1, $2", []any{1.5, float64(2)}},
		{"price", `t."price"`, "1.01, 12345678901234567890.1", "$1::numeric, $2::numeric", []any{"1.01", "12345678901234567890.1"}},
		{"active", `t."active"`, "TRUE, false", "$1, $2", []any{true, false}},
		{"day", `t."day"`, "'2024-02-29'", "$1", []any{day}},
		{"uid", `t."uid"`, "'0B6C4C1E-3D8E-4F43-9D5A-2A8A0D1D2C3B'", "$1", []any{"0b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b"}},
		{"status", `t."status"`, "'active', 'blocked'", "$1, $2", []any{"active", "blocked"}},
	}
	forms := []struct{ op, null, clause string }{
		{"IN", "", "%s IN (%s)"},
		{"IN", "NULL, ", "COALESCE(%s IN (%s), TRUE)"},
		{"NOT IN", "", "COALESCE(%s NOT IN (%s), TRUE)"},
		{"NOT IN", "null, ", "COALESCE(%s NOT IN (%s), FALSE)"},
	}
	for _, typ := range types {
		for _, form := range forms {
			input := typ.field + " " + form.op + " (" + form.null + typ.items + ")"
			t.Run(input, func(t *testing.T) {
				sql, args := clause(t, input, time.Time{})
				want := fmt.Sprintf(form.clause, typ.column, typ.list)
				if sql != want || !reflect.DeepEqual(args, typ.args) || strings.Count(sql, typ.column) != 1 {
					t.Fatalf("got %q %#v; want %q args %#v and the field once", sql, args, want, typ.args)
				}
			})
		}
		for _, tc := range []struct{ input, clause string }{
			{typ.field + " IN (NULL)", "IS NULL"},
			{typ.field + " NOT IN (NULL, null)", "IS NOT NULL"},
		} {
			t.Run(tc.input, func(t *testing.T) {
				sql, args := clause(t, tc.input, time.Time{})
				if want := typ.column + " " + tc.clause; sql != want || len(args) != 0 {
					t.Fatalf("got %q %#v; want %q and no args", sql, args, want)
				}
			})
		}
	}
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

func TestRenderExprValues(t *testing.T) {
	london := londonZone(t)
	cases := []struct {
		name, filter string
		now          time.Time
		clause       string
		args         []any
	}{
		{"date field", "day > {TODAY + 5::DAYS}", nowUTC, `t."day" > $1`, []any{tm(2026, 11, 1, 0, 0, utc)}},
		{"date field in UTC+02:00", "day > {TODAY + 5::DAYS}", nowP2, `t."day" > $1`, []any{tm(2026, 11, 1, 0, 0, utc)}},
		{"date field after midnight in UTC+02:00", "day = TODAY", tm(2026, 10, 27, 23, 30, utc).In(plus2), `t."day" = $1`, []any{tm(2026, 10, 28, 0, 0, utc)}},
		{"date field != TODAY", "day != TODAY", nowUTC, `t."day" IS DISTINCT FROM $1`, []any{tm(2026, 10, 27, 0, 0, utc)}},
		{"timestamp field", "at > {NOW - 30::MINUTES}", nowUTC, `t."at" > $1`, []any{tm(2026, 10, 27, 9, 30, utc)}},
		{"timestamp field !=", "at != {NOW + 1::DAYS}", nowP2, `t."at" IS DISTINCT FROM $1`, []any{tm(2026, 10, 28, 8, 0, utc)}},
		{"whole last day included", "at <= END_OF_WEEK", nowUTC, `t."at" < $1`, []any{tm(2026, 11, 2, 0, 0, utc)}},
		{"> day in UTC+02:00", "at > TODAY", nowP2, `t."at" >= $1`, []any{tm(2026, 10, 27, 22, 0, utc)}},
		{">= day in UTC+02:00", "at >= TODAY", nowP2, `t."at" >= $1`, []any{tm(2026, 10, 26, 22, 0, utc)}},
		{"< day in UTC+02:00", "at < TODAY", nowP2, `t."at" < $1`, []any{tm(2026, 10, 26, 22, 0, utc)}},
		{"<= day in UTC+02:00", "at <= TODAY", nowP2, `t."at" < $1`, []any{tm(2026, 10, 27, 22, 0, utc)}},
		{"= day in UTC+02:00", "at = TODAY", nowP2, `(t."at" >= $1 AND t."at" < $2)`, []any{tm(2026, 10, 26, 22, 0, utc), tm(2026, 10, 27, 22, 0, utc)}},
		{"!= day in UTC+02:00", "at != TODAY", nowP2, `COALESCE(NOT (t."at" >= $1 AND t."at" < $2), TRUE)`, []any{tm(2026, 10, 26, 22, 0, utc), tm(2026, 10, 27, 22, 0, utc)}},
		{"= DST day", "at = TODAY", tm(2026, 10, 25, 12, 0, london), `(t."at" >= $1 AND t."at" < $2)`, []any{tm(2026, 10, 24, 23, 0, utc), tm(2026, 10, 26, 0, 0, utc)}},
		{"> DST day", "at > TODAY", tm(2026, 10, 25, 12, 0, london), `t."at" >= $1`, []any{tm(2026, 10, 26, 0, 0, utc)}},
		{"integer", "age > {2 * 3}", nowUTC, `t."age" > $1`, []any{int64(6)}},
		{"float", "score <= {10 / 4}", nowUTC, `t."score" <= $1`, []any{2.5}},
		{"numeric", "price > {10 / 4}", nowUTC, `t."price" > $1::numeric`, []any{"2.5"}},
		{"numeric exact", "price != {0.1 + 0.2}", nowUTC, `t."price" IS DISTINCT FROM $1::numeric`, []any{"0.3"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sql, args := clause(t, tc.filter, tc.now)
			if sql != tc.clause || !sameArgs(args, tc.args) {
				t.Fatalf("got %q %v; want %q %v", sql, args, tc.clause, tc.args)
			}
			noDBClock(t, sql)
			onlyNumericCasts(t, sql)
		})
	}
}

func TestRenderMicrosecondTruncation(t *testing.T) {
	now := time.Date(2026, 10, 27, 10, 0, 0, 123456700, utc)
	sql, args := clause(t, "at > {NOW - 30::MINUTES}", now)
	want := time.Date(2026, 10, 27, 9, 30, 0, 123456000, utc)
	if sql != `t."at" > $1` || len(args) != 1 || !args[0].(time.Time).Equal(want) {
		t.Fatalf("got %q %v; want argument %v", sql, args, want)
	}
}

func TestRenderExpressions(t *testing.T) {
	// Thursday 2026-10-29 15:00 in UTC+02:00.
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

func TestRenderRecompilingMovesTheWindow(t *testing.T) {
	reg, err := easylist.NewRegistry(predicateList)
	if err != nil {
		t.Fatal(err)
	}
	l, _ := reg.List("t")
	expr, err := filter.Parse("at = TODAY") // parsed once, planned twice
	if err != nil {
		t.Fatal(err)
	}
	want := [2][]any{{tm(2026, 10, 27, 0, 0, utc), tm(2026, 10, 28, 0, 0, utc)}, {tm(2026, 10, 28, 0, 0, utc), tm(2026, 10, 29, 0, 0, utc)}}
	var sqls [2]string
	for i, now := range []time.Time{nowUTC, nowUTC.AddDate(0, 0, 1)} {
		p, err := l.Plan(easylist.KindCount, easylist.Request{Filter: expr, Now: now})
		if err != nil {
			t.Fatal(err)
		}
		sql, args, err := refdriver.Driver{}.Compile(p)
		if err != nil {
			t.Fatal(err)
		}
		if !sameArgs(args, want[i]) {
			t.Errorf("reference %v: args %v, want %v", now, args, want[i])
		}
		sqls[i] = sql
	}
	if sqls[0] != sqls[1] {
		t.Errorf("SQL changed: %q / %q", sqls[0], sqls[1])
	}
}

func TestRenderConcurrently(t *testing.T) {
	d := refdriver.Driver{}
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
