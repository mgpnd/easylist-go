package behavior_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mgpnd/easylist-go/easylist-behavior/lists"
	easylistgql "github.com/mgpnd/easylist-go/easylist-gqlgen"

	"github.com/99designs/gqlgen/api"
	"github.com/99designs/gqlgen/codegen/config"
)

const (
	graphPkg      = "github.com/mgpnd/easylist-go/easylist-behavior/graph"
	graphRegistry = "github.com/mgpnd/easylist-go/easylist-behavior/lists.Registry"
)

// graphInputs are the hand-written files of graph/. Every other file in it
// is generated.
var graphInputs = []string{"gqlgen.yml", "schema.graphqls", "resolver.go"}

// TestGraphGeneration regenerates a copy of graph/ with the plugin setup of
// cmd/generate and fails when a committed generated file differs from the
// regenerated one, is missing, or is no longer generated, so the GraphQL
// snapshots never run an outdated plugin output. It works on a copy, inside
// the module because gqlgen needs an import path, so packages tested in
// parallel never see half-written files.
func TestGraphGeneration(t *testing.T) {
	if testing.Short() {
		t.Skip("runs gqlgen and go build")
	}
	dir, err := os.MkdirTemp(".", "regen")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	copyPkg := strings.TrimSuffix(graphPkg, "graph") + filepath.Base(dir)
	for _, f := range graphInputs {
		data, err := os.ReadFile(filepath.Join("graph", f))
		if err != nil {
			t.Fatal(err)
		}
		data = []byte(strings.ReplaceAll(string(data), graphPkg+"/", copyPkg+"/"))
		if err := os.WriteFile(filepath.Join(dir, f), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	cfg, err := config.LoadConfigFromDefaultLocations()
	if err != nil {
		t.Fatal(err)
	}
	plugin := easylistgql.NewPlugin(lists.Registry, easylistgql.Options{Registry: graphRegistry})
	if err := api.Generate(cfg, api.PrependPlugin(plugin)); err != nil {
		t.Fatal(err)
	}

	got, err := generatedFiles(".")
	if err != nil {
		t.Fatal(err)
	}
	committed := filepath.Join(wd, "graph")
	want, err := generatedFiles(committed)
	if err != nil {
		t.Fatal(err)
	}
	// gqlgen also spells import paths inside identifiers, with ᚋ for /.
	normalize := strings.NewReplacer(copyPkg+"/", graphPkg+"/", "behaviorᚋ"+filepath.Base(dir)+"ᚋ", "behaviorᚋgraphᚋ")
	for _, f := range got {
		regenerated, err := os.ReadFile(filepath.FromSlash(f))
		if err != nil {
			t.Fatal(err)
		}
		current, err := os.ReadFile(filepath.Join(committed, filepath.FromSlash(f)))
		if err != nil || normalize.Replace(string(regenerated)) != string(current) {
			t.Errorf("%s is stale; run go generate ./graph and commit the result", f)
		}
	}
	for _, f := range want {
		if !slices.Contains(got, f) {
			t.Errorf("%s is no longer generated; delete it, run go generate ./graph, and commit the result", f)
		}
	}
}

// generatedFiles returns the slash-separated paths, relative to dir, of
// the regular files under dir other than graphInputs.
func generatedFiles(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !slices.Contains(graphInputs, rel) {
			out = append(out, rel)
		}
		return nil
	})
	return out, err
}
