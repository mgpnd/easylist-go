package querysql

import (
	"strings"
	"testing"
)

func TestSchemaCatalog(t *testing.T) {
	sql := `CREATE TABLE public.users (name text, age integer, tags text[], born date,
 active boolean, amount numeric, omitted jsonb, CONSTRAINT age_ok CHECK (age > 0));
 ALTER TABLE public.users ADD COLUMN later text;`
	tables, err := parseSchema(sql)
	if err != nil {
		t.Fatal(err)
	}
	users, err := resolveTable(tables, "users")
	if err != nil {
		t.Fatal(err)
	}
	if len(users.columns) != 7 || users.columns["tags"].kind != kindText ||
		users.columns["tags"].arrayDimensions != 1 || users.columns["omitted"].kind != kindUnsupported {
		t.Fatalf("wrong columns: %#v", users.columns)
	}
	if _, ok := users.columns["later"]; ok {
		t.Fatal("ALTER TABLE changed metadata")
	}
}

func TestSchemaNamesAndUnresolvedTables(t *testing.T) {
	tables, err := parseSchema(`CREATE TABLE public.users (id int); CREATE TABLE audit.users (id int);`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolveTable(tables, "users"); err == nil {
		t.Fatal("expected ambiguity")
	}
	audit, err := resolveTable(tables, "audit.users")
	if err != nil || audit.schema != "audit" {
		t.Fatalf("%#v, %v", audit, err)
	}
	for _, ddl := range []string{
		"CREATE TABLE child (LIKE parent)",
		"CREATE TABLE child (id int) INHERITS (parent)",
		"CREATE TABLE child PARTITION OF parent FOR VALUES IN (1)",
	} {
		catalog, err := parseSchema(ddl)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := resolveTable(catalog, "child"); err == nil || !strings.Contains(err.Error(), "child") {
			t.Errorf("expected unresolved child for %q, got %v", ddl, err)
		}
	}
	tables, err = parseSchema(`CREATE TABLE "a.b"."c""d" ("Mixed" text); CREATE TABLE "a.b".c (id int); CREATE TABLE a."b.c" (id int); CREATE TABLE Users (id int);`)
	if err != nil {
		t.Fatal(err)
	}
	quoted := tables[tableKey{schema: "a.b", name: `c"d`}]
	if column, ok := quoted.columns["Mixed"]; !ok || column.name != "Mixed" {
		t.Fatalf("quoted column spelling was lost: %#v", quoted.columns)
	}
	for input, want := range map[string]tableKey{
		`"a.b"."c""d"`: {"a.b", `c"d`}, `"a.b".c`: {"a.b", "c"}, `a."b.c"`: {"a", "b.c"}, `USERS`: {"", "users"}, ` "a.b" . "c""d" `: {"a.b", `c"d`},
	} {
		got, err := resolveTable(tables, input)
		if err != nil || got.schema != want.schema || got.name != want.name {
			t.Errorf("resolve %q = %#v, %v; want %#v", input, got, err, want)
		}
	}
	for _, input := range []string{"missing", "public.users", `"Users"`, "a.b.c", `"unterminated`, "", "users; DROP TABLE users", "users.", ".users", `""`} {
		if _, err := resolveTable(tables, input); err == nil {
			t.Errorf("expected error for %q", input)
		}
	}
	if _, err := parseSchema(`CREATE TABLE users(id int); CREATE TABLE USERS(id text)`); err == nil {
		t.Fatal("expected duplicate error")
	}
	if _, err := parseSchema(`CREATE TABLE broken (`); err == nil {
		t.Fatal("expected invalid SQL error")
	}
}

func TestSchemaColumnTypes(t *testing.T) {
	groups := []struct {
		kind  columnKind
		types []string
	}{
		{kindText, []string{"text", "varchar(12)", "character varying(12)", "char(3)", "character(3)", "pg_catalog.text"}},
		{kindInteger, []string{"smallint", "integer", "bigint", "int", "int2", "int4", "int8"}},
		{kindFloat, []string{"real", "double precision", "float4", "float8", "float(24)", "float(53)"}},
		{kindNumeric, []string{"numeric", "numeric(12,2)", "decimal"}},
		{kindDate, []string{"date"}},
		{kindBoolean, []string{"boolean", "bool"}},
		{kindUnsupported, []string{"jsonb", "timestamp", "uuid", "custom.text", `"TEXT"`, "smallserial", "serial", "bigserial", "serial2", "serial4", "serial8"}},
	}
	for _, group := range groups {
		for _, typ := range group.types {
			for _, suffix := range []string{"", "[]", "[][]"} {
				t.Run(typ+suffix, func(t *testing.T) {
					tables, err := parseSchema("CREATE TABLE types (value " + typ + suffix + ")")
					if err != nil {
						t.Fatal(err)
					}
					got := tables[tableKey{name: "types"}].columns["value"]
					if got.name != "value" || got.kind != group.kind || got.arrayDimensions != strings.Count(suffix, "[") {
						t.Fatalf("got %#v, want kind %v array %v", got, group.kind, suffix != "")
					}
				})
			}
		}
	}
}

func TestSchemaQuotedTypeAliasesRemainUnsupported(t *testing.T) {
	for _, alias := range []string{"smallint", "integer", "bigint", "int", "real", "decimal", "boolean", "smallserial", "serial", "bigserial", "serial2", "serial4", "serial8"} {
		for _, suffix := range []string{"", "[]"} {
			t.Run(alias+suffix, func(t *testing.T) {
				tables, err := parseSchema(`CREATE TABLE t (value "` + alias + `"` + suffix + `)`)
				if err != nil {
					t.Fatal(err)
				}
				column := tables[tableKey{name: "t"}].columns["value"]
				if column.kind != kindUnsupported || column.arrayDimensions != strings.Count(suffix, "[") {
					t.Fatalf("quoted type alias %q classified as %#v", alias+suffix, column)
				}
			})
		}
	}
}
