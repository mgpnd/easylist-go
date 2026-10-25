package easylist

import (
	"slices"
	"strings"
	"testing"

	filter "github.com/mgpnd/easylist-go/easylist-filter"
)

func parse(t *testing.T, input string) filter.Expr {
	t.Helper()
	expr, err := filter.Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	return expr
}

func fieldNames(p *Plan) []string {
	out := []string{}
	for _, f := range p.Fields {
		out = append(out, f.Name)
	}
	return out
}

func joinKeys(p *Plan) []string {
	out := []string{}
	for _, j := range p.Joins {
		out = append(out, j.Key)
	}
	return out
}

func orderText(p *Plan) []string {
	out := []string{}
	for _, o := range p.Order {
		dir := "asc"
		if o.Desc {
			dir = "desc"
		}
		out = append(out, o.Field.Name+" "+dir)
	}
	return out
}

func TestDriverReadsFieldsThroughLookup(t *testing.T) {
	l := mustUsers(t)
	p, err := l.Plan(KindPage, Request{Filter: parse(t, "country = 'FR' AND status = 'active'")})
	if err != nil {
		t.Fatal(err)
	}
	var d Driver = fakeDriver{}
	if _, _, err := d.Compile(p); err != nil {
		t.Fatal(err)
	}
	country, ok := p.Lookup("country")
	if !ok || country.SQL != "c.name" || country.Type != Text || !country.Nullable {
		t.Errorf("country = %+v, %v", country, ok)
	}
	status, ok := p.Lookup("status")
	if !ok || status.Type != Enum || !slices.Equal(status.Values, []string{"active", "blocked"}) {
		t.Errorf("status = %+v, %v", status, ok)
	}
	if _, ok := p.Lookup("display"); ok {
		t.Error("lookup resolves a calculated field")
	}
	if _, ok := p.Lookup("nickname"); ok {
		t.Error("lookup resolves an unknown field")
	}
	if p.Table != "users" || p.Alias != "u" || p.Scope != "u.deleted_at IS NULL" || p.List != "users" {
		t.Errorf("plan identity = %q %q %q %q", p.List, p.Table, p.Alias, p.Scope)
	}
}

func TestPlanPage(t *testing.T) {
	cases := []struct {
		name   string
		req    Request
		fields []string
		joins  []string
		order  []string
		limit  int
		offset int
	}{
		{"no selected fields", Request{}, []string{"id"}, nil, []string{"id asc"}, 100, 0},
		{"calculated field pulls in requirements", Request{Fields: []string{"display"}},
			[]string{"id", "name", "country"}, []string{"country"}, nil, 100, 0},
		{"filter-only field is not selected", Request{Fields: []string{"name"}, Filter: parse(t, "country = 'FR'")},
			[]string{"id", "name"}, []string{"country"}, nil, 100, 0},
		{"unused join omitted", Request{Fields: []string{"name", "age"}}, []string{"id", "name", "age"}, nil, nil, 100, 0},
		{"transitive dependency included", Request{Fields: []string{"city"}}, []string{"id", "city"}, []string{"region", "city"}, nil, 100, 0},
		{"sort join included", Request{Sort: []Sort{{Field: "last_order", Direction: Desc}}},
			[]string{"id"}, []string{"last_order"}, []string{"last_order desc", "id asc"}, 100, 0},
		{"joins in topological order", Request{Fields: []string{"last_order", "city", "country"}},
			[]string{"id", "country", "city", "last_order"}, []string{"country", "region", "city", "last_order"}, nil, 100, 0},
		{"duplicates selected once", Request{Fields: []string{"name", "name", "display", "id"}},
			[]string{"id", "name", "country"}, []string{"country"}, nil, 100, 0},
		{"primary key tiebreaker", Request{Sort: []Sort{{Field: "name", Direction: Desc}}},
			[]string{"id"}, nil, []string{"name desc", "id asc"}, 100, 0},
		{"request sorts on primary key", Request{Sort: []Sort{{Field: "id", Direction: Desc}}},
			[]string{"id"}, nil, []string{"id desc"}, 100, 0},
		{"explicit limit and offset", Request{Limit: intp(50), Offset: intp(100)}, []string{"id"}, nil, nil, 50, 100},
		{"maximum limit", Request{Limit: intp(10000)}, []string{"id"}, nil, nil, 10000, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := mustUsers(t).Plan(KindPage, tc.req)
			if err != nil {
				t.Fatal(err)
			}
			if got := fieldNames(p); !slices.Equal(got, tc.fields) {
				t.Errorf("fields = %v, want %v", got, tc.fields)
			}
			if got := joinKeys(p); !slices.Equal(got, append([]string{}, tc.joins...)) {
				t.Errorf("joins = %v, want %v", got, tc.joins)
			}
			if tc.order == nil {
				tc.order = []string{"id asc"}
			}
			if got := orderText(p); !slices.Equal(got, tc.order) {
				t.Errorf("order = %v, want %v", got, tc.order)
			}
			if p.Limit != tc.limit || p.Offset != tc.offset {
				t.Errorf("limit/offset = %d/%d, want %d/%d", p.Limit, p.Offset, tc.limit, tc.offset)
			}
		})
	}
}

func TestPlanJoinSQL(t *testing.T) {
	p, err := mustUsers(t).Plan(KindPage, Request{Fields: []string{"country"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Joins) != 1 || p.Joins[0].SQL != "LEFT JOIN countries c ON c.id = u.country_id" {
		t.Errorf("joins = %+v", p.Joins)
	}
}

func TestPlanErrors(t *testing.T) {
	cases := []struct {
		name string
		kind Kind
		req  Request
		want []string
	}{
		{"unknown selected field", KindPage, Request{Fields: []string{"name", "nickname", "other"}}, []string{`"nickname"`}},
		{"filter on calculated field", KindPage, Request{Filter: parse(t, "name = 'a' OR display = 'x'")}, []string{"display", "position 15"}},
		{"filter on unknown field", KindPage, Request{Filter: parse(t, "nickname = 'a'")}, []string{"nickname", "position 1"}},
		{"first unknown filter field reported", KindPage, Request{Filter: parse(t, "a = 1 AND b = 2")}, []string{`"a"`, "position 1"}},
		{"sort on calculated field", KindPage, Request{Sort: []Sort{{Field: "display"}}}, []string{"display"}},
		{"sort on unknown field", KindPage, Request{Sort: []Sort{{Field: "nickname"}}}, []string{"nickname"}},
		{"sort field twice", KindPage, Request{Sort: []Sort{{Field: "name"}, {Field: "name", Direction: Desc}}}, []string{"name", "more than once"}},
		{"invalid direction", KindPage, Request{Sort: []Sort{{Field: "name", Direction: "UP"}}}, []string{"UP"}},
		{"limit above maximum", KindPage, Request{Limit: intp(20000)}, []string{"20000", "maximum page size 10000"}},
		{"limit below one", KindPage, Request{Limit: intp(0)}, []string{"limit 0"}},
		{"negative offset", KindPage, Request{Offset: intp(-1)}, []string{"offset -1"}},
		{"count validates limits too", KindCount, Request{Limit: intp(20000)}, []string{"20000"}},
		{"export rejects limit", KindExport, Request{Limit: intp(5)}, []string{"no limit"}},
		{"invalid kind", Kind(9), Request{}, []string{"kind"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := mustUsers(t).Plan(tc.kind, tc.req)
			if err == nil {
				t.Fatal("expected an error")
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

func TestPlanCount(t *testing.T) {
	l := mustUsers(t)
	p, err := l.Plan(KindCount, Request{
		Fields: []string{"country"},
		Sort:   []Sort{{Field: "last_order"}},
		Filter: parse(t, "name = 'a'"),
		Limit:  intp(10), Offset: intp(20),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Joins) != 0 || len(p.Fields) != 0 || len(p.Order) != 0 || p.Limit != 0 || p.Offset != 0 {
		t.Errorf("count plan carries page data: %+v", p)
	}
	if p.Filter == nil || p.Scope == "" {
		t.Error("count plan lost filter or scope")
	}

	p, err = l.Plan(KindCount, Request{Filter: parse(t, "city = 'Lyon'")})
	if err != nil {
		t.Fatal(err)
	}
	if got := joinKeys(p); !slices.Equal(got, []string{"region", "city"}) {
		t.Errorf("count joins = %v", got)
	}
}

func TestPlanExport(t *testing.T) {
	p, err := mustUsers(t).Plan(KindExport, Request{Fields: []string{"display"}, Sort: []Sort{{Field: "name"}}})
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != KindExport || p.Limit != 0 || p.Offset != 0 {
		t.Errorf("export pagination = %d/%d", p.Limit, p.Offset)
	}
	if got := fieldNames(p); !slices.Equal(got, []string{"id", "name", "country"}) {
		t.Errorf("export fields = %v", got)
	}
	if got := orderText(p); !slices.Equal(got, []string{"name asc", "id asc"}) {
		t.Errorf("export order = %v", got)
	}
}

func TestPlanIsDeterministic(t *testing.T) {
	l := mustUsers(t)
	req := Request{Fields: []string{"city", "last_order", "country", "display"}, Filter: parse(t, "region = 'x'")}
	first, err := l.Plan(KindPage, req)
	if err != nil {
		t.Fatal(err)
	}
	for range 50 {
		p, _ := l.Plan(KindPage, req)
		if !slices.Equal(joinKeys(p), joinKeys(first)) || !slices.Equal(fieldNames(p), fieldNames(first)) {
			t.Fatalf("plan changed: %v %v", joinKeys(p), fieldNames(p))
		}
	}
}
