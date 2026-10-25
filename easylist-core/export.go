package easylist

import (
	"context"
	"fmt"
	"strings"

	filter "github.com/mgpnd/easylist-go/easylist-filter"
)

// Format is an export file format.
type Format string

const (
	CSV  Format = "CSV"
	XLSX Format = "XLSX"
)

// ExportRequest is a serializable export job description. It carries no
// limit, offset, or scope: exports cover every matching row and the scope
// comes from the list definition when the job runs.
type ExportRequest struct {
	List   string   `json:"list"`
	Fields []string `json:"fields"` // Exported columns, in order.
	Filter string   `json:"filter,omitempty"`
	Sort   []Sort   `json:"sort,omitempty"`
	Format Format   `json:"format"`
}

// Exporter accepts validated export requests, for example by enqueueing a
// job, and returns its identifier. The application owns queuing, job status,
// and file storage.
type Exporter interface {
	Export(ctx context.Context, req ExportRequest) (jobID string, err error)
}

// ValidateExport checks req against the registry and returns its list and the
// planner request it describes. The mutation path and the export worker both
// use it, so they apply the same rules.
func (r *Registry) ValidateExport(req ExportRequest) (*CompiledList, Request, error) {
	l, ok := r.lists[req.List]
	if !ok {
		return nil, Request{}, fmt.Errorf("unknown list %q", req.List)
	}
	if req.Format != CSV && req.Format != XLSX {
		return nil, Request{}, fmt.Errorf("invalid export format %q", req.Format)
	}
	if len(req.Fields) == 0 {
		return nil, Request{}, fmt.Errorf("export of list %q names no fields", req.List)
	}
	seen := map[string]bool{}
	for _, name := range req.Fields {
		if _, ok := l.fields[name]; !ok {
			return nil, Request{}, fmt.Errorf("list %q has no field %q", req.List, name)
		}
		if seen[name] {
			return nil, Request{}, fmt.Errorf("field %q is exported more than once", name)
		}
		seen[name] = true
	}
	planReq := Request{Fields: req.Fields, Sort: req.Sort}
	if strings.TrimSpace(req.Filter) != "" {
		expr, err := filter.Parse(req.Filter)
		if err != nil {
			return nil, Request{}, err
		}
		planReq.Filter = expr
	}
	if _, err := l.Plan(KindExport, planReq); err != nil {
		return nil, Request{}, err
	}
	return l, planReq, nil
}
