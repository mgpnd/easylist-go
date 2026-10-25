package easylistgql

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql"
)

// The scalar marshalers below follow gqlgen's function convention: a model
// binding to easylistgql.<Name> uses Marshal<Name> and Unmarshal<Name>. The
// plugin binds them whatever GraphQL name the scalar is given.

var (
	decimalPattern = regexp.MustCompile(`^[+-]?[0-9]+(?:\.[0-9]+)?$`)
	uuidPattern    = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

// MarshalInt64 writes a 64-bit integer as a JSON number.
func MarshalInt64(v int64) graphql.Marshaler {
	return graphql.WriterFunc(func(w io.Writer) { io.WriteString(w, strconv.FormatInt(v, 10)) })
}

// UnmarshalInt64 accepts a JSON or GraphQL integer, or a string holding one.
func UnmarshalInt64(v any) (int64, error) {
	switch v := v.(type) {
	case int:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int64:
		return v, nil
	case json.Number:
		return parseInt64(v.String())
	case string:
		return parseInt64(v)
	case float64:
		if v == math.Trunc(v) && v >= -(1<<53) && v <= 1<<53 {
			return int64(v), nil
		}
	}
	return 0, fmt.Errorf("%v is not a 64-bit integer", v)
}

func parseInt64(s string) (int64, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not a 64-bit integer", s)
	}
	return n, nil
}

// MarshalDecimal writes exact decimal text as a JSON string.
func MarshalDecimal(v string) graphql.Marshaler {
	return graphql.WriterFunc(func(w io.Writer) { io.WriteString(w, strconv.Quote(v)) })
}

// UnmarshalDecimal accepts decimal text such as "-12.50" as a string, so no
// digits are lost to floating point.
func UnmarshalDecimal(v any) (string, error) {
	var s string
	switch v := v.(type) {
	case string:
		s = v
	case json.Number:
		s = v.String()
	default:
		return "", fmt.Errorf("decimal must be a string, got %T", v)
	}
	if !decimalPattern.MatchString(s) {
		return "", fmt.Errorf("%q is not a decimal number", s)
	}
	return s, nil
}

// MarshalDate writes a date as "YYYY-MM-DD".
func MarshalDate(v time.Time) graphql.Marshaler {
	return graphql.WriterFunc(func(w io.Writer) { io.WriteString(w, strconv.Quote(v.Format(time.DateOnly))) })
}

// UnmarshalDate accepts "YYYY-MM-DD" and returns midnight UTC.
func UnmarshalDate(v any) (time.Time, error) {
	s, ok := v.(string)
	if !ok {
		return time.Time{}, fmt.Errorf("date must be a string, got %T", v)
	}
	t, err := time.Parse(time.DateOnly, s)
	if err != nil || t.Format(time.DateOnly) != s {
		return time.Time{}, fmt.Errorf("%q is not a YYYY-MM-DD date", s)
	}
	return t, nil
}

// MarshalUUID writes a UUID in lower-case canonical form.
func MarshalUUID(v string) graphql.Marshaler {
	return graphql.WriterFunc(func(w io.Writer) { io.WriteString(w, strconv.Quote(strings.ToLower(v))) })
}

// UnmarshalUUID accepts canonical hyphenated hex in either case.
func UnmarshalUUID(v any) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("UUID must be a string, got %T", v)
	}
	if !uuidPattern.MatchString(s) {
		return "", fmt.Errorf("%q is not a canonical UUID", s)
	}
	return strings.ToLower(s), nil
}
