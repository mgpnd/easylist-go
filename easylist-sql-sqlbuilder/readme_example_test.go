package sqlsqlbuilder_test

// This file holds the Go snippets of README.md verbatim, so they compile.
// TestReadmeSnippets fails when a snippet and this file drift apart.

import (
	"database/sql"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	filter "github.com/mgpnd/easylist-go/easylist-filter"
	sqlsqlbuilder "github.com/mgpnd/easylist-go/easylist-sql-sqlbuilder"

	"github.com/huandu/go-sqlbuilder"
)

func activeUsers() (string, []any) {
	sb := sqlbuilder.PostgreSQL.NewSelectBuilder()
	sb.Select("id", "name").From("users").Where(sb.Equal("active", true))
	return sb.Build()
}

func newRuntime(db *sql.DB) *easylist.Runtime {
	return &easylist.Runtime{
		Driver: sqlsqlbuilder.New(),
		DB:     easylist.SQLDB(db),
	}
}

func showSQL(users *easylist.CompiledList) (string, []any, error) {
	expr, err := filter.Parse("name = 'Ann' AND age < 30")
	if err != nil {
		return "", nil, err
	}
	limit := 10
	plan, err := users.Plan(easylist.KindPage, easylist.Request{
		Fields: []string{"name", "country"},
		Filter: expr,
		Limit:  &limit,
	})
	if err != nil {
		return "", nil, err
	}
	return sqlsqlbuilder.New().Compile(plan)
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
	blocks := regexp.MustCompile("(?s)```go\\n(.*?)```").FindAllSubmatch(readme, -1)
	if len(blocks) == 0 {
		t.Fatal("README has no Go snippets")
	}
	for _, b := range blocks {
		if !strings.Contains(string(self), string(b[1])) {
			t.Errorf("README snippet is not in readme_example_test.go:\n%s", b[1])
		}
	}

	// The application's own go-sqlbuilder query, next to the driver.
	if query, args := activeUsers(); query != "SELECT id, name FROM users WHERE active = $1" || !reflect.DeepEqual(args, []any{true}) {
		t.Errorf("activeUsers = %s %#v", query, args)
	}
	if newRuntime(nil).Driver == nil {
		t.Error("runtime has no driver")
	}

	reg, err := easylist.NewRegistry(easylist.List{
		Name: "users", Table: "users", Alias: "u", PrimaryKey: "id",
		Fields: []easylist.Field{
			{Name: "id", Type: easylist.Integer},
			{Name: "name", Type: easylist.Text},
			{Name: "age", Type: easylist.Integer},
			{Name: "country", Type: easylist.Text, Nullable: true, SQL: "c.name", Joins: []string{"country"}},
		},
		Joins: []easylist.Join{{Key: "country", SQL: "LEFT JOIN countries c ON c.id = u.country_id"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	users, _ := reg.List("users")
	query, args, err := showSQL(users)
	if err != nil {
		t.Fatal(err)
	}
	sqlBlock := regexp.MustCompile("(?s)```sql\\n(.*?)\\n```").FindSubmatch(readme)
	if sqlBlock == nil {
		t.Fatal("README has no SQL snippet")
	}
	if want := strings.ReplaceAll(string(sqlBlock[1]), "\n", " "); query != want {
		t.Errorf("README SQL differs from output:\n%s\n%s", want, query)
	}
	if !reflect.DeepEqual(args, []any{"Ann", int64(30), int64(10), int64(0)}) {
		t.Errorf("args = %#v", args)
	}
}
