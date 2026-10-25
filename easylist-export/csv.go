package easylistexport

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"strconv"
	"time"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
)

// csvEncoder writes UTF-8, comma-separated, RFC 4180 quoted CSV. Values are
// written unchanged, so cells starting with =, +, -, or @ can be read as
// formulas by spreadsheet tools.
type csvEncoder struct {
	w      *csv.Writer
	record []string
}

func newCSV(w io.Writer) *csvEncoder { return &csvEncoder{w: csv.NewWriter(w)} }

func (e *csvEncoder) header(titles []string) error {
	return e.write(titles)
}

func (e *csvEncoder) row(fields []easylist.Field, values []any) error {
	e.record = e.record[:0]
	for i, v := range values {
		s, err := csvValue(fields[i], v)
		if err != nil {
			return err
		}
		e.record = append(e.record, s)
	}
	return e.write(e.record)
}

// write lets csv.Writer buffer output; it reaches w whenever the buffer fills.
func (e *csvEncoder) write(record []string) error {
	if err := e.w.Write(record); err != nil {
		return err
	}
	return e.w.Error()
}

func (e *csvEncoder) close() error {
	e.w.Flush()
	return e.w.Error()
}

func (*csvEncoder) abort() {}

func csvValue(f easylist.Field, v any) (string, error) {
	if v == nil {
		return "", nil
	}
	switch v := v.(type) {
	case string:
		return v, nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case float64:
		return formatFloat(v), nil
	case bool:
		return strconv.FormatBool(v), nil
	case time.Time:
		if f.Type == easylist.Date {
			return v.Format(time.DateOnly), nil
		}
		return v.UTC().Format(time.RFC3339Nano), nil
	}
	return "", fmt.Errorf("field %q: unexpected value type %T", f.Name, v)
}

// formatFloat uses the shortest exact decimal, switching to exponent form
// for very large or small magnitudes, as encoding/json does.
func formatFloat(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return strconv.FormatFloat(v, 'g', -1, 64)
	}
	if abs := math.Abs(v); abs != 0 && (abs < 1e-6 || abs >= 1e21) {
		return strconv.FormatFloat(v, 'e', -1, 64)
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}
