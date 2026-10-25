package easylistgql

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/mgpnd/easylist-go/easylist-gqlgen/internal/fixture/lists"

	"github.com/99designs/gqlgen/api"
	"github.com/99designs/gqlgen/codegen/config"
)

const fixtureRegistry = "github.com/mgpnd/easylist-go/easylist-gqlgen/internal/fixture/lists.Registry"

// generatedFiles are the fixture files the plugin and gqlgen write.
var generatedFiles = []string{
	"generated.go", "model/models_gen.go", "easylistgen/easylist_gen.go",
	"easylist/shared.graphqls", "easylist/users.graphqls", "easylist/order_items.graphqls",
}

const fixturePkg = "github.com/mgpnd/easylist-go/easylist-gqlgen/internal/fixture/graph"

// TestFixtureGeneration regenerates a copy of the fixture project with its
// entrypoint's plugin setup, checks that gqlgen's build validation passes,
// and checks that the committed output is current. It works on a copy so
// packages tested in parallel never see half-written files.
func TestFixtureGeneration(t *testing.T) {
	if testing.Short() {
		t.Skip("runs gqlgen and go build")
	}
	src := filepath.Join("internal", "fixture", "graph")
	dir, err := os.MkdirTemp(filepath.Join("internal", "fixture"), "regen")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	copyPkg := strings.TrimSuffix(fixturePkg, "graph") + filepath.Base(dir)
	for _, f := range []string{"gqlgen.yml", "schema.graphqls", "resolver.go"} {
		data, err := os.ReadFile(filepath.Join(src, f))
		if err != nil {
			t.Fatal(err)
		}
		data = []byte(strings.ReplaceAll(string(data), fixturePkg+"/", copyPkg+"/"))
		if err := os.WriteFile(filepath.Join(dir, f), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "easylist"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "easylist", "stale.graphqls"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	wd, _ := os.Getwd()
	t.Chdir(dir)
	cfg, err := config.LoadConfigFromDefaultLocations()
	if err != nil {
		t.Fatal(err)
	}
	if err := api.Generate(cfg, api.PrependPlugin(NewPlugin(lists.Registry, Options{Registry: fixtureRegistry}))); err != nil {
		t.Fatal(err)
	}
	for _, f := range generatedFiles {
		got, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join(wd, src, f))
		if err != nil {
			t.Fatal(err)
		}
		// gqlgen also spells import paths inside identifiers, with ᚋ for /.
		normalized := strings.NewReplacer(copyPkg+"/", fixturePkg+"/", "ᚋfixtureᚋ"+filepath.Base(dir)+"ᚋ", "ᚋfixtureᚋgraphᚋ").Replace(string(got))
		if normalized != string(want) {
			t.Errorf("%s is stale; run go generate ./internal/fixture/graph and commit the result", f)
		}
	}
	if _, err := os.Stat(filepath.Join("easylist", "stale.graphqls")); err == nil {
		t.Error("stale SDL file kept")
	}
}

// TestGeneratedImports checks that generated code imports only the standard
// library, easylist, and the list definitions, so the definitions never
// need to import generated code.
func TestGeneratedImports(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), filepath.Join("internal", "fixture", "graph", "easylistgen", "easylist_gen.go"), nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, imp := range file.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		got = append(got, path)
	}
	want := []string{"context", "io", "time", coreImport, selfImport, strings.TrimSuffix(fixtureRegistry, ".Registry")}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("imports = %v, want %v", got, want)
	}
}

func TestNameCollision(t *testing.T) {
	// gqlgen needs an import path, so the project lives inside this module.
	dir, err := os.MkdirTemp(filepath.Join("internal", "fixture"), "collision")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("gqlgen.yml", "schema:\n  - schema.graphqls\nexec:\n  filename: graph/generated.go\n  package: graph\nskip_mod_tidy: true\n")
	write("schema.graphqls", "type Query { ok: Boolean }\n\ntype UsersRow { id: ID! }\n")
	t.Chdir(dir)
	cfg, err := config.LoadConfigFromDefaultLocations()
	if err != nil {
		t.Fatal(err)
	}
	err = api.Generate(cfg, api.PrependPlugin(NewPlugin(lists.Registry, Options{Registry: fixtureRegistry})))
	if err == nil || !strings.Contains(err.Error(), "UsersRow") {
		t.Fatalf("err = %v, want an error naming UsersRow", err)
	}
}
