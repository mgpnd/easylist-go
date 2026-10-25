package querysql

import (
	"strings"

	"github.com/mgpnd/easylist-sql-goqu/internal/pgident"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
)

// goqu v9 surrounds identifier segments with quotes without escaping their
// contents. Escape decoded catalog names exactly once at the SQL boundary;
// catalog keys and lookup names must retain their original decoded spelling.
func catalogTableIdentifier(table tableMeta) exp.IdentifierExpression {
	return goqu.T(strings.ReplaceAll(table.name, `"`, `""`)).
		Schema(strings.ReplaceAll(table.schema, `"`, `""`))
}

func catalogColumnIdentifier(table tableMeta, column columnMeta) exp.IdentifierExpression {
	identifier := catalogTableIdentifier(table)
	if column.name == "*" {
		// goqu treats Col("*") as a wildcard. This fixed literal is used only
		// for the catalogued column named "*"; no input becomes raw SQL.
		return identifier.Col(goqu.L(`"*"`))
	}
	return identifier.Col(strings.ReplaceAll(column.name, `"`, `""`))
}

func truncateIdentifier(name string) string {
	return pgident.Truncate(name)
}
