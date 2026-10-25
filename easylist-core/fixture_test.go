package easylist

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// usersList is the shared test list: base fields, a joined field, a lateral
// join needed only for sorting, a two-step join chain, and calculated fields.
func usersList() List {
	return List{
		Name: "users", Table: "users", Alias: "u", PrimaryKey: "id",
		Scope: "u.deleted_at IS NULL",
		Fields: []Field{
			{Name: "id", Type: Integer},
			{Name: "name", Type: Text, Title: "Full name"},
			{Name: "age", Type: Integer, Nullable: true},
			{Name: "country", Type: Text, Nullable: true, SQL: "c.name", Joins: []string{"country"}},
			{Name: "region", Type: Text, Nullable: true, SQL: "r.name", Joins: []string{"region"}},
			{Name: "city", Type: Text, Nullable: true, SQL: "ci.name", Joins: []string{"city"}},
			{Name: "last_order", Type: Timestamp, Nullable: true, SQL: "lo.created_at", Joins: []string{"last_order"}},
			{Name: "status", Type: Enum, Values: []string{"active", "blocked"}},
			Calc("display", Text, func(_ context.Context, r Row) (string, error) {
				name, _ := r.String("name")
				country, ok := r.String("country")
				if !ok {
					return name, nil
				}
				return name + " (" + country + ")", nil
			}, "name", "country"),
		},
		Joins: []Join{
			{Key: "country", SQL: "LEFT JOIN countries c ON c.id = u.country_id"},
			{Key: "city", SQL: "LEFT JOIN cities ci ON ci.region_id = r.id AND ci.id = u.city_id", DependsOn: []string{"region"}},
			{Key: "region", SQL: "LEFT JOIN regions r ON r.id = u.region_id"},
			{Key: "last_order", SQL: "LEFT JOIN LATERAL (SELECT created_at FROM orders o WHERE o.user_id = u.id ORDER BY created_at DESC LIMIT 1) lo ON TRUE"},
		},
	}
}

func mustUsers(t *testing.T) *CompiledList {
	t.Helper()
	reg, err := NewRegistry(usersList())
	if err != nil {
		t.Fatal(err)
	}
	l, _ := reg.List("users")
	return l
}

// fakeDriver renders plans in a readable, dialect-free form.
type fakeDriver struct{}

func (fakeDriver) Compile(p *Plan) (string, []any, error) {
	var b strings.Builder
	switch p.Kind {
	case KindCount:
		b.WriteString("COUNT")
	default:
		names := make([]string, len(p.Fields))
		for i, f := range p.Fields {
			names[i] = f.Name
		}
		b.WriteString("SELECT " + strings.Join(names, ","))
	}
	for _, j := range p.Joins {
		b.WriteString(" JOIN " + j.Key)
	}
	return b.String(), nil, nil
}

type call struct {
	query string
	args  []any
}

// fakeDB returns scripted rows per query prefix and records every call.
type fakeDB struct {
	calls  []call
	rows   map[string][][]any // query prefix to rows
	err    error
	scanFn func(row []any, dest []any) error
}

func (db *fakeDB) QueryContext(_ context.Context, query string, args ...any) (Rows, error) {
	db.calls = append(db.calls, call{query, args})
	if db.err != nil {
		return nil, db.err
	}
	for prefix, rows := range db.rows {
		if strings.HasPrefix(query, prefix) {
			return &fakeRows{data: rows}, nil
		}
	}
	return &fakeRows{}, nil
}

type fakeRows struct {
	data    [][]any
	next    int
	err     error
	closed  bool
	onNext  func(i int)
	scanErr error
}

func (r *fakeRows) Next() bool {
	if r.onNext != nil {
		r.onNext(r.next)
	}
	if r.next >= len(r.data) {
		return false
	}
	r.next++
	return true
}

func (r *fakeRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	row := r.data[r.next-1]
	if len(row) != len(dest) {
		return fmt.Errorf("fake rows: %d values for %d destinations", len(row), len(dest))
	}
	for i, d := range dest {
		switch d := d.(type) {
		case sql.Scanner:
			if err := d.Scan(row[i]); err != nil {
				return err
			}
		case *int64:
			*d = row[i].(int64)
		default:
			return errors.New("fake rows: unsupported destination")
		}
	}
	return nil
}

func (r *fakeRows) Err() error   { return r.err }
func (r *fakeRows) Close() error { r.closed = true; return nil }

func intp(n int) *int { return &n }
