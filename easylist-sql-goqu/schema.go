package querysql

import (
	"fmt"
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

type columnKind uint8

const (
	kindText columnKind = iota
	kindInteger
	kindFloat
	kindNumeric
	kindDate
	kindBoolean
	kindUnsupported
)

type tableKey struct{ schema, name string }
type tableMeta struct {
	schema, name string
	columns      map[string]columnMeta
	unresolved   bool
}
type columnMeta struct {
	name            string
	kind            columnKind
	arrayDimensions int
}

func parseSchema(schemaSQL string) (map[tableKey]tableMeta, error) {
	parsed, err := pg_query.Parse(schemaSQL)
	if err != nil {
		return nil, fmt.Errorf("parse schema: %w", err)
	}
	tables := make(map[tableKey]tableMeta)
	for _, statement := range parsed.Stmts {
		create := statement.Stmt.GetCreateStmt()
		if create == nil {
			continue
		}
		relation := create.Relation
		key := tableKey{schema: relation.Schemaname, name: relation.Relname}
		if _, exists := tables[key]; exists {
			return nil, fmt.Errorf("duplicate table %q.%q", key.schema, key.name)
		}
		table := tableMeta{schema: key.schema, name: key.name, columns: make(map[string]columnMeta),
			unresolved: len(create.InhRelations) > 0 || create.Partbound != nil || create.OfTypename != nil}
		for _, element := range create.TableElts {
			if element.GetTableLikeClause() != nil {
				table.unresolved = true
			}
			column := element.GetColumnDef()
			if column == nil {
				continue
			}
			table.columns[column.Colname] = columnMeta{name: column.Colname, kind: classifyColumnType(column.TypeName), arrayDimensions: len(column.TypeName.GetArrayBounds())}
		}
		tables[key] = table
	}
	return tables, nil
}

func classifyColumnType(typ *pg_query.TypeName) columnKind {
	names := typ.GetNames()
	if len(names) == 0 || len(names) > 2 {
		return kindUnsupported
	}
	if len(names) == 2 && names[0].GetString_().GetSval() != "pg_catalog" {
		return kindUnsupported
	}
	// SQL spelling aliases (for example integer and boolean) have already
	// become canonical names in the AST. Accepting those spellings here would
	// also accept distinct user-defined types such as "integer". Serial pseudo-
	// types stay unsupported because the raw parser does not resolve them.
	switch names[len(names)-1].GetString_().GetSval() {
	case "text", "varchar", "bpchar", "char":
		return kindText
	case "int2", "int4", "int8":
		return kindInteger
	case "float4", "float8":
		return kindFloat
	case "numeric":
		return kindNumeric
	case "date":
		return kindDate
	case "bool":
		return kindBoolean
	default:
		return kindUnsupported
	}
}

func resolveTable(tables map[tableKey]tableMeta, input string) (tableMeta, error) {
	parts, err := parseTableName(input)
	if err != nil {
		return tableMeta{}, err
	}
	var found tableMeta
	matches := 0
	if len(parts) == 2 {
		if table, ok := tables[tableKey{schema: parts[0], name: parts[1]}]; ok {
			found, matches = table, 1
		}
	} else {
		for key, table := range tables {
			if key.name == parts[0] {
				found = table
				matches++
			}
		}
	}
	if matches == 0 {
		return tableMeta{}, fmt.Errorf("unknown table %q", input)
	}
	if matches > 1 {
		return tableMeta{}, fmt.Errorf("ambiguous table %q", input)
	}
	if found.unresolved {
		return tableMeta{}, fmt.Errorf("table %q has unresolved derived columns", input)
	}
	return found, nil
}

// parseTableName accepts one or two PostgreSQL identifiers, preserving quoted
// names and folding ASCII letters in unquoted names as PostgreSQL does in UTF-8.
func parseTableName(input string) ([]string, error) {
	var parts []string
	pos := 0
	skipSpace := func() {
		for pos < len(input) && strings.ContainsRune(" \t\r\n\f\v", rune(input[pos])) {
			pos++
		}
	}
	invalid := func() ([]string, error) { return nil, fmt.Errorf("invalid table name %q", input) }
	for {
		skipSpace()
		if pos == len(input) || len(parts) == 2 {
			return invalid()
		}
		var name strings.Builder
		if input[pos] == '"' {
			pos++
			closed := false
			for pos < len(input) {
				ch := input[pos]
				pos++
				if ch == '"' {
					if pos < len(input) && input[pos] == '"' {
						name.WriteByte('"')
						pos++
						continue
					}
					closed = true
					break
				}
				if ch == 0 {
					return invalid()
				}
				name.WriteByte(ch)
			}
			if !closed || name.Len() == 0 {
				return invalid()
			}
		} else {
			for pos < len(input) {
				ch := input[pos]
				letter := ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch == '_' || ch >= 128
				if !letter && !(name.Len() > 0 && (ch >= '0' && ch <= '9' || ch == '$')) {
					break
				}
				if ch >= 'A' && ch <= 'Z' {
					ch += 'a' - 'A'
				}
				name.WriteByte(ch)
				pos++
			}
			if name.Len() == 0 {
				return invalid()
			}
		}
		parts = append(parts, truncateIdentifier(name.String()))
		skipSpace()
		if pos == len(input) {
			return parts, nil
		}
		if input[pos] != '.' {
			return invalid()
		}
		pos++
	}
}
