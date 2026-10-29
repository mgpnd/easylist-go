package sqlgoqu

import (
	"fmt"

	"github.com/mgpnd/easylist-go/easylist-sqlpg"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
)

// fragment renders a sqlpg fragment as one goqu literal. An Expr slot becomes
// a nested literal without arguments, which goqu writes verbatim without
// scanning it for ?. An Arg slot passes its value, which the prepared dataset
// binds as the next $n; a numeric cast already follows the slot in the
// template.
func fragment(f sqlpg.Fragment) (exp.LiteralExpression, error) {
	args := make([]any, len(f.Slots))
	for i, slot := range f.Slots {
		switch s := slot.(type) {
		case sqlpg.Expr:
			args[i] = goqu.L(s.SQL)
		case sqlpg.Arg:
			args[i] = s.Value
		default:
			return nil, fmt.Errorf("unsupported condition slot of type %T", slot)
		}
	}
	return goqu.L(f.SQL, args...), nil
}
