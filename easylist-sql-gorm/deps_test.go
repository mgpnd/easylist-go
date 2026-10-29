package sqlgorm_test

import (
	"os/exec"
	"reflect"
	"strings"
	"testing"

	sqlgorm "github.com/mgpnd/easylist-go/easylist-sql-gorm"
)

// library is the module path prefix of the query library this driver uses.
const library = "gorm.io/gorm"

// foreign lists the module path prefixes of the other drivers' query
// libraries, gorm's own PostgreSQL dialector and the other gorm.io modules,
// the PostgreSQL wire drivers, and the conformance suite, which only tests
// may import.
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
	pkg := reflect.TypeOf(sqlgorm.Driver{}).PkgPath()
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

// isLibrary reports whether dep is a package of the gorm.io/gorm module, and
// not of another module under gorm.io, such as gorm.io/driver/postgres.
func isLibrary(dep string) bool {
	return dep == library || strings.HasPrefix(dep, library+"/")
}

// The library check accepts gorm's own packages and nothing else under
// gorm.io.
func TestIsLibrary(t *testing.T) {
	for dep, want := range map[string]bool{
		"gorm.io/gorm":                   true,
		"gorm.io/gorm/clause":            true,
		"gorm.io/driver/postgres":        false,
		"gorm.io/gormx":                  false,
		"gorm.io/plugin/dbresolver":      false,
		"github.com/jackc/pgx/v5/pgconn": false,
	} {
		if got := isLibrary(dep); got != want {
			t.Errorf("isLibrary(%q) = %v", dep, got)
		}
	}
}
