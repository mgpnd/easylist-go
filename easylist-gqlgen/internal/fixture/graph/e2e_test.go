package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	easylistexport "github.com/mgpnd/easylist-go/easylist-export"
	"github.com/mgpnd/easylist-go/easylist-gqlgen/internal/fixture/lists"
	sqlgoqu "github.com/mgpnd/easylist-go/easylist-sql-goqu"
)

// sqlDB records PostgreSQL queries from the real driver and answers them
// with rows whose columns are given in plan order.
type sqlDB struct {
	page, export [][]any
	total        int64
	calls        []sqlCall
}

type sqlCall struct {
	query string
	args  []any
}

func (db *sqlDB) QueryContext(_ context.Context, query string, args ...any) (easylist.Rows, error) {
	db.calls = append(db.calls, sqlCall{query, args})
	switch {
	case strings.HasPrefix(query, "SELECT COUNT(*)"):
		return &fakeRows{data: [][]any{{db.total}}}, nil
	case strings.Contains(query, " LIMIT "):
		return &fakeRows{data: db.page}, nil
	}
	return &fakeRows{data: db.export}, nil
}

// queueExporter stores requests as JSON, as an application queue would.
type queueExporter struct{ jobs [][]byte }

func (q *queueExporter) Export(_ context.Context, req easylist.ExportRequest) (string, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	q.jobs = append(q.jobs, data)
	return "job-1", nil
}

func TestEndToEnd(t *testing.T) {
	queue := &queueExporter{}
	db := &sqlDB{
		page:   [][]any{{int64(1), "Ann", "FR"}, {int64(4), "Dan", nil}},
		export: [][]any{{int64(1), "Ann", "FR"}, {int64(4), "Dan, Jr", nil}},
		total:  7,
	}
	rt := &easylist.Runtime{Driver: sqlgoqu.New(), DB: db, Exporter: queue}

	resp := run(t, rt, `{ people(query: {filter: "name CONTAINS 'a'", sort: [{field: COUNTRY, direction: DESC}], limit: 2}) {
		totalCount
		items { country display }
	} }`)
	if len(resp.Errors) > 0 {
		t.Fatalf("errors: %+v", resp.Errors)
	}
	if want := `{"people":{"totalCount":7,"items":[{"country":"FR","display":"Ann (FR)"},{"country":null,"display":"Dan"}]}}`; string(resp.Data) != want {
		t.Errorf("data = %s", resp.Data)
	}
	where := `WHERE ((u.deleted_at IS NULL) AND u."name" ILIKE $1 ESCAPE E'\\')`
	want := []sqlCall{
		{`SELECT u."id" AS "id", u."name" AS "name", c.name AS "country" FROM users AS u LEFT JOIN countries c ON c.id = u.country_id ` +
			where + ` ORDER BY c.name DESC, u."id" ASC LIMIT $2 OFFSET $3`, []any{"%a%", int64(2), int64(0)}},
		{`SELECT COUNT(*) FROM users AS u ` + where, []any{"%a%"}},
	}
	if !reflect.DeepEqual(db.calls, want) {
		t.Errorf("queries =\n%#v\nwant\n%#v", db.calls, want)
	}

	db.calls = nil
	resp = run(t, rt, `mutation { exportPeople(input: {fields: [NAME, COUNTRY, DISPLAY], filter: "name CONTAINS 'a'", format: CSV}) { id } }`)
	if len(resp.Errors) > 0 || string(resp.Data) != `{"exportPeople":{"id":"job-1"}}` {
		t.Fatalf("export = %s %+v", resp.Data, resp.Errors)
	}
	if len(db.calls) != 0 || len(queue.jobs) != 1 {
		t.Fatalf("mutation ran %d queries and queued %d jobs", len(db.calls), len(queue.jobs))
	}

	// A worker in another process decodes the job and writes the file.
	var req easylist.ExportRequest
	if err := json.Unmarshal(queue.jobs[0], &req); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := easylistexport.Run(context.Background(), lists.Registry, rt, req, &out); err != nil {
		t.Fatal(err)
	}
	if want := "Full name,country,display\nAnn,FR,Ann (FR)\n\"Dan, Jr\",,\"Dan, Jr\"\n"; out.String() != want {
		t.Errorf("csv = %q, want %q", out.String(), want)
	}
	wantExport := sqlCall{`SELECT u."id" AS "id", u."name" AS "name", c.name AS "country" FROM users AS u LEFT JOIN countries c ON c.id = u.country_id ` +
		where + ` ORDER BY u."id" ASC`, []any{"%a%"}}
	if len(db.calls) != 1 || !reflect.DeepEqual(db.calls[0], wantExport) {
		t.Errorf("export queries = %#v", db.calls)
	}
}
