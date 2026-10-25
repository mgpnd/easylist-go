package easylistexport_test

// This file holds the Go snippets of README.md verbatim, so they compile.
// TestReadmeSnippets fails when a snippet and this file drift apart.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	easylistexport "github.com/mgpnd/easylist-go/easylist-export"
)

type Worker struct {
	Registry *easylist.Registry
	Runtime  *easylist.Runtime
	Dir      string
}

// Handle runs one export job whose payload is a JSON-encoded easylist.ExportRequest.
func (w *Worker) Handle(ctx context.Context, jobID string, payload []byte) (path string, err error) {
	var req easylist.ExportRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return "", err
	}
	ext := ".csv"
	if req.Format == easylist.XLSX {
		ext = ".xlsx"
	}
	path = filepath.Join(w.Dir, jobID+ext)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	if err := easylistexport.Run(ctx, w.Registry, w.Runtime, req, f); err != nil {
		f.Close()
		os.Remove(path) // a failed export leaves an incomplete file
		return "", err
	}
	return path, f.Close()
}

type noRows struct{}

func (noRows) QueryContext(context.Context, string, ...any) (easylist.Rows, error) {
	return emptyRows{}, nil
}

type emptyRows struct{}

func (emptyRows) Next() bool        { return false }
func (emptyRows) Scan(...any) error { return nil }
func (emptyRows) Err() error        { return nil }
func (emptyRows) Close() error      { return nil }

type selectDriver struct{}

func (selectDriver) Compile(*easylist.Plan) (string, []any, error) { return "SELECT", nil, nil }

func TestReadmeSnippets(t *testing.T) {
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	self, err := os.ReadFile("readme_example_test.go")
	if err != nil {
		t.Fatal(err)
	}
	blocks := regexp.MustCompile("(?s)```go\\n(.*?)```").FindAllSubmatch(readme, -1)
	if len(blocks) == 0 {
		t.Fatal("README has no Go snippets")
	}
	for _, b := range blocks {
		if !strings.Contains(string(self), string(b[1])) {
			t.Errorf("README snippet is not in readme_example_test.go:\n%s", b[1])
		}
	}

	reg, err := easylist.NewRegistry(easylist.List{
		Name: "users", Table: "users", Alias: "u", PrimaryKey: "id",
		Fields: []easylist.Field{{Name: "id", Type: easylist.Integer}, {Name: "name", Type: easylist.Text}},
	})
	if err != nil {
		t.Fatal(err)
	}
	w := &Worker{Registry: reg, Runtime: &easylist.Runtime{Driver: selectDriver{}, DB: noRows{}}, Dir: t.TempDir()}
	payload, _ := json.Marshal(easylist.ExportRequest{List: "users", Fields: []string{"name"}, Format: easylist.CSV})
	path, err := w.Handle(context.Background(), "job-1", payload)
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != "name\n" {
		t.Errorf("file = %q", data)
	}
	bad, _ := json.Marshal(easylist.ExportRequest{List: "users", Fields: []string{"nickname"}, Format: easylist.CSV})
	if _, err := w.Handle(context.Background(), "job-2", bad); err == nil {
		t.Error("invalid request accepted")
	}
	if _, err := os.Stat(filepath.Join(w.Dir, "job-2.csv")); err == nil {
		t.Error("failed export left a file")
	}
}
