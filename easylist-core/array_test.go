package easylist

import (
	"context"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// arraysList declares an array field of every type, plus calculated arrays.
func arraysList() List {
	return List{
		Name: "items", Table: "items", Alias: "i", PrimaryKey: "id",
		Fields: []Field{
			{Name: "id", Type: Integer},
			{Name: "tags", Type: Text, Array: true, Nullable: true},
			{Name: "ids", Type: Integer, Array: true},
			{Name: "scores", Type: Float, Array: true},
			{Name: "prices", Type: Numeric, Array: true},
			{Name: "flags", Type: Boolean, Array: true},
			{Name: "days", Type: Date, Array: true},
			{Name: "seen_at", Type: Timestamp, Array: true},
			{Name: "group_ids", Type: UUID, Array: true},
			{Name: "statuses", Type: Enum, Array: true, Nullable: true, Values: []string{"ok", "failed"}},
			Calc("labels", Text, func(_ context.Context, r Row) ([]string, error) {
				tags, _ := r.Strings("tags")
				return tags, nil
			}, "tags"),
			CalcNullable("checks", Enum, func(_ context.Context, r Row) (*[]string, error) {
				statuses, ok := r.Strings("statuses")
				if !ok {
					return nil, nil
				}
				out := append(slices.Clone(statuses), "archived")
				return &out, nil
			}, "statuses").WithValues("ok", "failed"),
		},
	}
}

func mustArrays(t *testing.T) *CompiledList {
	t.Helper()
	reg, err := NewRegistry(arraysList())
	if err != nil {
		t.Fatal(err)
	}
	l, _ := reg.List("items")
	return l
}

func TestArrayFieldDeclarations(t *testing.T) {
	l := mustArrays(t)
	for _, name := range []string{"tags", "ids", "scores", "prices", "flags", "days", "seen_at", "group_ids", "statuses", "labels", "checks"} {
		if f, ok := l.Field(name); !ok || !f.Array {
			t.Errorf("%s: array = %v, found = %v", name, f.Array, ok)
		}
	}
	if f, _ := l.Field("id"); f.Array {
		t.Error("scalar field marked as array")
	}
	if !slices.Contains(l.typeNames(), "ItemsStatuses") {
		t.Errorf("enum array does not reserve its GraphQL enum name: %v", l.typeNames())
	}

	strs := func(context.Context, Row) ([]string, error) { return nil, nil }
	cases := []struct {
		name   string
		mutate func(l *List)
		want   []string
	}{
		{"array enum without values", func(l *List) { l.Fields[9].Values = nil }, []string{"statuses"}},
		{"calculated array with a mismatched result", func(l *List) {
			l.Fields = append(l.Fields, Calc("counts", Integer, strs))
		}, []string{"counts", "[]int64", "[]string"}},
		{"calculated scalar for an array result", func(l *List) {
			l.Fields = append(l.Fields, Calc("first", Text, strs).WithTitle("First"))
			l.Fields[len(l.Fields)-1].Array = false
		}, []string{"first", "string", "[]string"}},
		{"array primary key", func(l *List) { l.PrimaryKey = "ids" }, []string{"ids", "array"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			def := arraysList()
			tc.mutate(&def)
			_, err := NewRegistry(def)
			if err == nil {
				t.Fatalf("expected an error containing %q", tc.want)
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

func TestArrayPlanning(t *testing.T) {
	l := mustArrays(t)
	if _, err := l.Plan(KindPage, Request{Sort: []Sort{{Field: "tags"}}}); err == nil || !strings.Contains(err.Error(), `"tags"`) {
		t.Errorf("sort on array err = %v", err)
	}
	p, err := l.Plan(KindPage, Request{Fields: []string{"tags"}, Filter: parse(t, "tags CONTAINS 'go'")})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Fields) != 2 || p.Fields[1].Name != "tags" || !p.Fields[1].Array || p.Fields[0].Array {
		t.Errorf("fields = %+v", p.Fields)
	}
	if f, ok := p.Lookup("tags"); !ok || !f.Array || f.Type != Text {
		t.Errorf("lookup tags = %+v, %v", f, ok)
	}
}

func TestArrayRowValues(t *testing.T) {
	l := mustArrays(t)
	day := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	seen := time.Date(2026, 10, 25, 8, 0, 0, 0, time.UTC)
	db := &fakeDB{rows: map[string][][]any{"SELECT": {
		{int64(1), `["go","rust"]`, []byte(`[7,8]`), `[2.5,"NaN"]`, `[12345678901234567890.10]`, `[true,false]`,
			`["2026-10-25"]`, `["2026-10-25T10:00:00+02:00"]`, `["0b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b"]`, `["ok","failed"]`},
		{int64(2), `[]`, `[]`, `[]`, `[]`, `[]`, `[]`, `[]`, `[]`, nil},
		{int64(3), nil, `[]`, `[]`, `[]`, `[]`, `[]`, `[]`, `[]`, `[]`},
	}}}
	rt := &Runtime{Driver: fakeDriver{}, DB: db}
	fields := []string{"tags", "ids", "scores", "prices", "flags", "days", "seen_at", "group_ids", "statuses"}
	res, err := rt.Fetch(context.Background(), l, Request{Fields: fields}, Want{Rows: true})
	if err != nil {
		t.Fatal(err)
	}

	r := res.Rows[0]
	if v, ok := r.Strings("tags"); !ok || !slices.Equal(v, []string{"go", "rust"}) {
		t.Errorf("tags = %v, %v", v, ok)
	}
	if v, ok := r.Int64s("ids"); !ok || !slices.Equal(v, []int64{7, 8}) {
		t.Errorf("ids = %v, %v", v, ok)
	}
	if v, ok := r.Float64s("scores"); !ok || len(v) != 2 || v[0] != 2.5 || !math.IsNaN(v[1]) {
		t.Errorf("scores = %v, %v", v, ok)
	}
	if v, ok := r.Strings("prices"); !ok || !slices.Equal(v, []string{"12345678901234567890.10"}) {
		t.Errorf("prices = %v, %v", v, ok)
	}
	if v, ok := r.Bools("flags"); !ok || !slices.Equal(v, []bool{true, false}) {
		t.Errorf("flags = %v, %v", v, ok)
	}
	if v, ok := r.Times("days"); !ok || len(v) != 1 || !v[0].Equal(day) || v[0].Location() != time.UTC {
		t.Errorf("days = %v, %v", v, ok)
	}
	if v, ok := r.Times("seen_at"); !ok || len(v) != 1 || !v[0].Equal(seen) {
		t.Errorf("seen_at = %v, %v", v, ok)
	}
	if v, ok := r.Strings("group_ids"); !ok || !slices.Equal(v, []string{"0b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b"}) {
		t.Errorf("group_ids = %v, %v", v, ok)
	}
	if v, ok := r.Strings("statuses"); !ok || !slices.Equal(v, []string{"ok", "failed"}) {
		t.Errorf("statuses = %v, %v", v, ok)
	}

	if v, ok := res.Rows[1].Strings("tags"); !ok || v == nil || len(v) != 0 {
		t.Errorf("empty tags = %#v, %v; want a present, empty slice", v, ok)
	}
	if v, ok := res.Rows[1].Int64s("ids"); !ok || v == nil || len(v) != 0 {
		t.Errorf("empty ids = %#v, %v; want a present, empty slice", v, ok)
	}
	if _, ok := res.Rows[1].Value("statuses"); ok {
		t.Error("NULL statuses reported as present")
	}
	if _, ok := res.Rows[2].Value("tags"); ok {
		t.Error("NULL tags reported as present")
	}
}

func TestArrayElementErrors(t *testing.T) {
	cases := []struct {
		name  string
		field string
		value string
		want  []string
	}{
		{"NULL element", "tags", `["go",null]`, []string{`"tags"`, "NULL"}},
		{"undeclared enum element", "statuses", `["ok","archived"]`, []string{`"statuses"`, `"archived"`}},
		{"invalid integer", "ids", `[7,"x"]`, []string{`"ids"`, `"x"`}},
		{"not an array", "tags", `{"a":1}`, []string{`"tags"`, "not an array"}},
		{"invalid JSON", "tags", `{go,rust}`, []string{`"tags"`, "JSON"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := &fakeDB{rows: map[string][][]any{"SELECT": {{int64(1), tc.value}}}}
			rt := &Runtime{Driver: fakeDriver{}, DB: db}
			_, err := rt.Fetch(context.Background(), mustArrays(t), Request{Fields: []string{tc.field}}, Want{Rows: true})
			if err == nil {
				t.Fatalf("expected an error containing %q", tc.want)
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

func TestDecodeArrayElements(t *testing.T) {
	cases := []struct {
		typ  Type
		text string
		want any
	}{
		{Text, `["a, b","say \"hi\""]`, []string{"a, b", `say "hi"`}},
		{UUID, `["0b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b"]`, []string{"0b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b"}},
		{Enum, `["ok"]`, []string{"ok"}},
		{Integer, `[-9223372036854775808,0,9223372036854775807]`, []int64{math.MinInt64, 0, math.MaxInt64}},
		{Float, `[1.5,1e300,"Infinity","-Infinity"]`, []float64{1.5, 1e300, math.Inf(1), math.Inf(-1)}},
		{Numeric, `[10.50,-0.000001,"NaN"]`, []string{"10.50", "-0.000001", "NaN"}},
		{Boolean, `[true,false]`, []bool{true, false}},
		{Date, `["2026-10-25","1999-01-01"]`, []time.Time{time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC), time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC)}},
		{Timestamp, `["2026-10-25T10:00:00+02:00","2026-10-25T10:00:00.123456","2026-10-25T08:00:00Z"]`, []time.Time{
			time.Date(2026, 10, 25, 8, 0, 0, 0, time.UTC),
			time.Date(2026, 10, 25, 10, 0, 0, 123456000, time.UTC),
			time.Date(2026, 10, 25, 8, 0, 0, 0, time.UTC),
		}},
		{Integer, `[]`, []int64{}},
		{Text, ` ["go"] `, []string{"go"}},
	}
	for _, tc := range cases {
		t.Run(tc.typ.String()+" "+tc.text, func(t *testing.T) {
			got, err := decodeArray(tc.typ, tc.text)
			if err != nil {
				t.Fatal(err)
			}
			if !equalArrays(got, tc.want) {
				t.Errorf("got %#v, want %#v", got, tc.want)
			}
		})
	}

	failures := []struct {
		typ  Type
		text string
		want string
	}{
		{Integer, `[1.5]`, "1.5"},
		{Integer, `[99999999999999999999]`, "99999999999999999999"},
		{Integer, `["7"]`, `"7"`},
		{Float, `["nan"]`, `"nan"`},
		{Numeric, `["Infinity"]`, `"Infinity"`},
		{Numeric, `[true]`, "true"},
		{Boolean, `["true"]`, `"true"`},
		{Date, `["infinity"]`, `"infinity"`},
		{Date, `["2026-10-25T00:00:00"]`, "2026-10-25T00:00:00"},
		{Timestamp, `["-infinity"]`, `"-infinity"`},
		{Text, `[1]`, "1"},
		{Text, `[["a"]]`, `["a"]`},
		{Text, `[null]`, "NULL"},
		{Text, `null`, "not an array"},
		{Text, `["a"] ["b"]`, "after the array"},
		{Text, ``, "JSON"},
	}
	for _, tc := range failures {
		t.Run("invalid "+tc.typ.String()+" "+tc.text, func(t *testing.T) {
			_, err := decodeArray(tc.typ, tc.text)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

// equalArrays compares decoded slices, treating times as instants that must
// be in UTC and NaN as equal to itself.
func equalArrays(got, want any) bool {
	if reflect.TypeOf(got) != reflect.TypeOf(want) {
		return false
	}
	switch want := want.(type) {
	case []time.Time:
		return slices.EqualFunc(got.([]time.Time), want, func(a, b time.Time) bool {
			return a.Equal(b) && a.Location() == time.UTC
		})
	case []float64:
		return slices.EqualFunc(got.([]float64), want, func(a, b float64) bool {
			return a == b || math.IsNaN(a) && math.IsNaN(b)
		})
	}
	return reflect.DeepEqual(got, want)
}

func TestCalculatedArrays(t *testing.T) {
	l := mustArrays(t)
	ctx := context.Background()
	r := NewRow(l, map[string]any{"tags": []string{"go"}, "statuses": []string{"ok"}})
	if v, err := r.Calc(ctx, "labels"); err != nil || !reflect.DeepEqual(v, []string{"go"}) {
		t.Errorf("labels = %#v, %v", v, err)
	}
	if v, err := NewRow(l, nil).Calc(ctx, "labels"); err != nil || !reflect.DeepEqual(v, []string{}) {
		t.Errorf("labels from a nil slice = %#v, %v; want an empty slice", v, err)
	}
	if v, err := NewRow(l, nil).Calc(ctx, "checks"); err != nil || v != nil {
		t.Errorf("NULL checks = %#v, %v", v, err)
	}
	_, err := r.Calc(ctx, "checks")
	if err == nil || !strings.Contains(err.Error(), `"checks"`) || !strings.Contains(err.Error(), `"archived"`) {
		t.Errorf("undeclared calculated enum element err = %v", err)
	}
}

func TestArrayRowAccessors(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	r := NewRow(mustArrays(t), map[string]any{
		"tags": []string{"go"}, "ids": []int64{7}, "scores": []float64{1.5},
		"flags": []bool{true}, "seen_at": []time.Time{at}, "statuses": nil,
	})
	if v, ok := r.Strings("tags"); !ok || !slices.Equal(v, []string{"go"}) {
		t.Errorf("Strings = %v, %v", v, ok)
	}
	if v, ok := r.Int64s("ids"); !ok || !slices.Equal(v, []int64{7}) {
		t.Errorf("Int64s = %v, %v", v, ok)
	}
	if v, ok := r.Float64s("scores"); !ok || !slices.Equal(v, []float64{1.5}) {
		t.Errorf("Float64s = %v, %v", v, ok)
	}
	if v, ok := r.Bools("flags"); !ok || !slices.Equal(v, []bool{true}) {
		t.Errorf("Bools = %v, %v", v, ok)
	}
	if v, ok := r.Times("seen_at"); !ok || !slices.Equal(v, []time.Time{at}) {
		t.Errorf("Times = %v, %v", v, ok)
	}
	if _, ok := r.Strings("statuses"); ok {
		t.Error("absent array reported as present")
	}
	if _, ok := r.Int64s("tags"); ok {
		t.Error("wrong-type array access reported as present")
	}
	if _, ok := r.String("tags"); ok {
		t.Error("scalar access to an array reported as present")
	}
	if _, ok := r.Strings("missing"); ok {
		t.Error("unselected field reported as present")
	}
}
