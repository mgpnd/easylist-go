package sqlgorm_test

// This file holds the Go snippets of README.md verbatim, so they compile.
// TestReadmeSnippets fails when a snippet and this file drift apart.

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	filter "github.com/mgpnd/easylist-go/easylist-filter"
	sqlgorm "github.com/mgpnd/easylist-go/easylist-sql-gorm"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func newRuntime(gormDB *gorm.DB) (*easylist.Runtime, error) {
	db, err := gormDB.DB()
	if err != nil {
		return nil, err
	}
	return &easylist.Runtime{
		Driver: sqlgorm.New(),
		DB:     easylist.SQLDB(db),
	}, nil
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
	return sqlgorm.New().Compile(plan)
}

// offline is a database/sql connector that never connects, standing in for
// an application's PostgreSQL connection.
type offline struct{}

func (offline) Connect(context.Context) (driver.Conn, error) { return nil, errors.New("offline") }
func (offline) Driver() driver.Driver                        { return nil }

// appDialector stands in for an application's gorm dialector, such as
// gorm.io/driver/postgres, over an existing *sql.DB.
type appDialector struct{ db *sql.DB }

func (d appDialector) Name() string                                      { return "app" }
func (d appDialector) Initialize(db *gorm.DB) error                      { db.ConnPool = d.db; return nil }
func (appDialector) Migrator(*gorm.DB) gorm.Migrator                     { return nil }
func (appDialector) DataTypeOf(*schema.Field) string                     { return "" }
func (appDialector) DefaultValueOf(*schema.Field) clause.Expression      { return nil }
func (appDialector) BindVarTo(w clause.Writer, _ *gorm.Statement, _ any) { w.WriteByte('?') }
func (appDialector) QuoteTo(w clause.Writer, s string)                   { w.WriteString(s) }
func (appDialector) Explain(sql string, _ ...any) string                 { return sql }

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

	db := sql.OpenDB(offline{})
	defer db.Close()
	gormDB, err := gorm.Open(appDialector{db}, &gorm.Config{DisableAutomaticPing: true, Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	if rt, err := newRuntime(gormDB); err != nil || rt.Driver == nil || rt.DB == nil {
		t.Errorf("runtime = %+v, %v", rt, err)
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
	if want := strings.ReplaceAll(string(sqlBlock[1]), "\n", " "); query != want {
		t.Errorf("README SQL differs from output:\n%s\n%s", want, query)
	}
	if !reflect.DeepEqual(args, []any{"Ann", int64(30), int64(10), int64(0)}) {
		t.Errorf("args = %#v", args)
	}
}
