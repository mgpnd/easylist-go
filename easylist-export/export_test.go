package easylistexport

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	easylist "github.com/mgpnd/easylist-go/easylist-core"

	"github.com/xuri/excelize/v2"
)

var usersList = easylist.List{
	Name: "users", Table: "users", Alias: "u", PrimaryKey: "id",
	Fields: []easylist.Field{
		{Name: "id", Type: easylist.Integer},
		{Name: "name", Type: easylist.Text, Title: "Full name"},
		{Name: "age", Type: easylist.Integer, Nullable: true},
		{Name: "score", Type: easylist.Float, Nullable: true},
		{Name: "balance", Type: easylist.Numeric},
		{Name: "active", Type: easylist.Boolean},
		{Name: "born", Type: easylist.Date, Nullable: true},
		{Name: "created_at", Type: easylist.Timestamp},
		{Name: "uid", Type: easylist.UUID},
		{Name: "status", Type: easylist.Enum, Values: []string{"active", "on_hold"}},
		{Name: "country", Type: easylist.Text, Nullable: true, SQL: "c.name", Joins: []string{"country"}},
		easylist.Calc("display", easylist.Text, func(_ context.Context, r easylist.Row) (string, error) {
			name, _ := r.String("name")
			if name == "Boom" {
				return "", errors.New("boom")
			}
			return "<" + name + ">", nil
		}, "name"),
	},
	Joins: []easylist.Join{{Key: "country", SQL: "LEFT JOIN countries c ON c.id = u.country_id"}},
}

func registry(t *testing.T, lists ...easylist.List) *easylist.Registry {
	t.Helper()
	if len(lists) == 0 {
		lists = []easylist.List{usersList}
	}
	reg, err := easylist.NewRegistry(lists...)
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

// fakeDriver puts the selected field names in the query so the fake
// database can return columns in plan order.
type fakeDriver struct{}

func (fakeDriver) Compile(p *easylist.Plan) (string, []any, error) {
	names := make([]string, len(p.Fields))
	for i, f := range p.Fields {
		names[i] = f.Name
	}
	return fmt.Sprintf("KIND%d %s LIMIT %d", p.Kind, strings.Join(names, ","), p.Limit), nil, nil
}

type fakeDB struct {
	rows    []map[string]any
	queries []string
	onNext  func(i int) // called before row i is produced
	err     error
}

func (db *fakeDB) QueryContext(_ context.Context, query string, _ ...any) (easylist.Rows, error) {
	db.queries = append(db.queries, query)
	if db.err != nil {
		return nil, db.err
	}
	cols := strings.Split(strings.Fields(query)[1], ",")
	return &fakeRows{db: db, cols: cols}, nil
}

type fakeRows struct {
	db   *fakeDB
	cols []string
	next int
}

func (r *fakeRows) Next() bool {
	if r.db.onNext != nil {
		r.db.onNext(r.next)
	}
	r.next++
	return r.next <= len(r.db.rows)
}

func (r *fakeRows) Scan(dest ...any) error {
	row := r.db.rows[r.next-1]
	for i, d := range dest {
		if err := d.(sql.Scanner).Scan(row[r.cols[i]]); err != nil {
			return err
		}
	}
	return nil
}

func (*fakeRows) Err() error   { return nil }
func (*fakeRows) Close() error { return nil }

var (
	born    = time.Date(1996, 2, 29, 0, 0, 0, 0, time.UTC)
	created = time.Date(2026, 10, 25, 12, 30, 0, 500_000_000, time.FixedZone("", 2*3600))
)

func sampleRows() []map[string]any {
	return []map[string]any{
		{"id": int64(1), "name": `Smith, "Jr"`, "age": int64(30), "score": 1.5, "balance": "12345678901234567890.10",
			"active": true, "born": born, "created_at": created, "uid": "0b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b",
			"status": "on_hold", "country": "FR"},
		{"id": int64(2), "name": "=SUM(A1:A9)", "age": nil, "score": nil, "balance": "0",
			"active": false, "born": nil, "created_at": created, "uid": "1b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b",
			"status": "active", "country": nil},
		{"id": int64(9007199254740993), "name": "line\nbreak", "age": int64(-9007199254740992), "score": 1e21, "balance": "-0.5",
			"active": true, "born": nil, "created_at": created, "uid": "2b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b",
			"status": "active", "country": nil},
	}
}

var allFields = []string{"id", "name", "age", "score", "balance", "active", "born", "created_at", "uid", "status", "country", "display"}

func run(t *testing.T, db *fakeDB, req easylist.ExportRequest) ([]byte, error) {
	t.Helper()
	var out bytes.Buffer
	err := Run(context.Background(), registry(t), &easylist.Runtime{Driver: fakeDriver{}, DB: db}, req, &out)
	return out.Bytes(), err
}

func TestCSVGolden(t *testing.T) {
	db := &fakeDB{rows: sampleRows()}
	out, err := run(t, db, easylist.ExportRequest{List: "users", Fields: allFields, Format: easylist.CSV})
	if err != nil {
		t.Fatal(err)
	}
	want := "id,Full name,age,score,balance,active,born,created_at,uid,status,country,display\n" +
		`1,"Smith, ""Jr""",30,1.5,12345678901234567890.10,true,1996-02-29,2026-10-25T10:30:00.5Z,0b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b,on_hold,FR,"<Smith, ""Jr"">"` + "\n" +
		"2,=SUM(A1:A9),,,0,false,,2026-10-25T10:30:00.5Z,1b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b,active,,<=SUM(A1:A9)>\n" +
		"9007199254740993,\"line\nbreak\",-9007199254740992,1e+21,-0.5,true,,2026-10-25T10:30:00.5Z,2b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b,active,,\"<line\nbreak>\"\n"
	if string(out) != want {
		t.Errorf("csv =\n%s\nwant\n%s", out, want)
	}
	if len(db.queries) != 1 || !strings.HasPrefix(db.queries[0], fmt.Sprintf("KIND%d ", easylist.KindExport)) || !strings.HasSuffix(db.queries[0], "LIMIT 0") {
		t.Errorf("queries = %q", db.queries)
	}
}

func TestHeaderUsesTitles(t *testing.T) {
	out, err := run(t, &fakeDB{}, easylist.ExportRequest{List: "users", Fields: []string{"name", "display"}, Format: easylist.CSV})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "Full name,display\n" {
		t.Errorf("csv = %q", out)
	}
}

func TestRevalidation(t *testing.T) {
	reduced := usersList
	reduced.Fields = nil
	for _, f := range usersList.Fields {
		if f.Name != "country" {
			reduced.Fields = append(reduced.Fields, f)
		}
	}
	db := &fakeDB{rows: sampleRows()}
	var out bytes.Buffer
	err := Run(context.Background(), registry(t, reduced), &easylist.Runtime{Driver: fakeDriver{}, DB: db},
		easylist.ExportRequest{List: "users", Fields: []string{"name", "country"}, Format: easylist.CSV}, &out)
	if err == nil || !strings.Contains(err.Error(), `"country"`) {
		t.Fatalf("err = %v", err)
	}
	if out.Len() != 0 || len(db.queries) != 0 {
		t.Errorf("wrote %d bytes and ran %d queries", out.Len(), len(db.queries))
	}
}

func TestStreamsWhileReading(t *testing.T) {
	const n = 2000
	db := &fakeDB{}
	for i := range n {
		db.rows = append(db.rows, map[string]any{"id": int64(i), "name": strings.Repeat("x", 40)})
	}
	var out bytes.Buffer
	var writtenBeforeLast int
	db.onNext = func(i int) {
		if i == n-1 {
			writtenBeforeLast = out.Len()
		}
	}
	err := Run(context.Background(), registry(t), &easylist.Runtime{Driver: fakeDriver{}, DB: db},
		easylist.ExportRequest{List: "users", Fields: []string{"name"}, Format: easylist.CSV}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if len(db.queries) != 1 {
		t.Errorf("queries = %d, want 1", len(db.queries))
	}
	if writtenBeforeLast < out.Len()/2 {
		t.Errorf("only %d of %d bytes written before the last row was read", writtenBeforeLast, out.Len())
	}
}

func TestStopsOnErrors(t *testing.T) {
	t.Run("calculated field", func(t *testing.T) {
		rows := sampleRows()
		for i := range 600 {
			rows = append(rows, map[string]any{"id": int64(10 + i), "name": "ok"})
		}
		rows[499+3]["name"] = "Boom"
		db := &fakeDB{rows: rows}
		read := 0
		db.onNext = func(i int) { read = i }
		_, err := run(t, db, easylist.ExportRequest{List: "users", Fields: []string{"display"}, Format: easylist.CSV})
		if err == nil || !strings.Contains(err.Error(), `"display"`) || !strings.Contains(err.Error(), "boom") {
			t.Fatalf("err = %v", err)
		}
		if read > 503 {
			t.Errorf("kept reading to row %d", read)
		}
	})

	t.Run("query", func(t *testing.T) {
		dbErr := errors.New("relation does not exist")
		_, err := run(t, &fakeDB{err: dbErr}, easylist.ExportRequest{List: "users", Fields: []string{"name"}, Format: easylist.XLSX})
		if !errors.Is(err, dbErr) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		db := &fakeDB{rows: sampleRows()}
		db.onNext = func(i int) {
			if i == 1 {
				cancel()
			}
		}
		err := Run(ctx, registry(t), &easylist.Runtime{Driver: fakeDriver{}, DB: db},
			easylist.ExportRequest{List: "users", Fields: []string{"name"}, Format: easylist.CSV}, &bytes.Buffer{})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("writer", func(t *testing.T) {
		wErr := errors.New("disk full")
		db := &fakeDB{}
		for i := range 2000 {
			db.rows = append(db.rows, map[string]any{"id": int64(i), "name": strings.Repeat("x", 40)})
		}
		err := Run(context.Background(), registry(t), &easylist.Runtime{Driver: fakeDriver{}, DB: db},
			easylist.ExportRequest{List: "users", Fields: []string{"name"}, Format: easylist.CSV}, failingWriter{wErr})
		if !errors.Is(err, wErr) {
			t.Fatalf("err = %v", err)
		}
	})
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestXLSX(t *testing.T) {
	out, err := run(t, &fakeDB{rows: sampleRows()}, easylist.ExportRequest{List: "users", Fields: allFields, Format: easylist.XLSX})
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if sheets := f.GetSheetList(); len(sheets) != 1 || sheets[0] != "users" {
		t.Fatalf("sheets = %v", sheets)
	}
	type want struct {
		cell, value string
		typ         excelize.CellType
	}
	cases := []want{
		{"A1", "id", excelize.CellTypeInlineString},
		{"B1", "Full name", excelize.CellTypeInlineString},
		{"A2", "1", excelize.CellTypeUnset},
		{"B2", `Smith, "Jr"`, excelize.CellTypeInlineString},
		{"C2", "30", excelize.CellTypeUnset},
		{"D2", "1.5", excelize.CellTypeUnset},
		{"E2", "12345678901234567890.10", excelize.CellTypeInlineString},
		{"F2", "1", excelize.CellTypeBool},
		{"I2", "0b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b", excelize.CellTypeInlineString},
		{"J2", "on_hold", excelize.CellTypeInlineString},
		{"K2", "FR", excelize.CellTypeInlineString},
		{"L2", `<Smith, "Jr">`, excelize.CellTypeInlineString},
		{"B3", "=SUM(A1:A9)", excelize.CellTypeInlineString},
		{"C3", "", excelize.CellTypeUnset},
		{"F3", "0", excelize.CellTypeBool},
		{"G3", "", excelize.CellTypeUnset},
		{"K3", "", excelize.CellTypeUnset},
		{"A4", "9007199254740993", excelize.CellTypeInlineString},
		{"C4", "-9007199254740992", excelize.CellTypeUnset},
	}
	for _, c := range cases {
		v, err := f.GetCellValue("users", c.cell, excelize.Options{RawCellValue: true})
		if err != nil {
			t.Fatal(err)
		}
		typ, err := f.GetCellType("users", c.cell)
		if err != nil {
			t.Fatal(err)
		}
		if v != c.value || typ != c.typ {
			t.Errorf("%s = %q (type %v), want %q (type %v)", c.cell, v, typ, c.value, c.typ)
		}
	}
	if formula, _ := f.GetCellFormula("users", "B3"); formula != "" {
		t.Errorf("B3 has formula %q", formula)
	}
	for _, c := range []struct {
		cell   string
		want   time.Time
		numFmt int
	}{{"G2", born, numFmtDate}, {"H2", created.UTC(), numFmtDateTime}} {
		raw, _ := f.GetCellValue("users", c.cell, excelize.Options{RawCellValue: true})
		var serial float64
		fmt.Sscan(raw, &serial)
		// Spreadsheet serial dates count days from 1899-12-30.
		want := float64(c.want.Sub(time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC))) / float64(24*time.Hour)
		if math.Abs(serial-want) > 1e-9 {
			t.Errorf("%s = %s, want serial %v for %v", c.cell, raw, want, c.want)
		}
		styleID, _ := f.GetCellStyle("users", c.cell)
		style, _ := f.GetStyle(styleID)
		if style == nil || style.NumFmt != c.numFmt {
			t.Errorf("%s style = %+v, want number format %d", c.cell, style, c.numFmt)
		}
	}
}

func TestInvalidFormat(t *testing.T) {
	_, err := run(t, &fakeDB{}, easylist.ExportRequest{List: "users", Fields: []string{"name"}, Format: "PDF"})
	if err == nil || !strings.Contains(err.Error(), "PDF") {
		t.Fatalf("err = %v", err)
	}
}
