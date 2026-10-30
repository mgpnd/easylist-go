// Package lists holds the list definitions whose behaviour the module
// snapshots. The seed in ../seed.sql is written for them. The package must
// import only easylist-core, because the module's generated GraphQL code
// imports it.
package lists

import (
	"context"
	"fmt"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
)

// People declares every field type as a scalar and as an array, nullable and
// non-nullable fields, an enum, a joined field, fields from a lateral join,
// and calculated fields of every kind. Its scope hides soft-deleted people.
var People = easylist.List{
	Name: "people", Table: "people", Alias: "p", PrimaryKey: "id",
	Scope: "p.deleted_at IS NULL",
	Fields: []easylist.Field{
		{Name: "id", Type: easylist.Integer},
		{Name: "uid", Type: easylist.UUID, Title: "Public ID"},
		{Name: "name", Type: easylist.Text, Title: "Full name"},
		{Name: "nickname", Type: easylist.Text, Nullable: true},
		{Name: "age", Type: easylist.Integer, Nullable: true},
		{Name: "height", Type: easylist.Float, Nullable: true},
		{Name: "balance", Type: easylist.Numeric, Nullable: true},
		{Name: "active", Type: easylist.Boolean, Nullable: true},
		{Name: "plan", Type: easylist.Enum, Values: []string{"free", "pro", "team"}},
		{Name: "due", Type: easylist.Date, Nullable: true, Title: "Due date"},
		{Name: "last_seen", Type: easylist.Timestamp, Nullable: true, Title: "Last seen"},
		{Name: "created_at", Type: easylist.Timestamp},

		// Joined fields.
		{Name: "country", Type: easylist.Text, Nullable: true, SQL: "c.name", Joins: []string{"country"}},
		{Name: "country_code", Type: easylist.Text, Nullable: true, SQL: "c.code", Joins: []string{"country"}},
		{Name: "order_count", Type: easylist.Integer, SQL: "ord.order_count", Joins: []string{"orders"}},
		{Name: "last_order_at", Type: easylist.Timestamp, Nullable: true, SQL: "ord.last_order_at", Joins: []string{"orders"}},

		// Arrays, one per type. roles is the only non-nullable one.
		{Name: "tags", Type: easylist.Text, Array: true, Nullable: true},
		{Name: "lucky_numbers", Type: easylist.Integer, Array: true, Nullable: true},
		{Name: "ratings", Type: easylist.Float, Array: true, Nullable: true},
		{Name: "prices", Type: easylist.Numeric, Array: true, Nullable: true},
		{Name: "answers", Type: easylist.Boolean, Array: true, Nullable: true},
		{Name: "holidays", Type: easylist.Date, Array: true, Nullable: true},
		{Name: "logins", Type: easylist.Timestamp, Array: true, Nullable: true},
		{Name: "friend_ids", Type: easylist.UUID, Array: true, Nullable: true},
		{Name: "roles", Type: easylist.Enum, Array: true, Values: []string{"admin", "editor", "viewer"}},

		// Calculated fields.
		easylist.Calc("display", easylist.Text, func(_ context.Context, r easylist.Row) (string, error) {
			name, _ := r.String("name")
			if country, ok := r.String("country"); ok {
				return name + " (" + country + ")", nil
			}
			return name, nil
		}, "name", "country"),
		// age_group is NULL without an age and fails on a negative one, so
		// one seeded row shows a calculated field error.
		easylist.CalcNullable("age_group", easylist.Text, func(_ context.Context, r easylist.Row) (*string, error) {
			age, ok := r.Int64("age")
			if !ok {
				return nil, nil
			}
			var group string
			switch {
			case age < 0:
				return nil, fmt.Errorf("age %d is negative", age)
			case age < 18:
				group = "minor"
			case age < 65:
				group = "adult"
			default:
				group = "senior"
			}
			return &group, nil
		}, "age"),
		easylist.Calc("tier", easylist.Enum, func(_ context.Context, r easylist.Row) (string, error) {
			switch n, _ := r.Int64("order_count"); {
			case n == 0:
				return "new", nil
			case n < 3:
				return "regular", nil
			default:
				return "loyal", nil
			}
		}, "order_count").WithValues("new", "regular", "loyal"),
		easylist.Calc("labels", easylist.Text, func(_ context.Context, r easylist.Row) ([]string, error) {
			plan, _ := r.String("plan")
			roles, _ := r.Strings("roles")
			labels := []string{"plan:" + plan}
			for _, role := range roles {
				labels = append(labels, "role:"+role)
			}
			return labels, nil
		}, "plan", "roles"),
	},
	Joins: []easylist.Join{
		{Key: "country", SQL: "LEFT JOIN countries c ON c.id = p.country_id"},
		// An aggregate returns exactly one row per person, even without orders.
		{Key: "orders", SQL: `LEFT JOIN LATERAL (
			SELECT count(*) AS order_count, max(o.placed_at) AS last_order_at FROM orders o WHERE o.person_id = p.id
		) ord ON TRUE`},
	},
}

// Countries is a small plain list.
var Countries = easylist.List{
	Name: "countries", Table: "countries", Alias: "c", PrimaryKey: "id",
	Fields: []easylist.Field{
		{Name: "id", Type: easylist.Integer},
		{Name: "name", Type: easylist.Text},
		{Name: "code", Type: easylist.Text},
	},
}

// Registry holds every fixture list. The snapshot tests and the generated
// GraphQL code use it.
var Registry = mustRegistry(People, Countries)

func mustRegistry(lists ...easylist.List) *easylist.Registry {
	reg, err := easylist.NewRegistry(lists...)
	if err != nil {
		panic(err)
	}
	return reg
}
