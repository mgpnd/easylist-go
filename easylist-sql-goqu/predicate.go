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

	// A NULL comparison is valid for every type, nullable or not, and binds no
	// argument. The parser only puts NULL after = and !=, but the AST allows more.
	if value.Kind() == filter.LiteralNull {
		switch operator {
		case filter.OpEq:
			return goqu.L("? IS NULL", column), nil
		case filter.OpNeq:
			return goqu.L("? IS NOT NULL", column), nil
		}
		return unsupported()
	}

	if value.Kind() == filter.LiteralExpr {
		v, ok := p.Value(node)
		if !ok {
			// The planner evaluates every expression in the plan's filter.
			return nil, fmt.Errorf("expression %s for field %q has no evaluated value at position %d", value.Text(), name, value.Pos())
		}
		return exprPredicate(field, column, operator, v, value.Pos())
	}

	if field.Array {
		if operator != filter.OpContains && operator != filter.OpExcludes {
			return unsupported()
		}
		return arrayPredicate(field, column, operator, value)
	}

	if operator == filter.OpContains || operator == filter.OpExcludes {
		if field.Type != easylist.Text {
			return unsupported()
		}
		arg, err := predicateArgument(field, value.Kind(), value.Text(), value.Pos())
		if err != nil {
			return nil, err
		}
		pattern := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(arg.(string)) + "%"
		if operator == filter.OpExcludes {
			// NOT ILIKE is NULL for a NULL field; COALESCE makes such rows match
			// while emitting the author SQL only once.
			return goqu.L(`COALESCE(? NOT ILIKE ? ESCAPE E'\\', TRUE)`, column, pattern), nil
		}
		return goqu.L(`? ILIKE ? ESCAPE E'\\'`, column, pattern), nil
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
	return comparison(column, operator, arg), nil
}

// comparison compiles one of = != > >= < <= against a bound argument.
func comparison(column exp.LiteralExpression, operator filter.PredicateOp, arg any) exp.Expression {
	switch operator {
	case filter.OpEq:
		// An explicit comparison keeps Boolean values in the prepared argument
		// list; native goqu Boolean equality emits IS TRUE/FALSE.
		return goqu.L("? = ?", column, arg)
	case filter.OpNeq:
		// != matches exactly the rows = does not, NULL included. goqu's Neq
		// emits != and, for Booleans, IS NOT TRUE/FALSE, which drops the bound
		// argument; an explicit IS DISTINCT FROM keeps every value bound.
		return goqu.L("? IS DISTINCT FROM ?", column, arg)
	case filter.OpGt:
		return goqu.L("? > ?", column, arg)
	case filter.OpGte:
		return goqu.L("? >= ?", column, arg)
	case filter.OpLt:
		return goqu.L("? < ?", column, arg)
	}
	return goqu.L("? <= ?", column, arg)
}

// exprPredicate compiles a comparison with the planner's evaluated result of
// an expression literal at pos. It never evaluates the expression. The
// planner checks the result kind against the field; the checks here only
// guard against a plan that breaks that contract.
func exprPredicate(field easylist.PlanField, column exp.LiteralExpression, operator filter.PredicateOp, v easylist.Value, pos int) (exp.Expression, error) {
	if field.Array {
		return nil, fmt.Errorf("array field %q does not accept an expression value at position %d", field.Name, pos)
	}
	switch operator {
	case filter.OpEq, filter.OpNeq, filter.OpGt, filter.OpGte, filter.OpLt, filter.OpLte:
	default:
		return nil, fmt.Errorf("operator %q does not accept an expression value at position %d", operatorText[operator], pos)
	}
	mismatch := func() (exp.Expression, error) {
		return nil, fmt.Errorf("cannot compare %s field %q with a %s value at position %d", field.Type, field.Name, v.Kind, pos)
	}
	switch v.Kind {
	case easylist.ValueNumber:
		switch field.Type {
		case easylist.Integer, easylist.Float, easylist.Numeric:
			// The exact decimal text converts like a number literal, so an
			// integer field still rejects 2.5.
			arg, err := predicateArgument(field, filter.LiteralNumber, v.Number, pos)
			if err != nil {
				return nil, err
			}
			return comparison(column, operator, arg), nil
		}
	case easylist.ValueDate:
		switch field.Type {
		case easylist.Date:
			// Bound like a YYYY-MM-DD literal: the calendar date at UTC midnight.
			y, m, d := v.Time.Date()
			return comparison(column, operator, time.Date(y, m, d, 0, 0, 0, 0, time.UTC)), nil
		case easylist.Timestamp:
			return wholeDay(column, operator, v.Time), nil
		}
	case easylist.ValueTimestamp:
		if field.Type == easylist.Timestamp {
			return comparison(column, operator, v.Time.Truncate(time.Microsecond)), nil
		}
	}
	return mismatch()
}

// wholeDay compares a timestamp with the whole day that starts at start, in
// start's zone. The next day starts at E = start + 1 calendar day, which is
// 23 or 25 hours later across a DST change. > D matches values >= E, >= D
// values >= start, < D values < start, <= D values < E, = D values in
// [start, E), and != D every other row, NULL included.
func wholeDay(column exp.LiteralExpression, operator filter.PredicateOp, start time.Time) exp.Expression {
	s := start.Truncate(time.Microsecond)
	e := start.AddDate(0, 0, 1).Truncate(time.Microsecond)
	switch operator {
	case filter.OpGt:
		return goqu.L("? >= ?", column, e)
	case filter.OpGte:
		return goqu.L("? >= ?", column, s)
	case filter.OpLt:
		return goqu.L("? < ?", column, s)
	case filter.OpLte:
		return goqu.L("? < ?", column, e)
	case filter.OpEq:
		return goqu.L("(? >= ? AND ? < ?)", column, s, column, e)
	}
	// The range test is NULL for a NULL field; COALESCE makes such rows
	// match, as EXCLUDES does.
	return goqu.L("COALESCE(NOT (? >= ? AND ? < ?), TRUE)", column, s, column, e)
}

// arrayPredicate compiles CONTAINS or EXCLUDES on an array field. Each value
// is bound once and compared with "= ANY", so PostgreSQL infers its type from
// the column's element type, as it does for scalar comparisons. CONTAINS
// requires every value; EXCLUDES rejects any, and COALESCE makes NULL arrays
// and NULL comparison results match.
func arrayPredicate(field easylist.PlanField, column exp.LiteralExpression, operator filter.PredicateOp, value filter.Literal) (exp.Expression, error) {
	values, err := arrayValues(field, value)
	if err != nil {
		return nil, err
	}
	tests := make([]string, len(values))
	args := make([]any, 0, 2*len(values))
	for i, v := range values {
		tests[i] = "? = ANY(?)"
		args = append(args, v, column)
	}
	if operator == filter.OpExcludes {
		return goqu.L("COALESCE(NOT ("+strings.Join(tests, " OR ")+"), TRUE)", args...), nil
	}
	return goqu.L("("+strings.Join(tests, " AND ")+")", args...), nil
}

// arrayValues converts an array predicate's literal into its list of element
// values. A string literal is always a comma-separated list whose trimmed
// items take the literal kind of the element type; an unquoted number or
// boolean is one value.
func arrayValues(field easylist.PlanField, value filter.Literal) ([]any, error) {
	if value.Kind() != filter.LiteralString {
		arg, err := predicateArgument(field, value.Kind(), value.Text(), value.Pos())
		if err != nil {
			return nil, err
		}
		return []any{arg}, nil
	}
	itemKind := filter.LiteralString
	switch field.Type {
	case easylist.Integer, easylist.Float, easylist.Numeric:
		itemKind = filter.LiteralNumber
	case easylist.Boolean:
		itemKind = filter.LiteralBool
	}
	items := strings.Split(value.Text(), ",")
	values := make([]any, len(items))
	for i, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			return nil, fmt.Errorf("empty item in the value list for field %q at position %d", field.Name, value.Pos())
		}
		arg, err := predicateArgument(field, itemKind, item, value.Pos())
		if err != nil {
			return nil, err
		}
		values[i] = arg
	}
	return values, nil
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
