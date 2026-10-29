package conformance

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"reflect"
	"slices"
	"testing"
	"time"

	easylist "github.com/mgpnd/easylist-go/easylist-core"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// liveDB is the fixture schema of one Run.
type liveDB struct {
	db     *sql.DB
	schema string
}

// queryTimeout bounds every statement the suite runs.
const queryTimeout = 30 * time.Second

// openLive connects to EASYLIST_PG_DSN and creates the fixture in a new
// schema named easylist_conf_<random>, which t.Cleanup drops. Every
// connection uses that schema alone and the UTC session time zone, so the
// JSON text of timestamp arrays does not depend on the server's settings. It
// returns nil when the variable is unset.
func openLive(t *testing.T) *liveDB {
	t.Helper()
	dsn := os.Getenv(DSNEnv)
	if dsn == "" {
		return nil
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("conformance: invalid %s: %v", DSNEnv, err)
	}
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	schema := "easylist_conf_" + hex.EncodeToString(suffix[:])
	cfg.RuntimeParams["search_path"] = schema
	cfg.RuntimeParams["timezone"] = "UTC"
	db := stdlib.OpenDB(*cfg)

	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		t.Fatalf("conformance: cannot connect to the database in %s: %v", DSNEnv, err)
	}
	if _, err := db.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		db.Close()
		t.Fatalf("conformance: create schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
		defer cancel()
		if _, err := db.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("conformance: drop schema %s: %v", schema, err)
		}
		db.Close()
	})
	for _, stmt := range []string{fixtureDDL, fixtureRows} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("conformance: create fixture in schema %s: %v", schema, err)
		}
	}
	return &liveDB{db: db, schema: schema}
}

// check runs c's page or export query and its count query and compares the
// ids in order, the selected values, and the count.
func (l *liveDB) check(t *testing.T, d easylist.Driver, c testCase) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	kind := c.kinds()[0]
	query, args, err := d.Compile(c.plan(t, kind))
	if err != nil {
		t.Fatalf("%T: %v", d, err)
	}
	want := c.ids
	if kind == easylist.KindPage {
		want = pageOf(c.ids, c.limit, c.offset)
	}
	ids, values, err := l.rows(ctx, query, args)
	if err != nil {
		t.Fatalf("%T: %s query: %v\n%s %s", d, kindNames[kind], err, query, describe(args))
	}
	if !slices.Equal(ids, want) {
		t.Errorf("%T: %s query returns ids %v, want %v\n%s %s", d, kindNames[kind], ids, want, query, describe(args))
	}
	for name, wantValues := range c.values {
		got, ok := values[name]
		if !ok {
			t.Errorf("%T: %s query has no column %q\n%s", d, kindNames[kind], name, query)
			continue
		}
		if !reflect.DeepEqual(got, wantValues) {
			t.Errorf("%T: %s query returns %s values %#v, want %#v\n%s", d, kindNames[kind], name, got, wantValues, query)
		}
	}

	query, args, err = d.Compile(c.plan(t, easylist.KindCount))
	if err != nil {
		t.Fatalf("%T: %v", d, err)
	}
	var count int64
	if err := l.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		t.Fatalf("%T: count query: %v\n%s %s", d, err, query, describe(args))
	}
	if count != int64(len(c.ids)) {
		t.Errorf("%T: count query returns %d, want %d\n%s %s", d, count, len(c.ids), query, describe(args))
	}
}

// pageOf returns the ids a page with limit and offset holds. A nil limit is
// the fixture list's default page size, larger than the fixture.
func pageOf(ids []int64, limit, offset *int) []int64 {
	lo, hi := 0, len(ids)
	if offset != nil {
		lo = min(*offset, len(ids))
	}
	if limit != nil {
		hi = min(lo+*limit, len(ids))
	}
	return ids[lo:hi]
}

// rows runs a page or export query and returns the id column and every
// column's values in row order. Byte slices are returned as strings and
// NULL as nil.
func (l *liveDB) rows(ctx context.Context, query string, args []any) ([]int64, map[string][]any, error) {
	rows, err := l.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	if !slices.Contains(cols, "id") {
		return nil, nil, fmt.Errorf("query selects no id column")
	}
	ids := []int64{}
	values := map[string][]any{}
	for rows.Next() {
		row := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range row {
			ptrs[i] = &row[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, nil, err
		}
		for i, name := range cols {
			v := row[i]
			if b, ok := v.([]byte); ok {
				v = string(b)
			}
			values[name] = append(values[name], v)
			if name == "id" {
				id, ok := v.(int64)
				if !ok {
					return nil, nil, fmt.Errorf("id column holds %T", v)
				}
				ids = append(ids, id)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return ids, values, nil
}
