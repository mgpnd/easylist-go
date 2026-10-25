package easylist

import (
	"context"
	"fmt"
	"time"
)

// Row is a read-only view of one scanned row. Its SQL values use the Go
// representation of their field type; NULL and unselected fields are absent.
// Typed accessors report ok=false when the value is absent or has another type.
type Row struct {
	list   *CompiledList
	values map[string]any
}

// NewRow builds a row for list l from SQL field values, for tests and
// alternative executors. Nil values are absent. Values must already have the
// Go representation of their field type.
func NewRow(l *CompiledList, values map[string]any) Row {
	r := Row{list: l, values: make(map[string]any, len(values))}
	for k, v := range values {
		if v != nil {
			r.values[k] = v
		}
	}
	return r
}

// Value returns the value of an SQL field.
func (r Row) Value(name string) (any, bool) {
	v, ok := r.values[name]
	return v, ok
}

// String returns a text, numeric, uuid, or enum value.
func (r Row) String(name string) (string, bool) { return rowValue[string](r, name) }

// Int64 returns an integer value.
func (r Row) Int64(name string) (int64, bool) { return rowValue[int64](r, name) }

// Float64 returns a float value.
func (r Row) Float64(name string) (float64, bool) { return rowValue[float64](r, name) }

// Bool returns a boolean value.
func (r Row) Bool(name string) (bool, bool) { return rowValue[bool](r, name) }

// Time returns a date or timestamp value.
func (r Row) Time(name string) (time.Time, bool) { return rowValue[time.Time](r, name) }

func rowValue[T any](r Row, name string) (T, bool) {
	v, ok := r.values[name].(T)
	return v, ok
}

// Calc evaluates calculated field name for this row. It returns the field's
// Go representation, or nil for NULL from a nullable field. Errors name the field.
func (r Row) Calc(ctx context.Context, name string) (any, error) {
	if r.list == nil {
		return nil, fmt.Errorf("calculated field %q: row has no list", name)
	}
	f, ok := r.list.fields[name]
	if !ok || f.calc == nil {
		return nil, fmt.Errorf("list %q has no calculated field %q", r.list.def.Name, name)
	}
	v, err := f.calc.fn(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("calculated field %q: %w", name, err)
	}
	return v, nil
}
