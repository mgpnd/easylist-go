package easylist

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Runtime wires a driver and the application's database handle.
type Runtime struct {
	Driver   Driver
	DB       Queryer
	Exporter Exporter // Optional; needed only to accept export requests.
	// Clock returns the reference time for requests without one, in the
	// time zone relative values are evaluated in. Nil means time.Now, the
	// server's local time.
	Clock func() time.Time
	// WeekStart is the first day of the week for requests without one. Nil
	// means Monday.
	WeekStart *time.Weekday
}

// Now returns the clock's current time, or time.Now when there is no clock.
func (rt *Runtime) Now() time.Time {
	if rt.Clock != nil {
		return rt.Clock()
	}
	return time.Now()
}

// Queryer runs a query. The library never opens connections itself.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (Rows, error)
}

// Rows iterates over query results. *sql.Rows satisfies it.
type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close() error
}

// SQLDB adapts a database/sql handle. pgx users can open one with pgx/stdlib.
func SQLDB(db *sql.DB) Queryer { return sqlDB{db} }

type sqlDB struct{ db *sql.DB }

func (s sqlDB) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// Want selects which queries Fetch runs.
type Want struct {
	Rows  bool
	Count bool
}

// Result is a fetched page. Total is set only when the count was requested.
type Result struct {
	Rows  []Row
	Total int64
}

// Fetch plans req and runs the page query when want.Rows is set and the count
// query when want.Count is set. A request without Now or WeekStart gets the
// runtime's. Planning errors are returned before any query runs; database
// errors are wrapped with the list name.
func (rt *Runtime) Fetch(ctx context.Context, l *CompiledList, req Request, want Want) (*Result, error) {
	if req.Now.IsZero() {
		req.Now = rt.Now()
	}
	if req.WeekStart == nil {
		req.WeekStart = rt.WeekStart
	}
	var page, count *Plan
	var err error
	if want.Rows {
		if page, err = l.Plan(KindPage, req); err != nil {
			return nil, err
		}
	}
	if want.Count {
		if count, err = l.Plan(KindCount, req); err != nil {
			return nil, err
		}
	}
	res := &Result{}
	if page != nil {
		err := rt.Stream(ctx, l, page, func(r Row) error {
			res.Rows = append(res.Rows, r)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if count != nil {
		if res.Total, err = rt.Count(ctx, l, count); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// Count runs a count plan.
func (rt *Runtime) Count(ctx context.Context, l *CompiledList, p *Plan) (int64, error) {
	rows, err := rt.query(ctx, l, p)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, l.wrap(err)
		}
		return 0, fmt.Errorf("list %q: count query returned no rows", l.def.Name)
	}
	var n int64
	if err := rows.Scan(&n); err != nil {
		return 0, l.wrap(err)
	}
	if err := rows.Close(); err != nil {
		return 0, l.wrap(err)
	}
	return n, nil
}

// Stream runs a page or export plan and calls fn for each row as it is read.
// It stops at the first error from the database, scanning, fn, or ctx.
func (rt *Runtime) Stream(ctx context.Context, l *CompiledList, p *Plan, fn func(Row) error) error {
	if p.Kind == KindCount {
		return fmt.Errorf("list %q: cannot stream a count plan", l.def.Name)
	}
	rows, err := rt.query(ctx, l, p)
	if err != nil {
		return err
	}
	defer rows.Close()
	holders := make([]any, len(p.Fields))
	for i, f := range p.Fields {
		holders[i] = newHolder(f)
	}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := rows.Scan(holders...); err != nil {
			return l.wrap(err)
		}
		row := Row{list: l, values: make(map[string]any, len(p.Fields))}
		for i, f := range p.Fields {
			v, ok := holderValue(holders[i])
			if !ok {
				if !f.Nullable {
					return fmt.Errorf("list %q: non-nullable field %q is NULL", l.def.Name, f.Name)
				}
				continue
			}
			if f.Array {
				if v, err = decodeArray(f.Type, v.(string)); err != nil {
					return fmt.Errorf("list %q: field %q: %w", l.def.Name, f.Name, err)
				}
			}
			if err := checkEnumValue(l.fields[f.Name], v); err != nil {
				return fmt.Errorf("list %q: field %q: %w", l.def.Name, f.Name, err)
			}
			row.values[f.Name] = v
		}
		if err := fn(row); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return l.wrap(err)
	}
	return l.wrap(rows.Close())
}

func (rt *Runtime) query(ctx context.Context, l *CompiledList, p *Plan) (Rows, error) {
	if rt.Driver == nil || rt.DB == nil {
		return nil, fmt.Errorf("list %q: runtime needs a driver and a database", l.def.Name)
	}
	query, args, err := rt.Driver.Compile(p)
	if err != nil {
		return nil, err
	}
	rows, err := rt.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, l.wrap(err)
	}
	return rows, nil
}

// wrap names the list without adding bound argument values.
func (c *CompiledList) wrap(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("list %q: %w", c.def.Name, err)
}

// newHolder returns the scan destination for f. Arrays arrive as JSON text.
func newHolder(f PlanField) any {
	if f.Array {
		return new(sql.NullString)
	}
	switch f.Type {
	case Integer:
		return new(sql.NullInt64)
	case Float:
		return new(sql.NullFloat64)
	case Boolean:
		return new(sql.NullBool)
	case Date, Timestamp:
		return new(sql.NullTime)
	}
	return new(sql.NullString)
}

func holderValue(h any) (any, bool) {
	switch h := h.(type) {
	case *sql.NullString:
		return h.String, h.Valid
	case *sql.NullInt64:
		return h.Int64, h.Valid
	case *sql.NullFloat64:
		return h.Float64, h.Valid
	case *sql.NullBool:
		return h.Bool, h.Valid
	case *sql.NullTime:
		return h.Time, h.Valid
	}
	return nil, false
}
