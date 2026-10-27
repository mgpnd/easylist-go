// Package lists holds the fixture project's list definitions. It must not
// import the gqlgen-generated packages, which import it.
package lists

import (
	"context"
	"errors"
	"strings"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
)

// Users uses every field type, nullable fields, enums, arrays, and calculated
// fields.
var Users = easylist.List{
	Name: "users", Table: "users", Alias: "u", PrimaryKey: "id",
	Scope: "u.deleted_at IS NULL",
	Fields: []easylist.Field{
		{Name: "id", Type: easylist.Integer},
		{Name: "uid", Type: easylist.UUID},
		{Name: "name", Type: easylist.Text, Title: "Full name"},
		{Name: "age", Type: easylist.Integer, Nullable: true},
		{Name: "score", Type: easylist.Float, Nullable: true},
		{Name: "balance", Type: easylist.Numeric},
		{Name: "active", Type: easylist.Boolean},
		{Name: "born", Type: easylist.Date, Nullable: true},
		{Name: "created_at", Type: easylist.Timestamp},
		{Name: "status", Type: easylist.Enum, Values: []string{"active", "on_hold"}},
		{Name: "country", Type: easylist.Text, Nullable: true, SQL: "c.name", Joins: []string{"country"}},
		{Name: "tags", Type: easylist.Text, Array: true, Nullable: true},
		{Name: "roles", Type: easylist.Enum, Array: true, Values: []string{"admin", "editor"}},
		easylist.Calc("display", easylist.Text, func(_ context.Context, r easylist.Row) (string, error) {
			name, _ := r.String("name")
			if name == "Boom" {
				return "", errors.New("cannot display Boom")
			}
			if country, ok := r.String("country"); ok {
				return name + " (" + country + ")", nil
			}
			return name, nil
		}, "name", "country"),
		easylist.CalcNullable("initials", easylist.Text, func(_ context.Context, r easylist.Row) (*string, error) {
			name, _ := r.String("name")
			if name == "" {
				return nil, nil
			}
			s := strings.ToUpper(name[:1])
			return &s, nil
		}, "name"),
		easylist.Calc("tier", easylist.Enum, func(_ context.Context, r easylist.Row) (string, error) {
			if active, _ := r.Bool("active"); active {
				return "gold", nil
			}
			return "basic", nil
		}, "active").WithValues("gold", "basic"),
		easylist.Calc("labels", easylist.Text, func(_ context.Context, r easylist.Row) ([]string, error) {
			roles, _ := r.Strings("roles")
			tags, _ := r.Strings("tags")
			var labels []string
			for _, role := range roles {
				labels = append(labels, "role:"+role)
			}
			for _, tag := range tags {
				labels = append(labels, "tag:"+tag)
			}
			return labels, nil
		}, "roles", "tags"),
	},
	Joins: []easylist.Join{
		{Key: "country", SQL: "LEFT JOIN countries c ON c.id = u.country_id"},
	},
}

// OrderItems shows a multi-word list name.
var OrderItems = easylist.List{
	Name: "order_items", Table: "order_items", Alias: "oi", PrimaryKey: "id",
	Fields: []easylist.Field{
		{Name: "id", Type: easylist.Integer},
		{Name: "sku", Type: easylist.Text},
		{Name: "quantity", Type: easylist.Integer},
	},
}

// Registry is used by the gqlgen entrypoint and by the generated code.
var Registry = mustRegistry(Users, OrderItems)

func mustRegistry(lists ...easylist.List) *easylist.Registry {
	reg, err := easylist.NewRegistry(lists...)
	if err != nil {
		panic(err)
	}
	return reg
}
