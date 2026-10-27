package graph

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	filter "github.com/mgpnd/easylist-go/easylist-filter"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
)

// fakeDriver renders a plan as text the fake database can read back:
// "PAGE id,name|country|name DESC,id ASC|100,0|filter".
type fakeDriver struct{}

func (fakeDriver) Compile(p *easylist.Plan) (string, []any, error) {
	var fields, joins, order []string
	for _, f := range p.Fields {
		fields = append(fields, f.Name)
	}
	for _, j := range p.Joins {
		joins = append(joins, j.Key)
	}
	for _, o := range p.Order {
		dir := "ASC"
		if o.Desc {
			dir = "DESC"
		}
		order = append(order, o.Field.Name+" "+dir)
	}
	kind := map[easylist.Kind]string{easylist.KindPage: "PAGE", easylist.KindCount: "COUNT", easylist.KindExport: "EXPORT"}[p.Kind]
	return fmt.Sprintf("%s %s|%s|%s|%d,%d|%v", kind, strings.Join(fields, ","), strings.Join(joins, ","),
		strings.Join(order, ","), p.Limit, p.Offset, p.Filter != nil), nil, nil
}

// fakeDB serves rows keyed by field name in the column order the query asks for.
type fakeDB struct {
	rows    []map[string]any
	total   int64
	queries []string
}

func (db *fakeDB) QueryContext(_ context.Context, query string, _ ...any) (easylist.Rows, error) {
	db.queries = append(db.queries, query)
	if strings.HasPrefix(query, "COUNT") {
		return &fakeRows{data: [][]any{{db.total}}}, nil
	}
	cols := strings.Split(strings.SplitN(strings.Fields(query)[1], "|", 2)[0], ",")
	rows := &fakeRows{}
	for _, r := range db.rows {
		vals := make([]any, len(cols))
		for i, c := range cols {
			vals[i] = r[c]
		}
		rows.data = append(rows.data, vals)
	}
	return rows, nil
}

type fakeRows struct {
	data [][]any
	next int
}

func (r *fakeRows) Next() bool {
	r.next++
	return r.next <= len(r.data)
}

func (r *fakeRows) Scan(dest ...any) error {
	for i, d := range dest {
		switch d := d.(type) {
		case sql.Scanner:
			if err := d.Scan(r.data[r.next-1][i]); err != nil {
				return err
			}
		case *int64:
			*d = r.data[r.next-1][i].(int64)
		}
	}
	return nil
}

func (*fakeRows) Err() error   { return nil }
func (*fakeRows) Close() error { return nil }

type recordingExporter struct {
	got []easylist.ExportRequest
	id  string
	err error
}

func (e *recordingExporter) Export(_ context.Context, req easylist.ExportRequest) (string, error) {
	e.got = append(e.got, req)
	return e.id, e.err
}

var created = time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)

func sampleRows() []map[string]any {
	return []map[string]any{
		{"id": int64(1), "uid": "0b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b", "name": "Ann", "age": int64(30), "score": 1.5,
			"balance": "10.50", "active": true, "born": time.Date(1996, 2, 29, 0, 0, 0, 0, time.UTC),
			"created_at": created, "status": "on_hold", "country": "FR"},
		{"id": int64(2), "uid": "1b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b", "name": "Boom", "age": nil, "score": nil,
			"balance": "12345678901234567890.123", "active": false, "born": nil,
			"created_at": created, "status": "active", "country": nil},
		{"id": int64(3), "uid": "2b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b", "name": "Cy", "age": int64(9223372036854775807), "score": nil,
			"balance": "0", "active": true, "born": nil, "created_at": created, "status": "active", "country": nil},
	}
}

type gqlError struct {
	Message string `json:"message"`
	Path    []any  `json:"path"`
}

type response struct {
	Data   json.RawMessage `json:"data"`
	Errors []gqlError      `json:"errors"`
}

func run(t *testing.T, rt *easylist.Runtime, query string) response {
	t.Helper()
	srv := handler.New(NewExecutableSchema(Config{Resolvers: &Resolver{Runtime: rt}}))
	srv.AddTransport(transport.POST{})
	body, _ := json.Marshal(map[string]string{"query": query})
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	var resp response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode %s: %v", w.Body.String(), err)
	}
	return resp
}

func newRuntime() (*easylist.Runtime, *fakeDB) {
	db := &fakeDB{rows: sampleRows()[:1], total: 42}
	return &easylist.Runtime{Driver: fakeDriver{}, DB: db}, db
}

func TestSelectionDrivesQueries(t *testing.T) {
	cases := []struct {
		name    string
		query   string
		queries []string
		rows    []map[string]any
	}{
		{"fragment", `{ people(query: {}) { items { ...UserBits } } } fragment UserBits on UsersRow { name }`,
			[]string{"PAGE id,name||id ASC|100,0|false"}, nil},
		{"alias", `{ people(query: {}) { items { n: name } } }`,
			[]string{"PAGE id,name||id ASC|100,0|false"}, nil},
		{"inline fragment, typename, and directives", `{ people(query: {}) { items {
			__typename
			... on UsersRow { age }
			country @skip(if: true)
			uid @include(if: false)
			status @include(if: true)
		} } }`, []string{"PAGE id,age,status||id ASC|100,0|false"}, nil},
		{"only totalCount", `{ people(query: {}) { totalCount } }`,
			[]string{"COUNT |||0,0|false"}, nil},
		{"items and totalCount", `{ people(query: {filter: "country = 'FR'"}) { totalCount items { name } } }`,
			[]string{"PAGE id,name|country|id ASC|100,0|true", "COUNT |country||0,0|true"}, nil},
		{"calculated field pulls requirements", `{ people(query: {}) { items { display } } }`,
			[]string{"PAGE id,name,country|country|id ASC|100,0|false"}, nil},
		{"sort, limit, and offset", `{ people(query: {sort: [{field: NAME, direction: DESC}, {field: AGE}], limit: 5, offset: 10}) { items { id } } }`,
			[]string{"PAGE id||name DESC,age ASC,id ASC|5,10|false"}, nil},
		{"second list", `{ orderItems(query: {}) { items { sku } } }`,
			[]string{"PAGE id,sku||id ASC|100,0|false"}, []map[string]any{{"id": int64(1), "sku": "A-1"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, db := newRuntime()
			if tc.rows != nil {
				db.rows = tc.rows
			}
			resp := run(t, rt, tc.query)
			if len(resp.Errors) > 0 {
				t.Fatalf("errors: %+v", resp.Errors)
			}
			if !slices.Equal(db.queries, tc.queries) {
				t.Errorf("queries = %q, want %q", db.queries, tc.queries)
			}
		})
	}
}

func TestValuesAndCalculatedFields(t *testing.T) {
	rt, db := newRuntime()
	db.rows = sampleRows()
	db.rows[1]["name"] = "Bob"
	resp := run(t, rt, `{ people(query: {}) { totalCount items {
		id uid name age score balance active born created_at status country display initials tier
	} } }`)
	if len(resp.Errors) > 0 {
		t.Fatalf("errors: %+v", resp.Errors)
	}
	want := `{"people":{"totalCount":42,"items":[` +
		`{"id":1,"uid":"0b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b","name":"Ann","age":30,"score":1.5,"balance":"10.50","active":true,"born":"1996-02-29","created_at":"2026-10-25T10:00:00Z","status":"ON_HOLD","country":"FR","display":"Ann (FR)","initials":"A","tier":"GOLD"},` +
		`{"id":2,"uid":"1b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b","name":"Bob","age":null,"score":null,"balance":"12345678901234567890.123","active":false,"born":null,"created_at":"2026-10-25T10:00:00Z","status":"ACTIVE","country":null,"display":"Bob","initials":"B","tier":"BASIC"},` +
		`{"id":3,"uid":"2b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b","name":"Cy","age":9223372036854775807,"score":null,"balance":"0","active":true,"born":null,"created_at":"2026-10-25T10:00:00Z","status":"ACTIVE","country":null,"display":"Cy","initials":"C","tier":"GOLD"}` +
		`]}}`
	if string(resp.Data) != want {
		t.Errorf("data =\n%s\nwant\n%s", resp.Data, want)
	}
}

func TestArrayValues(t *testing.T) {
	rt, db := newRuntime()
	db.rows = []map[string]any{
		{"id": int64(1), "tags": `["go","rust"]`, "roles": `["admin","editor"]`},
		{"id": int64(2), "tags": nil, "roles": `[]`},
		{"id": int64(3), "tags": `[]`, "roles": `["editor"]`},
	}
	resp := run(t, rt, `{ people(query: {}) { items { id tags roles labels } } }`)
	if len(resp.Errors) > 0 {
		t.Fatalf("errors: %+v", resp.Errors)
	}
	want := `{"people":{"items":[` +
		`{"id":1,"tags":["go","rust"],"roles":["ADMIN","EDITOR"],"labels":["role:admin","role:editor","tag:go","tag:rust"]},` +
		`{"id":2,"tags":null,"roles":[],"labels":[]},` +
		`{"id":3,"tags":[],"roles":["EDITOR"],"labels":["role:editor"]}` +
		`]}}`
	if string(resp.Data) != want {
		t.Errorf("data =\n%s\nwant\n%s", resp.Data, want)
	}
	if want := []string{"PAGE id,tags,roles||id ASC|100,0|false"}; !slices.Equal(db.queries, want) {
		t.Errorf("queries = %q, want %q", db.queries, want)
	}

	db.rows = []map[string]any{{"id": int64(1), "roles": `["admin","owner"]`}}
	resp = run(t, rt, `{ people(query: {}) { items { roles } } }`)
	if string(resp.Data) != "null" || len(resp.Errors) != 1 || !strings.Contains(resp.Errors[0].Message, `"owner"`) {
		t.Errorf("undeclared enum element: data = %s, errors = %+v", resp.Data, resp.Errors)
	}

	resp = run(t, rt, `{ people(query: {sort: [{field: TAGS}]}) { items { id } } }`)
	if len(resp.Errors) != 1 || !strings.Contains(resp.Errors[0].Message, "TAGS") {
		t.Errorf("sort on an array field: errors = %+v", resp.Errors)
	}
}

func TestCalculatedFieldError(t *testing.T) {
	rt, db := newRuntime()
	db.rows = sampleRows()
	resp := run(t, rt, `{ people(query: {}) { items { name display } } }`)
	if len(resp.Errors) != 1 {
		t.Fatalf("errors = %+v", resp.Errors)
	}
	e := resp.Errors[0]
	if !reflect.DeepEqual(e.Path, []any{"people", "items", float64(1), "display"}) || !strings.Contains(e.Message, "cannot display Boom") {
		t.Errorf("error = %+v", e)
	}
	// display and every parent up to the non-null people field are non-null,
	// so the request fails.
	if string(resp.Data) != "null" {
		t.Errorf("data = %s, want null", resp.Data)
	}
}

func TestUndeclaredEnumFailsRequest(t *testing.T) {
	rt, db := newRuntime()
	db.rows[0]["status"] = "archived"
	resp := run(t, rt, `{ people(query: {}) { items { status } } }`)
	if string(resp.Data) != "null" || len(resp.Errors) != 1 || !strings.Contains(resp.Errors[0].Message, `"archived"`) {
		t.Fatalf("data = %s, errors = %+v", resp.Data, resp.Errors)
	}
}

func TestListErrorsRunNoQuery(t *testing.T) {
	cases := []struct{ name, input, want string }{
		{"filter syntax error", `{filter: "name = "}`, "position 8"},
		{"unknown filter field", `{filter: "nickname = 'a'"}`, `"nickname" at position 1`},
		{"calculated filter field", `{filter: "display = 'a'"}`, `"display" at position 1`},
		{"limit above maximum", `{limit: 20000}`, "maximum page size 10000"},
		{"negative offset", `{offset: -1}`, "offset -1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, db := newRuntime()
			resp := run(t, rt, `{ people(query: `+tc.input+`) { totalCount items { name } } }`)
			if len(resp.Errors) != 1 || !strings.Contains(resp.Errors[0].Message, tc.want) {
				t.Fatalf("errors = %+v, want %q", resp.Errors, tc.want)
			}
			if !reflect.DeepEqual(resp.Errors[0].Path, []any{"people"}) {
				t.Errorf("path = %v", resp.Errors[0].Path)
			}
			if len(db.queries) != 0 {
				t.Errorf("queries ran: %q", db.queries)
			}
		})
	}
}

func TestExportMutation(t *testing.T) {
	const mutation = `mutation { exportPeople(input: {fields: [NAME, DISPLAY], filter: %q, sort: [{field: NAME, direction: DESC}], format: XLSX}) { id } }`

	t.Run("accepted", func(t *testing.T) {
		rt, db := newRuntime()
		rt.Clock = fixedClock(created)
		exp := &recordingExporter{id: "job-42"}
		rt.Exporter = exp
		resp := run(t, rt, fmt.Sprintf(mutation, "name CONTAINS 'a'"))
		if len(resp.Errors) > 0 || string(resp.Data) != `{"exportPeople":{"id":"job-42"}}` {
			t.Fatalf("resp = %s %+v", resp.Data, resp.Errors)
		}
		want := easylist.ExportRequest{
			List: "users", Fields: []string{"name", "display"}, Filter: "name CONTAINS 'a'",
			Sort: []easylist.Sort{{Field: "name", Direction: easylist.Desc}}, Format: easylist.XLSX, AsOf: created,
		}
		if len(exp.got) != 1 || !reflect.DeepEqual(exp.got[0], want) {
			t.Errorf("exporter got %+v", exp.got)
		}
		if len(db.queries) != 0 {
			t.Errorf("export mutation queried the database: %q", db.queries)
		}
	})

	t.Run("invalid filter", func(t *testing.T) {
		rt, _ := newRuntime()
		exp := &recordingExporter{id: "job-42"}
		rt.Exporter = exp
		resp := run(t, rt, fmt.Sprintf(mutation, "nickname = 'a'"))
		if len(resp.Errors) != 1 || !strings.Contains(resp.Errors[0].Message, "nickname") || len(exp.got) != 0 {
			t.Fatalf("errors = %+v, exporter calls = %d", resp.Errors, len(exp.got))
		}
	})

	t.Run("exporter rejects", func(t *testing.T) {
		rt, _ := newRuntime()
		rt.Exporter = &recordingExporter{err: errors.New("queue full")}
		resp := run(t, rt, fmt.Sprintf(mutation, ""))
		if len(resp.Errors) != 1 || !strings.Contains(resp.Errors[0].Message, "queue full") {
			t.Fatalf("errors = %+v", resp.Errors)
		}
	})

	t.Run("not configured", func(t *testing.T) {
		rt, _ := newRuntime()
		resp := run(t, rt, fmt.Sprintf(mutation, ""))
		if len(resp.Errors) != 1 || !strings.Contains(resp.Errors[0].Message, "exports are not configured") {
			t.Fatalf("errors = %+v", resp.Errors)
		}
	})
}

// planDriver is a fakeDriver that also records the plans it compiles.
type planDriver struct {
	fakeDriver
	plans []*easylist.Plan
}

func (d *planDriver) Compile(p *easylist.Plan) (string, []any, error) {
	d.plans = append(d.plans, p)
	return d.fakeDriver.Compile(p)
}

func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

func TestListTimezone(t *testing.T) {
	clock := time.Date(2026, 10, 28, 2, 0, 0, 0, time.UTC)
	cases := []struct {
		name, input string
		want        time.Time
	}{
		{"client time zone", `{filter: "created_at = TODAY", timezone: -300}`,
			time.Date(2026, 10, 27, 0, 0, 0, 0, time.FixedZone("", -300*60))},
		{"UTC+14:00", `{filter: "created_at = TODAY", timezone: 840}`,
			time.Date(2026, 10, 28, 0, 0, 0, 0, time.FixedZone("", 840*60))},
		{"no timezone uses the runtime clock's zone", `{filter: "created_at = TODAY"}`,
			time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			drv := &planDriver{}
			db := &fakeDB{}
			rt := &easylist.Runtime{Driver: drv, DB: db, Clock: fixedClock(clock)}
			resp := run(t, rt, `{ people(query: `+tc.input+`) { items { id } } }`)
			if len(resp.Errors) > 0 {
				t.Fatalf("errors: %+v", resp.Errors)
			}
			if len(drv.plans) != 1 {
				t.Fatalf("compiled %d plans, want 1", len(drv.plans))
			}
			pred, ok := drv.plans[0].Filter.(filter.Predicate)
			if !ok {
				t.Fatalf("filter = %#v", drv.plans[0].Filter)
			}
			v, ok := drv.plans[0].Value(pred)
			if !ok || v.Kind != easylist.ValueDate || !v.Time.Equal(tc.want) {
				t.Fatalf("TODAY = %+v (%v), want date %v", v, ok, tc.want)
			}
			_, gotOff := v.Time.Zone()
			_, wantOff := tc.want.Zone()
			if gotOff != wantOff {
				t.Errorf("TODAY zone offset = %d, want %d", gotOff, wantOff)
			}
		})
	}

	t.Run("timezone out of range", func(t *testing.T) {
		for _, tz := range []string{"-800", "-721", "841"} {
			rt, db := newRuntime()
			rt.Clock = fixedClock(clock)
			resp := run(t, rt, `{ people(query: {filter: "created_at = TODAY", timezone: `+tz+`}) { totalCount items { id } } }`)
			if len(resp.Errors) != 1 || !strings.Contains(resp.Errors[0].Message, "between -720 and 840") {
				t.Fatalf("timezone %s: errors = %+v", tz, resp.Errors)
			}
			if !reflect.DeepEqual(resp.Errors[0].Path, []any{"people"}) {
				t.Errorf("path = %v", resp.Errors[0].Path)
			}
			if len(db.queries) != 0 {
				t.Errorf("queries ran: %q", db.queries)
			}
		}
	})
}

func TestExportRequestMoment(t *testing.T) {
	const mutation = `mutation { exportPeople(input: {fields: [NAME], filter: "created_at >= TODAY", format: CSV%s}) { id } }`
	tz := func(m int) *int { return &m }
	plus2 := time.FixedZone("", 2*60*60)
	cases := []struct {
		name, args string
		clock      time.Time
		wantAsOf   time.Time
		wantTZ     *int
	}{
		{"request moment defaults to server time", ", timezone: 120", time.Date(2026, 10, 27, 10, 0, 0, 0, plus2),
			time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC), tz(120)},
		{"client-supplied asOf", `, asOf: "2026-10-01T09:00:00+02:00"`, time.Date(2026, 10, 27, 10, 0, 0, 0, plus2),
			time.Date(2026, 10, 1, 7, 0, 0, 0, time.UTC), nil},
		{"asOf and timezone", `, asOf: "2026-10-01T09:00:00+02:00", timezone: -300`, time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 10, 1, 7, 0, 0, 0, time.UTC), tz(-300)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, _ := newRuntime()
			rt.Clock = fixedClock(tc.clock)
			exp := &recordingExporter{id: "job-1"}
			rt.Exporter = exp
			resp := run(t, rt, fmt.Sprintf(mutation, tc.args))
			if len(resp.Errors) > 0 {
				t.Fatalf("errors: %+v", resp.Errors)
			}
			if len(exp.got) != 1 {
				t.Fatalf("exporter calls = %d", len(exp.got))
			}
			got := exp.got[0]
			if got.AsOf != tc.wantAsOf || got.AsOf.Location() != time.UTC {
				t.Errorf("AsOf = %v, want %v", got.AsOf, tc.wantAsOf)
			}
			if !reflect.DeepEqual(got.Timezone, tc.wantTZ) {
				t.Errorf("Timezone = %v, want %v", got.Timezone, tc.wantTZ)
			}
		})
	}

	t.Run("system clock survives a JSON round trip", func(t *testing.T) {
		rt, _ := newRuntime()
		exp := &recordingExporter{id: "job-1"}
		rt.Exporter = exp
		before := time.Now()
		resp := run(t, rt, fmt.Sprintf(mutation, ""))
		if len(resp.Errors) > 0 || len(exp.got) != 1 {
			t.Fatalf("errors = %+v, exporter calls = %d", resp.Errors, len(exp.got))
		}
		got := exp.got[0]
		if got.AsOf.Before(before.Add(-time.Second)) || got.AsOf.After(time.Now()) {
			t.Errorf("AsOf = %v, want about %v", got.AsOf, before)
		}
		data, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		var back easylist.ExportRequest
		if err := json.Unmarshal(data, &back); err != nil {
			t.Fatal(err)
		}
		if back.AsOf != got.AsOf {
			t.Errorf("AsOf after round trip = %v, want %v", back.AsOf, got.AsOf)
		}
	})

	t.Run("timezone out of range", func(t *testing.T) {
		rt, _ := newRuntime()
		exp := &recordingExporter{id: "job-1"}
		rt.Exporter = exp
		resp := run(t, rt, fmt.Sprintf(mutation, ", timezone: -800"))
		if len(resp.Errors) != 1 || !strings.Contains(resp.Errors[0].Message, "between -720 and 840") || len(exp.got) != 0 {
			t.Fatalf("errors = %+v, exporter calls = %d", resp.Errors, len(exp.got))
		}
	})
}
