package easylist_test

// This file holds the Go snippets of README.md verbatim, so they compile.
// TestReadmeSnippets fails when a snippet and this file drift apart.

import (
	"context"
	"database/sql"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	filter "github.com/mgpnd/easylist-go/easylist-filter"
)

var Users = easylist.List{
	Name:       "users",
	Table:      "users",
	Alias:      "u",
	PrimaryKey: "id",
	Scope:      "u.deleted_at IS NULL",
	Fields: []easylist.Field{
		{Name: "id", Type: easylist.Integer},
		{Name: "name", Type: easylist.Text, Title: "Full name"},
		{Name: "status", Type: easylist.Enum, Values: []string{"active", "blocked"}},
		{Name: "tags", Type: easylist.Text, Array: true, Nullable: true},
		{Name: "country", Type: easylist.Text, Nullable: true, SQL: "c.name", Joins: []string{"country"}},
		{Name: "last_order", Type: easylist.Timestamp, Nullable: true, SQL: "lo.created_at", Joins: []string{"last_order"}},
		easylist.Calc("display", easylist.Text, func(ctx context.Context, r easylist.Row) (string, error) {
			name, _ := r.String("name")
			if country, ok := r.String("country"); ok {
				return name + " (" + country + ")", nil
			}
			return name, nil
		}, "name", "country"),
	},
	Joins: []easylist.Join{
		{Key: "country", SQL: "LEFT JOIN countries c ON c.id = u.country_id"},
		{Key: "last_order", SQL: `LEFT JOIN LATERAL (
			SELECT o.created_at FROM orders o WHERE o.user_id = u.id ORDER BY o.created_at DESC LIMIT 1
		) lo ON TRUE`},
	},
}

var Labels = easylist.Calc("labels", easylist.Text, func(ctx context.Context, r easylist.Row) ([]string, error) {
	tags, _ := r.Strings("tags")
	return append([]string{"user"}, tags...), nil
}, "tags")

func newRegistry() (*easylist.Registry, error) {
	return easylist.NewRegistry(Users)
}

func firstPage(ctx context.Context, reg *easylist.Registry, db *sql.DB, driver easylist.Driver) ([]easylist.Row, int64, error) {
	rt := &easylist.Runtime{Driver: driver, DB: easylist.SQLDB(db)}
	expr, err := filter.Parse("name CONTAINS 'an' AND status = 'active'")
	if err != nil {
		return nil, 0, err
	}
	users, _ := reg.List("users")
	limit := 20
	res, err := rt.Fetch(ctx, users, easylist.Request{
		Fields: []string{"name", "display"},
		Filter: expr,
		Sort:   []easylist.Sort{{Field: "last_order", Direction: easylist.Desc}},
		Limit:  &limit,
	}, easylist.Want{Rows: true, Count: true})
	if err != nil {
		return nil, 0, err
	}
	for _, row := range res.Rows {
		if _, err := row.Calc(ctx, "display"); err != nil {
			return nil, 0, err
		}
	}
	return res.Rows, res.Total, nil
}

func newRuntime(driver easylist.Driver, db *sql.DB) *easylist.Runtime {
	sunday := time.Sunday
	return &easylist.Runtime{
		Driver:    driver,
		DB:        easylist.SQLDB(db),
		Clock:     func() time.Time { return time.Now().UTC() },
		WeekStart: &sunday,
	}
}

func TestReadmeSnippets(t *testing.T) {
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	self, err := os.ReadFile("readme_example_test.go")
	if err != nil {
		t.Fatal(err)
	}
	blocks := regexp.MustCompile("(?s)```go\n(.*?)```").FindAllSubmatch(readme, -1)
	if len(blocks) == 0 {
		t.Fatal("README has no Go snippets")
	}
	for _, b := range blocks {
		if !strings.Contains(string(self), string(b[1])) {
			t.Errorf("README snippet is not in readme_example_test.go:\n%s", b[1])
		}
	}
	if rt := newRuntime(nil, nil); rt.WeekStart == nil || *rt.WeekStart != time.Sunday || rt.Now().Location() != time.UTC {
		t.Errorf("runtime = %+v", rt)
	}
	if _, err := newRegistry(); err != nil {
		t.Fatal(err)
	}
	withLabels := Users
	withLabels.Fields = append(slices.Clone(Users.Fields), Labels)
	if _, err := easylist.NewRegistry(withLabels); err != nil {
		t.Fatal(err)
	}
}
