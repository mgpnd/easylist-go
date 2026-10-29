package sqlsqlbuilder_test

import (
	"os/exec"
	"reflect"
	"strings"
	"testing"

	sqlsqlbuilder "github.com/mgpnd/easylist-go/easylist-sql-sqlbuilder"
)

// library is the module path of the query library this driver uses.
const library = "github.com/huandu/go-sqlbuilder"

// foreign lists the module path prefixes of the other drivers' query
// libraries, the PostgreSQL wire drivers, and the conformance suite, which
// only tests may import.
var foreign = []string{
	"github.com/doug-martin/goqu",
	"github.com/stephenafamo/bob",
	"github.com/huandu/go-sqlbuilder",
	"github.com/go-jet/jet",
	"entgo.io/ent",
	"gorm.io/",
	"github.com/jackc/pgx",
	"github.com/lib/pq",
	"github.com/mgpnd/easylist-go/easylist-sqlpg/conformance",
}

// The driver package, without its tests, compiles in its own library and
// none of the foreign ones.
func TestCompiledInPackages(t *testing.T) {
	pkg := reflect.TypeOf(sqlsqlbuilder.Driver{}).PkgPath()
	out, err := exec.Command("go", "list", "-deps", pkg).CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps %s: %v\n%s", pkg, err, out)
	}
	deps := strings.Fields(string(out))
	found := false
	for _, dep := range deps {
		if isLibrary(dep) {
			found = true
			continue
		}
		for _, prefix := range foreign {
			if strings.HasPrefix(dep, prefix) {
				t.Errorf("%s compiles in %s", pkg, dep)
			}
		}
	}
	if !found {
		t.Errorf("%s does not compile in %s:\n%s", pkg, library, out)
	}
}

// isLibrary reports whether dep is a package of the go-sqlbuilder module, and not of
// another module whose path merely starts with the same text.
func isLibrary(dep string) bool {
	return dep == library || strings.HasPrefix(dep, library+"/")
}

// The library check accepts go-sqlbuilder's own package and nothing else.
func TestIsLibrary(t *testing.T) {
	for dep, want := range map[string]bool{
		"github.com/huandu/go-sqlbuilder":     true,
		"github.com/huandu/go-sqlbuilder/x":   true,
		"github.com/huandu/go-sqlbuilderx":    false,
		"github.com/huandu/go-clone":          false,
		"github.com/huandu/xstrings":          false,
		"github.com/stephenafamo/bob/dialect": false,
	} {
		if got := isLibrary(dep); got != want {
			t.Errorf("isLibrary(%q) = %v", dep, got)
		}
	}
}
