package behavior_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/stdlib"

	behavior "github.com/mgpnd/easylist-go/easylist-behavior"
)

// dsnEnv names the environment variable holding the connection string of
// the PostgreSQL database the snapshot tests run against.
const dsnEnv = "EASYLIST_PG_DSN"

// skipNoDSN is the message every snapshot test skips with when dsnEnv is
// unset. The guard step in .github/workflows/ci.yml fails a run whose log
// contains it, as it does for the skip of the easylist-sqlpg conformance
// suite's live cases, so keep the two texts identical.
const skipNoDSN = dsnEnv + " is not set"

// queryTimeout bounds every statement the harness runs.
const queryTimeout = 30 * time.Second

// refTime is the reference time of every snapshot: Tuesday 2026-10-27
// 10:00 UTC, the instant the conformance suite uses too. The seed places
// its values relative to it.
var refTime = time.Date(2026, time.October, 27, 10, 0, 0, 0, time.UTC)

// clock is the runtime Clock of every snapshot. It returns refTime.
func clock() time.Time { return refTime }

// update makes the snapshot tests rewrite their outcome files instead of
// comparing them, and delete outcome files whose fixture is gone.
var update = flag.Bool("update", false, "rewrite outcome files")

// shared is the fixture database of one test binary run. The first openDB
// call creates it, and TestMain drops its schema after every test ran.
var shared struct {
	once   sync.Once
	db     *sql.DB
	schema string
	err    error
}

func TestMain(m *testing.M) {
	code := m.Run()
	if err := closeShared(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

// openDB returns the fixture database: a schema named
// easylist_behavior_<random> holding behavior.Schema and behavior.Seed,
// reached by connections whose search_path is that schema alone and whose
// session time zone is UTC. All tests of one run share it, so they must not
// change its data. It skips t when EASYLIST_PG_DSN is unset and fails t when
// the variable is set but the database cannot be prepared. Call it before
// computing any outcome, so that no outcome file is written without a
// database.
func openDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv(dsnEnv)
	if dsn == "" {
		// .github/workflows/ci.yml fails the run when it finds this message.
		t.Skip(skipNoDSN)
	}
	shared.once.Do(func() {
		shared.db, shared.schema, shared.err = createFixture(dsn)
	})
	if shared.err != nil {
		t.Fatal(shared.err)
	}
	return shared.db
}

// createFixture connects to dsn and loads the fixture into a new schema.
// On error, nothing is left behind.
func createFixture(dsn string) (*sql.DB, string, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, "", fmt.Errorf("behavior: invalid %s: %v", dsnEnv, err)
	}
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return nil, "", err
	}
	schema := "easylist_behavior_" + hex.EncodeToString(suffix[:])
	cfg.RuntimeParams["search_path"] = schema
	cfg.RuntimeParams["timezone"] = "UTC"
	// pgx scans timestamptz into time.Local by default, so outcomes would
	// depend on the machine's time zone. Scan them in UTC instead.
	db := stdlib.OpenDB(*cfg, stdlib.OptionAfterConnect(func(_ context.Context, conn *pgx.Conn) error {
		conn.TypeMap().RegisterType(&pgtype.Type{
			Name: "timestamptz", OID: pgtype.TimestamptzOID, Codec: &pgtype.TimestamptzCodec{ScanLocation: time.UTC},
		})
		return nil
	}))

	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, "", fmt.Errorf("behavior: cannot connect to the database in %s: %v", dsnEnv, err)
	}
	if _, err := db.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		db.Close()
		return nil, "", fmt.Errorf("behavior: create schema %s: %v", schema, err)
	}
	for _, step := range []struct{ name, sql string }{
		{"schema.sql", behavior.Schema},
		{"seed.sql", behavior.Seed},
	} {
		if _, err := db.ExecContext(ctx, step.sql); err != nil {
			err = fmt.Errorf("behavior: load %s into schema %s: %v", step.name, schema, err)
			return nil, "", errors.Join(err, dropSchema(db, schema))
		}
	}
	return db, schema, nil
}

// closeShared drops the shared fixture schema, if a test created it.
func closeShared() error {
	if shared.db == nil {
		return nil
	}
	return dropSchema(shared.db, shared.schema)
}

// dropSchema drops schema with everything in it and closes db.
func dropSchema(db *sql.DB, schema string) error {
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	if _, err := db.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
		return fmt.Errorf("behavior: drop schema %s: %v", schema, err)
	}
	return nil
}

// golden compares got with the outcome file testdata/<path>, after ending
// got with exactly one newline. On a difference it fails t with the file
// name and a line diff; a missing file fails t with a hint to run -update.
// With -update it writes the file instead, creating its directory.
func golden(t *testing.T, path, got string) {
	t.Helper()
	got = strings.TrimRight(got, "\n") + "\n"
	file := filepath.Join("testdata", filepath.FromSlash(path))
	if *update {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%s does not exist; run go test . -update to create it", file)
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		diff := lineDiff(string(want), got)
		if diff == "" {
			diff = "(only trailing newlines differ)\n"
		}
		t.Errorf("%s differs from the computed outcome; run go test . -update to accept it\n--- %s\n+++ computed\n%s",
			file, file, diff)
	}
}

// prune checks testdata/<dir> for orphaned outcome files: files whose name
// ends in one of exts and is not in keep. keep holds the outcome file names
// (relative to the directory) that the fixtures in it call for, computed
// from the fixtures rather than from the tests that ran, so -run does not
// make outcome files look orphaned. With -update, prune deletes orphans;
// otherwise it fails t naming each. A missing directory has no orphans.
func prune(t *testing.T, dir string, keep map[string]bool, exts ...string) {
	t.Helper()
	full := filepath.Join("testdata", filepath.FromSlash(dir))
	names, err := orphans(full, keep, exts...)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		file := filepath.Join(full, name)
		if *update {
			if err := os.Remove(file); err != nil {
				t.Fatal(err)
			}
			continue
		}
		t.Errorf("%s has no fixture; run go test . -update to delete it", file)
	}
}

// orphans returns the sorted names of the regular files in dir that end in
// one of exts and are not in keep.
func orphans(dir string, keep map[string]bool, exts ...string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		name := e.Name()
		if !e.Type().IsRegular() || keep[name] {
			continue
		}
		if slices.ContainsFunc(exts, func(ext string) bool { return strings.HasSuffix(name, ext) }) {
			names = append(names, name)
		}
	}
	return names, nil // os.ReadDir sorts by name
}

// diffContext is the number of unchanged lines lineDiff shows around each
// change.
const diffContext = 2

// lineDiff returns a line diff turning want into got. Each line starts with
// "-" (only in want), "+" (only in got), or " " (in both); runs of unchanged
// lines further than diffContext from a change are elided to a "@@ line N @@"
// header naming the next shown line of want. It uses a longest common
// subsequence, which is quadratic but fine for outcome files.
func lineDiff(want, got string) string {
	a := strings.Split(strings.TrimSuffix(want, "\n"), "\n")
	b := strings.Split(strings.TrimSuffix(got, "\n"), "\n")

	// lcs[i][j] is the LCS length of a[i:] and b[j:].
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}

	type op struct {
		kind byte // ' ', '-', '+'
		line string
		at   int // 1-based line in want for ' ' and '-'; the next want line for '+'
	}
	var ops []op
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			ops = append(ops, op{' ', a[i], i + 1})
			i++
			j++
		case i < len(a) && (j == len(b) || lcs[i+1][j] >= lcs[i][j+1]):
			ops = append(ops, op{'-', a[i], i + 1})
			i++
		default:
			ops = append(ops, op{'+', b[j], i + 1})
			j++
		}
	}

	// show marks the ops within diffContext of a change.
	show := make([]bool, len(ops))
	for k, o := range ops {
		if o.kind == ' ' {
			continue
		}
		for c := max(0, k-diffContext); c <= min(len(ops)-1, k+diffContext); c++ {
			show[c] = true
		}
	}
	var sb strings.Builder
	for k, o := range ops {
		if !show[k] {
			continue
		}
		if k == 0 || !show[k-1] {
			fmt.Fprintf(&sb, "@@ line %d @@\n", o.at)
		}
		sb.WriteByte(o.kind)
		sb.WriteString(o.line)
		sb.WriteByte('\n')
	}
	return sb.String()
}
