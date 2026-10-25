package easylist

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
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
