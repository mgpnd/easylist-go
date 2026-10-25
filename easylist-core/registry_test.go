package easylist

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestNewRegistryValidation(t *testing.T) {
	text := func(_ context.Context, _ Row) (string, error) { return "", nil }
	cases := []struct {
		name   string
		mutate func(l *List)
		extra  []List // registered after the mutated list
		want   []string
	}{
		{"valid identity", func(*List) {}, nil, nil},
		{"invalid list name", func(l *List) { l.Name = "Users-List" }, nil, []string{"Users-List"}},
		{"duplicate list names", func(*List) {}, []List{usersList()}, []string{`duplicate list name "users"`}},
		{"duplicate field name", func(l *List) {
			l.Fields = append(l.Fields, Field{Name: "display", Type: Text})
		}, nil, []string{"duplicate field", "display"}},
		{"invalid field name", func(l *List) { l.Fields[1].Name = "firstName" }, nil, []string{"firstName"}},
		{"field without type", func(l *List) { l.Fields[1].Type = 0 }, nil, []string{"name", "type"}},
		{"enum without values", func(l *List) { l.Fields[7].Values = nil }, nil, []string{"status"}},
		{"enum values collide", func(l *List) { l.Fields[7].Values = []string{"in-progress", "in_progress"} }, nil, []string{"IN_PROGRESS"}},
		{"enum value repeated", func(l *List) { l.Fields[7].Values = []string{"active", "active"} }, nil, []string{"twice"}},
		{"enum value invalid in GraphQL", func(l *List) { l.Fields[7].Values = []string{"1st"} }, nil, []string{"1st"}},
		{"enum value reserved in GraphQL", func(l *List) { l.Fields[7].Values = []string{"true"} }, nil, []string{"TRUE"}},
		{"values on non-enum", func(l *List) { l.Fields[1].Values = []string{"a"} }, nil, []string{"name", "not an enum"}},
		{"missing primary key", func(l *List) { l.PrimaryKey = "" }, nil, []string{"primary key"}},
		{"unknown primary key", func(l *List) { l.PrimaryKey = "uid" }, nil, []string{"uid"}},
		{"calculated primary key", func(l *List) { l.PrimaryKey = "display" }, nil, []string{"display"}},
		{"nullable primary key", func(l *List) { l.Fields[0].Nullable = true }, nil, []string{"nullable"}},
		{"field references unknown join", func(l *List) { l.Fields[3].Joins = []string{"planet"} }, nil, []string{"country", "planet"}},
		{"join depends on unknown join", func(l *List) { l.Joins[1].DependsOn = []string{"planet"} }, nil, []string{"city", "planet"}},
		{"join dependency cycle", func(l *List) { l.Joins[2].DependsOn = []string{"city"} }, nil, []string{"cycle", "city -> region -> city"}},
		{"join depends on itself", func(l *List) { l.Joins[0].DependsOn = []string{"country"} }, nil, []string{"country -> country"}},
		{"duplicate join key", func(l *List) { l.Joins = append(l.Joins, Join{Key: "country", SQL: "x"}) }, nil, []string{"duplicate join", "country"}},
		{"join without SQL", func(l *List) { l.Joins[0].SQL = " " }, nil, []string{"country", "no SQL"}},
		{"calculated requires unknown field", func(l *List) {
			l.Fields[8] = Calc("display", Text, text, "nickname")
		}, nil, []string{"display", "nickname"}},
		{"calculated requires calculated field", func(l *List) {
			l.Fields = append(l.Fields, Calc("label", Text, text, "display"))
		}, nil, []string{"label", "display", "only SQL fields"}},
		{"calculated return type mismatch", func(l *List) {
			l.Fields[8] = Calc("display", Integer, text, "name")
		}, nil, []string{"display", "int64", "string"}},
		{"calculated with SQL", func(l *List) { l.Fields[8].SQL = "u.x" }, nil, []string{"display", "SQL"}},
		{"default page size above maximum", func(l *List) { l.DefaultPageSize, l.MaxPageSize = 500, 200 }, nil, []string{"500", "200"}},
		{"negative default page size", func(l *List) { l.DefaultPageSize = -1 }, nil, []string{"-1"}},
		{"negative maximum page size", func(l *List) { l.MaxPageSize = -5 }, nil, []string{"-5"}},
		{"no table", func(l *List) { l.Table = "" }, nil, []string{"table"}},
		{"no alias", func(l *List) { l.Alias = "" }, nil, []string{"alias"}},
		{"only calculated fields", func(l *List) {
			l.Fields = []Field{Calc("x", Text, text)}
			l.PrimaryKey = "x"
		}, nil, []string{"SQL field"}},
		{"GraphQL prefix collision", func(*List) {}, []List{{Name: "users_"}}, []string{"Users"}},
		{"GraphQL type collision", func(l *List) {
			l.Fields = append(l.Fields, Field{Name: "row", Type: Enum, Values: []string{"a"}})
		}, nil, []string{"UsersRow"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := usersList()
			tc.mutate(&l)
			extra := slices.Clone(tc.extra)
			for i := range extra {
				if extra[i].Name != "users" {
					name := extra[i].Name
					extra[i] = usersList()
					extra[i].Name = name
				}
			}
			_, err := NewRegistry(append([]List{l}, extra...)...)
			if tc.want == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error containing %q", tc.want)
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

func TestRegistryDefaults(t *testing.T) {
	l := mustUsers(t)
	if l.DefaultPageSize() != 100 || l.MaxPageSize() != 10000 {
		t.Fatalf("page sizes = %d/%d, want 100/10000", l.DefaultPageSize(), l.MaxPageSize())
	}
	f, _ := l.Field("last_order")
	if f.Title != "last_order" {
		t.Errorf("default title = %q", f.Title)
	}
	f, _ = l.Field("name")
	if f.Title != "Full name" {
		t.Errorf("declared title = %q", f.Title)
	}

	small := usersList()
	small.MaxPageSize = 20
	reg, err := NewRegistry(small)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := reg.List("users")
	if c.DefaultPageSize() != 20 {
		t.Errorf("default page size under a small maximum = %d, want 20", c.DefaultPageSize())
	}
}

func TestRegistryIsolatedFromCaller(t *testing.T) {
	def := usersList()
	reg, err := NewRegistry(def)
	if err != nil {
		t.Fatal(err)
	}
	def.Fields[1].Name = "changed"
	def.Joins[0].SQL = "changed"
	l, _ := reg.List("users")
	if _, ok := l.Field("name"); !ok {
		t.Error("registry shares the caller's field slice")
	}
	if l.JoinOrder()[0].SQL == "changed" {
		t.Error("registry shares the caller's join slice")
	}
}

func TestJoinOrder(t *testing.T) {
	keys := func(js []Join) []string {
		out := make([]string, len(js))
		for i, j := range js {
			out[i] = j.Key
		}
		return out
	}
	cases := []struct {
		name  string
		joins []Join
		want  []string
	}{
		{"declaration order without dependencies", []Join{{Key: "b"}, {Key: "a"}, {Key: "c"}}, []string{"b", "a", "c"}},
		{"transitive", []Join{
			{Key: "city", DependsOn: []string{"region"}},
			{Key: "region", DependsOn: []string{"country"}},
			{Key: "country"},
		}, []string{"country", "region", "city"}},
		{"diamond", []Join{
			{Key: "d", DependsOn: []string{"c", "b"}},
			{Key: "c", DependsOn: []string{"a"}},
			{Key: "b", DependsOn: []string{"a"}},
			{Key: "a"},
		}, []string{"a", "c", "b", "d"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := List{Name: "t", Table: "t", Alias: "t", PrimaryKey: "id", Fields: []Field{{Name: "id", Type: Integer}}}
			for _, j := range tc.joins {
				j.SQL = "JOIN " + j.Key
				l.Joins = append(l.Joins, j)
			}
			for range 20 {
				reg, err := NewRegistry(l)
				if err != nil {
					t.Fatal(err)
				}
				c, _ := reg.List("t")
				if got := keys(c.JoinOrder()); !slices.Equal(got, tc.want) {
					t.Fatalf("order = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestFieldJoinClosure(t *testing.T) {
	l := mustUsers(t)
	if got := l.joinsOf["city"]; !slices.Equal(got, []string{"region", "city"}) {
		t.Errorf("city joins = %v", got)
	}
	if got := l.joinsOf["name"]; len(got) != 0 {
		t.Errorf("name joins = %v", got)
	}
	f, _ := l.Field("display")
	if !f.Calculated() || !slices.Equal(f.Requires(), []string{"name", "country"}) {
		t.Errorf("display requires = %v", f.Requires())
	}
}

func TestNaming(t *testing.T) {
	if got := PascalCase("order_items"); got != "OrderItems" {
		t.Errorf("PascalCase = %q", got)
	}
	if got := EnumName("on_hold"); got != "ON_HOLD" {
		t.Errorf("EnumName = %q", got)
	}
	if got := EnumName("in-progress"); got != "IN_PROGRESS" {
		t.Errorf("EnumName = %q", got)
	}
}
