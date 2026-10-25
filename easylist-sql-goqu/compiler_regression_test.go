package querysql

import (
	"reflect"
	"strings"
	"testing"

	pg_query "github.com/pganalyze/pg_query_go/v6"
)

// These cases catch missing escaping at each identifier boundary, including a
// name that would otherwise turn one equality into an OR expression.
func TestCompileEscapesIdentifierQuotes(t *testing.T) {
	for _, tc := range []struct {
		name, ddl, table, filter, wantSQL string
		wantSchema, wantTable, wantColumn string
	}{
		{"column injection", `CREATE TABLE t (name text, "name"" = $1 OR TRUE OR ""name" text)`, "t", `"name"" = $1 OR TRUE OR ""name" = 'value'`,
			`SELECT * FROM "t" WHERE ("t"."name"" = $1 OR TRUE OR ""name" = $1)`, "", "t", `name" = $1 OR TRUE OR "name`},
		{"quoted segments", `CREATE TABLE "s""?;--"."t"".x" ("a""?b" text)`, `"s""?;--"."t"".x"`, `"a""?b" = 'value'`,
			`SELECT * FROM "s""?;--"."t"".x" WHERE ("s""?;--"."t"".x"."a""?b" = $1)`, `s"?;--`, `t".x`, `a"?b`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Convert(tc.ddl, tc.table, tc.filter)
			if err != nil || sql != tc.wantSQL || !reflect.DeepEqual(args, []any{"value"}) {
				t.Errorf("got %q %#v %v; want %q with bound value", sql, args, err, tc.wantSQL)
			}
			assertParsedEquality(t, sql, tc.wantSchema, tc.wantTable, tc.wantColumn)
		})
	}
}

func TestCompileLiteralStarColumn(t *testing.T) {
	for _, tc := range []struct{ ddl, table, schema, name, want string }{
		{`CREATE TABLE t ("*" text)`, "t", "", "t", `SELECT * FROM "t" WHERE ("t"."*" = $1)`},
		{`CREATE TABLE "s""?"."*" ("*" text)`, `"s""?"."*"`, `s"?`, "*", `SELECT * FROM "s""?"."*" WHERE ("s""?"."*"."*" = $1)`},
	} {
		t.Run(tc.table, func(t *testing.T) {
			sql, args, err := Convert(tc.ddl, tc.table, `"*" = 'value'`)
			if err != nil || sql != tc.want || !reflect.DeepEqual(args, []any{"value"}) {
				t.Errorf("got %q %#v %v; want %q with bound value", sql, args, err, tc.want)
			}
			assertParsedEquality(t, sql, tc.schema, tc.name, "*")
		})
	}
}

func assertParsedEquality(t *testing.T, sql, schema, table, column string) {
	t.Helper()
	parsed, err := pg_query.Parse(sql)
	if err != nil {
		t.Fatalf("generated SQL does not parse: %v", err)
	}
	if len(parsed.Stmts) != 1 {
		t.Fatalf("generated %d statements, want one", len(parsed.Stmts))
	}
	selectStmt := parsed.Stmts[0].Stmt.GetSelectStmt()
	if selectStmt == nil || len(selectStmt.FromClause) != 1 {
		t.Fatalf("expected one SELECT relation: %v", parsed)
	}
	relation := selectStmt.FromClause[0].GetRangeVar()
	if relation.GetSchemaname() != schema || relation.GetRelname() != table {
		t.Errorf("relation = %v; want %q.%q", relation, schema, table)
	}
	equality := selectStmt.WhereClause.GetAExpr()
	if equality == nil || len(equality.Name) != 1 || equality.Name[0].GetString_().GetSval() != "=" || equality.Rexpr.GetParamRef().GetNumber() != 1 {
		t.Fatalf("expected a single equality to $1, got %v", selectStmt.WhereClause)
	}
	ref := equality.Lexpr.GetColumnRef()
	wantFields := []string{table, column}
	if schema != "" {
		wantFields = append([]string{schema}, wantFields...)
	}
	if ref == nil || len(ref.Fields) != len(wantFields) {
		t.Fatalf("column = %v; want segments %q", ref, wantFields)
	}
	for i, want := range wantFields {
		if ref.Fields[i].GetString_() == nil || ref.Fields[i].GetString_().GetSval() != want {
			t.Errorf("column segment %d = %v; want identifier %q", i, ref.Fields[i], want)
		}
	}
}

func TestCompileRejectsMultidimensionalArrays(t *testing.T) {
	for _, declaration := range []string{"integer[][]", "text[2][3]", "numeric[][][]"} {
		t.Run(declaration, func(t *testing.T) {
			compiler, err := NewCompiler("CREATE TABLE t (id integer, matrix " + declaration + ")")
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := compiler.tables[tableKey{name: "t"}].columns["matrix"]; !ok {
				t.Fatal("multidimensional column was dropped from catalog")
			}
			if _, _, err := compiler.Compile("t", "id = 1"); err != nil {
				t.Fatalf("unrelated scalar filter: %v", err)
			}
			value := "1"
			if strings.HasPrefix(declaration, "text") {
				value = "'x'"
			}
			for _, op := range []string{"CONTAINS", "EXCLUDES", "="} {
				sql, args, err := compiler.Compile("t", "matrix "+op+" "+value)
				if err == nil || !strings.Contains(err.Error(), "unsupported") || !strings.Contains(err.Error(), "dimension") || !strings.Contains(err.Error(), "matrix") || sql != "" || args != nil {
					t.Errorf("%s: got %q %#v %v; want atomic unsupported dimension error", op, sql, args, err)
				}
			}
		})
	}
}

func TestCompileTruncatesOverlengthIdentifiers(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{"unquoted ASCII", strings.Repeat("A", 70), strings.Repeat("a", 63)},
		{"quoted ASCII", `"` + strings.Repeat("A", 70) + `"`, strings.Repeat("A", 63)},
		{"unquoted UTF8 boundary", strings.Repeat("a", 62) + "界tail", strings.Repeat("a", 62)},
		{"quoted UTF8 boundary", `"` + strings.Repeat("é", 31) + "界tail" + `"`, strings.Repeat("é", 31)},
		{"exact UTF8 boundary", `"` + strings.Repeat("a", 60) + "界tail" + `"`, strings.Repeat("a", 60) + "界"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("field", func(t *testing.T) {
				sql, args, err := Convert("CREATE TABLE t ("+tc.input+" text)", "t", tc.input+" = 'value'")
				if err != nil || !reflect.DeepEqual(args, []any{"value"}) {
					t.Fatalf("got %q %#v %v; want successful bound equality", sql, args, err)
				}
				assertParsedEquality(t, sql, "", "t", tc.want)
			})
			ddl := "CREATE TABLE " + tc.input + "." + tc.input + " (" + tc.input + " text)"
			sql, args, err := Convert(ddl, tc.input+"."+tc.input, tc.input+" = 'value'")
			if err != nil || !reflect.DeepEqual(args, []any{"value"}) {
				t.Fatalf("got %q %#v %v; want successful bound equality", sql, args, err)
			}
			assertParsedEquality(t, sql, tc.want, tc.want, tc.want)
			if _, _, err := Convert(ddl, tc.input, tc.input+" = 'value'"); err != nil {
				t.Errorf("unqualified lookup: %v", err)
			}
		})
	}
}
