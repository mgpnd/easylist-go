package behavior_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestFixtureDatabase checks that openDB yields the seeded fixture in its
// own schema with a UTC session.
func TestFixtureDatabase(t *testing.T) {
	db := openDB(t)
	var schema, zone string
	var people int
	if err := db.QueryRow("SELECT current_schema(), current_setting('TimeZone'), (SELECT count(*) FROM people)").
		Scan(&schema, &zone, &people); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(schema, "easylist_behavior_") || len(schema) != len("easylist_behavior_")+16 {
		t.Errorf("current schema is %q, want easylist_behavior_<16 hex digits>", schema)
	}
	if zone != "UTC" {
		t.Errorf("session time zone is %q, want UTC", zone)
	}
	if people == 0 {
		t.Error("people is empty; the seed did not load")
	}
}

func TestLineDiff(t *testing.T) {
	for _, c := range []struct{ name, want, got, diff string }{
		{"equal", "a\nb\n", "a\nb\n", ""},
		{
			"changed line",
			"1\n2\n3\n4\n5\n6\n7\n8\n",
			"1\n2\n3\n4\nfive\n6\n7\n8\n",
			"@@ line 3 @@\n 3\n 4\n-5\n+five\n 6\n 7\n",
		},
		{"added at end", "a\n", "a\nb\n", "@@ line 1 @@\n a\n+b\n"},
		{"removed at start", "a\nb\nc\nd\n", "b\nc\nd\n", "@@ line 1 @@\n-a\n b\n c\n"},
		{
			"two hunks",
			"a\n1\n2\n3\n4\n5\n6\nb\n",
			"A\n1\n2\n3\n4\n5\n6\nB\n",
			"@@ line 1 @@\n-a\n+A\n 1\n 2\n@@ line 6 @@\n 5\n 6\n-b\n+B\n",
		},
	} {
		if got := lineDiff(c.want, c.got); got != c.diff {
			t.Errorf("%s: lineDiff =\n%s\nwant\n%s", c.name, got, c.diff)
		}
	}
}

func TestOrphans(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.txt", "a.golden", "a.sql.golden", "b.golden", "b.sql.golden", "c.json", "notes.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub.golden"), 0o755); err != nil {
		t.Fatal(err)
	}
	keep := map[string]bool{"a.golden": true, "a.sql.golden": true}
	got, err := orphans(dir, keep, ".golden", ".json")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"b.golden", "b.sql.golden", "c.json"}; !slices.Equal(got, want) {
		t.Errorf("orphans = %v, want %v", got, want)
	}
	got, err = orphans(filepath.Join(dir, "missing"), keep, ".golden")
	if err != nil || got != nil {
		t.Errorf("orphans of a missing directory = %v, %v; want none", got, err)
	}
}
