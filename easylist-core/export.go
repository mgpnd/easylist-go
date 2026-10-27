package easylist

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	// AsOf is the moment relative values such as TODAY are evaluated at, so
	// a queued job exports what the user saw. Store it in UTC, for example
	// with t.UTC().Round(0), so it survives a JSON round trip unchanged. Zero
	// means the worker's clock.
	AsOf time.Time `json:"asOf,omitzero"`
	// Timezone is the client's UTC offset in minutes, positive east of UTC,
	// between -720 and 840. Nil means the server's local time zone.
	Timezone *int `json:"timezone,omitempty"`
}

// UTCOffset returns a fixed time zone the given number of minutes east of
// UTC. The offset must be between -720 (UTC-12:00) and 840 (UTC+14:00).
func UTCOffset(minutes int) (*time.Location, error) {
	if minutes < -720 || minutes > 840 {
		return nil, fmt.Errorf("UTC offset %d minutes must be between -720 and 840", minutes)
	}
	return time.FixedZone("", minutes*60), nil
}

// Exporter accepts validated export requests, for example by enqueueing a
// job, and returns its identifier. The application owns queuing, job status,
// and file storage.
type Exporter interface {
	Export(ctx context.Context, req ExportRequest) (jobID string, err error)
}

// ValidateExport checks req against the registry and returns its list and the
// planner request it describes. The mutation path and the export worker both
// use it, so they apply the same rules. When AsOf is set, the planner request's
// Now is AsOf in the request's zone: the Timezone offset, or time.Local when
// it is nil. Otherwise Now is left zero for the worker to fill, and relative
// values are only type-checked.
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
	zone := time.Local
	if req.Timezone != nil {
		var err error
		if zone, err = UTCOffset(*req.Timezone); err != nil {
			return nil, Request{}, err
		}
	}
	planReq := Request{Fields: req.Fields, Sort: req.Sort}
	if !req.AsOf.IsZero() {
		planReq.Now = req.AsOf.In(zone)
	}
	if strings.TrimSpace(req.Filter) != "" {
		expr, err := filter.Parse(req.Filter)
		if err != nil {
			return nil, Request{}, err
		}
		planReq.Filter = expr
	}
	if _, err := l.plan(KindExport, planReq, planReq.Now.IsZero()); err != nil {
		return nil, Request{}, err
	}
	return l, planReq, nil
}
