package querysql

import (
	filter "github.com/mgpnd/easylist-filter"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
)

// Compiler holds immutable schema metadata and can compile filters concurrently.
type Compiler struct {
	tables map[tableKey]tableMeta
}

// NewCompiler parses PostgreSQL CREATE TABLE definitions for reuse across filters.
func NewCompiler(schemaSQL string) (*Compiler, error) {
	tables, err := parseSchema(schemaSQL)
	if err != nil {
		return nil, err
	}
	return &Compiler{tables: tables}, nil
}

// Compile validates the whole filter and returns a prepared PostgreSQL SELECT.
// Errors always return an empty SQL string and nil arguments.
func (c *Compiler) Compile(table, input string) (sql string, args []any, err error) {
	meta, err := resolveTable(c.tables, table)
	if err != nil {
		return "", nil, err
	}
	expr, err := filter.Parse(input)
	if err != nil {
		return "", nil, err
	}
	return compileFilter(meta, expr)
}

// CompileAST validates a parsed filter against the selected table and returns a
// prepared PostgreSQL SELECT. The AST can be reused concurrently. A nil AST is
// an error; all errors return an empty SQL string and nil arguments.
func (c *Compiler) CompileAST(table string, expr filter.Expr) (sql string, args []any, err error) {
	meta, err := resolveTable(c.tables, table)
	if err != nil {
		return "", nil, err
	}
	return compileFilter(meta, expr)
}

func compileFilter(meta tableMeta, root filter.Expr) (sql string, args []any, err error) {
	expr, err := lowerFilter(meta, root)
	if err != nil {
		return "", nil, err
	}
	tableIdentifier := catalogTableIdentifier(meta)
	sql, args, err = goqu.Dialect("postgres").From(tableIdentifier).Where(expr).Prepared(true).ToSQL()
	if err != nil {
		return "", nil, err
	}
	return sql, args, nil
}

// Convert parses a PostgreSQL schema and compiles one filter into a prepared SELECT.
func Convert(schemaSQL, table, filter string) (sql string, args []any, err error) {
	compiler, err := NewCompiler(schemaSQL)
	if err != nil {
		return "", nil, err
	}
	return compiler.Compile(table, filter)
}
