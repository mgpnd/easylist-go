package easylistgql

import (
	"context"
	"errors"
	"fmt"
	"strings"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	filter "github.com/mgpnd/easylist-go/easylist-filter"

	"github.com/99designs/gqlgen/graphql"
)

// ListInput is the dialect-neutral form of a generated <P>ListInput.
type ListInput struct {
	Filter *string
	Sort   []easylist.Sort
	Limit  *int
	Offset *int
}

// Page is the result of ResolveList. Rows are in plan order; TotalCount is
// set only when totalCount was selected.
type Page struct {
	Rows       []easylist.Row
	TotalCount int
}

// ResolveList resolves a list query field. It reads the GraphQL selection
// from ctx: fields under items become the request's fields, the page query
// runs only when items is selected, and the count query only when
// totalCount is. Outside a GraphQL operation it fetches every field and the
// count. Parse and planning errors are returned before any query runs.
func ResolveList(ctx context.Context, rt *easylist.Runtime, reg *easylist.Registry, list string, in ListInput) (*Page, error) {
	l, ok := reg.List(list)
	if !ok {
		return nil, fmt.Errorf("easylist: unknown list %q", list)
	}
	req := easylist.Request{Sort: in.Sort, Limit: in.Limit, Offset: in.Offset}
	if in.Filter != nil && strings.TrimSpace(*in.Filter) != "" {
		expr, err := filter.Parse(*in.Filter)
		if err != nil {
			return nil, fmt.Errorf("invalid filter: %w", err)
		}
		req.Filter = expr
	}
	var want easylist.Want
	req.Fields, want = selection(ctx, l)
	res, err := rt.Fetch(ctx, l, req, want)
	if err != nil {
		return nil, err
	}
	return &Page{Rows: res.Rows, TotalCount: int(res.Total)}, nil
}

// selection collects the fields selected under items, through fragments,
// inline fragments, aliases, and @skip/@include, ignoring __typename.
func selection(ctx context.Context, l *easylist.CompiledList) ([]string, easylist.Want) {
	if !graphql.HasOperationContext(ctx) || graphql.GetFieldContext(ctx) == nil {
		var all []string
		for _, f := range l.Fields() {
			all = append(all, f.Name)
		}
		return all, easylist.Want{Rows: true, Count: true}
	}
	op := graphql.GetOperationContext(ctx)
	rowType := l.TypePrefix() + "Row"
	var fields []string
	seen := map[string]bool{}
	var want easylist.Want
	for _, page := range graphql.CollectFieldsCtx(ctx, []string{l.TypePrefix() + "Page"}) {
		switch page.Name {
		case "totalCount":
			want.Count = true
		case "items":
			want.Rows = true
			for _, f := range graphql.CollectFields(op, page.Selections, []string{rowType}) {
				if f.Name == "__typename" || seen[f.Name] {
					continue
				}
				seen[f.Name] = true
				fields = append(fields, f.Name)
			}
		}
	}
	return fields, want
}

// ExportInput is the dialect-neutral form of a generated <P>ExportInput.
type ExportInput struct {
	Fields []string
	Filter *string
	Sort   []easylist.Sort
	Format ExportFormat
}

// ErrExportsNotConfigured is returned when the runtime has no Exporter.
var ErrExportsNotConfigured = errors.New("easylist: exports are not configured: the runtime has no Exporter")

// Export validates an export input, hands it to the runtime's Exporter, and
// returns the job it reports.
func Export(ctx context.Context, rt *easylist.Runtime, reg *easylist.Registry, list string, in ExportInput) (*ExportJob, error) {
	if rt.Exporter == nil {
		return nil, ErrExportsNotConfigured
	}
	req := easylist.ExportRequest{List: list, Fields: in.Fields, Sort: in.Sort, Format: easylist.Format(in.Format)}
	if in.Filter != nil {
		req.Filter = *in.Filter
	}
	if _, _, err := reg.ValidateExport(req); err != nil {
		return nil, err
	}
	id, err := rt.Exporter.Export(ctx, req)
	if err != nil {
		return nil, err
	}
	return &ExportJob{ID: id}, nil
}
