package sqlent_test

import (
	"os/exec"
	"reflect"
	"strings"
	"testing"

	sqlent "github.com/mgpnd/easylist-go/easylist-sql-ent"
)

// library lists the packages of the query library this driver uses: ent's
// dialect and its SQL builder, and no generated code, schema, or codegen
// package.
var library = []string{"entgo.io/ent/dialect", "entgo.io/ent/dialect/sql"}

// foreign lists the module path prefixes of the other drivers' query
// libraries, the rest of ent, the PostgreSQL wire drivers, and the
// conformance suite, which only tests may import.
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

// The driver package, without its tests, compiles in ent's dialect and SQL
// builder packages and none of the foreign ones.
func TestCompiledInPackages(t *testing.T) {
	pkg := reflect.TypeOf(sqlent.Driver{}).PkgPath()
	out, err := exec.Command("go", "list", "-deps", pkg).CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps %s: %v\n%s", pkg, err, out)
	}
	deps := strings.Fields(string(out))
	found := map[string]bool{}
	for _, dep := range deps {
		if isLibrary(dep) {
			found[dep] = true
			continue
		}
		for _, prefix := range foreign {
			if strings.HasPrefix(dep, prefix) {
				t.Errorf("%s compiles in %s", pkg, dep)
			}
		}
	}
	for _, lib := range library {
		if !found[lib] {
			t.Errorf("%s does not compile in %s:\n%s", pkg, lib, out)
		}
	}
}

// isLibrary reports whether dep is one of the ent packages the driver may
// use, and not another package of the entgo.io/ent module.
func isLibrary(dep string) bool {
	for _, lib := range library {
		if dep == lib {
			return true
		}
	}
	return false
}

// The library check accepts ent's dialect and SQL builder and nothing else
// of ent.
func TestIsLibrary(t *testing.T) {
	for dep, want := range map[string]bool{
		"entgo.io/ent/dialect":              true,
		"entgo.io/ent/dialect/sql":          true,
		"entgo.io/ent":                      false,
		"entgo.io/ent/schema":               false,
		"entgo.io/ent/dialect/sql/sqlgraph": false,
		"entgo.io/ent/dialect/sql/schema":   false,
		"entgo.io/ent/entc/gen":             false,
		"github.com/jackc/pgx/v5/pgconn":    false,
	} {
		if got := isLibrary(dep); got != want {
			t.Errorf("isLibrary(%q) = %v", dep, got)
		}
	}
}
