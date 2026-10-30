// Package filter parses filter expressions into immutable, inspectable syntax
// trees without requiring a database schema or SQL dependencies.
package filter

import (
	"slices"
	"strconv"
)

// Expr is a filter expression. Only this package can implement it.
// Inspect expressions through the Predicate and Logical interfaces.
type Expr interface{ filterExpr() }

// Predicate is a field comparison with a literal value. Field names use
// PostgreSQL ASCII case folding and are never truncated; length limits belong
// to the consumer that resolves them.
// FieldPos and OperatorPos are 1-based byte offsets in the input; for NOT IN,
// OperatorPos is the position of NOT.
type Predicate interface {
	Expr
	Field() string
	FieldQuoted() bool
	FieldPos() int
	Operator() PredicateOp
	OperatorPos() int
	Value() Literal
}

// Logical combines two expressions with AND or OR. OperatorPos is the
// connector's 1-based byte offset in the input.
type Logical interface {
	Expr
	Operator() LogicalOp
	OperatorPos() int
	Left() Expr
	Right() Expr
}

// PredicateOp identifies a predicate operator.
type PredicateOp uint8

const (
	OpEq       PredicateOp = iota // Equal (=).
	OpGt                          // Greater than (>).
	OpGte                         // Greater than or equal (>=).
	OpLt                          // Less than (<).
	OpLte                         // Less than or equal (<=).
	OpContains                    // CONTAINS.
	OpExcludes                    // EXCLUDES.
	OpBetween                     // BETWEEN with a string literal or a two-item list.
	OpNeq                         // Not equal (!=).
	// IN with a LiteralList value whose items are strings, numbers,
	// Booleans, or NULL. Consumers that switch on PredicateOp must handle or
	// reject this operator.
	OpIn
	// NOT IN with a LiteralList value, like OpIn. Its operator position is
	// the position of NOT. Consumers that switch on PredicateOp must handle
	// or reject this operator.
	OpNotIn
)

var predicateOpNames = [...]string{
	OpEq: "=", OpGt: ">", OpGte: ">=", OpLt: "<", OpLte: "<=",
	OpContains: "CONTAINS", OpExcludes: "EXCLUDES", OpBetween: "BETWEEN",
	OpNeq: "!=", OpIn: "IN", OpNotIn: "NOT IN",
}

// String returns the operator's canonical spelling, such as ">=" or "NOT IN".
func (o PredicateOp) String() string { return enumName(predicateOpNames[:], int(o), "PredicateOp") }

// LogicalOp identifies a logical connector.
type LogicalOp uint8

const (
	And LogicalOp = iota // AND binds more tightly than OR.
	Or                   // OR.
)

var logicalOpNames = [...]string{And: "AND", Or: "OR"}

// String returns the connector's canonical uppercase spelling, such as "AND".
func (o LogicalOp) String() string { return enumName(logicalOpNames[:], int(o), "LogicalOp") }

// LiteralKind identifies a literal's syntax, before schema-based conversion.
type LiteralKind uint8

const (
	LiteralString LiteralKind = iota // A decoded single-quoted string.
	LiteralNumber                    // A number with its exact source spelling.
	LiteralBool                      // A Boolean with its source spelling.
	LiteralNull                      // The NULL keyword with its source spelling.
	// A bare time keyword or a brace expression, such as TODAY or
	// {TODAY + 5::DAYS}, unevaluated. Its text is the source spelling, braces
	// included, and Expr returns its tree. Consumers that switch on
	// LiteralKind must handle or reject this kind.
	LiteralExpr
	// A parenthesized value list, such as ('a', 'b'). Its text is the source
	// spelling, parentheses included, its position is the '(', and Items
	// returns its items. It is the value of IN and NOT IN, and may be the
	// value of CONTAINS, EXCLUDES, and BETWEEN. Consumers that switch on
	// LiteralKind must handle or reject this kind.
	LiteralList
)

// Literal is a read-only literal value. Positions are 1-based byte offsets.
type Literal struct {
	kind  LiteralKind
	text  string
	pos   int
	expr  ExprNode
	items []Literal
}

// Kind reports the literal's syntax kind.
func (l Literal) Kind() LiteralKind { return l.kind }

// Text returns decoded string text or the original numeric, Boolean, NULL,
// expression, or list spelling.
func (l Literal) Text() string { return l.text }

// Pos returns the literal's 1-based byte position in the input. For an
// expression it is the position of the keyword or of the opening '{', and for
// a list the position of the '('.
func (l Literal) Pos() int { return l.pos }

// Expr returns the root of an expression literal's tree, or nil for literals
// of every other kind. The tree mirrors the source and is never evaluated.
func (l Literal) Expr() ExprNode { return l.expr }

// Items returns a copy of a list literal's items in source order, or nil for
// literals of every other kind. Each item is a string, number, Boolean, or
// NULL literal with its own text and position; only IN and NOT IN lists
// contain NULL items.
func (l Literal) Items() []Literal { return slices.Clone(l.items) }

// ExprNode is a node of a value expression tree. Only this package can
// implement it. Inspect nodes through NumberNode, KeywordNode, DurationNode,
// BinaryNode, and NegationNode; every node implements exactly one of them.
// Pos is the node's 1-based byte offset in the input. Parentheses only shape
// the tree and have no node of their own.
type ExprNode interface {
	exprNode()
	Pos() int
}

// NumberNode is an unsigned number literal inside braces. Text is its exact
// source spelling, such as "1.50".
type NumberNode interface {
	ExprNode
	Text() string
	numberNode()
}

// KeywordNode is a relative time keyword. Text is its source spelling, such
// as "today" for TimeToday.
type KeywordNode interface {
	ExprNode
	Keyword() TimeKeyword
	Text() string
	keywordNode()
}

// DurationNode is a duration such as 5::DAYS. Amount is the amount's decimal
// digits as written, of any size; Pos is the amount's position and UnitPos the
// unit's.
type DurationNode interface {
	ExprNode
	Amount() string
	Unit() TimeUnit
	UnitPos() int
}

// BinaryNode applies an arithmetic operator to two operands. Pos is the
// operator's position.
type BinaryNode interface {
	ExprNode
	Operator() ArithOp
	Left() ExprNode
	Right() ExprNode
}

// NegationNode is a unary minus. Pos is the position of the '-'.
type NegationNode interface {
	ExprNode
	Operand() ExprNode
}

// TimeKeyword identifies a relative time keyword. Keywords are
// case-insensitive and recognised only on the value side of a predicate.
type TimeKeyword uint8

const (
	TimeNow          TimeKeyword = iota // NOW, a timestamp.
	TimeToday                           // TODAY, a date.
	TimeStartOfWeek                     // START_OF_WEEK, a date.
	TimeEndOfWeek                       // END_OF_WEEK, a date.
	TimeStartOfMonth                    // START_OF_MONTH, a date.
	TimeEndOfMonth                      // END_OF_MONTH, a date.
	TimeStartOfYear                     // START_OF_YEAR, a date.
	TimeEndOfYear                       // END_OF_YEAR, a date.
)

var timeKeywordNames = [...]string{
	TimeNow: "NOW", TimeToday: "TODAY",
	TimeStartOfWeek: "START_OF_WEEK", TimeEndOfWeek: "END_OF_WEEK",
	TimeStartOfMonth: "START_OF_MONTH", TimeEndOfMonth: "END_OF_MONTH",
	TimeStartOfYear: "START_OF_YEAR", TimeEndOfYear: "END_OF_YEAR",
}

// String returns the keyword's canonical uppercase spelling, such as "TODAY".
func (k TimeKeyword) String() string { return enumName(timeKeywordNames[:], int(k), "TimeKeyword") }

// TimeUnit identifies a duration unit. Units are case-insensitive.
type TimeUnit uint8

const (
	UnitDays         TimeUnit = iota // DAYS, calendar days.
	UnitBusinessDays                 // BUSINESS_DAYS, Monday to Friday.
	UnitHours                        // HOURS, elapsed hours.
	UnitMinutes                      // MINUTES, elapsed minutes.
)

var timeUnitNames = [...]string{
	UnitDays: "DAYS", UnitBusinessDays: "BUSINESS_DAYS", UnitHours: "HOURS", UnitMinutes: "MINUTES",
}

// String returns the unit's canonical uppercase spelling, such as "DAYS".
func (u TimeUnit) String() string { return enumName(timeUnitNames[:], int(u), "TimeUnit") }

// ArithOp identifies an arithmetic operator inside braces.
type ArithOp uint8

const (
	ArithAdd ArithOp = iota // Addition (+).
	ArithSub                // Subtraction (-).
	ArithMul                // Multiplication (*).
	ArithDiv                // Division (/).
)

var arithOpNames = [...]string{ArithAdd: "+", ArithSub: "-", ArithMul: "*", ArithDiv: "/"}

// String returns the operator's symbol, such as "+".
func (o ArithOp) String() string { return enumName(arithOpNames[:], int(o), "ArithOp") }

func enumName(names []string, i int, typ string) string {
	if i < len(names) {
		return names[i]
	}
	return typ + "(" + strconv.Itoa(i) + ")"
}

type numberNode struct {
	text string
	pos  int
}

func (*numberNode) exprNode()      {}
func (*numberNode) numberNode()    {}
func (n *numberNode) Pos() int     { return n.pos }
func (n *numberNode) Text() string { return n.text }

type keywordNode struct {
	keyword TimeKeyword
	text    string
	pos     int
}

func (*keywordNode) exprNode()              {}
func (*keywordNode) keywordNode()           {}
func (k *keywordNode) Pos() int             { return k.pos }
func (k *keywordNode) Keyword() TimeKeyword { return k.keyword }
func (k *keywordNode) Text() string         { return k.text }

type durationNode struct {
	amount  string
	pos     int
	unit    TimeUnit
	unitPos int
}

func (*durationNode) exprNode()        {}
func (d *durationNode) Pos() int       { return d.pos }
func (d *durationNode) Amount() string { return d.amount }
func (d *durationNode) Unit() TimeUnit { return d.unit }
func (d *durationNode) UnitPos() int   { return d.unitPos }

type binaryNode struct {
	operator    ArithOp
	pos         int
	left, right ExprNode
}

func (*binaryNode) exprNode()           {}
func (b *binaryNode) Pos() int          { return b.pos }
func (b *binaryNode) Operator() ArithOp { return b.operator }
func (b *binaryNode) Left() ExprNode    { return b.left }
func (b *binaryNode) Right() ExprNode   { return b.right }

type negationNode struct {
	pos     int
	operand ExprNode
}

func (*negationNode) exprNode()           {}
func (n *negationNode) Pos() int          { return n.pos }
func (n *negationNode) Operand() ExprNode { return n.operand }

type predicateNode struct {
	field       token
	operator    PredicateOp
	operatorPos int
	value       Literal
}

func (*predicateNode) filterExpr()             {}
func (p *predicateNode) Field() string         { return p.field.text }
func (p *predicateNode) FieldQuoted() bool     { return p.field.quoted }
func (p *predicateNode) FieldPos() int         { return p.field.pos }
func (p *predicateNode) Operator() PredicateOp { return p.operator }
func (p *predicateNode) OperatorPos() int      { return p.operatorPos }
func (p *predicateNode) Value() Literal        { return p.value }

type logicalNode struct {
	operator    LogicalOp
	operatorPos int
	left, right Expr
}

func (*logicalNode) filterExpr()           {}
func (l *logicalNode) Operator() LogicalOp { return l.operator }
func (l *logicalNode) OperatorPos() int    { return l.operatorPos }
func (l *logicalNode) Left() Expr          { return l.left }
func (l *logicalNode) Right() Expr         { return l.right }
