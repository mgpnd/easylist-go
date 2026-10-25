package easylist

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"
)

// pageRow is one scripted row for the plan selecting id, name, age, country.
func pageRow(id int64, name string, age, country any) []any {
	return []any{id, name, age, country}
}

func TestRowAccessors(t *testing.T) {
	l := mustUsers(t)
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	r := NewRow(l, map[string]any{
		"id": int64(7), "name": "Ann", "age": nil, "last_order": at,
		"status": "active", "score": 1.5, "flag": true,
	})
	if v, ok := r.Int64("id"); !ok || v != 7 {
		t.Errorf("Int64 = %v, %v", v, ok)
	}
	if v, ok := r.String("name"); !ok || v != "Ann" {
		t.Errorf("String = %v, %v", v, ok)
	}
	if _, ok := r.Int64("age"); ok {
		t.Error("NULL reported as present")
	}
	if _, ok := r.String("id"); ok {
		t.Error("wrong-type access reported as present")
	}
	if v, ok := r.Time("last_order"); !ok || !v.Equal(at) {
		t.Errorf("Time = %v, %v", v, ok)
	}
	if v, ok := r.Float64("score"); !ok || v != 1.5 {
		t.Errorf("Float64 = %v, %v", v, ok)
	}
	if v, ok := r.Bool("flag"); !ok || !v {
		t.Errorf("Bool = %v, %v", v, ok)
	}
	if _, ok := r.Value("missing"); ok {
		t.Error("unselected field reported as present")
	}
}

func TestStreamScansTypedValues(t *testing.T) {
	reg, err := NewRegistry(List{
		Name: "all", Table: "t", Alias: "t", PrimaryKey: "id",
		Fields: []Field{
			{Name: "id", Type: UUID},
			{Name: "txt", Type: Text}, {Name: "num", Type: Integer}, {Name: "flt", Type: Float},
			{Name: "dec", Type: Numeric}, {Name: "ok", Type: Boolean}, {Name: "day", Type: Date},
			{Name: "at", Type: Timestamp}, {Name: "kind", Type: Enum, Values: []string{"a"}},
			{Name: "opt", Type: Text, Nullable: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	l, _ := reg.List("all")
	day := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	at := time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)
	db := &fakeDB{rows: map[string][][]any{"SELECT": {{
		"0b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b", "hi", int64(5), 2.5,
		[]byte("12345678901234567890.123"), true, day, at, "a", nil,
	}}}}
	rt := &Runtime{Driver: fakeDriver{}, DB: db}
	fields := []string{"txt", "num", "flt", "dec", "ok", "day", "at", "kind", "opt"}
	res, err := rt.Fetch(context.Background(), l, Request{Fields: fields}, Want{Rows: true})
	if err != nil {
		t.Fatal(err)
	}
	r := res.Rows[0]
	want := map[string]any{
		"id": "0b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b", "txt": "hi", "num": int64(5), "flt": 2.5,
		"dec": "12345678901234567890.123", "ok": true, "day": day, "at": at, "kind": "a",
	}
	for name, v := range want {
		if got, ok := r.Value(name); !ok || got != v {
			t.Errorf("%s = %#v, %v; want %#v", name, got, ok, v)
		}
	}
	if _, ok := r.Value("opt"); ok {
		t.Error("NULL nullable field reported as present")
	}
}

func TestFetchSelectsQueries(t *testing.T) {
	cases := []struct {
		name  string
		want  Want
		calls []string
	}{
		{"rows only", Want{Rows: true}, []string{"SELECT id,name"}},
		{"count only", Want{Count: true}, []string{"COUNT"}},
		{"both", Want{Rows: true, Count: true}, []string{"SELECT id,name", "COUNT"}},
		{"neither", Want{}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := &fakeDB{rows: map[string][][]any{
				"SELECT": {{int64(1), "Ann"}, {int64(2), "Bob"}},
				"COUNT":  {{int64(42)}},
			}}
			rt := &Runtime{Driver: fakeDriver{}, DB: db}
			res, err := rt.Fetch(context.Background(), mustUsers(t), Request{Fields: []string{"name"}}, tc.want)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, c := range db.calls {
				got = append(got, c.query)
			}
			if !slices.Equal(got, tc.calls) {
				t.Errorf("queries = %v, want %v", got, tc.calls)
			}
			if tc.want.Rows && len(res.Rows) != 2 {
				t.Errorf("rows = %d", len(res.Rows))
			}
			if tc.want.Rows {
				if name, _ := res.Rows[1].String("name"); name != "Bob" {
					t.Errorf("row order lost: %q", name)
				}
			}
			if tc.want.Count && res.Total != 42 {
				t.Errorf("total = %d", res.Total)
			}
		})
	}
}

func TestFetchPlanErrorRunsNoQuery(t *testing.T) {
	db := &fakeDB{}
	rt := &Runtime{Driver: fakeDriver{}, DB: db}
	_, err := rt.Fetch(context.Background(), mustUsers(t), Request{Limit: intp(20000)}, Want{Rows: true, Count: true})
	if err == nil || len(db.calls) != 0 {
		t.Fatalf("err = %v, calls = %d", err, len(db.calls))
	}
}

func TestNullInNonNullableField(t *testing.T) {
	db := &fakeDB{rows: map[string][][]any{"SELECT": {{int64(1), nil}}}}
	rt := &Runtime{Driver: fakeDriver{}, DB: db}
	_, err := rt.Fetch(context.Background(), mustUsers(t), Request{Fields: []string{"name"}}, Want{Rows: true})
	if err == nil || !strings.Contains(err.Error(), `"name"`) || !strings.Contains(err.Error(), "NULL") {
		t.Fatalf("err = %v", err)
	}
}

func TestQueryErrorsNameList(t *testing.T) {
	dbErr := errors.New("relation does not exist")
	rt := &Runtime{Driver: fakeDriver{}, DB: &fakeDB{err: dbErr}}
	_, err := rt.Fetch(context.Background(), mustUsers(t), Request{Filter: parse(t, "name = 'secret'")}, Want{Rows: true})
	if !errors.Is(err, dbErr) || !strings.Contains(err.Error(), `"users"`) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Errorf("error leaks an argument: %v", err)
	}

	rowsErr := errors.New("connection reset")
	rt.DB = queryerFunc(func(context.Context, string, ...any) (Rows, error) {
		return &fakeRows{data: [][]any{{int64(1)}}, err: rowsErr}, nil
	})
	if _, err := rt.Fetch(context.Background(), mustUsers(t), Request{}, Want{Rows: true}); !errors.Is(err, rowsErr) || !strings.Contains(err.Error(), `"users"`) {
		t.Fatalf("rows err = %v", err)
	}
}

type queryerFunc func(context.Context, string, ...any) (Rows, error)

func (f queryerFunc) QueryContext(ctx context.Context, q string, args ...any) (Rows, error) {
	return f(ctx, q, args...)
}

func TestCalculatedEvaluation(t *testing.T) {
	l := mustUsers(t)
	db := &fakeDB{rows: map[string][][]any{"SELECT": {{int64(1), "Ann", nil}, {int64(2), "Bob", "FR"}}}}
	rt := &Runtime{Driver: fakeDriver{}, DB: db}
	res, err := rt.Fetch(context.Background(), l, Request{Fields: []string{"display"}}, Want{Rows: true})
	if err != nil {
		t.Fatal(err)
	}
	if db.calls[0].query != "SELECT id,name,country JOIN country" {
		t.Errorf("query = %q", db.calls[0].query)
	}
	for i, want := range []string{"Ann", "Bob (FR)"} {
		v, err := res.Rows[i].Calc(context.Background(), "display")
		if err != nil || v != want {
			t.Errorf("row %d display = %v, %v", i, v, err)
		}
	}
}

func TestCalculatedSeesRequirements(t *testing.T) {
	var seen []string
	def := usersList()
	def.Fields[8] = Calc("display", Text, func(_ context.Context, r Row) (string, error) {
		name, nameOK := r.String("name")
		_, countryOK := r.String("country")
		seen = append(seen, name, map[bool]string{true: "present", false: "absent"}[nameOK && !countryOK])
		return name, nil
	}, "name", "country")
	reg, err := NewRegistry(def)
	if err != nil {
		t.Fatal(err)
	}
	l, _ := reg.List("users")
	db := &fakeDB{rows: map[string][][]any{"SELECT": {{int64(1), "Ann", nil}}}}
	rt := &Runtime{Driver: fakeDriver{}, DB: db}
	res, err := rt.Fetch(context.Background(), l, Request{Fields: []string{"display"}}, Want{Rows: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 0 {
		t.Fatal("calculated function ran before it was requested")
	}
	if _, err := res.Rows[0].Calc(context.Background(), "display"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(seen, []string{"Ann", "present"}) {
		t.Errorf("function saw %v; want name Ann with country absent", seen)
	}
}

func TestCalculatedErrorsAndNullable(t *testing.T) {
	boom := errors.New("boom")
	def := usersList()
	def.Fields = append(def.Fields,
		Calc("fails", Text, func(context.Context, Row) (string, error) { return "", boom }, "name"),
		CalcNullable("maybe", Integer, func(_ context.Context, r Row) (*int64, error) {
			age, ok := r.Int64("age")
			if !ok {
				return nil, nil
			}
			return &age, nil
		}, "age"),
	)
	reg, err := NewRegistry(def)
	if err != nil {
		t.Fatal(err)
	}
	l, _ := reg.List("users")
	ctx := context.Background()
	if _, err := NewRow(l, nil).Calc(ctx, "fails"); !errors.Is(err, boom) || !strings.Contains(err.Error(), `"fails"`) {
		t.Errorf("err = %v", err)
	}
	if v, err := NewRow(l, nil).Calc(ctx, "maybe"); err != nil || v != nil {
		t.Errorf("NULL calc = %v, %v", v, err)
	}
	if v, err := NewRow(l, map[string]any{"age": int64(30)}).Calc(ctx, "maybe"); err != nil || v != int64(30) {
		t.Errorf("calc = %v, %v", v, err)
	}
	if _, err := NewRow(l, nil).Calc(ctx, "name"); err == nil {
		t.Error("Calc on an SQL field succeeded")
	}
}

func TestStreamStopsOnCallbackAndContext(t *testing.T) {
	l := mustUsers(t)
	p, _ := l.Plan(KindExport, Request{Fields: []string{"name"}})
	rows := [][]any{{int64(1), "a"}, {int64(2), "b"}, {int64(3), "c"}}
	stop := errors.New("stop")
	rt := &Runtime{Driver: fakeDriver{}, DB: &fakeDB{rows: map[string][][]any{"SELECT": rows}}}
	n := 0
	err := rt.Stream(context.Background(), l, p, func(Row) error {
		n++
		if n == 2 {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) || n != 2 {
		t.Errorf("err = %v after %d rows", err, n)
	}

	ctx, cancel := context.WithCancel(context.Background())
	n = 0
	err = rt.Stream(ctx, l, p, func(Row) error {
		n++
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) || n != 1 {
		t.Errorf("err = %v after %d rows", err, n)
	}
}

// recordingDriver is a database/sql driver whose queries return one count row.
type recordingDriver struct{ queries *[]call }

func (d recordingDriver) Open(string) (driver.Conn, error) { return recordingConn(d), nil }

type recordingConn recordingDriver

func (recordingConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not supported") }
func (recordingConn) Close() error                        { return nil }
func (recordingConn) Begin() (driver.Tx, error)           { return nil, errors.New("not supported") }

func (c recordingConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	values := make([]any, len(args))
	for i, a := range args {
		values[i] = a.Value
	}
	*c.queries = append(*c.queries, call{query, values})
	return &countRows{}, nil
}

type countRows struct{ done bool }

func (*countRows) Columns() []string { return []string{"count"} }
func (*countRows) Close() error      { return nil }
func (r *countRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = int64(3)
	return nil
}

// argDriver binds the filter literal so the adapter test can see arguments.
type argDriver struct{}

func (argDriver) Compile(p *Plan) (string, []any, error) {
	return "SELECT COUNT(*) WHERE name = $1", []any{"Ann"}, nil
}

func TestSQLDBAdapter(t *testing.T) {
	var queries []call
	sql.Register("easylist-recording", recordingDriver{&queries})
	db, err := sql.Open("easylist-recording", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rt := &Runtime{Driver: argDriver{}, DB: SQLDB(db)}
	res, err := rt.Fetch(context.Background(), mustUsers(t), Request{}, Want{Count: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 3 || len(queries) != 1 || queries[0].query != "SELECT COUNT(*) WHERE name = $1" || queries[0].args[0] != "Ann" {
		t.Errorf("total = %d, queries = %+v", res.Total, queries)
	}
}
