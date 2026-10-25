package easylistgql

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql"
)

func marshaled(m graphql.Marshaler) string {
	var b bytes.Buffer
	m.MarshalGQL(&b)
	return b.String()
}

func TestInt64Scalar(t *testing.T) {
	if got := marshaled(MarshalInt64(-9223372036854775808)); got != "-9223372036854775808" {
		t.Errorf("marshal = %s", got)
	}
	for _, in := range []any{int(5), int32(5), int64(5), json.Number("5"), "5", float64(5)} {
		if v, err := UnmarshalInt64(in); err != nil || v != 5 {
			t.Errorf("unmarshal %#v = %d, %v", in, v, err)
		}
	}
	for _, in := range []any{"9223372036854775808", "1.5", json.Number("1e3"), 1.5, true, nil, "abc"} {
		if _, err := UnmarshalInt64(in); err == nil {
			t.Errorf("unmarshal %#v succeeded", in)
		}
	}
}

func TestDecimalScalar(t *testing.T) {
	if got := marshaled(MarshalDecimal("10.50")); got != `"10.50"` {
		t.Errorf("marshal = %s", got)
	}
	for _, in := range []any{"12345678901234567890.10", json.Number("-0.5"), "+3"} {
		if _, err := UnmarshalDecimal(in); err != nil {
			t.Errorf("unmarshal %#v: %v", in, err)
		}
	}
	for _, in := range []any{"1e3", "NaN", "", "1.", 1.5, nil} {
		if _, err := UnmarshalDecimal(in); err == nil {
			t.Errorf("unmarshal %#v succeeded", in)
		}
	}
}

func TestDateScalar(t *testing.T) {
	day := time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC)
	if got := marshaled(MarshalDate(day)); got != `"2024-02-29"` {
		t.Errorf("marshal = %s", got)
	}
	if v, err := UnmarshalDate("2024-02-29"); err != nil || !v.Equal(day) {
		t.Errorf("unmarshal = %v, %v", v, err)
	}
	for _, in := range []any{"2025-02-29", "2024-2-29", "2024-02-29T00:00:00Z", 20240229, nil} {
		if _, err := UnmarshalDate(in); err == nil {
			t.Errorf("unmarshal %#v succeeded", in)
		}
	}
}

func TestUUIDScalar(t *testing.T) {
	const id = "0b6c4c1e-3d8e-4f43-9d5a-2a8a0d1d2c3b"
	if got := marshaled(MarshalUUID("0B6C4C1E-3D8E-4F43-9D5A-2A8A0D1D2C3B")); got != `"`+id+`"` {
		t.Errorf("marshal = %s", got)
	}
	if v, err := UnmarshalUUID("0B6C4C1E-3D8E-4F43-9D5A-2A8A0D1D2C3B"); err != nil || v != id {
		t.Errorf("unmarshal = %v, %v", v, err)
	}
	for _, in := range []any{"not-a-uuid", "0b6c4c1e3d8e4f439d5a2a8a0d1d2c3b", "{" + id + "}", 1, nil} {
		if _, err := UnmarshalUUID(in); err == nil {
			t.Errorf("unmarshal %#v succeeded", in)
		}
	}
}

func TestSharedEnums(t *testing.T) {
	var d SortDirection
	if err := d.UnmarshalGQL("DESC"); err != nil || d != SortDesc {
		t.Errorf("SortDirection = %v, %v", d, err)
	}
	if err := d.UnmarshalGQL("UP"); err == nil {
		t.Error("invalid SortDirection accepted")
	}
	if got := marshaled(SortAsc); got != `"ASC"` {
		t.Errorf("marshal = %s", got)
	}
	var f ExportFormat
	if err := f.UnmarshalGQL("XLSX"); err != nil || f != FormatXLSX {
		t.Errorf("ExportFormat = %v, %v", f, err)
	}
	if err := f.UnmarshalGQL("PDF"); err == nil {
		t.Error("invalid ExportFormat accepted")
	}

	type status string
	e := NewEnum[status]("Status", "on_hold", "ON_HOLD")
	var b bytes.Buffer
	e.Marshal(&b, "on_hold")
	e.Marshal(&b, "unknown")
	if b.String() != `"ON_HOLD"null` {
		t.Errorf("enum marshal = %s", b.String())
	}
	if v, err := e.Unmarshal("ON_HOLD"); err != nil || v != "on_hold" {
		t.Errorf("enum unmarshal = %v, %v", v, err)
	}
	if _, err := e.Unmarshal("on_hold"); err == nil {
		t.Error("enum accepted a Go value instead of a GraphQL name")
	}
}
