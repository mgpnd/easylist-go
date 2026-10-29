package sqljet_test

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
	sqljet "github.com/mgpnd/easylist-go/easylist-sql-jet"

	"github.com/go-jet/jet/v2/postgres"
)

func newRuntime(db *sql.DB) *easylist.Runtime {
	return &easylist.Runtime{
		Driver: sqljet.New(),
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
	return sqljet.New().Compile(plan)
}

func typedLiteral() (string, []any) {
	return postgres.SELECT(postgres.String("Ann")).Sql()
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

	if newRuntime(nil).Driver == nil {
		t.Error("runtime has no driver")
	}

	// jet's typed literal casts its placeholder, which the driver avoids.
	if query, args := typedLiteral(); !strings.Contains(query, "$1::text") || !reflect.DeepEqual(args, []any{"Ann"}) {
		t.Errorf("typedLiteral = %q %#v", query, args)
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
	if want := string(sqlBlock[1]); query != want {
		t.Errorf("README SQL differs from output:\n%s\n%s", want, query)
	}
	if !reflect.DeepEqual(args, []any{"Ann", int64(30), int64(10), int64(0)}) {
		t.Errorf("args = %#v", args)
	}
}
