package easylistgql

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestReadme checks that the documented entrypoint and resolvers are the
// fixture project's, with its module path replaced by example.com/app.
func TestReadme(t *testing.T) {
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	read := func(path string) string {
		data, err := os.ReadFile(filepath.FromSlash(path))
		if err != nil {
			t.Fatal(err)
		}
		return strings.ReplaceAll(string(data), "github.com/mgpnd/easylist-go/easylist-gqlgen/internal/fixture", "example.com/app")
	}
	main := read("internal/fixture/cmd/generate/main.go")
	resolvers := read("internal/fixture/graph/resolver.go")
	blocks := regexp.MustCompile("(?s)```go\n(.*?)```").FindAllStringSubmatch(string(readme), -1)
	if len(blocks) != 4 {
		t.Fatalf("README has %d Go snippets, want 4", len(blocks))
	}
	if !strings.Contains(main, blocks[0][1]) {
		t.Errorf("README entrypoint differs from internal/fixture/cmd/generate/main.go:\n%s", blocks[0][1])
	}
	for _, b := range blocks[1:3] {
		if !strings.Contains(resolvers, b[1]) {
			t.Errorf("README resolver differs from internal/fixture/graph/resolver.go:\n%s", b[1])
		}
	}
	if renamed := blocks[3][1]; !strings.Contains(renamed, `Scalars:  easylistgql.Scalars{UUID: "EasyUUID", Time: "EasyTime"}`) {
		t.Errorf("unexpected scalar snippet:\n%s", renamed)
	}
	_ = Options{Registry: "example.com/app/lists.Registry", Scalars: Scalars{UUID: "EasyUUID", Time: "EasyTime"}}
}
