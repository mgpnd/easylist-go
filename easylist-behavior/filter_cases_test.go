package behavior_test

import (
	"fmt"
	"strings"
	"testing"
)

// filterCase is one case of a filter fixture file: the comment lines before
// it, with their "#", and its filter expression.
type filterCase struct {
	Comments []string
	Expr     string
	Line     int // 1-based line of Expr in the fixture file
}

// parseFilterCases splits the filter fixture text src, read from file, into
// its cases. Cases are separated by one or more blank lines. A case is zero
// or more comment lines, starting with "#", followed by exactly one line
// holding the filter expression. Lines are trimmed of surrounding
// whitespace. Any other shape is an error naming file and line, as is a file
// with no case.
func parseFilterCases(file, src string) ([]filterCase, error) {
	var cases []filterCase
	var cur *filterCase
	start := 0 // first line of cur
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	endCase := func() error {
		if cur != nil && cur.Expr == "" {
			return fmt.Errorf("%s:%d: case has comments but no filter expression", file, start)
		}
		if cur != nil {
			cases = append(cases, *cur)
		}
		cur = nil
		return nil
	}
	for i, raw := range lines {
		n := i + 1
		line := strings.TrimSpace(raw)
		if line == "" {
			if err := endCase(); err != nil {
				return nil, err
			}
			continue
		}
		if cur == nil {
			cur, start = &filterCase{}, n
		}
		switch {
		case cur.Expr != "" && strings.HasPrefix(line, "#"):
			return nil, fmt.Errorf("%s:%d: comment after the filter expression of line %d; comments go before it", file, n, cur.Line)
		case cur.Expr != "":
			return nil, fmt.Errorf("%s:%d: second filter expression in the case of line %d; separate cases with a blank line", file, n, cur.Line)
		case strings.HasPrefix(line, "#"):
			cur.Comments = append(cur.Comments, line)
		default:
			cur.Expr, cur.Line = line, n
		}
	}
	if err := endCase(); err != nil {
		return nil, err
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("%s: no filter cases", file)
	}
	return cases, nil
}

func TestParseFilterCases(t *testing.T) {
	for _, c := range []struct {
		name, src string
		want      []filterCase
		err       string
	}{
		{
			name: "valid",
			src: "# != keeps NULL rows\nactive != TRUE\n\n" +
				"# two\n#   comment lines\nage > 30\n\n\n" +
				"name = 'x'\n",
			want: []filterCase{
				{Comments: []string{"# != keeps NULL rows"}, Expr: "active != TRUE", Line: 2},
				{Comments: []string{"# two", "#   comment lines"}, Expr: "age > 30", Line: 6},
				{Expr: "name = 'x'", Line: 9},
			},
		},
		{
			name: "surrounding blank lines, whitespace, and CRLF",
			src:  "\n\n  # c  \r\n  age > 30  \r\n   \r\n",
			want: []filterCase{{Comments: []string{"# c"}, Expr: "age > 30", Line: 4}},
		},
		{
			name: "two expressions in one case",
			src:  "# c\nage > 30\nage < 40\n",
			err:  "f.txt:3: second filter expression in the case of line 2; separate cases with a blank line",
		},
		{
			name: "comment after the expression",
			src:  "age > 30\n# c\n",
			err:  "f.txt:2: comment after the filter expression of line 1; comments go before it",
		},
		{
			name: "trailing comment with no expression",
			src:  "age > 30\n\n# dangling\n# comment\n",
			err:  "f.txt:3: case has comments but no filter expression",
		},
		{
			name: "comment with no expression between cases",
			src:  "age > 30\n\n# lost\n\nage < 40\n",
			err:  "f.txt:3: case has comments but no filter expression",
		},
		{
			name: "empty file",
			src:  "\n\n",
			err:  "f.txt: no filter cases",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseFilterCases("f.txt", c.src)
			if c.err != "" {
				if err == nil || err.Error() != c.err {
					t.Fatalf("error = %v, want %q", err, c.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprintf("%#v", got) != fmt.Sprintf("%#v", c.want) {
				t.Errorf("cases =\n%#v\nwant\n%#v", got, c.want)
			}
		})
	}
}
