// Package easylist declares lists as Go values, plans requests against them,
// and runs the planned queries through an application-supplied database.
// SQL dialects live in easylist-sql-* driver modules.
package easylist

import (
	"context"
	"reflect"
	"time"
)

// Type is a field's value type.
type Type uint8

const (
	Text      Type = iota + 1 // Go string.
	Integer                   // Go int64.
	Float                     // Go float64.
	Numeric                   // Go string holding exact decimal text.
	Boolean                   // Go bool.
	Date                      // Go time.Time at midnight UTC.
	Timestamp                 // Go time.Time.
	UUID                      // Go string in canonical hyphenated form.
	Enum                      // Go string holding one of the field's Values.
)

var typeNames = [...]string{
	Text: "text", Integer: "integer", Float: "float", Numeric: "numeric", Boolean: "boolean",
	Date: "date", Timestamp: "timestamp", UUID: "uuid", Enum: "enum",
}

// String returns the type's lower-case name.
func (t Type) String() string {
	if t.valid() {
		return typeNames[t]
	}
	return "invalid"
}

func (t Type) valid() bool { return t >= Text && t <= Enum }

// goType is the Go representation of a scalar value of type t.
func (t Type) goType() reflect.Type {
	switch t {
	case Integer:
		return reflect.TypeFor[int64]()
	case Float:
		return reflect.TypeFor[float64]()
	case Boolean:
		return reflect.TypeFor[bool]()
	case Date, Timestamp:
		return reflect.TypeFor[time.Time]()
	}
	return reflect.TypeFor[string]()
}

// List declares one list. Table, Alias, field SQL, join SQL, and Scope are
// trusted author SQL emitted verbatim; never build them from user input.
type List struct {
	Name       string // Matches ^[a-z][a-z0-9_]*$.
	Table      string // Base table, for example "users" or "app.users".
	Alias      string // Base table alias, for example "u".
	PrimaryKey string // Name of a non-nullable SQL field.
	// Scope is an optional static SQL condition applied to every query, ANDed
	// with the request filter.
	Scope  string
	Fields []Field
	// Joins are optional clauses added only when a request needs them. Each
	// join must yield at most one row per base row; this is not enforced.
	Joins []Join
	// DefaultPageSize and MaxPageSize default to 100 and 10000 when zero.
	DefaultPageSize int
	MaxPageSize     int
}

// Field is an SQL field, or a calculated field built with Calc or CalcNullable.
type Field struct {
	Name     string // Matches ^[a-z_][a-z0-9_]*$.
	Type     Type
	Nullable bool
	// Array makes the field an array of Type, read as a Go slice of Type's
	// representation. Nullable applies to the array; elements are never NULL.
	Array  bool
	Values []string // Declared values of an Enum field, or of its elements.
	// SQL is the field's expression. When empty, the field reads the column
	// named Name from the base table alias.
	SQL   string
	Joins []string // Keys of the joins SQL needs.
	Title string   // Export header; defaults to Name.
	calc  *calc
}

// Join is an optional join clause, such as a LEFT JOIN or LEFT JOIN LATERAL.
type Join struct {
	Key       string // Matches ^[a-z_][a-z0-9_]*$.
	SQL       string
	DependsOn []string // Keys of joins that must precede this one.
}

type calc struct {
	fn       func(context.Context, Row) (any, error)
	requires []string
	goType   reflect.Type
}

// Calc declares a non-nullable calculated field evaluated once per row from
// the SQL fields it requires. T must be the Go representation of typ, or a
// slice of it, which declares an array field.
func Calc[T any](name string, typ Type, fn func(ctx context.Context, r Row) (T, error), requires ...string) Field {
	return Field{Name: name, Type: typ, Array: isSlice[T](), calc: &calc{
		fn:       func(ctx context.Context, r Row) (any, error) { return fn(ctx, r) },
		requires: requires,
		goType:   reflect.TypeFor[T](),
	}}
}

// CalcNullable declares a nullable calculated field. A nil result is NULL.
func CalcNullable[T any](name string, typ Type, fn func(ctx context.Context, r Row) (*T, error), requires ...string) Field {
	return Field{Name: name, Type: typ, Nullable: true, Array: isSlice[T](), calc: &calc{
		fn: func(ctx context.Context, r Row) (any, error) {
			v, err := fn(ctx, r)
			if err != nil || v == nil {
				return nil, err
			}
			return *v, nil
		},
		requires: requires,
		goType:   reflect.TypeFor[T](),
	}}
}

func isSlice[T any]() bool { return reflect.TypeFor[T]().Kind() == reflect.Slice }

// goType is the Go representation of the field's values.
func (f Field) goType() reflect.Type {
	if f.Array {
		return reflect.SliceOf(f.Type.goType())
	}
	return f.Type.goType()
}

// WithTitle returns f with its export title set.
func (f Field) WithTitle(title string) Field {
	f.Title = title
	return f
}

// WithValues returns f with its enum values set.
func (f Field) WithValues(values ...string) Field {
	f.Values = values
	return f
}

// Calculated reports whether f is a calculated field.
func (f Field) Calculated() bool { return f.calc != nil }

// Requires returns the SQL fields a calculated field reads.
func (f Field) Requires() []string {
	if f.calc == nil {
		return nil
	}
	return append([]string(nil), f.calc.requires...)
}
