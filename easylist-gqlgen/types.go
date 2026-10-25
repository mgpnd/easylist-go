package easylistgql

import (
	"fmt"
	"io"
	"strconv"
)

// SortDirection is the shared GraphQL enum SortDirection { ASC DESC }.
type SortDirection string

const (
	SortAsc  SortDirection = "ASC"
	SortDesc SortDirection = "DESC"
)

// MarshalGQL implements graphql.Marshaler.
func (d SortDirection) MarshalGQL(w io.Writer) { io.WriteString(w, strconv.Quote(string(d))) }

// UnmarshalGQL implements graphql.Unmarshaler.
func (d *SortDirection) UnmarshalGQL(v any) error {
	s, _ := v.(string)
	if s != string(SortAsc) && s != string(SortDesc) {
		return fmt.Errorf("%v is not a valid SortDirection", v)
	}
	*d = SortDirection(s)
	return nil
}

// ExportFormat is the shared GraphQL enum ExportFormat { CSV XLSX }.
type ExportFormat string

const (
	FormatCSV  ExportFormat = "CSV"
	FormatXLSX ExportFormat = "XLSX"
)

// MarshalGQL implements graphql.Marshaler.
func (f ExportFormat) MarshalGQL(w io.Writer) { io.WriteString(w, strconv.Quote(string(f))) }

// UnmarshalGQL implements graphql.Unmarshaler.
func (f *ExportFormat) UnmarshalGQL(v any) error {
	s, _ := v.(string)
	if s != string(FormatCSV) && s != string(FormatXLSX) {
		return fmt.Errorf("%v is not a valid ExportFormat", v)
	}
	*f = ExportFormat(s)
	return nil
}

// ExportJob is the shared GraphQL type ExportJob { id: ID! }.
type ExportJob struct {
	ID string
}

// Enum maps the Go values of a generated enum type to GraphQL enum value
// names. Generated code uses it for MarshalGQL and UnmarshalGQL.
type Enum[T ~string] struct {
	name     string
	toName   map[T]string
	fromName map[string]T
}

// NewEnum builds an enum from alternating Go values and GraphQL names.
func NewEnum[T ~string](typeName string, valueNamePairs ...string) *Enum[T] {
	e := &Enum[T]{name: typeName, toName: map[T]string{}, fromName: map[string]T{}}
	for i := 0; i+1 < len(valueNamePairs); i += 2 {
		v, n := T(valueNamePairs[i]), valueNamePairs[i+1]
		e.toName[v] = n
		e.fromName[n] = v
	}
	return e
}

// Marshal writes the GraphQL name of v. easylist rejects undeclared enum
// values before they reach a row, so an unknown value is written as null
// only when generated types are filled by hand.
func (e *Enum[T]) Marshal(w io.Writer, v T) {
	n, ok := e.toName[v]
	if !ok {
		io.WriteString(w, "null")
		return
	}
	io.WriteString(w, strconv.Quote(n))
}

// Unmarshal parses a GraphQL enum value name.
func (e *Enum[T]) Unmarshal(v any) (T, error) {
	s, _ := v.(string)
	if out, ok := e.fromName[s]; ok {
		return out, nil
	}
	return "", fmt.Errorf("%v is not a valid %s", v, e.name)
}
