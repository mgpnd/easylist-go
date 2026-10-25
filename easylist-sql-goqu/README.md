# easylist-sql-goqu

`easylist-sql-goqu` turns a filter expression into a prepared PostgreSQL `SELECT` query. It parses the filter with [easylist-filter](../easylist-filter), reads table and column definitions from a SQL schema, checks the filter against them, and uses [goqu](https://github.com/doug-martin/goqu) to produce SQL with bound arguments. It generates queries; it does not connect to or query a database.

## Packages

- **Root package (`querysql`):** exposes `Convert` for a single conversion and `NewCompiler` / `Compiler.Compile` for reusing parsed schema metadata. Use `Compiler.CompileAST` to compile an AST from `filter.Parse`. Its schema reader uses `pg_query_go`; schema validation and literal conversion happen before goqu renders the SQL.
- **`cmd/query2sql` command:** reads a schema file and filter from command-line arguments, then prints a JSON object containing `sql` and `args`.

The filter syntax is described in [easylist-filter's query language doc](../easylist-filter/docs/query-language.md). Unit tests live alongside the package and command source files.

## Local development

`easylist-filter` is not published yet, so this module builds only inside the [easylist-go](https://github.com/mgpnd/easylist-go) repository, whose root `go.work` links both modules. Clone that repository and work from it.

After `easylist-filter` is tagged as `easylist-filter/v<version>`, add the requirement with `go get github.com/mgpnd/easylist-go/easylist-filter@v<version>`.

## Run locally

Install Go 1.25 or newer, then run these commands from the `easylist-sql-goqu` directory. The first command creates a small schema file for the example:

```sh
printf '%s\n' 'CREATE TABLE users (name text, age integer);' > schema.sql
go run ./cmd/query2sql --schema schema.sql --table users "name = 'Artem' AND age < 30"
```

The command prints JSON with a parameterized `SELECT` in `sql` and `["Artem", 30]` in `args`. Pass your own PostgreSQL `CREATE TABLE` schema file, table name, and one quoted filter argument to use it with another table. The CLI only generates the query; your application must execute it with a PostgreSQL driver.

## Parse once and compile later

Use the two modules separately when you want to inspect or reuse a filter:

```go
import (
    filter "github.com/mgpnd/easylist-go/easylist-filter"
    querysql "github.com/mgpnd/easylist-go/easylist-sql-goqu"
)

func compileUsers() (string, []any, error) {
    ast, err := filter.Parse("name = 'Artem' AND age < 30")
    if err != nil {
        return "", nil, err
    }
    compiler, err := querysql.NewCompiler("CREATE TABLE users (name text, age integer)")
    if err != nil {
        return "", nil, err
    }
    return compiler.CompileAST("users", ast)
}
```

`filter.Parse` checks the whole expression's syntax. `CompileAST` checks column
names, operators, and value types against the selected table and returns prepared
SQL with bound arguments. It checks and converts `BETWEEN` bounds, using the
original literal position for errors. `!=` is allowed wherever `=` is and
compiles to plain SQL `<>`, so rows where the column is NULL never match;
`IS DISTINCT FROM` is not used. Both the AST and compiler can be reused
concurrently, including across tables with compatible columns. A nil AST is an
error. Compilation errors return an empty SQL string and nil arguments.

`Compiler.Compile(table, input)` resolves the table, parses the complete input,
then uses the same AST compilation path, so syntax errors precede column or value
validation. `Convert(schemaSQL, table, input)` is a convenience wrapper that
creates a compiler and calls `Compile`.

## Run unit tests

From the `easylist-sql-goqu` directory, run all package and CLI tests with:

```sh
go test ./...
```
