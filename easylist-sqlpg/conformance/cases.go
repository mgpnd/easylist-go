package conformance

import (
	"slices"
	"time"
	_ "time/tzdata" // Europe/Berlin without a system tz database

	easylist "github.com/mgpnd/easylist-go/easylist-core"
)

// testCase is one conformance case over the users fixture.
type testCase struct {
	name   string
	filter string
	scope  string        // Replaces the list's scope when set.
	kind   easylist.Kind // KindPage, the zero value, or KindExport.
	fields []string
	sort   []easylist.Sort
	limit  *int
	offset *int
	now    time.Time // Zero means refUTC.
	// args are the page query's expected arguments, for cases whose scenario
	// names them. Every case also compares the driver's arguments with
	// sqlpg.Lower's.
	args []any
	// ids are every matching id in plan order, before limit and offset. The
	// page query must return ids[offset:offset+limit], and the count query
	// len(ids).
	ids []int64
	// values are the expected selected values of the page rows, by field
	// name. nil stands for SQL NULL.
	values map[string][]any
	// err lists substrings of the expected error. The driver's error text
	// must also equal sqlpg.Lower's.
	err []string
}

var (
	monday = time.Monday
	// refUTC is Tuesday 2026-10-27 10:00 UTC, the reference time of every case
	// that names none.
	refUTC = time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	refP2  = time.Date(2026, 10, 27, 10, 0, 0, 0, time.FixedZone("", 2*3600))
	berlin = mustLoad("Europe/Berlin")
	// Europe/Berlin leaves summer time on Sunday 2026-10-25, a 25-hour day
	// from 2026-10-24T22:00Z to 2026-10-25T23:00Z.
	refDST = time.Date(2026, 10, 25, 12, 0, 0, 0, berlin)
)

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

func intp(n int) *int { return &n }

func ids(n ...int64) []int64 { return n }

// allBut returns ids 1 to 17 without the given ones.
func allBut(n ...int64) []int64 {
	var out []int64
	for id := int64(1); id <= 17; id++ {
		if !slices.Contains(n, id) {
			out = append(out, id)
		}
	}
	return out
}

func utcDate(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// cases cover every contract-sql-driver and contract-sql-postgres scenario
// that arguments, errors, or matching rows can show.
var cases = []testCase{
	// Query shape and pagination.
	{name: "page without filter", ids: allBut()},
	{name: "limit and offset", limit: intp(3), offset: intp(2), ids: allBut(), args: []any{int64(3), int64(2)}},
	{name: "offset zero is bound", limit: intp(2), offset: intp(0), ids: allBut(), args: []any{int64(2), int64(0)}},
	{name: "offset past the end", limit: intp(5), offset: intp(17), ids: allBut(), args: []any{int64(5), int64(17)}},
	{
		// PostgreSQL sorts NULL first when descending; id breaks ties.
		name: "sort descending with tiebreaker",
		sort: []easylist.Sort{{Field: "age", Direction: easylist.Desc}},
		ids:  ids(7, 8, 10, 11, 12, 13, 14, 15, 16, 17, 5, 4, 2, 3, 9, 6, 1),
	},
	{name: "export", kind: easylist.KindExport, filter: "active = TRUE", ids: ids(1, 4, 6, 9), args: []any{true}},
	{
		name: "base and joined fields", filter: "id IN (1, 4)", fields: []string{"name", "country"},
		ids:    ids(1, 4),
		values: map[string][]any{"name": {"Ann", "b"}, "country": {"FR", nil}},
	},

	// Placeholders and argument types.
	{
		name: "placeholder order", filter: "name = 'Ann' AND age < 30", limit: intp(10), offset: intp(0),
		ids: ids(1), args: []any{"Ann", int64(30), int64(10), int64(0)},
	},
	{
		name: "same arguments", filter: "price > 9.50 AND due = '2000-01-02' AND active = TRUE", limit: intp(10), offset: intp(20),
		ids: ids(6), args: []any{"9.50", utcDate(2000, 1, 2), true, int64(10), int64(20)},
	},
	{
		name: "filter literal with SQL syntax", filter: "name = 'x''; DROP TABLE users; --'",
		ids: ids(13), args: []any{"x'; DROP TABLE users; --", int64(100), int64(0)},
	},
	{
		name: "numeric literal keeps exact digits", filter: "price = 12345678901234567890.10",
		ids: ids(2), args: []any{"12345678901234567890.10", int64(100), int64(0)},
	},
	{name: "numeric IN", filter: "price IN (2.5, 1.01)", ids: ids(3, 4, 5), args: []any{"2.5", "1.01", int64(100), int64(0)}},
	{name: "boolean equality", filter: "active = TRUE", ids: ids(1, 4, 6, 9), args: []any{true, int64(100), int64(0)}},
	{name: "float", filter: "score IN (1.5, 3)", ids: ids(1, 5), args: []any{1.5, float64(3), int64(100), int64(0)}},
	{
		name: "text and uuid without casts", filter: "name = 'Ann' AND uid = '7F1C0D2E-0000-4000-8000-000000000000'",
		ids: ids(1), args: []any{"Ann", "7f1c0d2e-0000-4000-8000-000000000000", int64(100), int64(0)},
	},

	// Inequality and NULL.
	{name: "not equal matches NULL", filter: "active != TRUE", ids: ids(2, 3, 5, 7, 8, 10, 11, 12, 13, 14, 15, 16, 17), args: []any{true, int64(100), int64(0)}},
	{name: "not equal on a joined field", filter: "country != 'FR'", ids: allBut(1, 5)},
	{name: "keeping the old behaviour", filter: "country != 'FR' AND country != NULL", ids: ids(2, 3, 7)},
	{name: "equal to NULL", filter: "country = NULL", ids: ids(4, 6, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17)},
	{name: "not equal to NULL", filter: "country != NULL", ids: ids(1, 2, 3, 5, 7)},
	{name: "NULL comparison on a non-nullable field", filter: "name = NULL", ids: ids(8)},
	{name: "NULL comparison on a boolean", filter: "active = NULL", ids: ids(3, 7, 8, 10, 11, 12, 13, 14, 15, 16, 17)},
	{
		name: "NULL comparisons bind nothing", filter: "country = NULL AND name != NULL", limit: intp(10), offset: intp(0),
		ids: ids(4, 6, 9, 10, 11, 12, 13, 14, 15, 16, 17), args: []any{int64(10), int64(0)},
	},

	// Text CONTAINS and EXCLUDES.
	{name: "CONTAINS escaping", filter: "name CONTAINS '50%_off'", ids: ids(5, 16), args: []any{`%50\%\_off%`, int64(100), int64(0)}},
	{name: "EXCLUDES escaping and NULL", filter: "name EXCLUDES '50%_off'", ids: allBut(5, 16), args: []any{`%50\%\_off%`, int64(100), int64(0)}},
	{name: "CONTAINS backslash", filter: `name CONTAINS '\'`, ids: ids(7), args: []any{`%\\%`, int64(100), int64(0)}},
	{name: "text CONTAINS does not split lists", filter: "name CONTAINS 'a,b'", ids: nil},
	{name: "EXCLUDES on a joined field", filter: "country EXCLUDES 'fr'", ids: allBut(1, 2, 5)},

	// BETWEEN and grouping.
	{name: "BETWEEN timestamps", filter: "created_at BETWEEN '2026-01-01T00:00:00Z, 2026-02-01T00:00:00Z'", ids: ids(13, 14)},
	{name: "BETWEEN quoted bounds", filter: "age BETWEEN '30,40'", ids: ids(2, 3, 4, 9)},
	{name: "BETWEEN list bounds", filter: "age BETWEEN (30, 40)", ids: ids(2, 3, 4, 9), args: []any{int64(30), int64(40), int64(100), int64(0)}},
	{name: "BETWEEN date list bounds", filter: "due BETWEEN ('2026-01-01', '2026-02-01')", ids: ids(3, 4)},
	{name: "grouped OR under AND", filter: "(name = 'a' OR name = 'b') AND active = TRUE", ids: ids(4)},

	// Scope and author SQL.
	{name: "OR filter without scope", filter: "name = 'a' OR name = 'b'", ids: ids(3, 4)},
	{name: "OR filter under scope", scope: "u.deleted_at IS NULL", filter: "name = 'a' OR name = 'b'", ids: ids(3), args: []any{"a", "b", int64(100), int64(0)}},
	{name: "scope without filter", scope: "u.deleted_at IS NULL", ids: allBut(2, 4)},
	{name: "jsonb question-mark operator in scope", scope: "u.meta ? 'vip'", filter: "name = 'Ann'", ids: ids(1), args: []any{"Ann", int64(100), int64(0)}},
	{
		name: "dollar and hash in a field expression", filter: "code = 'x'", fields: []string{"code"},
		ids: ids(1), args: []any{"x", int64(100), int64(0)}, values: map[string][]any{"code": {"x"}},
	},
	{
		name: "question mark and colon in a field expression", filter: "label CONTAINS 'vip:'", fields: []string{"label"},
		ids: ids(1, 3), values: map[string][]any{"label": {"vip:Ann", "vip:a"}},
	},
	{
		name: "question mark in a join", filter: "vip = TRUE", fields: []string{"tier"},
		ids: ids(1, 3), args: []any{true, int64(100), int64(0)}, values: map[string][]any{"tier": {"gold", nil}},
	},

	// IN and NOT IN.
	{name: "IN any listed value", filter: "status IN ('success', 'pending')", ids: ids(1, 2, 5, 6)},
	{name: "IN with a NULL item", filter: "status IN ('success', NULL)", ids: allBut(2, 3, 6), args: []any{"success", int64(100), int64(0)}},
	{name: "IN only NULL", filter: "status IN (NULL)", ids: allBut(1, 2, 3, 5, 6), args: []any{int64(100), int64(0)}},
	{name: "NOT IN keeps NULL rows", filter: "status NOT IN ('error')", ids: allBut(3)},
	{name: "NOT IN with a NULL item", filter: "status NOT IN ('error', NULL)", ids: ids(1, 2, 5, 6)},
	{name: "NOT IN only NULL", filter: "status NOT IN (NULL)", ids: ids(1, 2, 3, 5, 6), args: []any{int64(100), int64(0)}},
	{name: "IN duplicate items", filter: "status IN ('success', 'success')", ids: ids(1, 5)},
	{
		name: "IN argument order", filter: "name IN ('a', NULL, 'b') AND age < 30", limit: intp(10), offset: intp(0),
		ids: nil, args: []any{"a", "b", int64(30), int64(10), int64(0)},
	},
	{name: "IN on a boolean", filter: "active IN (TRUE)", ids: ids(1, 4, 6, 9)},
	{name: "NOT IN on a uuid", filter: "uid NOT IN ('7f1c0d2e-0000-4000-8000-000000000000')", ids: allBut(1)},
	{name: "NOT IN on an integer", filter: "age NOT IN (25, 30)", ids: allBut(1, 3, 9)},
	{name: "IN on a date with NULL", filter: "due IN ('2026-01-01', NULL)", ids: ids(3, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17)},
	{name: "IN on a timestamp", filter: "created_at IN ('2026-01-01T00:00:00Z', '2026-02-01T01:00:00+01:00')", ids: ids(13, 14)},

	// Arrays.
	{name: "array CONTAINS all values", filter: "statuses CONTAINS 'ok,passed'", ids: ids(1), args: []any{"ok", "passed", int64(100), int64(0)}},
	{name: "array CONTAINS list value", filter: "statuses CONTAINS ('ok', 'passed')", ids: ids(1)},
	{name: "enum array without a cast", filter: "statuses CONTAINS 'ok'", ids: ids(1, 2, 3)},
	{name: "array EXCLUDES any value", filter: "statuses EXCLUDES 'skipped,failed'", ids: allBut(3, 4)},
	{name: "array exact element match", filter: "tags CONTAINS 'go'", ids: ids(1, 7)},
	{name: "array EXCLUDES with empty and NULL arrays", filter: "tags EXCLUDES 'go'", ids: allBut(1, 7)},
	{name: "array NULL test", filter: "tags = NULL", ids: ids(4, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17)},
	{name: "array not NULL test", filter: "tags != NULL", ids: ids(1, 2, 3, 5, 6, 7)},
	{name: "comma always separates array items", filter: "tags CONTAINS 'a,b'", ids: ids(5), args: []any{"a", "b", int64(100), int64(0)}},
	{name: "list value keeps commas", filter: "tags CONTAINS ('a,b', ' c')", ids: ids(6), args: []any{"a,b", " c", int64(100), int64(0)}},
	{name: "quoted list on an integer array", filter: "ids CONTAINS '7, 8'", ids: ids(1), args: []any{int64(7), int64(8), int64(100), int64(0)}},
	{name: "single unquoted array value", filter: "ids CONTAINS 7", ids: ids(1, 2)},
	{name: "array EXCLUDES list", filter: "ids EXCLUDES (7, 8)", ids: allBut(1, 2, 5)},
	{name: "numeric array", filter: "prices CONTAINS 10.5", ids: ids(1), args: []any{"10.5", int64(100), int64(0)}},
	{name: "numeric array exact digits", filter: "prices CONTAINS '12345678901234567890.10'", ids: ids(1)},
	{name: "date array", filter: "days CONTAINS '2026-10-26'", ids: ids(1)},
	{name: "timestamp array with offset", filter: "seen CONTAINS '2026-10-27T10:00:00+02:00'", ids: ids(2)},
	{
		name: "selected arrays", filter: "id IN (1, 2, 4)",
		fields: []string{"tags", "ids", "statuses", "prices", "days", "seen", "local_seen"},
		ids:    ids(1, 2, 4),
		values: map[string][]any{
			"tags":       {`["go","rust"]`, `["golang","Go"]`, nil},
			"ids":        {`[7,8]`, `[7]`, nil},
			"statuses":   {`["ok","passed","retried"]`, `["ok"]`, `["skipped","failed"]`},
			"prices":     {`[10.50,12345678901234567890.10]`, `[]`, nil},
			"days":       {`["2026-10-25","2026-10-26"]`, nil, nil},
			"seen":       {`["2026-10-27T10:00:00+00:00"]`, `["2026-10-27T08:00:00+00:00"]`, nil},
			"local_seen": {`["2026-10-27T10:00:00"]`, nil, nil},
		},
	},

	// Relative values.
	{name: "relative date field", filter: "due > {TODAY + 5::DAYS}", now: refP2, ids: ids(1), args: []any{utcDate(2026, 11, 1), int64(100), int64(0)}},
	{name: "date field TODAY", filter: "due = TODAY", now: time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC), ids: ids(2)},
	{name: "recompiling moves the window", filter: "due = TODAY", now: time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC), ids: ids(1)},
	{
		name: "microsecond truncation", filter: "created_at > {NOW - 30::MINUTES}", now: time.Date(2026, 10, 27, 10, 0, 0, 123456700, time.UTC),
		ids: ids(4, 5, 6, 7, 8), args: []any{time.Date(2026, 10, 27, 9, 30, 0, 123456000, time.UTC), int64(100), int64(0)},
	},
	{name: "whole last day included", filter: "created_at <= END_OF_WEEK", ids: allBut(8, 17), args: []any{utcDate(2026, 11, 2), int64(100), int64(0)}},
	{
		name: "equal to a day", filter: "created_at = TODAY", now: refP2, ids: ids(2, 3, 4, 5),
		args: []any{time.Date(2026, 10, 26, 22, 0, 0, 0, time.UTC), time.Date(2026, 10, 27, 22, 0, 0, 0, time.UTC), int64(100), int64(0)},
	},
	{name: "not equal to a day matches NULL", filter: "created_at != TODAY", now: refP2, ids: allBut(2, 3, 4, 5)},
	{name: "whole-day upper bound", filter: "created_at <= TODAY", now: refP2, ids: allBut(6, 7, 8, 17), args: []any{time.Date(2026, 10, 27, 22, 0, 0, 0, time.UTC), int64(100), int64(0)}},
	{name: "whole day across DST", filter: "created_at = TODAY", now: refDST, ids: ids(10, 11)},
	{name: "not whole day across DST", filter: "created_at != TODAY", now: refDST, ids: allBut(10, 11)},
	{name: "after a DST day", filter: "created_at > TODAY", now: refDST, ids: ids(1, 2, 3, 4, 5, 6, 7, 8, 12)},
	{name: "evaluated number", filter: "age > {2 * 3}", ids: ids(1, 2, 3, 4, 5, 6, 9), args: []any{int64(6), int64(100), int64(0)}},
	{name: "numeric result", filter: "price > {10 / 4}", ids: ids(1, 2, 6, 7), args: []any{"2.5", int64(100), int64(0)}},

	// Errors.
	{name: "unsupported operator", filter: "active > TRUE", err: []string{`">"`, `"active"`, "position 8"}},
	{name: "EXCLUDES on a non-text field", filter: "age EXCLUDES 3", err: []string{`"EXCLUDES"`, `"age"`, "position 5"}},
	{name: "enum value not declared", filter: "status = 'archived'", err: []string{`"archived"`, "position 10"}},
	{name: "timestamp without offset", filter: "created_at > '2026-10-25T10:00:00'", err: []string{"offset", "position 14"}},
	{name: "invalid uuid", filter: "uid = 'not-a-uuid'", err: []string{`"not-a-uuid"`, "position 7"}},
	{name: "evaluated number not an integer", filter: "age > {10 / 4}", err: []string{`"2.5"`, "position 7"}},
	{name: "equality with a value on an array", filter: "tags = 'go'", err: []string{`"="`, `"tags"`, "position 6"}},
	{name: "ordering operator on an array", filter: "ids > 3", err: []string{`">"`, `"ids"`, "position 5"}},
	{name: "IN on an array", filter: "tags IN ('go')", err: []string{`"IN"`, `"tags"`, "position 6"}},
	{name: "empty array item", filter: "tags CONTAINS 'a,,b'", err: []string{"empty item", "position 15"}},
	{name: "invalid array item", filter: "ids CONTAINS '7,x'", err: []string{`"x"`, "position 14"}},
	{name: "undeclared enum array item", filter: "statuses EXCLUDES 'skipped,archived'", err: []string{`"archived"`, "position 19"}},
	{name: "undeclared enum list item", filter: "status IN ('success', 'archived')", err: []string{`"archived"`, "position 23"}},
	{name: "quoted number list item", filter: "age BETWEEN ('30', '40')", err: []string{`"30"`, "position 14"}},
	{name: "invalid array list item", filter: "ids CONTAINS (7, 'x')", err: []string{`"x"`, "position 18"}},
	{name: "list on a text field", filter: "name CONTAINS ('a', 'b')", err: []string{`"name"`, "position 15"}},
}
