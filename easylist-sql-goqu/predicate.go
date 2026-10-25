package sqlgoqu

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	filter "github.com/mgpnd/easylist-go/easylist-filter"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
)

var (
	predicateNumber = regexp.MustCompile(`^[+-]?[0-9]+(?:\.[0-9]+)?$`)
	canonicalUUID   = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

var operatorText = map[filter.PredicateOp]string{
	filter.OpEq: "=", filter.OpNeq: "!=", filter.OpGt: ">", filter.OpGte: ">=",
	filter.OpLt: "<", filter.OpLte: "<=", filter.OpContains: "CONTAINS",
	filter.OpExcludes: "EXCLUDES", filter.OpBetween: "BETWEEN",
}

// buildPredicate validates one predicate against the plan's fields before
// producing its goqu expression.
func buildPredicate(p *easylist.Plan, node filter.Predicate) (exp.Expression, error) {
	name, operator, value := node.Field(), node.Operator(), node.Value()
	field, ok := p.Lookup(name)
	if !ok {
		return nil, fmt.Errorf("unknown field %q at position %d", name, node.FieldPos())
	}
	// Author SQL is parenthesized so operators inside it cannot bind to the comparison.
	column := fieldExpression(p, field, true)
	unsupported := func() (exp.Expression, error) {
		return nil, fmt.Errorf("unsupported operator %q for field %q at position %d", operatorText[operator], name, node.OperatorPos())
	}

	if operator == filter.OpContains {
		if field.Type != easylist.Text {
			return unsupported()
		}
		arg, err := predicateArgument(field, value.Kind(), value.Text(), value.Pos())
		if err != nil {
			return nil, err
		}
		pattern := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(arg.(string))
		return goqu.L(`? ILIKE ? ESCAPE E'\\'`, column, "%"+pattern+"%"), nil
	}

	ordered := slices.Contains([]easylist.Type{easylist.Integer, easylist.Float, easylist.Numeric, easylist.Date, easylist.Timestamp}, field.Type)
	if operator == filter.OpBetween {
		if !ordered {
			return unsupported()
		}
		if value.Kind() != filter.LiteralString || strings.Count(value.Text(), ",") != 1 {
			return nil, fmt.Errorf("BETWEEN requires two quoted comma-separated bounds at position %d", value.Pos())
		}
		bounds := strings.SplitN(value.Text(), ",", 2)
		args := make([]any, 2)
		for i, bound := range bounds {
			boundKind := filter.LiteralNumber
			if field.Type == easylist.Date || field.Type == easylist.Timestamp {
				boundKind = filter.LiteralString
			}
			arg, err := predicateArgument(field, boundKind, strings.TrimSpace(bound), value.Pos())
			if err != nil {
				return nil, err
			}
			args[i] = arg
		}
		return goqu.L("? BETWEEN ? AND ?", column, args[0], args[1]), nil
	}
	if operator != filter.OpEq && operator != filter.OpNeq && (!ordered || (operator != filter.OpGt && operator != filter.OpGte && operator != filter.OpLt && operator != filter.OpLte)) {
		return unsupported()
	}
	arg, err := predicateArgument(field, value.Kind(), value.Text(), value.Pos())
	if err != nil {
		return nil, err
	}
	switch operator {
	case filter.OpEq:
		// An explicit comparison keeps Boolean values in the prepared argument
		// list; native goqu Boolean equality emits IS TRUE/FALSE.
		return goqu.L("? = ?", column, arg), nil
	case filter.OpNeq:
		// goqu's Neq emits != and, for Booleans, IS NOT TRUE/FALSE, which also
		// matches NULL. Plain <> keeps SQL semantics: NULL rows never match.
		return goqu.L("? <> ?", column, arg), nil
	case filter.OpGt:
		return goqu.L("? > ?", column, arg), nil
	case filter.OpGte:
		return goqu.L("? >= ?", column, arg), nil
	case filter.OpLt:
		return goqu.L("? < ?", column, arg), nil
	case filter.OpLte:
		return goqu.L("? <= ?", column, arg), nil
	}
	return unsupported()
}

// predicateArgument converts a single scalar value. Numeric expressions keep
// arbitrary precision decimal strings intact while adding PostgreSQL's cast.
func predicateArgument(field easylist.PlanField, valueKind filter.LiteralKind, text string, pos int) (any, error) {
	invalid := func(reason string) (any, error) {
		return nil, fmt.Errorf("invalid %s value %q for field %q at position %d%s", field.Type, text, field.Name, pos, reason)
	}
	switch field.Type {
	case easylist.Text:
		if valueKind == filter.LiteralString {
			return text, nil
		}
	case easylist.Integer:
		if valueKind == filter.LiteralNumber {
			parsed, err := strconv.ParseInt(text, 10, 64)
			if err == nil {
				return parsed, nil
			}
		}
	case easylist.Float:
		if valueKind == filter.LiteralNumber && predicateNumber.MatchString(text) {
			parsed, err := strconv.ParseFloat(text, 64)
			if err == nil && !math.IsInf(parsed, 0) && !math.IsNaN(parsed) {
				return parsed, nil
			}
		}
	case easylist.Numeric:
		if valueKind == filter.LiteralNumber && predicateNumber.MatchString(text) {
			return goqu.L("?::numeric", text), nil
		}
	case easylist.Date:
		if valueKind == filter.LiteralString {
			parsed, err := time.Parse(time.DateOnly, text)
			if err == nil && parsed.Format(time.DateOnly) == text {
				return parsed.UTC(), nil
			}
		}
	case easylist.Timestamp:
		if valueKind == filter.LiteralString {
			// time.RFC3339 requires an offset (Z or ±hh:mm) and accepts fractional seconds.
			parsed, err := time.Parse(time.RFC3339Nano, text)
			if err == nil {
				return parsed, nil
			}
			if len(text) >= 19 && text[10] == 'T' {
				return invalid(": timestamps need an offset such as Z or +02:00")
			}
		}
	case easylist.UUID:
		if valueKind == filter.LiteralString && canonicalUUID.MatchString(text) {
			return strings.ToLower(text), nil
		}
	case easylist.Enum:
		if valueKind == filter.LiteralString {
			if slices.Contains(field.Values, text) {
				return text, nil
			}
			return invalid(": not a declared value")
		}
	case easylist.Boolean:
		if valueKind == filter.LiteralBool {
			if strings.EqualFold(text, "true") {
				return true, nil
			}
			if strings.EqualFold(text, "false") {
				return false, nil
			}
		}
	}
	return invalid("")
}
