# easylist-filter

`easylist-filter` parses a user-facing filter expression such as `name = 'Artem' AND age < 30` into an immutable, inspectable AST. It has no schema, SQL, or third-party dependencies. To turn the AST into a prepared PostgreSQL query, use [easylist-sql-goqu](../easylist-sql-goqu).

## Package

- **Root package (`filter`):** exposes `filter.Parse` to build the AST. The parser is generated from `grammar.y`. Inspect nodes through `filter.Predicate` and `filter.Logical`. Parentheses are represented by tree shape.
- **Relative values:** a value can be a time keyword such as `TODAY` or a brace expression such as `{TODAY + 5::DAYS}`. It parses to a `filter.LiteralExpr` literal whose `Expr()` returns an unevaluated tree of `filter.NumberNode`, `filter.KeywordNode`, `filter.DurationNode`, `filter.BinaryNode`, and `filter.NegationNode`. The parser only checks syntax: the `easylist-core` planner type-checks and evaluates the tree, and drivers read its results from the plan. Code that switches on `filter.LiteralKind` must handle or reject `LiteralExpr`.

The filter syntax is described in [docs/query-language.md](docs/query-language.md), with its grammar in EBNF, and the lexer and parser internals in [docs/parser.md](docs/parser.md). Unit tests live alongside the package source files; `grammar_doc_test.go` checks that the documented EBNF and the parser accept and reject the same filters.

## Parse a filter

```go
import filter "github.com/mgpnd/easylist-go/easylist-filter"

func fields(input string) ([]string, error) {
    expr, err := filter.Parse(input)
    if err != nil {
        return nil, err
    }
    var names []string
    var walk func(filter.Expr)
    walk = func(e filter.Expr) {
        switch n := e.(type) {
        case filter.Predicate:
            names = append(names, n.Field())
        case filter.Logical:
            walk(n.Left())
            walk(n.Right())
        }
    }
    walk(expr)
    return names, nil
}
```

`filter.Parse` checks the whole expression's syntax and reports errors with 1-based byte positions. The AST can be reused concurrently.

## Run unit tests

From the `easylist-filter` directory, run:

```sh
go test ./...
```

## Regenerate the parser

After editing `grammar.y`, run:

```sh
go generate ./...
```

and commit `parser_gen.go`. The generator is fetched with `go run` at a pinned version, so the module has no dependencies. If `go generate` changes `parser_gen.go`, someone forgot to regenerate.
