package behavior_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mgpnd/easylist-go/easylist-behavior/graph"
	easylist "github.com/mgpnd/easylist-go/easylist-core"
	sqlgorm "github.com/mgpnd/easylist-go/easylist-sql-gorm"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
)

// TestGraphQLSnapshots sends every testdata/queries/<name>.graphql to the
// module's GraphQL server and writes the response, re-indented in the
// server's field order, to <name>.json.
func TestGraphQLSnapshots(t *testing.T) {
	db := openDB(t)
	files, err := filepath.Glob(filepath.Join("testdata", "queries", "*.graphql"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("testdata/queries holds no fixture")
	}
	rt := &easylist.Runtime{Driver: sqlgorm.New(), DB: easylist.SQLDB(db), Clock: clock}
	srv := handler.New(graph.NewExecutableSchema(graph.Config{Resolvers: &graph.Resolver{Runtime: rt}}))
	srv.AddTransport(transport.POST{})

	keep := map[string]bool{}
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".graphql")
		keep[name+".json"] = true
		t.Run(name, func(t *testing.T) {
			query, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			golden(t, "queries/"+name+".json", postQuery(t, srv, string(query)))
		})
	}
	prune(t, "queries", keep, ".json")
}

// postQuery sends query to srv as a GraphQL POST request and returns the
// response body indented with two spaces.
func postQuery(t *testing.T, srv http.Handler, query string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	var out bytes.Buffer
	if err := json.Indent(&out, w.Body.Bytes(), "", "  "); err != nil {
		t.Fatalf("response is not JSON: %v\n%s", err, w.Body.String())
	}
	return out.String()
}
