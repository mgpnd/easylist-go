// Package filter parses filter expressions into immutable, inspectable syntax
// trees without requiring a database schema or SQL dependencies.
package filter

// Expr is a filter expression. Only this package can implement it.
// Inspect expressions through the Predicate and Logical interfaces.
type Expr interface{ filterExpr() }

// Predicate is a field comparison with a literal value. Field names use
// PostgreSQL ASCII case folding and are never truncated; length limits belong
// to the consumer that resolves them.
// FieldPos and OperatorPos are 1-based byte offsets in the input.
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
	OpBetween                     // BETWEEN with a single string literal.
	OpNeq                         // Not equal (!=).
)

// LogicalOp identifies a logical connector.
type LogicalOp uint8

const (
	And LogicalOp = iota // AND binds more tightly than OR.
	Or                   // OR.
)

// LiteralKind identifies a literal's syntax, before schema-based conversion.
type LiteralKind uint8

const (
	LiteralString LiteralKind = iota // A decoded single-quoted string.
	LiteralNumber                    // A number with its exact source spelling.
	LiteralBool                      // A Boolean with its source spelling.
)

// Literal is a read-only literal value. Positions are 1-based byte offsets.
type Literal struct {
	kind LiteralKind
	text string
	pos  int
}

// Kind reports the literal's syntax kind.
func (l Literal) Kind() LiteralKind { return l.kind }

// Text returns decoded string text or the original numeric/Boolean spelling.
func (l Literal) Text() string { return l.text }

// Pos returns the literal's 1-based byte position in the input.
func (l Literal) Pos() int { return l.pos }

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
