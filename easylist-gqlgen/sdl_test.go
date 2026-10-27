package easylistgql

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	"github.com/mgpnd/easylist-go/easylist-gqlgen/internal/fixture/lists"
)

var update = flag.Bool("update", false, "rewrite golden files")

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("%s differs from golden file; run go test -update to review\ngot:\n%s", name, got)
	}
}

func TestListSDLGolden(t *testing.T) {
	s := Scalars{}.withDefaults()
	for _, l := range lists.Registry.Lists() {
		golden(t, l.Name()+".graphqls", listSDL(l, s))
	}
	golden(t, "shared.graphqls", sharedSDL(lists.Registry.Lists(), s))
}

func TestSDLScenarios(t *testing.T) {
	users, _ := lists.Registry.List("users")
	sdl := listSDL(users, Scalars{}.withDefaults())
	for _, want := range []string{
		"type UsersRow {", "  name: String!\n", "  country: String\n",
		"enum UsersStatus {\n  ACTIVE\n  ON_HOLD\n}", "  status: UsersStatus!\n",
		"  balance: Decimal!\n", "  id: Int64!\n", "  born: Date\n", "  created_at: Time!\n", "  uid: UUID!\n",
		"type UsersPage {\n  items: [UsersRow!]!\n  totalCount: Int!\n}",
		"input UsersSort {\n  field: UsersSortField!\n  direction: SortDirection! = ASC\n}",
		"input UsersListInput {\n  filter: String\n  sort: [UsersSort!]\n  limit: Int\n  offset: Int\n" + timezoneSDL + "}",
		"input UsersExportInput {\n  fields: [UsersExportField!]!\n  filter: String\n  sort: [UsersSort!]\n  format: ExportFormat!\n" +
			timezoneSDL + "  \"\"\"\n  The moment at which relative time values such as TODAY are evaluated. When\n  omitted, the server's current time is used.\n  \"\"\"\n  asOf: Time\n}",
	} {
		if !strings.Contains(sdl, want) {
			t.Errorf("SDL lacks %q", want)
		}
	}
	sortEnum := sdl[strings.Index(sdl, "enum UsersSortField"):]
	sortEnum = sortEnum[:strings.Index(sortEnum, "}")]
	if strings.Contains(sortEnum, "DISPLAY") || !strings.Contains(sortEnum, "NAME") {
		t.Errorf("sort enum = %s", sortEnum)
	}
	exportEnum := sdl[strings.Index(sdl, "enum UsersExportField"):]
	exportEnum = exportEnum[:strings.Index(exportEnum, "}")]
	for _, v := range []string{"ID", "NAME", "DISPLAY"} {
		if !strings.Contains(exportEnum, "  "+v+"\n") {
			t.Errorf("export enum lacks %s", v)
		}
	}

	items, _ := lists.Registry.List("order_items")
	if !strings.Contains(listSDL(items, Scalars{}.withDefaults()), "type OrderItemsRow {") {
		t.Error("multi-word prefix not PascalCase")
	}
}

func TestTimezoneAndAsOfFields(t *testing.T) {
	if !strings.Contains(timezoneSDL, "UTC offset in minutes, positive east of UTC") ||
		!strings.Contains(timezoneSDL, "server's local time zone") || !strings.HasSuffix(timezoneSDL, "  timezone: Int\n") {
		t.Errorf("timezone field = %q", timezoneSDL)
	}
	users, _ := lists.Registry.List("users")
	sdl := listSDL(users, Scalars{}.withDefaults())
	for _, input := range []string{"input UsersListInput {", "input UsersExportInput {"} {
		body := sdl[strings.Index(sdl, input):]
		body = body[:strings.Index(body, "\n}\n")+1]
		if strings.Count(body, "  timezone: Int\n") != 1 {
			t.Errorf("%s lacks an optional timezone: Int\n%s", input, body)
		}
	}
	if strings.Count(sdl, "  asOf: Time\n") != 1 {
		t.Errorf("asOf declared %d times, want once (on UsersExportInput)", strings.Count(sdl, "  asOf: Time\n"))
	}
	if _, err := gqlparser.LoadSchema(
		&ast.Source{Name: "shared.graphqls", Input: sharedSDL(lists.Registry.Lists(), Scalars{}.withDefaults())},
		&ast.Source{Name: "users.graphqls", Input: sdl},
		&ast.Source{Name: "query.graphqls", Input: "type Query { x: Int }"},
	); err != nil {
		t.Errorf("SDL does not load: %v", err)
	}
}

func TestRenamedScalar(t *testing.T) {
	s := Scalars{UUID: "EasyUUID"}.withDefaults()
	users, _ := lists.Registry.List("users")
	if !strings.Contains(listSDL(users, s), "  uid: EasyUUID!\n") {
		t.Error("uuid field does not use the renamed scalar")
	}
	items, _ := lists.Registry.List("order_items")
	renamedTime := Scalars{Time: "EasyTime"}.withDefaults()
	if !strings.Contains(listSDL(items, renamedTime), "  asOf: EasyTime\n") {
		t.Error("asOf does not use the renamed timestamp scalar")
	}
	if shared := sharedSDL([]*easylist.CompiledList{items}, renamedTime); !strings.Contains(shared, "scalar EasyTime\n") || strings.Contains(shared, "scalar Time\n") {
		t.Errorf("shared SDL with renamed Time = %s", shared)
	}
	shared := sharedSDL(lists.Registry.Lists(), s)
	if !strings.Contains(shared, "scalar EasyUUID\n") || strings.Contains(shared, "scalar UUID\n") {
		t.Errorf("shared SDL = %s", shared)
	}
}

func TestSharedSDLDeclaresOnlyUsedScalars(t *testing.T) {
	reg, err := easylist.NewRegistry(lists.OrderItems)
	if err != nil {
		t.Fatal(err)
	}
	shared := sharedSDL(reg.Lists(), Scalars{}.withDefaults())
	if !strings.Contains(shared, "scalar Int64") || strings.Contains(shared, "scalar UUID") || strings.Contains(shared, "scalar Date") {
		t.Errorf("shared SDL = %s", shared)
	}
	// order_items has no timestamp field, but its export input's asOf uses Time.
	items, _ := reg.List("order_items")
	if !strings.Contains(shared, "scalar Time\n") || !strings.Contains(listSDL(items, Scalars{}.withDefaults()), "  asOf: Time\n") {
		t.Errorf("list without timestamp fields does not declare Time for asOf:\n%s", shared)
	}
	if sharedSDL(nil, Scalars{}.withDefaults()) != "enum SortDirection {\n  ASC\n  DESC\n}\n\nenum ExportFormat {\n  CSV\n  XLSX\n}\n\ntype ExportJob {\n  id: ID!\n}\n" {
		t.Error("Time declared without any list")
	}
	if strings.Count(sharedSDL(lists.Registry.Lists(), Scalars{}.withDefaults()), "enum SortDirection") != 1 {
		t.Error("SortDirection declared more than once")
	}
}

func TestPluginChecksOptions(t *testing.T) {
	cases := []struct {
		name string
		opts Options
		want string
	}{
		{"missing registry ref", Options{}, "Options.Registry"},
		{"registry ref without name", Options{Registry: "example.com/app/lists"}, "Options.Registry"},
		{"invalid scalar name", Options{Registry: "x/y.R", Scalars: Scalars{Date: "My-Date"}}, "My-Date"},
		{"scalar name reused", Options{Registry: "x/y.R", Scalars: Scalars{Date: "UUID"}}, "UUID"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewPlugin(lists.Registry, tc.opts).InjectSourcesEarly()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

// arrayUsers declares the array fields of the gqlgen-schema-generation
// scenarios.
var arrayUsers = easylist.List{
	Name: "users", Table: "users", Alias: "u", PrimaryKey: "id",
	Fields: []easylist.Field{
		{Name: "id", Type: easylist.Integer},
		{Name: "tags", Type: easylist.Text, Array: true},
		{Name: "statuses", Type: easylist.Enum, Array: true, Nullable: true, Values: []string{"ok", "failed"}},
		{Name: "group_ids", Type: easylist.UUID, Array: true},
		easylist.Calc("labels", easylist.Text, func(context.Context, easylist.Row) ([]string, error) { return nil, nil }),
	},
}

func TestArraySDL(t *testing.T) {
	reg, err := easylist.NewRegistry(arrayUsers)
	if err != nil {
		t.Fatal(err)
	}
	users, _ := reg.List("users")
	s := Scalars{}.withDefaults()
	sdl := listSDL(users, s)
	golden(t, "array_users.graphqls", sdl)
	shared := sharedSDL(reg.Lists(), s)
	golden(t, "array_shared.graphqls", shared)
	for _, want := range []string{
		"  tags: [String!]!\n",
		"enum UsersStatuses {\n  OK\n  FAILED\n}", "  statuses: [UsersStatuses!]\n",
		"  group_ids: [UUID!]!\n", "  labels: [String!]!\n",
	} {
		if !strings.Contains(sdl, want) {
			t.Errorf("SDL lacks %q", want)
		}
	}
	if !strings.Contains(shared, "scalar UUID\n") {
		t.Errorf("shared SDL does not declare UUID:\n%s", shared)
	}
	sortEnum := sdl[strings.Index(sdl, "enum UsersSortField"):]
	sortEnum = sortEnum[:strings.Index(sortEnum, "}")]
	if sortEnum != "enum UsersSortField {\n  ID\n" {
		t.Errorf("sort enum = %q, want only ID", sortEnum)
	}
	exportEnum := sdl[strings.Index(sdl, "enum UsersExportField"):]
	exportEnum = exportEnum[:strings.Index(exportEnum, "}")]
	for _, v := range []string{"TAGS", "STATUSES", "GROUP_IDS", "LABELS"} {
		if !strings.Contains(exportEnum, "  "+v+"\n") {
			t.Errorf("export enum lacks %s", v)
		}
	}

	fixture, _ := lists.Registry.List("users")
	fixtureSDL := listSDL(fixture, s)
	fixtureSort := fixtureSDL[strings.Index(fixtureSDL, "enum UsersSortField"):]
	fixtureSort = fixtureSort[:strings.Index(fixtureSort, "}")]
	if strings.Contains(fixtureSort, "TAGS") || strings.Contains(fixtureSort, "ROLES") || !strings.Contains(fixtureSort, "COUNTRY") {
		t.Errorf("fixture sort enum = %s", fixtureSort)
	}
}
