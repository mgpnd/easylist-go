package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	querysql "github.com/mgpnd/easylist-go/easylist-sql-goqu"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fail := func(err error) int {
		fmt.Fprintln(stderr, "query2sql:", err)
		return 1
	}
	flags := flag.NewFlagSet("query2sql", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	schemaPath := flags.String("schema", "", "SQL schema file")
	table := flags.String("table", "", "table name")
	if err := flags.Parse(args); err != nil {
		return fail(err)
	}
	if *schemaPath == "" {
		return fail(fmt.Errorf("--schema is required"))
	}
	if *table == "" {
		return fail(fmt.Errorf("--table is required"))
	}
	if flags.NArg() != 1 {
		return fail(fmt.Errorf("exactly one filter argument is required"))
	}
	schema, err := os.ReadFile(*schemaPath)
	if err != nil {
		return fail(fmt.Errorf("read schema: %w", err))
	}
	sql, sqlArgs, err := querysql.Convert(string(schema), *table, flags.Arg(0))
	if err != nil {
		return fail(fmt.Errorf("compile filter: %w", err))
	}
	jsonArgs := make([]any, len(sqlArgs))
	for i, arg := range sqlArgs {
		if date, ok := arg.(time.Time); ok {
			jsonArgs[i] = date.Format("2006-01-02")
		} else {
			jsonArgs[i] = arg
		}
	}
	output, err := json.Marshal(struct {
		SQL  string `json:"sql"`
		Args []any  `json:"args"`
	}{SQL: sql, Args: jsonArgs})
	if err != nil {
		return fail(fmt.Errorf("encode output: %w", err))
	}
	if _, err := stdout.Write(append(output, '\n')); err != nil {
		return fail(fmt.Errorf("write output: %w", err))
	}
	return 0
}
