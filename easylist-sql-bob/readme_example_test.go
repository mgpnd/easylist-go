package sqlbob_test

// This file holds the Go snippets of README.md verbatim, so they compile.
// TestReadmeSnippets fails when a snippet and this file drift apart.

import (
	"context"
	"database/sql/driver"
	"errors"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	filter "github.com/mgpnd/easylist-go/easylist-filter"
	sqlbob "github.com/mgpnd/easylist-go/easylist-sql-bob"

	"github.com/stephenafamo/bob"
)

func newRuntime(db bob.DB) *easylist.Runtime {
	return &easylist.Runtime{
		Driver: sqlbob.New(),
		DB:     easylist.SQLDB(db.DB),
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
	return sqlbob.New().Compile(plan)
}

// offline is a database/sql connector that never connects, standing in for
// an application's PostgreSQL connection.
type offline struct{}

func (offline) Connect(context.Context) (driver.Conn, error) { return nil, errors.New("offline") }
func (offline) Driver() driver.Driver                        { return nil }

// trailingSpace matches the spaces bob leaves at the end of a line, which a
// Markdown file does not keep.
var trailingSpace = regexp.MustCompile(`(?m) +$`)

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

	// The bob handle an application would build its queries against.
	db := bob.OpenDB(offline{})
	defer db.Close()
	if rt := newRuntime(db); rt.Driver == nil || rt.DB == nil {
		t.Errorf("runtime = %+v", rt)
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
	sqlBlock := regexp.MustCompile("(?s)```sql\\n(.*?)```").FindSubmatch(readme)
	if sqlBlock == nil {
		t.Fatal("README has no SQL snippet")
	}
	if got := trailingSpace.ReplaceAllString(query, ""); got != string(sqlBlock[1]) {
		t.Errorf("README SQL differs from output:\n%s\n%s", sqlBlock[1], got)
	}
	if !reflect.DeepEqual(args, []any{"Ann", int64(30), int64(10), int64(0)}) {
		t.Errorf("args = %#v", args)
	}
}
