package conformance_test

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
	"github.com/mgpnd/easylist-go/easylist-sqlpg/conformance"
	"github.com/mgpnd/easylist-go/easylist-sqlpg/internal/refdriver"
)

// The reference driver renders every node through its fragment, so it
// passes the suite by construction of the contract.
func TestReferenceDriver(t *testing.T) {
	conformance.Run(t, refdriver.Driver{})
}

// brokenEnv selects the broken driver TestBrokenDriver runs the suite with.
const brokenEnv = "EASYLIST_CONFORMANCE_BROKEN"

var brokenDrivers = map[string]easylist.Driver{
	// The highest placeholder is $n-1 for n arguments.
	"placeholders": wrap(func(sql string, args []any) (string, []any) {
		if n := len(args); n > 1 {
			sql = strings.Replace(sql, "$"+strconv.Itoa(n), "$"+strconv.Itoa(n-1), 1)
		}
		return sql, args
	}),
	// Limit and offset are bound as int rather than int64.
	"types": wrap(func(sql string, args []any) (string, []any) {
		out := make([]any, len(args))
		for i, a := range args {
			if n, ok := a.(int64); ok {
				a = int(n)
			}
			out[i] = a
		}
		return sql, out
	}),
	// Numeric arguments lose their cast.
	"casts": wrap(func(sql string, args []any) (string, []any) {
		return strings.ReplaceAll(sql, "::numeric", ""), args
	}),
	// A trailing semicolon.
	"semicolon": wrap(func(sql string, args []any) (string, []any) { return sql + ";", args }),
}

type wrapped struct {
	edit func(string, []any) (string, []any)
}

func wrap(edit func(string, []any) (string, []any)) easylist.Driver { return wrapped{edit} }

func (w wrapped) Compile(p *easylist.Plan) (string, []any, error) {
	sql, args, err := refdriver.Driver{}.Compile(p)
	if err != nil {
		return "", nil, err
	}
	sql, args = w.edit(sql, args)
	return sql, args, nil
}

// TestBrokenDriver runs the suite with one broken driver. It runs only as
// the child process of TestSuiteFailsBrokenDrivers.
func TestBrokenDriver(t *testing.T) {
	name := os.Getenv(brokenEnv)
	if name == "" {
		t.Skip("run by TestSuiteFailsBrokenDrivers")
	}
	conformance.Run(t, brokenDrivers[name])
}

// The suite fails each broken driver, offline, and names the failed case.
func TestSuiteFailsBrokenDrivers(t *testing.T) {
	cases := map[string][]string{
		"placeholders": {"--- FAIL: TestBrokenDriver/placeholder_order/offline", "page query does not use placeholder $4 of 4 arguments"},
		"types":        {"--- FAIL: TestBrokenDriver/offset_zero_is_bound/offline", "[int(2), int(0)]"},
		"casts":        {"--- FAIL: TestBrokenDriver/numeric_literal_keeps_exact_digits/offline", "page query does not cast placeholder $1 to numeric"},
		"semicolon":    {"--- FAIL: TestBrokenDriver/page_without_filter/offline", "page query ends with a semicolon"},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestBrokenDriver$", "-test.v")
			cmd.Env = append(os.Environ(), brokenEnv+"="+name, conformance.DSNEnv+"=")
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("the suite passes the broken driver:\n%s", out)
			}
			for _, w := range want {
				if !strings.Contains(string(out), w) {
					t.Errorf("output does not contain %q:\n%s", w, out)
				}
			}
		})
	}
}
