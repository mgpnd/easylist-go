package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeSchema(t *testing.T) string {
	t.Helper()
	name := filepath.Join(t.TempDir(), "schema.sql")
	if err := os.WriteFile(name, []byte("CREATE TABLE users (name text, age integer, signup_date date, active boolean)"), 0600); err != nil {
		t.Fatal(err)
	}
	return name
}

func TestRunSuccess(t *testing.T) {
	schema := writeSchema(t)
	for _, tc := range []struct {
		name, filter, sql string
		args              []any
	}{
		{"date", "signup_date = '2026-01-01'", `SELECT * FROM "users" WHERE ("users"."signup_date" = $1)`, []any{"2026-01-01"}},
		{"text and integer", "name = 'Artem' AND age < 30", `SELECT * FROM "users" WHERE (("users"."name" = $1) AND ("users"."age" < $2))`, []any{"Artem", float64(30)}},
		{"boolean", "active = TRUE", `SELECT * FROM "users" WHERE "users"."active" = $1`, []any{true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run([]string{"--schema", schema, "--table", "users", tc.filter}, &stdout, &stderr)
			if code != 0 || stderr.Len() != 0 {
				t.Fatalf("code = %d, stderr = %q", code, stderr.String())
			}
			decoder := json.NewDecoder(&stdout)
			var result map[string]any
			if err := decoder.Decode(&result); err != nil {
				t.Fatalf("decode output: %v", err)
			}
			if len(result) != 2 || result["sql"] != tc.sql || !reflect.DeepEqual(result["args"], tc.args) {
				t.Fatalf("result = %#v; want sql %q and args %#v", result, tc.sql, tc.args)
			}
			var extra any
			if err := decoder.Decode(&extra); err != io.EOF {
				t.Fatalf("expected exactly one JSON object, extra = %#v, error = %v", extra, err)
			}
		})
	}
}

func TestRunErrors(t *testing.T) {
	schema := writeSchema(t)
	badSchema := filepath.Join(t.TempDir(), "bad.sql")
	if err := os.WriteFile(badSchema, []byte("CREATE TABLE ("), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"missing file", []string{"--schema", filepath.Join(t.TempDir(), "missing.sql"), "--table", "users", "age = 1"}, "schema"},
		{"missing flags", []string{"age = 1"}, "schema"},
		{"missing schema flag", []string{"--table", "users", "age = 1"}, "schema"},
		{"missing table flag", []string{"--schema", schema, "age = 1"}, "table"},
		{"missing filter", []string{"--schema", schema, "--table", "users"}, "filter"},
		{"extra filter", []string{"--schema", schema, "--table", "users", "age = 1", "age = 2"}, "filter"},
		{"invalid filter", []string{"--schema", schema, "--table", "users", "age ="}, "filter"},
		{"empty filter", []string{"--schema", schema, "--table", "users", ""}, "filter"},
		{"invalid schema", []string{"--schema", badSchema, "--table", "users", "age = 1"}, "schema"},
		{"unknown table", []string{"--schema", schema, "--table", "missing", "age = 1"}, "table"},
		{"unknown flag", []string{"--unknown"}, "flag"},
		{"flag missing value", []string{"--schema"}, "flag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(tc.args, &stdout, &stderr); code == 0 {
				t.Fatal("expected nonzero status")
			}
			if stdout.Len() != 0 {
				t.Fatalf("failure wrote stdout: %q", stdout.String())
			}
			message := strings.TrimSpace(stderr.String())
			if !strings.Contains(message, tc.want) || strings.Contains(message, "\n") {
				t.Fatalf("expected concise error containing %q, got %q", tc.want, stderr.String())
			}
		})
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }

func TestRunOutputFailure(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]string{"--schema", writeSchema(t), "--table", "users", "age = 1"}, failingWriter{}, &stderr)
	if code == 0 || !strings.Contains(stderr.String(), "output unavailable") {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
}
