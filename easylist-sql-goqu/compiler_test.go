package querysql

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	filter "github.com/mgpnd/easylist-go/easylist-filter"
)

func TestCompilePreparedSQL(t *testing.T) {
	sql, args, err := Convert("CREATE TABLE users (name text, age integer)",
		"users", "name = 'Artem' AND age < 30")
	if err != nil || !strings.HasPrefix(sql, "SELECT * FROM \"users\" WHERE") ||
		!strings.Contains(sql, "\"users\".\"name\" = $1") ||
		!strings.Contains(sql, "\"users\".\"age\" < $2") ||
		strings.Contains(sql, "Artem") || !reflect.DeepEqual(args, []any{"Artem", int64(30)}) {
		t.Fatalf("%q %#v %v", sql, args, err)
	}
}

func TestCompileGrammarCompatibility(t *testing.T) {
	schema := "CREATE TABLE t (a integer, b integer, c integer)"
	for _, tc := range []struct {
		filter string
		sql    string
	}{
		{"a = 1 OR b = 2 AND c = 3", `SELECT * FROM "t" WHERE (("t"."a" = $1) OR (("t"."b" = $2) AND ("t"."c" = $3)))`},
		{"a BETWEEN '1,2' OR b = 3", `SELECT * FROM "t" WHERE (("t"."a" BETWEEN $1 AND $2) OR ("t"."b" = $3))`},
	} {
		t.Run(tc.filter, func(t *testing.T) {
			sql, args, err := Convert(schema, "t", tc.filter)
			if err != nil || sql != tc.sql || !reflect.DeepEqual(args, []any{int64(1), int64(2), int64(3)}) {
				t.Fatalf("got %q %#v %v; want %q and [1 2 3]", sql, args, err, tc.sql)
			}
		})
	}
}

func TestCompilerReuseAndAtomicFailure(t *testing.T) {
	compiler, err := NewCompiler("CREATE TABLE users (name text, age integer)")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("person %d", i)
			filter := fmt.Sprintf("name = '%s' AND age < %d", name, i)
			sql, args, err := compiler.Compile("users", filter)
			wantSQL := `SELECT * FROM "users" WHERE (("users"."name" = $1) AND ("users"."age" < $2))`
			if err != nil || sql != wantSQL || !reflect.DeepEqual(args, []any{name, int64(i)}) {
				t.Errorf("call %d: %q %#v %v", i, sql, args, err)
			}
		}(i)
	}
	wg.Wait()

	for _, filter := range []string{"name = 'valid' AND missing = 2", "age = 1 OR age = 'bad'", "age = 1 AND", "age = 1 extra"} {
		sql, args, err := compiler.Compile("users", filter)
		if err == nil || sql != "" || args != nil {
			t.Errorf("invalid filter %q returned %q %#v %v", filter, sql, args, err)
		}
	}
	sql, args, err := compiler.Compile("users", "age = 99")
	if err != nil || sql != `SELECT * FROM "users" WHERE ("users"."age" = $1)` || !reflect.DeepEqual(args, []any{int64(99)}) {
		t.Fatalf("reuse after errors: %q %#v %v", sql, args, err)
	}
}

func TestCompileTableResolutionAndQuoting(t *testing.T) {
	compiler, err := NewCompiler(`CREATE TABLE public.users (age integer);
		CREATE TABLE archive.users (age integer);
		CREATE TABLE "odd.schema"."odd.table" ("a.b" text);`)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ table, filter, sql string }{
		{"public.users", "age = 7", `SELECT * FROM "public"."users" WHERE ("public"."users"."age" = $1)`},
		{`"odd.schema"."odd.table"`, `"a.b" = 'value'`, `SELECT * FROM "odd.schema"."odd.table" WHERE ("odd.schema"."odd.table"."a.b" = $1)`},
	} {
		sql, args, err := compiler.Compile(tc.table, tc.filter)
		if err != nil || sql != tc.sql || len(args) != 1 {
			t.Errorf("table %s: %q %#v %v", tc.table, sql, args, err)
		}
	}
	for _, table := range []string{"users", "missing", "public.users; DROP TABLE users"} {
		sql, args, err := compiler.Compile(table, "age = 1")
		if err == nil || sql != "" || args != nil {
			t.Errorf("invalid table %q: %q %#v %v", table, sql, args, err)
		}
	}
}

func TestCompileSchemaFailure(t *testing.T) {
	if compiler, err := NewCompiler("CREATE TABLE ("); err == nil || compiler != nil {
		t.Fatalf("malformed schema: %#v %v", compiler, err)
	}
	sql, args, err := Convert("CREATE TABLE (", "users", "age = 1")
	if err == nil || sql != "" || args != nil {
		t.Fatalf("malformed schema conversion: %q %#v %v", sql, args, err)
	}
}

// These tests catch lost argument order, schema validation before syntax,
// mutation of shared ASTs, literal conversion drift, and recursive lowering.
func TestCompileASTPreparedSQL(t *testing.T) {
	const schema = "CREATE TABLE users (name text, age integer)"
	compiler, err := NewCompiler(schema)
	if err != nil {
		t.Fatal(err)
	}
	ast, err := filter.Parse("name = 'Artem' AND age < 30")
	if err != nil {
		t.Fatal(err)
	}
	sql, args, err := compiler.CompileAST("users", ast)
	wantSQL := `SELECT * FROM "users" WHERE (("users"."name" = $1) AND ("users"."age" < $2))`
	if err != nil || sql != wantSQL || !reflect.DeepEqual(args, []any{"Artem", int64(30)}) {
		t.Fatalf("got %q %#v %v", sql, args, err)
	}
	for _, compile := range []func() (string, []any, error){
		func() (string, []any, error) { return compiler.Compile("users", "name = 'Artem' AND age < 30") },
		func() (string, []any, error) { return Convert(schema, "users", "name = 'Artem' AND age < 30") },
	} {
		gotSQL, gotArgs, err := compile()
		if err != nil || gotSQL != sql || !reflect.DeepEqual(gotArgs, args) {
			t.Fatalf("wrapper: %q %#v %v", gotSQL, gotArgs, err)
		}
	}
}

func TestCompileASTNilAndAtomicErrors(t *testing.T) {
	compiler, err := NewCompiler("CREATE TABLE users (age integer)")
	if err != nil {
		t.Fatal(err)
	}
	sql, args, err := compiler.CompileAST("users", nil)
	if err == nil || sql != "" || args != nil {
		t.Fatalf("nil AST: %q %#v %v", sql, args, err)
	}
	sql, args, err = compiler.Compile("users", "missing = 1 AND")
	if err == nil || !strings.HasSuffix(err.Error(), "position 16") || sql != "" || args != nil {
		t.Fatalf("syntax before validation: %q %#v %v", sql, args, err)
	}
	for _, input := range []string{"age = 1 AND missing = 2", "age = 1 OR age = 'bad'"} {
		ast, err := filter.Parse(input)
		if err != nil {
			t.Fatal(err)
		}
		sql, args, err := compiler.CompileAST("users", ast)
		if err == nil || sql != "" || args != nil {
			t.Fatalf("invalid AST %q: %q %#v %v", input, sql, args, err)
		}
	}
	ast, err := filter.Parse("age = 1")
	if err != nil {
		t.Fatal(err)
	}
	sql, args, err = compiler.CompileAST("missing", ast)
	if err == nil || sql != "" || args != nil {
		t.Fatalf("missing table: %q %#v %v", sql, args, err)
	}
	sql, args, err = compiler.Compile("missing", "age = 1 AND")
	if err == nil || !strings.Contains(err.Error(), "unknown table") || sql != "" || args != nil {
		t.Fatalf("table before syntax: %q %#v %v", sql, args, err)
	}
}

func TestCompileASTSharedTree(t *testing.T) {
	compiler, err := NewCompiler("CREATE TABLE users (name text, age integer)")
	if err != nil {
		t.Fatal(err)
	}
	ast, err := filter.Parse("name = 'Artem' AND age < 30")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sql, args, err := compiler.CompileAST("users", ast)
			wantSQL := `SELECT * FROM "users" WHERE (("users"."name" = $1) AND ("users"."age" < $2))`
			if err != nil || sql != wantSQL || !reflect.DeepEqual(args, []any{"Artem", int64(30)}) {
				t.Errorf("shared AST: %q %#v %v", sql, args, err)
			}
		}()
	}
	wg.Wait()
}

func TestCompileASTLiteralCompatibility(t *testing.T) {
	compiler, err := NewCompiler("CREATE TABLE users (price numeric, age integer)")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		input, sql string
		args       []any
	}{
		{"price = 12345678901234567890.123456789", `SELECT * FROM "users" WHERE ("users"."price" = $1::numeric)`, []any{"12345678901234567890.123456789"}},
		{"age BETWEEN ' 40, 30 '", `SELECT * FROM "users" WHERE ("users"."age" BETWEEN $1 AND $2)`, []any{int64(40), int64(30)}},
	} {
		t.Run(tc.input, func(t *testing.T) {
			ast, err := filter.Parse(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			sql, args, err := compiler.CompileAST("users", ast)
			if err != nil || sql != tc.sql || !reflect.DeepEqual(args, tc.args) {
				t.Fatalf("AST: %q %#v %v", sql, args, err)
			}
			stringSQL, stringArgs, err := compiler.Compile("users", tc.input)
			if err != nil || sql != stringSQL || !reflect.DeepEqual(args, stringArgs) {
				t.Fatalf("string: %q %#v %v", stringSQL, stringArgs, err)
			}
		})
	}
}

func TestCompileASTDeepChain(t *testing.T) {
	compiler, err := NewCompiler("CREATE TABLE users (age integer)")
	if err != nil {
		t.Fatal(err)
	}
	input := strings.Repeat("age = 1 OR ", 1999) + "age = 1"
	ast, err := filter.Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	sql, args, err := compiler.CompileAST("users", ast)
	if err != nil || !strings.HasPrefix(sql, `SELECT * FROM "users" WHERE `) || !strings.Contains(sql, `"users"."age" = $2000`) || len(args) != 2000 {
		t.Fatalf("deep chain: SQL bytes=%d args=%d error=%v", len(sql), len(args), err)
	}
	for i, arg := range args {
		if arg != int64(1) {
			t.Fatalf("argument %d: %#v", i, arg)
		}
	}
}

func TestCompileNotEqualSQL(t *testing.T) {
	compiler, err := NewCompiler("CREATE TABLE t (name text, age integer, score double precision, price numeric, day date, active boolean)")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		filter, where string
		args          []any
	}{
		{"name != 'Artem'", `"t"."name" <> $1`, []any{"Artem"}},
		{"age != 30", `"t"."age" <> $1`, []any{int64(30)}},
		{"score != 2.5", `"t"."score" <> $1`, []any{float64(2.5)}},
		{"price != 1.50", `"t"."price" <> $1::numeric`, []any{"1.50"}},
		{"day != '2024-02-29'", `"t"."day" <> $1`, []any{time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC)}},
		{"active != FALSE", `"t"."active" <> $1`, []any{false}},
		{"age != 1 OR name != 'x' AND active != TRUE", `("t"."age" <> $1 OR ("t"."name" <> $2 AND "t"."active" <> $3))`, []any{int64(1), "x", true}},
	} {
		t.Run(tc.filter, func(t *testing.T) {
			sql, args, err := compiler.Compile("t", tc.filter)
			if err != nil || sql != `SELECT * FROM "t" WHERE `+tc.where || !reflect.DeepEqual(args, tc.args) {
				t.Fatalf("got %q %#v %v; want WHERE %s args %#v", sql, args, err, tc.where, tc.args)
			}
		})
	}
}

func TestCompileGroupedSQL(t *testing.T) {
	compiler, err := NewCompiler("CREATE TABLE t (a integer, b integer, c integer)")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		filter, where string
		args          []any
	}{
		{"a = 1 OR b = 2 AND c = 3", `(("t"."a" = $1) OR (("t"."b" = $2) AND ("t"."c" = $3)))`, []any{int64(1), int64(2), int64(3)}},
		{"(a = 1 OR b = 2) AND c = 3", `((("t"."a" = $1) OR ("t"."b" = $2)) AND ("t"."c" = $3))`, []any{int64(1), int64(2), int64(3)}},
		{"a = 1 AND (b = 2 OR c = 3)", `(("t"."a" = $1) AND (("t"."b" = $2) OR ("t"."c" = $3)))`, []any{int64(1), int64(2), int64(3)}},
		{"a BETWEEN '1,2' oR b = 3 aNd c = 4", `(("t"."a" BETWEEN $1 AND $2) OR (("t"."b" = $3) AND ("t"."c" = $4)))`, []any{int64(1), int64(2), int64(3), int64(4)}},
		{"((a = 1))", `("t"."a" = $1)`, []any{int64(1)}},
	}
	for _, tc := range cases {
		t.Run(tc.filter, func(t *testing.T) {
			sql, args, err := compiler.Compile("t", tc.filter)
			if err != nil {
				t.Fatal(err)
			}
			if err != nil || sql != `SELECT * FROM "t" WHERE `+tc.where || !reflect.DeepEqual(args, tc.args) {
				t.Fatalf("got %q %#v %v; want WHERE %s args %#v", sql, args, err, tc.where, tc.args)
			}
		})
	}
}
