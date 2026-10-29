package sqljet_test

import (
	"os/exec"
	"reflect"
	"strings"
	"testing"

	sqljet "github.com/mgpnd/easylist-go/easylist-sql-jet"
)

// library is the module path prefix of the query library this driver uses.
const library = "github.com/go-jet/jet/"

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

// jetPostgres is jet's postgres package. Since jet v2.14.0 it imports
// github.com/lib/pq for its array literals, so lib/pq is compiled in with
// it; jetImports names the only lib/pq package it may import that way.
const (
	jetPostgres = "github.com/go-jet/jet/v2/postgres"
	jetImports  = "github.com/lib/pq"
)

// The driver package, without its tests, compiles in jet and none of the
// foreign packages, except lib/pq where jet's postgres package itself
// imports it.
func TestCompiledInPackages(t *testing.T) {
	pkg := reflect.TypeOf(sqljet.Driver{}).PkgPath()
	out, err := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}{{range .Imports}} {{.}}{{end}}", pkg).CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps %s: %v\n%s", pkg, err, out)
	}
	imports := map[string][]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		imports[fields[0]] = fields[1:]
	}
	found := false
	for dep := range imports {
		if strings.HasPrefix(dep, library) {
			found = true
			continue
		}
		if viaJet(dep, imports) {
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
	if !viaJet(jetImports, imports) {
		t.Logf("jet no longer compiles in %s; drop the exception", jetImports)
	}
}

// viaJet reports whether dep is lib/pq or one of its packages, compiled in
// only because jet's postgres package imports lib/pq: no package outside
// lib/pq imports any lib/pq package except jet's postgres package, which
// imports exactly jetImports.
func viaJet(dep string, imports map[string][]string) bool {
	if dep != jetImports && !strings.HasPrefix(dep, jetImports+"/") {
		return false
	}
	if !contains(imports[jetPostgres], jetImports) {
		return false
	}
	for importer, deps := range imports {
		if importer == jetPostgres || importer == jetImports || strings.HasPrefix(importer, jetImports+"/") {
			continue
		}
		for _, d := range deps {
			if d == jetImports || strings.HasPrefix(d, jetImports+"/") {
				return false
			}
		}
	}
	return true
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// The lib/pq exception holds only while jet's postgres package is the one
// package outside lib/pq that imports it.
func TestViaJet(t *testing.T) {
	jetOnly := map[string][]string{
		jetPostgres:              {"fmt", jetImports},
		jetImports:               {"github.com/lib/pq/oid"},
		"github.com/lib/pq/oid":  nil,
		"github.com/mgpnd/other": {jetPostgres},
	}
	direct := map[string][]string{
		jetPostgres:              {"fmt", jetImports},
		jetImports:               nil,
		"github.com/mgpnd/other": {jetPostgres, jetImports},
	}
	noJet := map[string][]string{
		jetPostgres:              {"fmt"},
		jetImports:               nil,
		"github.com/mgpnd/other": {jetImports},
	}
	for _, tc := range []struct {
		name    string
		dep     string
		imports map[string][]string
		want    bool
	}{
		{"lib/pq through jet", jetImports, jetOnly, true},
		{"lib/pq subpackage through jet", "github.com/lib/pq/oid", jetOnly, true},
		{"lib/pq imported directly too", jetImports, direct, false},
		{"lib/pq without jet", jetImports, noJet, false},
		{"other wire driver", "github.com/jackc/pgx/v5/stdlib", jetOnly, false},
		{"lib/pq look-alike", "github.com/lib/pqx", jetOnly, false},
	} {
		if got := viaJet(tc.dep, tc.imports); got != tc.want {
			t.Errorf("%s: viaJet(%q) = %v", tc.name, tc.dep, got)
		}
	}
}
