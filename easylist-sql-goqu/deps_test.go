package sqlgoqu_test

import (
	"os/exec"
	"reflect"
	"strings"
	"testing"

	sqlgoqu "github.com/mgpnd/easylist-go/easylist-sql-goqu"
)

// library is the module path prefix of the query library this driver uses.
const library = "github.com/doug-martin/goqu"

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
	pkg := reflect.TypeOf(sqlgoqu.Driver{}).PkgPath()
	out, err := exec.Command("go", "list", "-deps", pkg).CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps %s: %v\n%s", pkg, err, out)
	}
	deps := strings.Fields(string(out))
	found := false
	for _, dep := range deps {
		if strings.HasPrefix(dep, library) {
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
