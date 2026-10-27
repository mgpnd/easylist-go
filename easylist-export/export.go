// Package easylistexport writes an easylist export request to CSV or XLSX.
// The application owns the job queue, job status, and file storage; a
// worker decodes an easylist.ExportRequest and calls Run.
package easylistexport

import (
	"context"
	"fmt"
	"io"
	"time"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
)

// Run validates req against reg, runs one export query through rt, and
// writes the complete file to w. Rows are encoded as they are read, so memory
// use does not grow with the row count. Run stops at the first error from
// validation, the query, a calculated field, the encoder, w, or ctx; output
// written before an error is incomplete.
//
// Relative values such as TODAY are evaluated at req.AsOf, or at rt.Now()
// when AsOf is zero, in the zone given by req.Timezone (time.Local when it is
// nil), with the runtime's week start.
func Run(ctx context.Context, reg *easylist.Registry, rt *easylist.Runtime, req easylist.ExportRequest, w io.Writer) error {
	// Definitions can change between enqueueing and running, so validate again.
	l, planReq, err := reg.ValidateExport(req)
	if err != nil {
		return err
	}
	if planReq.Now.IsZero() {
		zone := time.Local
		if req.Timezone != nil {
			// ValidateExport has already checked the offset.
			zone, _ = easylist.UTCOffset(*req.Timezone)
		}
		planReq.Now = rt.Now().In(zone)
	}
	if planReq.WeekStart == nil {
		planReq.WeekStart = rt.WeekStart
	}
	plan, err := l.Plan(easylist.KindExport, planReq)
	if err != nil {
		return err
	}
	fields := make([]easylist.Field, len(req.Fields))
	titles := make([]string, len(req.Fields))
	for i, name := range req.Fields {
		fields[i], _ = l.Field(name)
		titles[i] = fields[i].Title
	}

	var enc encoder
	switch req.Format {
	case easylist.CSV:
		enc = newCSV(w)
	case easylist.XLSX:
		if enc, err = newXLSX(w, l.Name()); err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid export format %q", req.Format)
	}
	defer enc.abort()

	if err := enc.header(titles); err != nil {
		return err
	}
	values := make([]any, len(fields))
	err = rt.Stream(ctx, l, plan, func(r easylist.Row) error {
		for i, f := range fields {
			if f.Calculated() {
				v, err := r.Calc(ctx, f.Name)
				if err != nil {
					return err
				}
				values[i] = v
			} else {
				values[i], _ = r.Value(f.Name)
			}
		}
		return enc.row(fields, values)
	})
	if err != nil {
		return err
	}
	return enc.close()
}

// encoder writes one file. Values are nil for NULL and otherwise use the Go
// representation of their field type.
type encoder interface {
	header(titles []string) error
	row(fields []easylist.Field, values []any) error
	close() error
	abort() // releases resources after an error; a no-op after close
}
