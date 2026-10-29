package sqlgorm

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	easylist "github.com/mgpnd/easylist-go/easylist-core"

	"gorm.io/gorm"
	"gorm.io/gorm/callbacks"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// appDialector stands in for an application's dialector. It binds ? and
// quotes with backticks, so its output differs from the driver's everywhere.
type appDialector struct{}

func (appDialector) Name() string { return "app" }
func (appDialector) Initialize(db *gorm.DB) error {
	callbacks.RegisterDefaultCallbacks(db, &callbacks.Config{})
	return nil
}
func (appDialector) Migrator(*gorm.DB) gorm.Migrator                     { return nil }
func (appDialector) DataTypeOf(*schema.Field) string                     { return "" }
func (appDialector) DefaultValueOf(*schema.Field) clause.Expression      { return nil }
func (appDialector) BindVarTo(w clause.Writer, _ *gorm.Statement, _ any) { w.WriteByte('?') }
func (appDialector) QuoteTo(w clause.Writer, s string)                   { w.WriteString("`" + s + "`") }
func (appDialector) Explain(sql string, _ ...any) string                 { return sql }

// tenantPlugin adds a tenant condition, a limit, and an ORDER BY to every
// query of the handle it is installed on, as multi-tenancy plugins do.
type tenantPlugin struct{}

func (tenantPlugin) Name() string { return "tenant" }
func (tenantPlugin) Initialize(db *gorm.DB) error {
	return db.Callback().Query().Before("gorm:query").Register("tenant:scope", func(tx *gorm.DB) {
		limit := 1
		tx.Statement.AddClause(clause.Where{Exprs: []clause.Expression{clause.Eq{Column: clause.Column{Name: "tenant_id"}, Value: 7}}})
		tx.Statement.AddClause(clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "tenant_id"}}}})
		tx.Statement.AddClause(clause.Limit{Limit: &limit})
	})
}

// The driver's output does not depend on any gorm handle the application
// opens: its dialector, naming strategy, clause builders, plugins, and
// callbacks stay on that handle.
func TestCompileIgnoresApplicationGorm(t *testing.T) {
	scoped := usersList
	scoped.Scope = "u.deleted_at IS NULL"
	plans := []*easylist.Plan{
		plan(t, scoped, easylist.KindPage, easylist.Request{Fields: []string{"name", "country"}, Limit: intp(10), Offset: intp(0)}, "name = 'Ann' AND (age < 30 OR status IN ('a', 'b'))"),
		plan(t, scoped, easylist.KindExport, easylist.Request{Fields: []string{"name"}}, "country != 'FR'"),
		plan(t, scoped, easylist.KindCount, easylist.Request{}, "age BETWEEN '1,2'"),
	}
	compileAll := func() []string {
		out := make([]string, len(plans))
		for i, p := range plans {
			sql, args, err := New().Compile(p)
			if err != nil {
				t.Fatal(err)
			}
			out[i] = fmt.Sprintf("%s %#v", sql, args)
		}
		return out
	}
	before := compileAll()

	app, err := gorm.Open(appDialector{}, &gorm.Config{
		NamingStrategy:       schema.NamingStrategy{TablePrefix: "app_", SingularTable: true, NoLowerCase: true},
		DisableAutomaticPing: true,
		Logger:               logger.Discard,
		ClauseBuilders: map[string]clause.ClauseBuilder{
			"WHERE": func(c clause.Clause, b clause.Builder) {
				b.WriteString("/* app */ ")
				c.Build(b)
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Use(tenantPlugin{}); err != nil {
		t.Fatal(err)
	}
	// The application's handle does rewrite its own queries.
	type User struct{ ID int64 }
	stmt := app.Session(&gorm.Session{DryRun: true}).Where("name = ?", "x").Find(&[]User{}).Statement
	got := stmt.SQL.String()
	for _, part := range []string{"`app_User`", "/* app */", "`tenant_id`", "LIMIT ?"} {
		if !strings.Contains(got, part) {
			t.Errorf("application query %q lacks %q", got, part)
		}
	}

	if after := compileAll(); !reflect.DeepEqual(after, before) {
		t.Errorf("output changed with an application gorm handle:\n%v\n%v", after, before)
	}
}
