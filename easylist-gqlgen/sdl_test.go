package easylistgql

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
		"input UsersListInput {\n  filter: String\n  sort: [UsersSort!]\n  limit: Int\n  offset: Int\n}",
		"input UsersExportInput {\n  fields: [UsersExportField!]!\n  filter: String\n  sort: [UsersSort!]\n  format: ExportFormat!\n}",
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

func TestRenamedScalar(t *testing.T) {
	s := Scalars{UUID: "EasyUUID"}.withDefaults()
	users, _ := lists.Registry.List("users")
	if !strings.Contains(listSDL(users, s), "  uid: EasyUUID!\n") {
		t.Error("uuid field does not use the renamed scalar")
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
	if !strings.Contains(shared, "scalar Int64") || strings.Contains(shared, "scalar UUID") || strings.Contains(shared, "scalar Time") {
		t.Errorf("shared SDL = %s", shared)
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
