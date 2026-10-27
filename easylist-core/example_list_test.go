package easylist_test

import (
	"context"
	"strings"
	"time"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
)

// ExampleList declares a list using every field type and array fields. It has
// no Output comment, so it is compiled but not run.
func ExampleList() {
	_ = easylist.List{
		Name: "products", Table: "products", Alias: "p", PrimaryKey: "id",
		Scope:           "p.archived = FALSE",
		DefaultPageSize: 50,
		MaxPageSize:     500,
		Fields: []easylist.Field{
			{Name: "id", Type: easylist.UUID},
			{Name: "name", Type: easylist.Text, Title: "Product name"},
			{Name: "stock", Type: easylist.Integer},
			{Name: "rating", Type: easylist.Float, Nullable: true},
			{Name: "price", Type: easylist.Numeric},
			{Name: "visible", Type: easylist.Boolean},
			{Name: "launched_on", Type: easylist.Date, Nullable: true},
			{Name: "updated_at", Type: easylist.Timestamp},
			{Name: "kind", Type: easylist.Enum, Values: []string{"physical", "digital"}},
			{Name: "vendor", Type: easylist.Text, Nullable: true, SQL: "v.name", Joins: []string{"vendor"}},
			{Name: "tags", Type: easylist.Text, Array: true, Nullable: true},
			{Name: "sizes", Type: easylist.Integer, Array: true, SQL: "ARRAY(SELECT s.size FROM sizes s WHERE s.product_id = p.id ORDER BY s.size)"},
			{Name: "channels", Type: easylist.Enum, Array: true, Values: []string{"web", "store"}},
			easylist.Calc("label", easylist.Text, func(_ context.Context, r easylist.Row) (string, error) {
				name, _ := r.String("name")
				return strings.ToUpper(name), nil
			}, "name").WithTitle("Label"),
			easylist.CalcNullable("age_days", easylist.Integer, func(_ context.Context, r easylist.Row) (*int64, error) {
				day, ok := r.Time("launched_on")
				if !ok {
					return nil, nil
				}
				days := int64(time.Since(day) / (24 * time.Hour))
				return &days, nil
			}, "launched_on"),
			easylist.Calc("tier", easylist.Enum, func(_ context.Context, r easylist.Row) (string, error) {
				if stock, _ := r.Int64("stock"); stock > 100 {
					return "high", nil
				}
				return "low", nil
			}, "stock").WithValues("high", "low"),
			easylist.Calc("badges", easylist.Text, func(_ context.Context, r easylist.Row) ([]string, error) {
				var badges []string
				if visible, _ := r.Bool("visible"); visible {
					badges = append(badges, "visible")
				}
				return badges, nil
			}, "visible"),
		},
		Joins: []easylist.Join{
			{Key: "vendor", SQL: "LEFT JOIN vendors v ON v.id = p.vendor_id"},
		},
	}
}
