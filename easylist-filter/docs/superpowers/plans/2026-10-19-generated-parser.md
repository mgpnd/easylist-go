# Generated Parser Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the hand-written recursive-descent parser in `filter.Parse` with a goyacc-generated LALR parser built from `grammar.y`, and enforce that the documented grammar matches it.

**Architecture:** The hand-written lexer stays. A `parseState` adapter feeds its tokens to the generated `yyParse`, tracks open parentheses, and maps goyacc's verbose errors to `... at position N` messages. A test keeps the grammar block in `docs/query-language.md` identical to the rules in `grammar.y`.

**Tech Stack:** Go 1.25, goyacc from `golang.org/x/tools/cmd/goyacc@v0.49.0` (run through `go run`, so it is not a module dependency).

**Spec:** `docs/superpowers/specs/2026-10-19-generated-parser-design.md`

## Global Constraints

- Every error position (1-based byte offset) stays identical. Message wording may change.
- The public API is unchanged: `Parse`, `Expr`, `Predicate`, `Logical`, `Literal`, `PredicateOp`, `LogicalOp`, `LiteralKind`, and all accessors. `ast.go` is not edited.
- `lexer.go` and `lexer_test.go` are not edited. Lexer errors stay byte-for-byte the same and take priority over syntax errors.
- `go.mod` is not edited. `parser_gen.go` imports only the standard library.
- goyacc is pinned to `golang.org/x/tools/cmd/goyacc@v0.49.0`. It is the newest release whose module requires `go 1.25.0`. v0.50.0 and later require Go 1.26 and would force a toolchain download.
- Every error is a single line ending in `at position N`.
- `Parse` stays safe to call concurrently.
- Run all commands from `easylist-filter/` with `GOTOOLCHAIN=local` set, unless a step says otherwise.
- Work happens on branch `generated-parser`, which already exists and holds the spec.
- The working tree has the user's uncommitted edit to `docs/query-language.md` and an untracked `todo.txt`. Never `git add -A`. Leave `todo.txt` untracked. Task 3 deliberately replaces the grammar block that edit touched.

## Review Focus

- A stray `)` after a complete group (`(a = 1))`) must say `unmatched ')'` at the second `)`. The test goes in Task 2's message table.
- A `)` in value position inside an open group (`(a = )`) must *not* say "unmatched". It must report the unexpected `)`. The test goes in Task 2's message table.
- A lexer error later in the input beats an earlier syntax error (`a a 'x` gives `unterminated quote at position 5`, not position 3). The test goes in Task 1.
- Concurrent `Parse` calls must not race on generated package-level state. The test goes in Task 1 and runs under `-race`.
- Errors must contain no newline, because easylist-sql-goqu's `query2sql` rejects multi-line errors. Task 2's message table asserts this.

---

### Task 1: Swap in the goyacc parser

**Files:**
- Create: `grammar.y`
- Create (generated): `parser_gen.go`
- Modify: `parser.go` (replace everything below `Parse`)
- Test: `parser_test.go`

**Interfaces:**
- Consumes: `lexFilter(src string) ([]token, error)`, `token{kind, text, pos, quoted}`, and the `tokenKind` constants from `lexer.go`. Consumes `predicateNode`, `logicalNode`, and `Literal` from `ast.go`.
- Produces:
  - `type opAt struct{ op PredicateOp; pos int }`
  - `type parseState struct` implementing `yyLexer` (`Lex(*yySymType) int`, `Error(string)`)
  - The generated token constants `IDENT STRING NUMBER BOOL NEQ GTE LTE CONTAINS EXCLUDES BETWEEN AND OR`, plus the char tokens `'=' '>' '<' '(' ')'`
  - The goyacc globals `yyErrorVerbose` and `yyNewParser()`

- [ ] **Step 1: Add characterization tests to `parser_test.go`**

These must pass on the current parser before the swap, and still pass after it.

```go
func TestParseLexerErrorWinsOverSyntaxError(t *testing.T) {
	_, err := Parse(`a a 'x`)
	if err == nil || err.Error() != "unterminated quote at position 5" {
		t.Fatalf("got %v", err)
	}
}

func TestParseConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				if root, err := Parse(`(a = 1 OR b != 'x') AND c BETWEEN '1,2'`); err != nil || root == nil {
					t.Error(err)
				}
				if _, err := Parse(`a = 1 AND (`); err == nil {
					t.Error("want error")
				}
			}
		}()
	}
	wg.Wait()
}
```

Also append these rows to the `TestParseSyntaxErrors` table: `{"(a = 1))", 8}`, `{"(a = )", 6}`, `{"a = )", 5}`, `{"((a = 1) OR (b = 2", 13}`.

- [ ] **Step 2: Run the tests on the old parser**

Run: `GOTOOLCHAIN=local go test -race -count=1 ./...`
Expected: PASS. These tests describe current behavior.

- [ ] **Step 3: Create `grammar.y`**

Keep the rule names, token names, and rule order exactly as below. Task 3's doc test and the error display names depend on them.

```yacc
%{
package filter
%}

%union {
	tok  token
	expr Expr
	op   opAt
	lit  Literal
}

%token <tok> IDENT STRING NUMBER BOOL NEQ GTE LTE CONTAINS EXCLUDES BETWEEN AND OR '=' '>' '<' '(' ')'

%type <expr> or_expr and_expr primary predicate
%type <op> ordinary_op
%type <lit> value

%%

filter
	: or_expr { yylex.(*parseState).result = $1 }
	;

or_expr
	: and_expr
	| or_expr OR and_expr { $$ = &logicalNode{operator: Or, operatorPos: $2.pos, left: $1, right: $3} }
	;

and_expr
	: primary
	| and_expr AND primary { $$ = &logicalNode{operator: And, operatorPos: $2.pos, left: $1, right: $3} }
	;

primary
	: predicate
	| '(' or_expr ')' { $$ = $2 }
	;

predicate
	: IDENT ordinary_op value { $$ = &predicateNode{field: $1, operator: $2.op, operatorPos: $2.pos, value: $3} }
	| IDENT BETWEEN STRING    { $$ = &predicateNode{field: $1, operator: OpBetween, operatorPos: $2.pos, value: Literal{kind: LiteralString, text: $3.text, pos: $3.pos}} }
	;

ordinary_op
	: '='      { $$ = opAt{OpEq, $1.pos} }
	| NEQ      { $$ = opAt{OpNeq, $1.pos} }
	| '>'      { $$ = opAt{OpGt, $1.pos} }
	| GTE      { $$ = opAt{OpGte, $1.pos} }
	| '<'      { $$ = opAt{OpLt, $1.pos} }
	| LTE      { $$ = opAt{OpLte, $1.pos} }
	| CONTAINS { $$ = opAt{OpContains, $1.pos} }
	| EXCLUDES { $$ = opAt{OpExcludes, $1.pos} }
	;

value
	: STRING { $$ = Literal{kind: LiteralString, text: $1.text, pos: $1.pos} }
	| NUMBER { $$ = Literal{kind: LiteralNumber, text: $1.text, pos: $1.pos} }
	| BOOL   { $$ = Literal{kind: LiteralBool, text: $1.text, pos: $1.pos} }
	;

%%
```

- [ ] **Step 4: Rewrite `parser.go`**

Keep `Parse`'s doc comment and signature. Delete the `parser` type, `parseFilterSyntax`, all `parse*` methods, `eofError`, `peek`, `advance`, `predicateOps`, and `literalKinds`. Add the following:

- The directive below, after the imports at the top of `parser.go`:
  `//go:generate go run golang.org/x/tools/cmd/goyacc@v0.49.0 -o parser_gen.go -p yy -v "" grammar.y`
- `func init() { yyErrorVerbose = true }`
- The `Parse` body: call `lexFilter`, returning its error unchanged. Otherwise build `&parseState{tokens: tokens, eofPos: len(input) + 1}`, run `yyNewParser().Parse(s)`, and return `nil, s.err` or `s.result, nil`.
- `type opAt struct{ op PredicateOp; pos int }`
- `parseState`:
  ```go
  type parseState struct {
  	tokens     []token
  	next       int
  	last       token // most recent lookahead, for error positions
  	atEOF      bool
  	strayClose bool  // last was a ')' with no '(' open
  	eofPos     int   // len(input)+1
  	open       []int // positions of '(' handed to the parser and not yet closed, innermost last
  	result     Expr
  	err        error // first error only
  }
  ```
- `var yyCodes = [...]int{tokIdent: IDENT, ..., tokLParen: '(', tokRParen: ')', tokNeq: NEQ}`, indexed by `tokenKind` and covering all 17 kinds.
- `func (s *parseState) Lex(lval *yySymType) int`:
  - At the end of the tokens, set `atEOF` and return 0.
  - Otherwise advance, set `last` and `lval.tok`, and return `yyCodes[tok.kind]`.
  - On `tokLParen`, push `pos`. On `tokRParen`, pop if `open` is non-empty, else set `strayClose`.
- `func (s *parseState) Error(msg string)`: ignore the call if `s.err` is already set. Otherwise:
  1. If `atEOF && len(open) > 0`, report `unmatched '(' at position <open[len-1]>`.
  2. If `!atEOF && strayClose`, report `unmatched ')' at position <last.pos>`.
  3. Otherwise, report `fmt.Errorf("%s at position %d", msg, pos)`, where `pos` is `last.pos`, or `eofPos` when `atEOF`. Task 2 replaces the raw `msg`.

- [ ] **Step 5: Generate the parser**

Run: `GOTOOLCHAIN=local go generate ./...`
Expected: `parser_gen.go` is created, and its first line is `// Code generated by goyacc -o parser_gen.go -p yy -v  grammar.y. DO NOT EDIT.`. No `y.output` file appears.

- [ ] **Step 6: Run all tests**

Run: `GOTOOLCHAIN=local go test -race -count=1 ./... && go vet ./...`
Expected: PASS, including the 2,000-deep nesting test and every position in `TestParseSyntaxErrors`.

- [ ] **Step 7: Commit**

```bash
git add grammar.y parser_gen.go parser.go parser_test.go
git commit -m "refactor: generate the parser from grammar.y with goyacc"
```

---

### Task 2: User-facing syntax error messages

**Files:**
- Modify: `parser.go`
- Test: `parser_test.go`

**Interfaces:**
- Consumes: `parseState.Error` from Task 1.
- Produces: `func describeSyntaxError(msg string) string` and `var tokenDisplay map[string]string`.

- [ ] **Step 1: Write the failing message test**

```go
// Mirrors the error table in docs/parser.md, plus edge cases around parentheses.
func TestParseErrorMessages(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"", "unexpected end of input, expected field or '(' at position 1"},
		{"a 1", "unexpected number at position 3"},
		{"a =", "unexpected end of input, expected string, number, or boolean at position 4"},
		{"a BETWEEN 1", "unexpected number, expected string at position 11"},
		{"(a = 1 OR)", "unexpected ')', expected field or '(' at position 10"},
		{"a = 1 b = 2", "unexpected field at position 7"},
		{"a = 1)", "unmatched ')' at position 6"},
		{"a = 1 AND (", "unmatched '(' at position 11"},
		{"((a = 1) OR (b = 2", "unmatched '(' at position 13"},
		{"a = 'x", "unterminated quote at position 5"},
		{"1abc = 1", "invalid number at position 2"},
		{"a = 1 @", "unexpected character '@' at position 7"},
		{"AND a = 1", "unexpected AND, expected field or '(' at position 1"},
		{"a = 1 != 2", "unexpected '!=' at position 7"},
		{"a = )", "unmatched ')' at position 5"},
		{"(a = )", "unexpected ')', expected string, number, or boolean at position 6"},
		{"(a = 1))", "unmatched ')' at position 8"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			_, err := Parse(tc.input)
			if err == nil || err.Error() != tc.want || strings.Contains(err.Error(), "\n") {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `GOTOOLCHAIN=local go test -count=1 -run TestParseErrorMessages ./...`
Expected: FAIL. Rows that go through rule 3 show raw goyacc text such as `syntax error: unexpected $end, expecting IDENT or '('`.

- [ ] **Step 3: Implement `describeSyntaxError(msg string) string` in `parser.go`, and use it in rule 3 of `Error`**

- Strip the `syntax error: ` and `unexpected ` prefixes. Split the remainder on `, expecting `, then split the expected part on ` or `.
- Map every name through `tokenDisplay`. Names missing from the map pass through unchanged, which covers keywords and char tokens like `'('`.
  ```go
  var tokenDisplay = map[string]string{
  	"$end": "end of input", "IDENT": "field", "STRING": "string", "NUMBER": "number",
  	"BOOL": "boolean", "NEQ": "'!='", "GTE": "'>='", "LTE": "'<='",
  }
  ```
- Build `unexpected X`. If there are expected names, append `, expected ` followed by `A`, `A or B`, or `A, B, or C` (with a serial comma).

- [ ] **Step 4: Run all tests**

Run: `GOTOOLCHAIN=local go test -race -count=1 ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add parser.go parser_test.go
git commit -m "feat: readable syntax errors from the generated parser"
```

---

### Task 3: Keep the documented grammar in sync with `grammar.y`

**Files:**
- Create: `grammar_doc_test.go`
- Modify: `docs/query-language.md` (the grammar block and the sentence before it)

**Interfaces:**
- Consumes: the rules in `grammar.y` from Task 1.
- Produces: the test helpers `yaccRules(src string) []string` and `docGrammarRules(md string) []string`. Each returns one normalized string per rule, such as `"or_expr : and_expr | or_expr OR and_expr"`.

- [ ] **Step 1: Write the failing tests in `grammar_doc_test.go`**

```go
func TestYaccRulesStripsActions(t *testing.T) {
	src := "%{\npackage x\n%}\n%token A\n%%\n\nr\n\t: A { s := \"}\"; _ = '{' }\n\t| r A\n\t;\n%%\n"
	if got, want := yaccRules(src), []string{"r : A | r A"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestGrammarDocMatchesYacc(t *testing.T) {
	y, err := os.ReadFile("grammar.y")
	if err != nil {
		t.Fatal(err)
	}
	md, err := os.ReadFile("docs/query-language.md")
	if err != nil {
		t.Fatal(err)
	}
	got, want := docGrammarRules(string(md)), yaccRules(string(y))
	if len(want) == 0 || !reflect.DeepEqual(got, want) {
		t.Fatalf("docs/query-language.md grammar is out of sync with grammar.y\ndoc:  %q\nyacc: %q", got, want)
	}
}
```

- [ ] **Step 2: Implement the helpers in `grammar_doc_test.go`**

- `yaccRules`:
  - Take the text between the first and second lines that are exactly `%%`.
  - Drop `{ ... }` action blocks with brace-depth counting. While inside a block, skip Go string literals (`"..."` with `\` escapes, and raw strings in backquotes) and rune literals (`'...'`), so braces inside them don't count.
  - Split on `;`, collapse runs of whitespace to single spaces, trim, and drop empty entries.
- `docGrammarRules`: take the body of the first fenced block opened with three backticks followed by `yacc`, then split, collapse, and trim the same way.

- [ ] **Step 3: Run them to verify the doc test fails**

Run: `GOTOOLCHAIN=local go test -count=1 -run 'TestYaccRules|TestGrammarDoc' ./...`
Expected: `TestYaccRulesStripsActions` PASS and `TestGrammarDocMatchesYacc` FAIL, because the doc has no `yacc` block yet.

- [ ] **Step 4: Update `docs/query-language.md`**

- Replace the grammar block, whose fence is currently `ebnf` in the working tree, with a block fenced as `yacc`:
  ```
  filter      : or_expr ;
  or_expr     : and_expr | or_expr OR and_expr ;
  and_expr    : primary | and_expr AND primary ;
  primary     : predicate | '(' or_expr ')' ;
  predicate   : IDENT ordinary_op value | IDENT BETWEEN STRING ;
  ordinary_op : '=' | NEQ | '>' | GTE | '<' | LTE | CONTAINS | EXCLUDES ;
  value       : STRING | NUMBER | BOOL ;
  ```
- Replace the two sentences about `{ … }`/`( … | … )` notation and LL(1) with these sentences:
  - "The grammar block is the rules section of `grammar.y` with its Go actions removed, and a test keeps them identical."
  - "`|` separates alternatives and `;` ends a rule."
  - "`NEQ`, `GTE`, and `LTE` are the `!=`, `>=`, and `<=` tokens."
  - "The parser is generated from `grammar.y`; see [parser.md](parser.md)."
- Drop the `<filter> ... EOF` rule. End of input is implicit in yacc, and the prose already says what `EOF` means. Remove that sentence too.
- Leave the rest of the file unchanged.

- [ ] **Step 5: Run all tests**

Run: `GOTOOLCHAIN=local go test -race -count=1 ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add grammar_doc_test.go docs/query-language.md
git commit -m "test: enforce that the documented grammar matches grammar.y"
```

---

### Task 4: Internals docs, README, and final verification

**Files:**
- Modify: `docs/parser.md`
- Modify: `README.md`

**Interfaces:**
- Consumes: everything above. This task changes no code.

- [ ] **Step 1: Rewrite the Stage 2 section of `docs/parser.md`**

- Update the pipeline diagram to `[]token ──parseState──▶ yyParse (parser_gen.go, from grammar.y) ──▶ Expr`.
- Replace the recursive-descent method table, the `parser` struct, `parseLogical`, and the call-trace example with these points:
  - `grammar.y` holds the rules, and goyacc builds an LALR(1) parser from them.
  - Left recursion (`or_expr OR and_expr`) makes both connectors left-associative.
  - Because `or_expr` is built from `and_expr`, AND binds more tightly than OR. No precedence declarations are used.
  - Parentheses return their inner expression.
  - `BETWEEN STRING` is enforced by the grammar.
  - `parseState` feeds tokens to the parser and tracks open `(`.
  - The parser's value stack grows as needed, so nesting depth does not recurse.
  - Keep the `OR @7` tree example.
- Under "Error reporting", describe the three rules from Task 1 Step 4 and the display names from Task 2. Replace the sample table with the first 12 rows of `TestParseErrorMessages`, verbatim.
- Under "Extending the parser":
  - New operator: add a lexer kind, a `%token` and an `ordinary_op` alternative in `grammar.y`, a `yyCodes` entry, and a `tokenDisplay` entry if the name isn't user-facing. Then run `go generate`.
  - New rule: edit `grammar.y`, update the `yacc` block in `query-language.md` (the test enforces this), then run `go generate`.
  - Mention that 2,000 nested groups are tested.

- [ ] **Step 2: Add a "Regenerate the parser" section to `README.md`**, after "Run unit tests"

Include this text: after editing `grammar.y`, run

```sh
go generate ./...
```

and commit `parser_gen.go`. The generator is fetched with `go run` at a pinned version, so the module has no dependencies. If `go generate` changes `parser_gen.go`, someone forgot to regenerate.

In the Package section, mention that the parser is generated from `grammar.y`.

- [ ] **Step 3: Check that the generated file is fresh**

Run: `GOTOOLCHAIN=local go generate ./... && git diff --exit-code parser_gen.go && git status --short`
Expected: no diff in `parser_gen.go`. `git status` shows only the doc edits from this task and the untracked `todo.txt`.

- [ ] **Step 4: Run the whole workspace**

Run from `/Users/ar/code/go-workspaces`: `GOTOOLCHAIN=local go test -race -count=1 ./easylist-filter/... ./easylist-sql-goqu/... && go vet ./easylist-filter/... ./easylist-sql-goqu/...`
Expected: PASS. easylist-sql-goqu's `compiler_test.go` and `cmd/query2sql` tests pass unchanged.

- [ ] **Step 5: Commit**

```bash
git add docs/parser.md README.md
git commit -m "docs: describe the generated parser and how to regenerate it"
```
