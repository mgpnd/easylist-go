package easylistexport

import (
	"bytes"
	"context"
	"encoding/csv"
	"math"
	"strings"
	"testing"
	"time"

	easylist "github.com/mgpnd/easylist-go/easylist-core"

	"github.com/xuri/excelize/v2"
)

var arraysList = easylist.List{
	Name: "items", Table: "items", Alias: "i", PrimaryKey: "id",
	Fields: []easylist.Field{
		{Name: "id", Type: easylist.Integer},
		{Name: "tags", Type: easylist.Text, Array: true, Nullable: true},
		{Name: "ids", Type: easylist.Integer, Array: true},
		{Name: "days", Type: easylist.Date, Array: true},
		{Name: "seen_at", Type: easylist.Timestamp, Array: true},
		{Name: "prices", Type: easylist.Numeric, Array: true},
		{Name: "flags", Type: easylist.Boolean, Array: true},
		easylist.Calc("labels", easylist.Text, func(_ context.Context, r easylist.Row) ([]string, error) {
			tags, _ := r.Strings("tags")
			out := make([]string, len(tags))
			for i, tag := range tags {
				out[i] = "#" + tag
			}
			return out, nil
		}, "tags"),
	},
}

func arrayRows() []map[string]any {
	return []map[string]any{
		{"id": int64(1), "tags": `["go","rust"]`, "ids": `[7,8]`, "days": `["2026-10-25"]`,
			"seen_at": `["2026-10-25T10:00:00+02:00"]`, "prices": `[10.50]`, "flags": `[true,false]`},
		{"id": int64(2), "tags": `["say \"hi\"","a,b"]`, "ids": `[]`, "days": `[]`, "seen_at": `[]`, "prices": `[]`, "flags": `[]`},
		{"id": int64(3), "tags": nil, "ids": `[-9007199254740993]`, "days": `[]`, "seen_at": `[]`, "prices": `["NaN"]`, "flags": `[]`},
	}
}

var arrayFields = []string{"id", "tags", "ids", "days", "seen_at", "prices", "flags", "labels"}

func runArrays(t *testing.T, format easylist.Format) []byte {
	t.Helper()
	var out bytes.Buffer
	rt := &easylist.Runtime{Driver: fakeDriver{}, DB: &fakeDB{rows: arrayRows()}}
	req := easylist.ExportRequest{List: "items", Fields: arrayFields, Format: format}
	if err := Run(context.Background(), registry(t, arraysList), rt, req, &out); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestArrayText(t *testing.T) {
	field := func(typ easylist.Type) easylist.Field {
		return easylist.Field{Name: "x", Type: typ, Array: true}
	}
	at := time.Date(2026, 10, 25, 10, 0, 0, 0, time.FixedZone("", 2*3600))
	cases := []struct {
		name string
		typ  easylist.Type
		v    any
		want string
	}{
		{"text array", easylist.Text, []string{"go", "rust"}, `"go","rust"`},
		{"integer array", easylist.Integer, []int64{7, 8}, `7,8`},
		{"embedded quote and comma", easylist.Text, []string{`say "hi"`, "a,b"}, `"say ""hi""","a,b"`},
		{"date array", easylist.Date, []time.Time{time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)}, `"2026-10-25"`},
		{"timestamp array", easylist.Timestamp, []time.Time{at}, `"2026-10-25T08:00:00Z"`},
		{"numeric array", easylist.Numeric, []string{"10.50"}, `10.50`},
		{"boolean array", easylist.Boolean, []bool{true, false}, `true,false`},
		{"float array", easylist.Float, []float64{1.5, 1e21, math.NaN()}, `1.5,1e+21,NaN`},
		{"uuid array", easylist.UUID, []string{"0b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b"}, `"0b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b"`},
		{"enum array", easylist.Enum, []string{"ok", "failed"}, `"ok","failed"`},
		{"empty array", easylist.Text, []string{}, ``},
		{"empty integer array", easylist.Integer, []int64{}, ``},
		{"empty text element", easylist.Text, []string{""}, `""`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := csvValue(field(tc.typ), tc.v)
			if err != nil || got != tc.want {
				t.Errorf("csvValue = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
	if _, err := csvValue(field(easylist.Integer), int64(7)); err == nil || !strings.Contains(err.Error(), `"x"`) {
		t.Errorf("scalar on an array field: err = %v", err)
	}
}

func TestCSVArrays(t *testing.T) {
	out := string(runArrays(t, easylist.CSV))
	want := "id,tags,ids,days,seen_at,prices,flags,labels\n" +
		`1,"""go"",""rust""","7,8","""2026-10-25""","""2026-10-25T08:00:00Z""",10.50,"true,false","""#go"",""#rust"""` + "\n" +
		`2,"""say """"hi"""""",""a,b""",,,,,,"""#say """"hi"""""",""#a,b"""` + "\n" +
		"3,,-9007199254740993,,,NaN,,\n"
	if out != want {
		t.Errorf("csv =\n%s\nwant\n%s", out, want)
	}
	records, err := csv.NewReader(strings.NewReader(out)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if got := records[1][1]; got != `"go","rust"` {
		t.Errorf("decoded tags = %q", got)
	}
	if got := records[2][1]; got != `"say ""hi""","a,b"` {
		t.Errorf("decoded tags = %q", got)
	}
}

func TestXLSXArrays(t *testing.T) {
	f, err := excelize.OpenReader(bytes.NewReader(runArrays(t, easylist.XLSX)))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cases := []struct {
		cell, value string
		typ         excelize.CellType
	}{
		{"B2", `"go","rust"`, excelize.CellTypeInlineString},
		{"C2", "7,8", excelize.CellTypeInlineString},
		{"D2", `"2026-10-25"`, excelize.CellTypeInlineString},
		{"E2", `"2026-10-25T08:00:00Z"`, excelize.CellTypeInlineString},
		{"F2", "10.50", excelize.CellTypeInlineString},
		{"G2", "true,false", excelize.CellTypeInlineString},
		{"H2", `"#go","#rust"`, excelize.CellTypeInlineString},
		{"B3", `"say ""hi""","a,b"`, excelize.CellTypeInlineString},
		{"C3", "", excelize.CellTypeUnset},
		{"H3", `"#say ""hi""","#a,b"`, excelize.CellTypeInlineString},
		{"B4", "", excelize.CellTypeUnset},
		{"C4", "-9007199254740993", excelize.CellTypeInlineString},
		{"H4", "", excelize.CellTypeUnset},
	}
	for _, c := range cases {
		v, err := f.GetCellValue("items", c.cell, excelize.Options{RawCellValue: true})
		if err != nil {
			t.Fatal(err)
		}
		typ, err := f.GetCellType("items", c.cell)
		if err != nil {
			t.Fatal(err)
		}
		if v != c.value || typ != c.typ {
			t.Errorf("%s = %q (type %v), want %q (type %v)", c.cell, v, typ, c.value, c.typ)
		}
	}
}
