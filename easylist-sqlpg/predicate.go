package sqlpg

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
)

var (
	predicateNumber = regexp.MustCompile(`^[+-]?[0-9]+(?:\.[0-9]+)?$`)
	canonicalUUID   = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

var operatorText = map[filter.PredicateOp]string{
	filter.OpEq: "=", filter.OpNeq: "!=", filter.OpGt: ">", filter.OpGte: ">=",
	filter.OpLt: "<", filter.OpLte: "<=", filter.OpContains: "CONTAINS",
	filter.OpExcludes: "EXCLUDES", filter.OpBetween: "BETWEEN",
	filter.OpIn: "IN", filter.OpNotIn: "NOT IN",
}

// buildPredicate validates one predicate against the plan's fields before
// producing its condition.
func buildPredicate(p *easylist.Plan, node filter.Predicate) (Cond, error) {
	name, operator, value := node.Field(), node.Operator(), node.Value()
	field, ok := p.Lookup(name)
	if !ok {
		return nil, fmt.Errorf("unknown field %q at position %d", name, node.FieldPos())
	}
	column := fieldExpression(p, field, true)
	unsupported := func() (Cond, error) {
		return nil, fmt.Errorf("unsupported operator %q for field %q at position %d", operatorText[operator], name, node.OperatorPos())
	}

	// A NULL comparison is valid for every type, nullable or not, and binds no
	// argument. The parser only puts NULL after = and !=, but the AST allows more.
	if value.Kind() == filter.LiteralNull {
		switch operator {
		case filter.OpEq:
			return IsNull{Col: column}, nil
		case filter.OpNeq:
			return IsNull{Col: column, Not: true}, nil
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
		values, err := arrayValues(field, value)
		if err != nil {
			return nil, err
		}
		return ArrayHas{Col: column, Values: values, None: operator == filter.OpExcludes}, nil
	}

	if operator == filter.OpIn || operator == filter.OpNotIn {
		return inPredicate(field, column, operator, value)
	}

	if operator == filter.OpContains || operator == filter.OpExcludes {
		if field.Type != easylist.Text {
			return unsupported()
		}
		if value.Kind() == filter.LiteralList {
			// Reserved so that a later change can give text lists a meaning.
			return nil, fmt.Errorf("operator %q on text field %q does not accept a value list at position %d", operatorText[operator], name, value.Pos())
		}
		arg, err := predicateArgument(field, value.Kind(), value.Text(), value.Pos())
		if err != nil {
			return nil, err
		}
		pattern := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(arg.Value.(string)) + "%"
		return ILike{Col: column, Pattern: Arg{Value: pattern}, Not: operator == filter.OpExcludes}, nil
	}

	ordered := slices.Contains([]easylist.Type{easylist.Integer, easylist.Float, easylist.Numeric, easylist.Date, easylist.Timestamp}, field.Type)
	if operator == filter.OpBetween {
		if !ordered {
			return unsupported()
		}
		if value.Kind() == filter.LiteralList {
			// Each bound keeps the kind it was written as and is not trimmed.
			if len(value.Items()) != 2 {
				return nil, fmt.Errorf("BETWEEN requires a list of two bounds at position %d", value.Pos())
			}
			bounds, _, err := listArguments(field, value, false)
			if err != nil {
				return nil, err
			}
			return Between{Col: column, Low: bounds[0], High: bounds[1]}, nil
		}
		if value.Kind() != filter.LiteralString || strings.Count(value.Text(), ",") != 1 {
			return nil, fmt.Errorf("BETWEEN requires two quoted comma-separated bounds at position %d", value.Pos())
		}
		bounds := strings.SplitN(value.Text(), ",", 2)
		args := make([]Arg, 2)
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
		return Between{Col: column, Low: args[0], High: args[1]}, nil
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

// comparison compiles one of = != > >= < <= against a bound argument. != is
// IS DISTINCT FROM, so it matches exactly the rows = does not, NULL included,
// and Booleans keep their bound argument instead of becoming IS NOT TRUE.
func comparison(column Expr, operator filter.PredicateOp, arg Arg) Cond {
	return Compare{Col: column, Op: comparisonOp[operator], Arg: arg}
}

var comparisonOp = map[filter.PredicateOp]Op{
	filter.OpEq: OpEq, filter.OpNeq: OpDistinct, filter.OpGt: OpGt,
	filter.OpGte: OpGte, filter.OpLt: OpLt, filter.OpLte: OpLte,
}

// exprPredicate compiles a comparison with the planner's evaluated result of
// an expression literal at pos. It never evaluates the expression. The
// planner checks the result kind against the field; the checks here only
// guard against a plan that breaks that contract.
func exprPredicate(field easylist.PlanField, column Expr, operator filter.PredicateOp, v easylist.Value, pos int) (Cond, error) {
	if field.Array {
		return nil, fmt.Errorf("array field %q does not accept an expression value at position %d", field.Name, pos)
	}
	switch operator {
	case filter.OpEq, filter.OpNeq, filter.OpGt, filter.OpGte, filter.OpLt, filter.OpLte:
	default:
		return nil, fmt.Errorf("operator %q does not accept an expression value at position %d", operatorText[operator], pos)
	}
	mismatch := func() (Cond, error) {
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
			return comparison(column, operator, Arg{Value: time.Date(y, m, d, 0, 0, 0, 0, time.UTC)}), nil
		case easylist.Timestamp:
			return wholeDay(column, operator, v.Time), nil
		}
	case easylist.ValueTimestamp:
		if field.Type == easylist.Timestamp {
			return comparison(column, operator, Arg{Value: v.Time.Truncate(time.Microsecond)}), nil
		}
	}
	return mismatch()
}

// wholeDay compares a timestamp with the whole day that starts at start, in
// start's zone. The next day starts at E = start + 1 calendar day, which is
// 23 or 25 hours later across a DST change. > D matches values >= E, >= D
// values >= start, < D values < start, <= D values < E, = D values in
// [start, E), and != D every other row, NULL included.
func wholeDay(column Expr, operator filter.PredicateOp, start time.Time) Cond {
	s := Arg{Value: start.Truncate(time.Microsecond)}
	e := Arg{Value: start.AddDate(0, 0, 1).Truncate(time.Microsecond)}
	switch operator {
	case filter.OpGt:
		return Compare{Col: column, Op: OpGte, Arg: e}
	case filter.OpGte:
		return Compare{Col: column, Op: OpGte, Arg: s}
	case filter.OpLt:
		return Compare{Col: column, Op: OpLt, Arg: s}
	case filter.OpLte:
		return Compare{Col: column, Op: OpLt, Arg: e}
	case filter.OpEq:
		return DayRange{Col: column, Start: s, End: e}
	}
	return DayRange{Col: column, Start: s, End: e, Not: true}
}

// inPredicate compiles IN or NOT IN on a scalar field. The non-NULL items are
// bound in list order inside an SQL IN list, so PostgreSQL infers each
// argument's type from the column, and NULL never enters the list. The SQL IN
// or NOT IN is then NULL only for a NULL field, and COALESCE replaces it with
// whether such rows match: IN with a NULL item and NOT IN without one match
// them. A list of only NULL items is = NULL or != NULL. The field's SQL is
// emitted once.
func inPredicate(field easylist.PlanField, column Expr, operator filter.PredicateOp, value filter.Literal) (Cond, error) {
	if value.Kind() != filter.LiteralList {
		// The parser always gives IN and NOT IN a list.
		return nil, fmt.Errorf("operator %q requires a value list for field %q at position %d", operatorText[operator], field.Name, value.Pos())
	}
	args, hasNull, err := listArguments(field, value, true)
	if err != nil {
		return nil, err
	}
	in := operator == filter.OpIn
	if len(args) == 0 {
		return IsNull{Col: column, Not: !in}, nil
	}
	cond := In{Col: column, Args: args, Not: !in}
	switch {
	case in && !hasNull:
		// NULL for a NULL field, which WHERE already treats as no match.
		cond.Null = NullKeep
	case in == hasNull:
		// IN with a NULL item, or NOT IN without one, matches a NULL field.
		cond.Null = NullMatch
	default:
		cond.Null = NullNoMatch
	}
	return cond, nil
}

// arrayValues converts an array predicate's literal into its list of element
// values. A list literal gives one value per item, never split or trimmed. A
// string literal is always a comma-separated list whose trimmed items take
// the literal kind of the element type; an unquoted number or boolean is one
// value.
func arrayValues(field easylist.PlanField, value filter.Literal) ([]Arg, error) {
	if value.Kind() == filter.LiteralList {
		values, _, err := listArguments(field, value, false)
		return values, err
	}
	if value.Kind() != filter.LiteralString {
		arg, err := predicateArgument(field, value.Kind(), value.Text(), value.Pos())
		if err != nil {
			return nil, err
		}
		return []Arg{arg}, nil
	}
	itemKind := filter.LiteralString
	switch field.Type {
	case easylist.Integer, easylist.Float, easylist.Numeric:
		itemKind = filter.LiteralNumber
	case easylist.Boolean:
		itemKind = filter.LiteralBool
	}
	items := strings.Split(value.Text(), ",")
	values := make([]Arg, len(items))
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

// listArguments converts the non-NULL items of a list literal, each under its
// own kind and position, and reports whether any item was NULL. NULL items are
// an error unless allowNull is set; the parser allows them only in IN and NOT
// IN lists.
func listArguments(field easylist.PlanField, list filter.Literal, allowNull bool) (args []Arg, hasNull bool, err error) {
	items := list.Items()
	if len(items) == 0 {
		return nil, false, fmt.Errorf("empty value list for field %q at position %d", field.Name, list.Pos())
	}
	args = make([]Arg, 0, len(items))
	for _, item := range items {
		if item.Kind() == filter.LiteralNull {
			if !allowNull {
				return nil, false, fmt.Errorf("NULL item in the value list for field %q at position %d", field.Name, item.Pos())
			}
			hasNull = true
			continue
		}
		// predicateArgument rejects items of any unexpected kind.
		arg, err := predicateArgument(field, item.Kind(), item.Text(), item.Pos())
		if err != nil {
			return nil, false, err
		}
		args = append(args, arg)
	}
	return args, hasNull, nil
}

// predicateArgument converts a single scalar value. Numeric values keep
// arbitrary precision decimal strings intact and carry PostgreSQL's cast.
func predicateArgument(field easylist.PlanField, valueKind filter.LiteralKind, text string, pos int) (Arg, error) {
	invalid := func(reason string) (Arg, error) {
		return Arg{}, fmt.Errorf("invalid %s value %q for field %q at position %d%s", field.Type, text, field.Name, pos, reason)
	}
	switch field.Type {
	case easylist.Text:
		if valueKind == filter.LiteralString {
			return Arg{Value: text}, nil
		}
	case easylist.Integer:
		if valueKind == filter.LiteralNumber {
			parsed, err := strconv.ParseInt(text, 10, 64)
			if err == nil {
				return Arg{Value: parsed}, nil
			}
		}
	case easylist.Float:
		if valueKind == filter.LiteralNumber && predicateNumber.MatchString(text) {
			parsed, err := strconv.ParseFloat(text, 64)
			if err == nil && !math.IsInf(parsed, 0) && !math.IsNaN(parsed) {
				return Arg{Value: parsed}, nil
			}
		}
	case easylist.Numeric:
		if valueKind == filter.LiteralNumber && predicateNumber.MatchString(text) {
			return Arg{Value: text, Cast: CastNumeric}, nil
		}
	case easylist.Date:
		if valueKind == filter.LiteralString {
			parsed, err := time.Parse(time.DateOnly, text)
			if err == nil && parsed.Format(time.DateOnly) == text {
				return Arg{Value: parsed.UTC()}, nil
			}
		}
	case easylist.Timestamp:
		if valueKind == filter.LiteralString {
			// time.RFC3339 requires an offset (Z or ±hh:mm) and accepts fractional seconds.
			parsed, err := time.Parse(time.RFC3339Nano, text)
			if err == nil {
				return Arg{Value: parsed}, nil
			}
			if len(text) >= 19 && text[10] == 'T' {
				return invalid(": timestamps need an offset such as Z or +02:00")
			}
		}
	case easylist.UUID:
		if valueKind == filter.LiteralString && canonicalUUID.MatchString(text) {
			return Arg{Value: strings.ToLower(text)}, nil
		}
	case easylist.Enum:
		if valueKind == filter.LiteralString {
			if slices.Contains(field.Values, text) {
				return Arg{Value: text}, nil
			}
			return invalid(": not a declared value")
		}
	case easylist.Boolean:
		if valueKind == filter.LiteralBool {
			if strings.EqualFold(text, "true") {
				return Arg{Value: true}, nil
			}
			if strings.EqualFold(text, "false") {
				return Arg{Value: false}, nil
			}
		}
	}
	return invalid("")
}
