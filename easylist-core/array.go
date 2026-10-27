package easylist

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
)

// Timestamp layouts accepted in array JSON. A timestamp without an offset
// is read as UTC, as drivers return scalar timestamp values.
var timestampLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05.999999999Z07",
	"2006-01-02T15:04:05.999999999Z07:00:00",
	"2006-01-02T15:04:05.999999999",
}

// decodeArray decodes the JSON array text a driver selects for an array
// field into a non-nil slice of t's Go representation. A NULL element or one
// of the wrong kind fails.
func decodeArray(t Type, text string) (any, error) {
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("invalid array JSON: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("invalid array JSON: unexpected data after the array")
	}
	elems, ok := v.([]any)
	if !ok {
		return nil, errors.New("invalid array JSON: not an array")
	}
	for i, e := range elems {
		if e == nil {
			return nil, fmt.Errorf("array element %d is NULL", i+1)
		}
	}
	switch t {
	case Integer:
		return decodeElems(elems, t, func(e any) (int64, bool) {
			n, ok := e.(json.Number)
			if !ok {
				return 0, false
			}
			v, err := strconv.ParseInt(string(n), 10, 64)
			return v, err == nil
		})
	case Float:
		return decodeElems(elems, t, func(e any) (float64, bool) {
			switch e := e.(type) {
			case json.Number:
				v, err := strconv.ParseFloat(string(e), 64)
				return v, err == nil
			case string:
				switch e {
				case "NaN":
					return math.NaN(), true
				case "Infinity":
					return math.Inf(1), true
				case "-Infinity":
					return math.Inf(-1), true
				}
			}
			return 0, false
		})
	case Numeric:
		return decodeElems(elems, t, func(e any) (string, bool) {
			switch e := e.(type) {
			case json.Number:
				return string(e), true
			case string:
				return e, e == "NaN"
			}
			return "", false
		})
	case Boolean:
		return decodeElems(elems, t, func(e any) (bool, bool) {
			v, ok := e.(bool)
			return v, ok
		})
	case Date:
		return decodeElems(elems, t, func(e any) (time.Time, bool) {
			s, _ := e.(string)
			v, err := time.Parse(time.DateOnly, s)
			return v, err == nil
		})
	case Timestamp:
		return decodeElems(elems, t, func(e any) (time.Time, bool) {
			s, _ := e.(string)
			for _, layout := range timestampLayouts {
				if v, err := time.Parse(layout, s); err == nil {
					return v, true
				}
			}
			return time.Time{}, false
		})
	}
	return decodeElems(elems, t, func(e any) (string, bool) {
		v, ok := e.(string)
		return v, ok
	})
}

func decodeElems[T any](elems []any, t Type, conv func(any) (T, bool)) ([]T, error) {
	out := make([]T, 0, len(elems))
	for i, e := range elems {
		v, ok := conv(e)
		if !ok {
			return nil, fmt.Errorf("array element %d: %s is not a valid %s value", i+1, elemText(e), t)
		}
		out = append(out, v)
	}
	return out, nil
}

// elemText renders a decoded JSON element for an error message.
func elemText(e any) string {
	b, err := json.Marshal(e)
	if err != nil {
		return fmt.Sprint(e)
	}
	return string(b)
}
