package easylist

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func usersRegistry(t *testing.T) *Registry {
	t.Helper()
	reg, err := NewRegistry(usersList())
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

func TestExportRequestJSONRoundTrip(t *testing.T) {
	req := ExportRequest{
		List: "users", Fields: []string{"name", "country"}, Filter: "name CONTAINS 'a'",
		Sort: []Sort{{Field: "name", Direction: Desc}, {Field: "age"}}, Format: XLSX,
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var got ExportRequest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, req) {
		t.Errorf("round trip = %+v, want %+v", got, req)
	}
	if want := `{"list":"users","fields":["name","country"],"filter":"name CONTAINS 'a'","sort":[{"field":"name","direction":"DESC"},{"field":"age"}],"format":"XLSX"}`; string(data) != want {
		t.Errorf("json = %s", data)
	}
}

func TestValidateExport(t *testing.T) {
	valid := ExportRequest{List: "users", Fields: []string{"name", "display"}, Format: CSV}
	cases := []struct {
		name   string
		mutate func(r *ExportRequest)
		want   string
	}{
		{"calculated field allowed", func(*ExportRequest) {}, ""},
		{"filter and sort", func(r *ExportRequest) {
			r.Filter, r.Sort = "country = 'FR'", []Sort{{Field: "name", Direction: Desc}}
		}, ""},
		{"blank filter", func(r *ExportRequest) { r.Filter = "  " }, ""},
		{"unknown list", func(r *ExportRequest) { r.List = "orders" }, "orders"},
		{"unknown field", func(r *ExportRequest) { r.Fields = []string{"nickname"} }, "nickname"},
		{"duplicate field", func(r *ExportRequest) { r.Fields = []string{"name", "name"} }, "more than once"},
		{"no fields", func(r *ExportRequest) { r.Fields = nil }, "no fields"},
		{"invalid format", func(r *ExportRequest) { r.Format = "PDF" }, "PDF"},
		{"filter syntax", func(r *ExportRequest) { r.Filter = "name = " }, "position 8"},
		{"filter unknown field", func(r *ExportRequest) { r.Filter = "nickname = 'a'" }, "nickname"},
		{"sort on calculated field", func(r *ExportRequest) { r.Sort = []Sort{{Field: "display"}} }, "display"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := valid
			tc.mutate(&req)
			l, planReq, err := usersRegistry(t).ValidateExport(req)
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				if l.Name() != "users" || !reflect.DeepEqual(planReq.Fields, req.Fields) {
					t.Errorf("validated = %v %+v", l.Name(), planReq)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

type recordingExporter struct {
	got []ExportRequest
	id  string
	err error
}

func (e *recordingExporter) Export(_ context.Context, req ExportRequest) (string, error) {
	e.got = append(e.got, req)
	return e.id, e.err
}

func TestExporterInterface(t *testing.T) {
	var e Exporter = &recordingExporter{id: "job-42"}
	id, err := e.Export(context.Background(), ExportRequest{List: "users"})
	if id != "job-42" || err != nil {
		t.Errorf("Export = %q, %v", id, err)
	}
}

func TestExportRequestTimeJSON(t *testing.T) {
	offset := 120
	req := ExportRequest{
		List: "users", Fields: []string{"name"}, Filter: "last_order = TODAY", Format: CSV,
		AsOf: time.Date(2026, 10, 27, 21, 0, 0, 0, time.UTC), Timezone: &offset,
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"list":"users","fields":["name"],"filter":"last_order = TODAY","format":"CSV","asOf":"2026-10-27T21:00:00Z","timezone":120}`; string(data) != want {
		t.Errorf("json = %s", data)
	}
	var got ExportRequest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, req) {
		t.Errorf("round trip = %+v, want %+v", got, req)
	}

	var old ExportRequest
	if err := json.Unmarshal([]byte(`{"list":"users","fields":["name"],"format":"CSV"}`), &old); err != nil {
		t.Fatal(err)
	}
	if !old.AsOf.IsZero() || old.Timezone != nil {
		t.Errorf("old request = %+v", old)
	}
}

func TestUTCOffset(t *testing.T) {
	for _, minutes := range []int{-720, -300, 0, 120, 330, 840} {
		loc, err := UTCOffset(minutes)
		if err != nil {
			t.Fatalf("UTCOffset(%d): %v", minutes, err)
		}
		if _, off := time.Date(2026, 10, 27, 0, 0, 0, 0, loc).Zone(); off != minutes*60 {
			t.Errorf("UTCOffset(%d) offset = %ds", minutes, off)
		}
	}
	for _, minutes := range []int{-721, -800, 841, 900} {
		if _, err := UTCOffset(minutes); err == nil || !strings.Contains(err.Error(), "between -720 and 840") {
			t.Errorf("UTCOffset(%d) err = %v", minutes, err)
		}
	}
}

func TestValidateExportReferenceTime(t *testing.T) {
	asOf := time.Date(2026, 10, 27, 23, 30, 0, 0, time.UTC)
	offset := func(m int) *int { return &m }
	cases := []struct {
		name     string
		filter   string
		asOf     time.Time
		timezone *int
		wantLoc  func(*time.Location) bool
		want     string
	}{
		{"offset zone", "last_order = TODAY", asOf, offset(120), func(l *time.Location) bool {
			_, off := asOf.In(l).Zone()
			return off == 7200
		}, ""},
		{"server local zone", "last_order = TODAY", asOf, nil, func(l *time.Location) bool { return l == time.Local }, ""},
		{"no request moment", "last_order = TODAY", time.Time{}, offset(120), nil, ""},
		{"no request moment still type-checks", "age > TODAY", time.Time{}, nil, nil, "position 7"},
		{"offset out of range", "", asOf, offset(900), nil, "between -720 and 840"},
		{"offset out of range without asOf", "", time.Time{}, offset(-721), nil, "between -720 and 840"},
		{"evaluation error", "last_order > {TODAY + 10000000::DAYS}", asOf, nil, nil, "years 1 to 9999"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := ExportRequest{List: "users", Fields: []string{"name"}, Filter: tc.filter, Format: CSV, AsOf: tc.asOf, Timezone: tc.timezone}
			_, planReq, err := usersRegistry(t).ValidateExport(req)
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("err = %v, want %q", err, tc.want)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.asOf.IsZero() {
				if !planReq.Now.IsZero() {
					t.Errorf("Now = %v, want zero", planReq.Now)
				}
				return
			}
			if !planReq.Now.Equal(tc.asOf) || !tc.wantLoc(planReq.Now.Location()) {
				t.Errorf("Now = %v", planReq.Now)
			}
		})
	}
}

func TestValidateExportPlansAtAsOf(t *testing.T) {
	reg := usersRegistry(t)
	l, planReq, err := reg.ValidateExport(ExportRequest{
		List: "users", Fields: []string{"name"}, Filter: "last_order = TODAY", Format: CSV,
		AsOf: time.Date(2026, 10, 27, 23, 30, 0, 0, time.UTC), Timezone: func() *int { m := 120; return &m }(),
	})
	if err != nil {
		t.Fatal(err)
	}
	p, err := l.Plan(KindExport, planReq)
	if err != nil {
		t.Fatal(err)
	}
	v, ok := p.Value(predicates(planReq.Filter)[0])
	if !ok || v.Kind != ValueDate || v.Time.Format(time.RFC3339) != "2026-10-28T00:00:00+02:00" {
		t.Errorf("TODAY = %+v, %v", v, ok)
	}
}
