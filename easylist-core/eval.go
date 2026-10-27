package easylist

import (
	"fmt"
	"math/big"
	"strings"
	"time"

	filter "github.com/mgpnd/easylist-go/easylist-filter"
)

// ValueKind identifies the type of an evaluated expression value.
type ValueKind uint8

const (
	ValueNumber    ValueKind = iota + 1 // An exact decimal number in Value.Number.
	ValueDate                           // A date, as midnight in the reference zone in Value.Time.
	ValueTimestamp                      // An instant in Value.Time.
)

var valueKindNames = [...]string{ValueNumber: "number", ValueDate: "date", ValueTimestamp: "timestamp"}

// String returns the kind's lower-case name.
func (k ValueKind) String() string {
	if k >= ValueNumber && k <= ValueTimestamp {
		return valueKindNames[k]
	}
	return "invalid"
}

// Value is the evaluated result of an expression literal, such as TODAY or
// {100 * 3}. The planner computes it; drivers only bind it.
type Value struct {
	Kind ValueKind
	// Number is exact decimal text in plain notation, with no exponent and no
	// trailing fractional zeros, such as "2.4" or "-300". Set for ValueNumber.
	Number string
	// Time is midnight at the start of the date in the zone of the request's
	// reference time for ValueDate, and the instant, in that zone, for
	// ValueTimestamp.
	Time time.Time
}

// maxDurationAmount bounds each amount of a duration, so time arithmetic
// cannot overflow.
const maxDurationAmount = 10_000_000

// exprType is the static type of an expression node.
type exprType uint8

const (
	typeNumber exprType = iota
	typeDuration
	typeDate
	typeTimestamp
)

var exprTypeNames = [...]string{typeNumber: "number", typeDuration: "duration", typeDate: "date", typeTimestamp: "timestamp"}

func (t exprType) String() string { return exprTypeNames[t] }

func (t exprType) isTime() bool { return t == typeDate || t == typeTimestamp }

// term is one unit amount of a duration.
type term struct {
	amount int64
	unit   filter.TimeUnit
}

// result is an evaluated node. Only the field matching typ is set; times are
// zero when the evaluator has no reference time.
type result struct {
	typ exprType
	num *big.Rat
	dur []term
	t   time.Time
}

// evaluator walks one expression literal. Without a reference time it still
// determines every type, evaluates numbers and durations, and records the
// first keyword, but leaves times unset.
type evaluator struct {
	now       time.Time
	hasNow    bool
	weekStart time.Weekday
	litPos    int // position of the literal, for range errors
	keyword   filter.KeywordNode
}

func (e *evaluator) eval(n filter.ExprNode) (result, error) {
	switch n := n.(type) {
	case filter.NumberNode:
		r, ok := new(big.Rat).SetString(n.Text())
		if !ok {
			return result{}, fmt.Errorf("invalid number %s at position %d", n.Text(), n.Pos())
		}
		return result{typ: typeNumber, num: r}, nil
	case filter.KeywordNode:
		return e.keywordValue(n)
	case filter.DurationNode:
		amount, ok := new(big.Int).SetString(n.Amount(), 10)
		if !ok || amount.CmpAbs(big.NewInt(maxDurationAmount)) > 0 {
			return result{}, fmt.Errorf("duration amount %s exceeds %d at position %d", n.Amount(), maxDurationAmount, n.Pos())
		}
		return result{typ: typeDuration, dur: []term{{amount.Int64(), n.Unit()}}}, nil
	case filter.NegationNode:
		v, err := e.eval(n.Operand())
		if err != nil {
			return result{}, err
		}
		switch v.typ {
		case typeNumber:
			return result{typ: typeNumber, num: new(big.Rat).Neg(v.num)}, nil
		case typeDuration:
			return result{typ: typeDuration, dur: negate(v.dur)}, nil
		}
		return result{}, fmt.Errorf("cannot negate a %s at position %d", v.typ, n.Pos())
	case filter.BinaryNode:
		l, err := e.eval(n.Left())
		if err != nil {
			return result{}, err
		}
		r, err := e.eval(n.Right())
		if err != nil {
			return result{}, err
		}
		return e.binary(n, l, r)
	}
	return result{}, fmt.Errorf("invalid expression syntax tree")
}

func (e *evaluator) binary(n filter.BinaryNode, l, r result) (result, error) {
	op, pos := n.Operator(), n.Pos()
	switch op {
	case filter.ArithAdd, filter.ArithSub:
		switch {
		case l.typ == typeNumber && r.typ == typeNumber:
			if op == filter.ArithAdd {
				return result{typ: typeNumber, num: new(big.Rat).Add(l.num, r.num)}, nil
			}
			return result{typ: typeNumber, num: new(big.Rat).Sub(l.num, r.num)}, nil
		case l.typ == typeDuration && r.typ == typeDuration:
			right := r.dur
			if op == filter.ArithSub {
				right = negate(right)
			}
			return result{typ: typeDuration, dur: append(append([]term(nil), l.dur...), right...)}, nil
		case l.typ.isTime() && r.typ == typeDuration:
			d := r.dur
			if op == filter.ArithSub {
				d = negate(d)
			}
			return e.apply(l, d)
		case op == filter.ArithAdd && l.typ == typeDuration && r.typ.isTime():
			return e.apply(r, l.dur)
		}
	case filter.ArithMul:
		switch {
		case l.typ == typeNumber && r.typ == typeNumber:
			return result{typ: typeNumber, num: new(big.Rat).Mul(l.num, r.num)}, nil
		case l.typ == typeDuration && r.typ == typeNumber:
			return scale(l.dur, r.num, pos)
		case l.typ == typeNumber && r.typ == typeDuration:
			return scale(r.dur, l.num, pos)
		}
	case filter.ArithDiv:
		switch {
		case l.typ == typeNumber && r.typ == typeNumber:
			if r.num.Sign() == 0 {
				return result{}, fmt.Errorf("division by zero at position %d", pos)
			}
			q := new(big.Rat).Quo(l.num, r.num)
			if _, ok := decimalDigits(q); !ok {
				return result{}, fmt.Errorf("quotient %s has no finite decimal form at position %d", q.RatString(), pos)
			}
			return result{typ: typeNumber, num: q}, nil
		case l.typ == typeDuration && r.typ == typeNumber:
			return divide(l.dur, r.num, pos)
		}
	}
	return result{}, fmt.Errorf("cannot apply %s to a %s and a %s at position %d", op, l.typ, r.typ, pos)
}

func negate(d []term) []term {
	out := make([]term, len(d))
	for i, t := range d {
		out[i] = term{-t.amount, t.unit}
	}
	return out
}

// integer returns k as an integer, or false when it has a fraction.
func integer(k *big.Rat) (*big.Int, bool) {
	if !k.IsInt() {
		return nil, false
	}
	return new(big.Int).Set(k.Num()), true
}

func scale(d []term, k *big.Rat, pos int) (result, error) {
	m, ok := integer(k)
	if !ok {
		return result{}, fmt.Errorf("duration multiplier %s is not an integer at position %d", decimalOrRat(k), pos)
	}
	out := make([]term, len(d))
	limit := big.NewInt(maxDurationAmount)
	for i, t := range d {
		p := new(big.Int).Mul(big.NewInt(t.amount), m)
		if p.CmpAbs(limit) > 0 {
			return result{}, fmt.Errorf("duration amount %s exceeds %d at position %d", p, maxDurationAmount, pos)
		}
		out[i] = term{p.Int64(), t.unit}
	}
	return result{typ: typeDuration, dur: out}, nil
}

func divide(d []term, k *big.Rat, pos int) (result, error) {
	m, ok := integer(k)
	if !ok {
		return result{}, fmt.Errorf("duration divisor %s is not an integer at position %d", decimalOrRat(k), pos)
	}
	if m.Sign() == 0 {
		return result{}, fmt.Errorf("division by zero at position %d", pos)
	}
	out := make([]term, len(d))
	for i, t := range d {
		q, rem := new(big.Int).QuoRem(big.NewInt(t.amount), m, new(big.Int))
		if rem.Sign() != 0 {
			return result{}, fmt.Errorf("%d::%s is not divisible by %s at position %d", t.amount, t.unit, m, pos)
		}
		out[i] = term{q.Int64(), t.unit}
	}
	return result{typ: typeDuration, dur: out}, nil
}

func decimalOrRat(r *big.Rat) string {
	if s, ok := decimalText(r); ok {
		return s
	}
	return r.RatString()
}

// decimalDigits returns the number of fractional digits of r's exact decimal
// form, or false when r has none.
func decimalDigits(r *big.Rat) (int, bool) {
	d := new(big.Int).Set(r.Denom())
	twos := d.TrailingZeroBits()
	d.Rsh(d, twos)
	fives := uint(0)
	five, rem := big.NewInt(5), new(big.Int)
	for d.Cmp(big.NewInt(1)) != 0 {
		q, m := new(big.Int).QuoRem(d, five, rem)
		if m.Sign() != 0 {
			return 0, false
		}
		d = q
		fives++
	}
	return int(max(twos, fives)), true
}

// decimalText renders r in plain decimal notation with no trailing
// fractional zeros, or reports false when r has no finite decimal form.
func decimalText(r *big.Rat) (string, bool) {
	digits, ok := decimalDigits(r)
	if !ok {
		return "", false
	}
	s := r.FloatString(digits)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return s, true
}

func (e *evaluator) keywordValue(n filter.KeywordNode) (result, error) {
	if e.keyword == nil {
		e.keyword = n
	}
	typ := typeDate
	if n.Keyword() == filter.TimeNow {
		typ = typeTimestamp
	}
	if !e.hasNow {
		return result{typ: typ}, nil
	}
	now := e.now
	y, m, d := now.Date()
	loc := now.Location()
	date := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, loc) }
	var t time.Time
	switch n.Keyword() {
	case filter.TimeNow:
		t = now
	case filter.TimeToday:
		t = date(y, m, d)
	case filter.TimeStartOfWeek, filter.TimeEndOfWeek:
		back := (int(now.Weekday()) - int(e.weekStart) + 7) % 7
		if n.Keyword() == filter.TimeEndOfWeek {
			back -= 6
		}
		t = date(y, m, d-back)
	case filter.TimeStartOfMonth:
		t = date(y, m, 1)
	case filter.TimeEndOfMonth:
		t = date(y, m+1, 0)
	case filter.TimeStartOfYear:
		t = date(y, time.January, 1)
	case filter.TimeEndOfYear:
		t = date(y, time.December, 31)
	default:
		return result{}, fmt.Errorf("unknown time keyword %s at position %d", n.Keyword(), n.Pos())
	}
	if err := e.checkYear(t); err != nil {
		return result{}, err
	}
	return result{typ: typ, t: t}, nil
}

// apply moves a time by each amount of d in order. The result stays a date
// only when every amount is in days or business days.
func (e *evaluator) apply(v result, d []term) (result, error) {
	typ := v.typ
	for _, step := range d {
		if step.unit == filter.UnitHours || step.unit == filter.UnitMinutes {
			typ = typeTimestamp
		}
	}
	if !e.hasNow {
		return result{typ: typ}, nil
	}
	t := v.t
	for _, step := range d {
		switch step.unit {
		case filter.UnitDays:
			t = t.AddDate(0, 0, int(step.amount))
		case filter.UnitBusinessDays:
			t = t.AddDate(0, 0, int(businessDays(t.Weekday(), step.amount)))
		case filter.UnitHours:
			t = t.Add(time.Duration(step.amount) * time.Hour)
		case filter.UnitMinutes:
			t = t.Add(time.Duration(step.amount) * time.Minute)
		}
		if err := e.checkYear(t); err != nil {
			return result{}, err
		}
	}
	return result{typ: typ, t: t}, nil
}

func (e *evaluator) checkYear(t time.Time) error {
	if y := t.Year(); y < 1 || y > 9999 {
		return fmt.Errorf("relative time is outside the years 1 to 9999 at position %d", e.litPos)
	}
	return nil
}

// businessDays returns the number of calendar days from a date on weekday wd
// to the nth Monday-to-Friday date after it, or before it when n is negative.
// Zero moves a Saturday or Sunday to the next Monday.
func businessDays(wd time.Weekday, n int64) int64 {
	weekend := func(w time.Weekday) bool { return w == time.Saturday || w == time.Sunday }
	var delta int64
	switch {
	case n == 0:
		switch wd {
		case time.Saturday:
			return 2
		case time.Sunday:
			return 1
		}
		return 0
	case n > 0:
		// The weekdays after a weekend are those after the Friday before it.
		for weekend(wd) {
			wd, delta = (wd+6)%7, delta-1
		}
		weeks := (n - 1) / 5
		delta += 7 * weeks
		for left := n - 5*weeks; left > 0; {
			wd, delta = (wd+1)%7, delta+1
			if !weekend(wd) {
				left--
			}
		}
	default:
		// The weekdays before a weekend are those before the Monday after it.
		for weekend(wd) {
			wd, delta = (wd+1)%7, delta+1
		}
		weeks := (-n - 1) / 5
		delta -= 7 * weeks
		for left := -n - 5*weeks; left > 0; {
			wd, delta = (wd+6)%7, delta-1
			if !weekend(wd) {
				left--
			}
		}
	}
	return delta
}

// exprEnv holds what the planner evaluates expression literals with.
type exprEnv struct {
	now       time.Time
	weekStart time.Weekday
	// typeOnly checks types and numbers but needs no reference time and
	// evaluates no times.
	typeOnly bool
}

// exprValue type-checks the expression literal of p against field f and
// evaluates it. Types and field compatibility are checked before the
// reference time is needed, so they never depend on it.
func exprValue(p filter.Predicate, f *Field, env exprEnv) (Value, bool, error) {
	lit := p.Value()
	if f.Array {
		return Value{}, false, fmt.Errorf("array field %q does not accept an expression value at position %d", f.Name, lit.Pos())
	}
	root := lit.Expr()
	if root == nil {
		return Value{}, false, fmt.Errorf("invalid expression literal at position %d", lit.Pos())
	}
	check := &evaluator{litPos: lit.Pos()}
	res, err := check.eval(root)
	if err != nil {
		return Value{}, false, err
	}
	var kind ValueKind
	switch res.typ {
	case typeDuration:
		return Value{}, false, fmt.Errorf("expression is a duration, expected a number or a time at position %d", lit.Pos())
	case typeNumber:
		kind = ValueNumber
		if f.Type != Integer && f.Type != Float && f.Type != Numeric {
			return Value{}, false, fmt.Errorf("cannot compare %s field %q with a number at position %d", f.Type, f.Name, lit.Pos())
		}
	case typeDate:
		kind = ValueDate
		if f.Type != Date && f.Type != Timestamp {
			return Value{}, false, fmt.Errorf("cannot compare %s field %q with a date at position %d", f.Type, f.Name, lit.Pos())
		}
	case typeTimestamp:
		kind = ValueTimestamp
		if f.Type != Timestamp {
			return Value{}, false, fmt.Errorf("cannot compare %s field %q with a timestamp at position %d", f.Type, f.Name, lit.Pos())
		}
	}
	if kind == ValueNumber {
		text, _ := decimalText(res.num) // every quotient was checked at its '/'
		return Value{Kind: kind, Number: text}, true, nil
	}
	if env.typeOnly {
		return Value{}, false, nil
	}
	if env.now.IsZero() {
		k := check.keyword
		return Value{}, false, fmt.Errorf("%s needs a reference time, but the request has none at position %d", k.Keyword(), k.Pos())
	}
	run := &evaluator{now: env.now, hasNow: true, weekStart: env.weekStart, litPos: lit.Pos()}
	if res, err = run.eval(root); err != nil {
		return Value{}, false, err
	}
	return Value{Kind: kind, Time: res.t}, true, nil
}
