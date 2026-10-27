package easylistexport

import (
	"fmt"
	"io"
	"math"
	"time"

	easylist "github.com/mgpnd/easylist-go/easylist-core"

	"github.com/xuri/excelize/v2"
)

// Built-in Excel number formats.
const (
	numFmtDate     = 14 // m/d/yyyy, shown in the reader's locale
	numFmtDateTime = 22 // m/d/yyyy h:mm
)

// maxExactInt is the largest integer a spreadsheet number holds exactly.
const maxExactInt = 1 << 53

// xlsxEncoder writes one worksheet with excelize's StreamWriter, which keeps
// rows in temporary storage rather than memory. The zip file is written to w
// when the export completes.
type xlsxEncoder struct {
	w         io.Writer
	file      *excelize.File
	sheet     *excelize.StreamWriter
	rowNum    int
	dateStyle int
	timeStyle int
	cells     []any
	closed    bool
}

func newXLSX(w io.Writer, listName string) (*xlsxEncoder, error) {
	f := excelize.NewFile()
	e := &xlsxEncoder{w: w, file: f}
	// Sheet names are limited to 31 characters; list names never contain
	// characters a sheet name forbids.
	name := listName
	if len(name) > 31 {
		name = name[:31]
	}
	if err := f.SetSheetName(f.GetSheetName(0), name); err != nil {
		e.abort()
		return nil, err
	}
	var err error
	if e.dateStyle, err = f.NewStyle(&excelize.Style{NumFmt: numFmtDate}); err != nil {
		e.abort()
		return nil, err
	}
	if e.timeStyle, err = f.NewStyle(&excelize.Style{NumFmt: numFmtDateTime}); err != nil {
		e.abort()
		return nil, err
	}
	if e.sheet, err = f.NewStreamWriter(name); err != nil {
		e.abort()
		return nil, err
	}
	return e, nil
}

func (e *xlsxEncoder) header(titles []string) error {
	e.cells = e.cells[:0]
	for _, t := range titles {
		e.cells = append(e.cells, t)
	}
	return e.write()
}

func (e *xlsxEncoder) row(fields []easylist.Field, values []any) error {
	e.cells = e.cells[:0]
	for i, v := range values {
		c, err := e.cell(fields[i], v)
		if err != nil {
			return err
		}
		e.cells = append(e.cells, c)
	}
	return e.write()
}

func (e *xlsxEncoder) write() error {
	e.rowNum++
	ref, err := excelize.CoordinatesToCellName(1, e.rowNum)
	if err != nil {
		return err
	}
	return e.sheet.SetRow(ref, e.cells)
}

// cell maps a value to an excelize cell. Strings become inline text cells,
// never formulas. Values a spreadsheet number cannot hold exactly stay text.
// Arrays are text cells holding their CSV text; an empty array is an empty cell.
func (e *xlsxEncoder) cell(f easylist.Field, v any) (any, error) {
	if f.Array && v != nil {
		s, err := arrayText(f, v)
		if err != nil || s == "" {
			return nil, err
		}
		return s, nil
	}
	switch v := v.(type) {
	case nil:
		return nil, nil
	case string:
		return v, nil
	case int64:
		if v < -maxExactInt || v > maxExactInt {
			return fmt.Sprint(v), nil
		}
		return v, nil
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return formatFloat(v), nil
		}
		return v, nil
	case bool:
		return v, nil
	case time.Time:
		if f.Type == easylist.Date {
			return excelize.Cell{StyleID: e.dateStyle, Value: v}, nil
		}
		return excelize.Cell{StyleID: e.timeStyle, Value: v.UTC()}, nil
	}
	return nil, fmt.Errorf("field %q: unexpected value type %T", f.Name, v)
}

func (e *xlsxEncoder) close() error {
	defer e.abort()
	if err := e.sheet.Flush(); err != nil {
		return err
	}
	_, err := e.file.WriteTo(e.w)
	return err
}

// abort removes excelize's temporary files.
func (e *xlsxEncoder) abort() {
	if !e.closed {
		e.closed = true
		e.file.Close()
	}
}
