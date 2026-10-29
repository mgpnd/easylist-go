package sqlgorm

import (
	"regexp"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/callbacks"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"
)

// dialector is the private PostgreSQL dialector of the driver's own gorm
// handle. It only numbers placeholders and quotes identifiers the way
// PostgreSQL does; it never opens a connection, so the driver needs neither
// gorm.io/driver/postgres nor a wire driver.
type dialector struct{}

var _ gorm.Dialector = dialector{}

func (dialector) Name() string { return "easylist-postgres" }

// Initialize registers gorm's default callbacks, whose query callback builds
// the statement. The handle has no connection pool.
func (dialector) Initialize(db *gorm.DB) error {
	callbacks.RegisterDefaultCallbacks(db, &callbacks.Config{})
	return nil
}

// Migrator returns gorm's generic migrator. The handle is private and only
// ever builds SELECT statements, so it is never used.
func (d dialector) Migrator(db *gorm.DB) gorm.Migrator {
	return migrator.Migrator{Config: migrator.Config{DB: db, Dialector: d}}
}

// DataTypeOf is unused: the driver declares no models.
func (dialector) DataTypeOf(*schema.Field) string { return "" }

// DefaultValueOf is unused: the driver declares no models.
func (dialector) DefaultValueOf(*schema.Field) clause.Expression {
	return clause.Expr{SQL: "DEFAULT"}
}

// BindVarTo writes $n for the value gorm has just appended to stmt.Vars.
func (dialector) BindVarTo(w clause.Writer, stmt *gorm.Statement, _ any) {
	w.WriteByte('$')
	w.WriteString(strconv.Itoa(len(stmt.Vars)))
}

// QuoteTo writes name as one quoted PostgreSQL identifier, doubling any
// quote inside it.
func (dialector) QuoteTo(w clause.Writer, name string) {
	w.WriteByte('"')
	w.WriteString(strings.ReplaceAll(name, `"`, `""`))
	w.WriteByte('"')
}

var numericPlaceholder = regexp.MustCompile(`\$(\d+)`)

// Explain inlines vars for gorm's logger, which the handle discards.
func (dialector) Explain(sql string, vars ...any) string {
	return logger.ExplainSQL(sql, numericPlaceholder, `'`, vars...)
}

// db is the driver's gorm handle, opened once without a connection. DryRun
// makes the query callback build the statement without running it. It is
// never handed out, so no application configuration, plugin, or callback
// reaches it, and each Compile starts a new session from it.
var db = mustOpen()

func mustOpen() *gorm.DB {
	db, err := gorm.Open(dialector{}, &gorm.Config{
		DryRun:                 true,
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 logger.Discard,
	})
	if err != nil {
		panic("sqlgorm: open gorm: " + err.Error())
	}
	return db
}
