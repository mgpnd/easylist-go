// Package conformance is the shared test suite of easylist's PostgreSQL
// drivers. A driver module calls Run from one of its tests. The offline
// checks always run: arguments and their Go types, error texts, determinism,
// and placeholder numbering, all compared with sqlpg.Lower. When
// EASYLIST_PG_DSN is set, Run also creates a fixture schema of its own in
// that database, runs every case's page and count queries, and compares the
// matching rows. Only tests import this package, so a driver's compiled-in
// packages stay free of a PostgreSQL wire driver.
package conformance

import (
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	filter "github.com/mgpnd/easylist-go/easylist-filter"
	"github.com/mgpnd/easylist-go/easylist-sqlpg"
)

// DSNEnv names the environment variable holding the connection string of
// the PostgreSQL database for the live cases, such as
// postgres://postgres@127.0.0.1:5432/postgres?sslmode=disable.
const DSNEnv = "EASYLIST_PG_DSN"

// Run checks d against the PostgreSQL driver contract. Each case is a
// subtest named after it, with an "offline" subtest and, unless the case
// expects an error, a "live" subtest. The live subtests are skipped when
// EASYLIST_PG_DSN is unset; when it is set and the database cannot be
// reached, Run fails.
func Run(t *testing.T, d easylist.Driver) {
	t.Helper()
	db := openLive(t)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Run("offline", func(t *testing.T) { offline(t, d, c) })
			if c.err != nil {
				return
			}
			t.Run("live", func(t *testing.T) {
				if db == nil {
					// .github/workflows/ci.yml fails the run when it finds this message.
					t.Skip(DSNEnv + " is not set")
				}
				db.check(t, d, c)
			})
		})
	}
	t.Run("concurrent compile", func(t *testing.T) { concurrent(t, d) })
}

// kinds are the plan kinds a case is compiled as offline. Export takes no
// limit or offset.
func (c testCase) kinds() []easylist.Kind {
	if c.kind == easylist.KindExport {
		return []easylist.Kind{easylist.KindExport, easylist.KindCount}
	}
	if c.limit != nil || c.offset != nil {
		return []easylist.Kind{easylist.KindPage, easylist.KindCount}
	}
	return []easylist.Kind{easylist.KindPage, easylist.KindCount, easylist.KindExport}
}

// plan plans c as kind at its reference time with a Monday week start.
func (c testCase) plan(t *testing.T, kind easylist.Kind) *easylist.Plan {
	t.Helper()
	def := users
	if c.scope != "" {
		def.Scope = c.scope
	}
	reg, err := easylist.NewRegistry(def)
	if err != nil {
		t.Fatalf("conformance fixture: %v", err)
	}
	l, _ := reg.List(def.Name)
	req := easylist.Request{Fields: c.fields, Sort: c.sort, Now: c.now, WeekStart: &monday}
	if req.Now.IsZero() {
		req.Now = refUTC
	}
	if kind != easylist.KindExport {
		req.Limit, req.Offset = c.limit, c.offset
	}
	if c.filter != "" {
		if req.Filter, err = filter.Parse(c.filter); err != nil {
			t.Fatalf("conformance case %q: %v", c.name, err)
		}
	}
	p, err := l.Plan(kind, req)
	if err != nil {
		t.Fatalf("conformance case %q: %v", c.name, err)
	}
	return p
}

var kindNames = map[easylist.Kind]string{easylist.KindPage: "page", easylist.KindCount: "count", easylist.KindExport: "export"}

func offline(t *testing.T, d easylist.Driver, c testCase) {
	for _, kind := range c.kinds() {
		p := c.plan(t, kind)
		q, lowerErr := sqlpg.Lower(p)
		sql, args, err := d.Compile(p)
		sql2, args2, err2 := d.Compile(p)
		if sql != sql2 || fmt.Sprintf("%#v", args) != fmt.Sprintf("%#v", args2) || fmt.Sprint(err) != fmt.Sprint(err2) {
			t.Errorf("%T: %s query differs between two compiles:\n%s %#v %v\n%s %#v %v", d, kindNames[kind], sql, args, err, sql2, args2, err2)
		}

		if c.err != nil {
			if lowerErr == nil {
				t.Fatalf("conformance case %q: sqlpg.Lower accepts the plan", c.name)
			}
			for _, want := range c.err {
				if !strings.Contains(lowerErr.Error(), want) {
					t.Fatalf("conformance case %q: sqlpg.Lower error %q does not contain %s", c.name, lowerErr, want)
				}
			}
			if err == nil {
				t.Errorf("%T: %s query compiles, want error %q", d, kindNames[kind], lowerErr)
			} else if err.Error() != lowerErr.Error() {
				t.Errorf("%T: %s query error\n%s\nwant\n%s", d, kindNames[kind], err, lowerErr)
			}
			continue
		}
		if lowerErr != nil {
			t.Fatalf("conformance case %q: sqlpg.Lower: %v", c.name, lowerErr)
		}
		if err != nil {
			t.Errorf("%T: %s query: %v", d, kindNames[kind], err)
			continue
		}

		want := q.Args()
		if !sameArgs(args, want, true) {
			t.Errorf("%T: %s query arguments\n%s\nwant\n%s", d, kindNames[kind], describe(args), describe(want))
		}
		if kind == easylist.KindPage && c.args != nil && !sameArgs(want, c.args, false) {
			t.Fatalf("conformance case %q: sqlpg.Lower arguments\n%s\nwant\n%s", c.name, describe(want), describe(c.args))
		}
		if msg := checkPlaceholders(sql, casts(q)); msg != "" {
			t.Errorf("%T: %s query %s:\n%s", d, kindNames[kind], msg, sql)
		}
		if strings.HasSuffix(strings.TrimSpace(sql), ";") {
			t.Errorf("%T: %s query ends with a semicolon: %s", d, kindNames[kind], sql)
		}
		lower := strings.ToLower(sql)
		for _, fn := range []string{"now(", "current_date", "current_time", "localtime", "clock_timestamp", "statement_timestamp", "transaction_timestamp", "time zone"} {
			if strings.Contains(lower, fn) {
				t.Errorf("%T: %s query reads the database clock with %s: %s", d, kindNames[kind], fn, sql)
			}
		}
	}
}

// casts returns the cast of each argument of q in Args order.
func casts(q *sqlpg.Query) []string {
	var out []string
	if q.Where != nil {
		for _, slot := range q.Where.Fragment().Slots {
			if a, ok := slot.(sqlpg.Arg); ok {
				out = append(out, a.Cast)
			}
		}
	}
	if q.Page != nil {
		out = append(out, "", "")
	}
	return out
}

var placeholder = regexp.MustCompile(`\$([0-9]+)`)

// checkPlaceholders describes what is wrong with the $n placeholders of sql
// for arguments with the given casts, or returns "". Every number from 1 to
// len(casts) must be used and no other. A ::numeric cast must follow the
// placeholders of numeric arguments, and no cast any other placeholder.
func checkPlaceholders(sql string, casts []string) string {
	used := map[int]bool{}
	for _, m := range placeholder.FindAllStringSubmatchIndex(sql, -1) {
		n, err := strconv.Atoi(sql[m[2]:m[3]])
		if err != nil || n < 1 || n > len(casts) {
			return fmt.Sprintf("has placeholder %s for %d arguments", sql[m[0]:m[1]], len(casts))
		}
		used[n] = true
		rest := sql[m[1]:]
		switch want := casts[n-1]; {
		case want != "" && !strings.HasPrefix(rest, "::"+want):
			return fmt.Sprintf("does not cast placeholder $%d to %s", n, want)
		case want == "" && strings.HasPrefix(rest, "::"):
			return fmt.Sprintf("casts placeholder $%d", n)
		}
	}
	for n := 1; n <= len(casts); n++ {
		if !used[n] {
			return fmt.Sprintf("does not use placeholder $%d of %d arguments", n, len(casts))
		}
	}
	return ""
}

// sameArgs compares argument lists value by value, with the same Go types.
// Times compare as instants; with sameZone they must also have the same
// offset, as when a driver passes on sqlpg.Lower's values.
func sameArgs(got, want []any, sameZone bool) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if reflect.TypeOf(got[i]) != reflect.TypeOf(want[i]) {
			return false
		}
		if w, ok := want[i].(time.Time); ok {
			g := got[i].(time.Time)
			if !g.Equal(w) || (sameZone && g.Format(time.RFC3339Nano) != w.Format(time.RFC3339Nano)) {
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

// describe lists arguments with their Go types.
func describe(args []any) string {
	parts := make([]string, len(args))
	for i, a := range args {
		if tm, ok := a.(time.Time); ok {
			parts[i] = "time.Time(" + tm.Format(time.RFC3339Nano) + ")"
			continue
		}
		parts[i] = fmt.Sprintf("%T(%#v)", a, a)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// concurrent compiles every valid case's page plan from several goroutines
// at once and expects the result of compiling it alone.
func concurrent(t *testing.T, d easylist.Driver) {
	var plans []*easylist.Plan
	var want []string
	for _, c := range cases {
		if c.err != nil {
			continue
		}
		kind := c.kinds()[0]
		p := c.plan(t, kind)
		sql, args, err := d.Compile(p)
		plans = append(plans, p)
		want = append(want, fmt.Sprintf("%s %#v %v", sql, args, err))
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var failed []string
	for g := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range plans {
				i = (i + g*7) % len(plans)
				sql, args, err := d.Compile(plans[i])
				if got := fmt.Sprintf("%s %#v %v", sql, args, err); got != want[i] {
					mu.Lock()
					failed = append(failed, got)
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	slices.Sort(failed)
	for _, f := range slices.Compact(failed) {
		t.Errorf("%T: concurrent compile differs from compiling alone: %s", d, f)
	}
}
