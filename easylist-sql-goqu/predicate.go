package querysql

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	filter "github.com/mgpnd/easylist-go/easylist-filter"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
)

var predicateNumber = regexp.MustCompile(`^[+-]?[0-9]+(?:\.[0-9]+)?$`)

// buildPredicate validates one predicate before producing its goqu expression.
// Fields arrive case-folded from filter.Parse; truncate them here the same way
// schema names are, so lookups match PostgreSQL's 63-byte identifier limit.
func buildPredicate(table tableMeta, node filter.Predicate) (exp.Expression, error) {
	field, operator, value := truncateIdentifier(node.Field()), node.Operator(), node.Value()
	meta, ok := table.columns[field]
	if !ok {
		return nil, fmt.Errorf("unknown column %q at position %d", field, node.FieldPos())
	}
	if meta.arrayDimensions > 1 {
		return nil, fmt.Errorf("unsupported array dimensions (%d) for column %q at position %d", meta.arrayDimensions, field, node.FieldPos())
	}
	if meta.kind == kindUnsupported {
		return nil, fmt.Errorf("unsupported type for column %q at position %d", field, node.FieldPos())
	}
	column := catalogColumnIdentifier(table, meta)
	unsupported := func() (exp.Expression, error) {
		operatorText := map[filter.PredicateOp]string{
			filter.OpEq: "=", filter.OpNeq: "!=", filter.OpGt: ">", filter.OpGte: ">=",
			filter.OpLt: "<", filter.OpLte: "<=", filter.OpContains: "CONTAINS",
			filter.OpExcludes: "EXCLUDES", filter.OpBetween: "BETWEEN",
		}[operator]
		return nil, fmt.Errorf("unsupported operator %q for column %q at position %d", operatorText, field, node.OperatorPos())
	}

	if meta.arrayDimensions > 0 {
		if operator != filter.OpContains && operator != filter.OpExcludes {
			return unsupported()
		}
		arg, err := predicateArgument(meta.kind, value.Kind(), value.Text(), value.Pos())
		if err != nil {
			return nil, err
		}
		if operator == filter.OpContains {
			return goqu.L("array_position(?, ?) IS NOT NULL", column, arg), nil
		}
		// PostgreSQL returns NULL for array_position on a NULL array, so its
		// absence check also needs a non-null array guard. Empty arrays qualify.
		return goqu.And(column.IsNotNull(), goqu.L("array_position(?, ?) IS NULL", column, arg)), nil
	}

	if operator == filter.OpContains {
		if meta.kind != kindText {
			return unsupported()
		}
		arg, err := predicateArgument(meta.kind, value.Kind(), value.Text(), value.Pos())
		if err != nil {
			return nil, err
		}
		pattern := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(arg.(string))
		return goqu.L(`? ILIKE ? ESCAPE E'\\'`, column, "%"+pattern+"%"), nil
	}

	ordered := meta.kind == kindInteger || meta.kind == kindFloat || meta.kind == kindNumeric || meta.kind == kindDate
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
			if meta.kind == kindDate {
				boundKind = filter.LiteralString
			}
			arg, err := predicateArgument(meta.kind, boundKind, strings.TrimSpace(bound), value.Pos())
			if err != nil {
				return nil, err
			}
			args[i] = arg
		}
		return column.Between(goqu.Range(args[0], args[1])), nil
	}
	if operator != filter.OpEq && operator != filter.OpNeq && (!ordered || (operator != filter.OpGt && operator != filter.OpGte && operator != filter.OpLt && operator != filter.OpLte)) {
		return unsupported()
	}
	arg, err := predicateArgument(meta.kind, value.Kind(), value.Text(), value.Pos())
	if err != nil {
		return nil, err
	}
	switch operator {
	case filter.OpEq:
		// Native goqu Boolean equality emits IS TRUE/FALSE; an explicit
		// comparison keeps Boolean values in the prepared argument list.
		if meta.kind == kindBoolean {
			return goqu.L("? = ?", column, arg), nil
		}
		return column.Eq(arg), nil
	case filter.OpNeq:
		// goqu's Neq emits != and, for Booleans, IS NOT TRUE/FALSE, which also
		// matches NULL. Plain <> keeps SQL semantics: NULL rows never match.
		return goqu.L("? <> ?", column, arg), nil
	case filter.OpGt:
		return column.Gt(arg), nil
	case filter.OpGte:
		return column.Gte(arg), nil
	case filter.OpLt:
		return column.Lt(arg), nil
	case filter.OpLte:
		return column.Lte(arg), nil
	}
	return unsupported()
}

// predicateArgument converts a single scalar value. Numeric expressions keep
// arbitrary precision decimal strings intact while adding PostgreSQL's cast.
func predicateArgument(kind columnKind, valueKind filter.LiteralKind, text string, pos int) (any, error) {
	invalid := func() (any, error) {
		return nil, fmt.Errorf("invalid value %q for column type at position %d", text, pos)
	}
	switch kind {
	case kindText:
		if valueKind == filter.LiteralString {
			return text, nil
		}
	case kindInteger:
		if valueKind == filter.LiteralNumber {
			parsed, err := strconv.ParseInt(text, 10, 64)
			if err == nil {
				return parsed, nil
			}
		}
	case kindFloat:
		if valueKind == filter.LiteralNumber && predicateNumber.MatchString(text) {
			parsed, err := strconv.ParseFloat(text, 64)
			if err == nil && !math.IsInf(parsed, 0) && !math.IsNaN(parsed) {
				return parsed, nil
			}
		}
	case kindNumeric:
		if valueKind == filter.LiteralNumber && predicateNumber.MatchString(text) {
			return goqu.L("?::numeric", text), nil
		}
	case kindDate:
		if valueKind == filter.LiteralString {
			parsed, err := time.Parse("2006-01-02", text)
			if err == nil && parsed.Format("2006-01-02") == text {
				return parsed.UTC(), nil
			}
		}
	case kindBoolean:
		if valueKind == filter.LiteralBool {
			if strings.EqualFold(text, "true") {
				return true, nil
			}
			if strings.EqualFold(text, "false") {
				return false, nil
			}
		}
	}
	return invalid()
}
