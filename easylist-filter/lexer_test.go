package filter

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestLexFilter(t *testing.T) {
	got, err := lexFilter(`"a.b" CONTAINS 'it''s %_\' AND n >= -12.5`)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 7 || got[0] != (token{kind: tokIdent, text: "a.b", pos: 1, quoted: true}) ||
		got[2].text != "it's %_\\" || got[2].kind != tokString ||
		got[5].kind != tokGte || got[6].text != "-12.5" {
		t.Fatalf("wrong tokens: %#v", got)
	}
	bounds, err := lexFilter(`date BETWEEN '2026-01-01,2026-12-31'`)
	if err != nil || len(bounds) != 3 || bounds[2].text != "2026-01-01,2026-12-31" {
		t.Fatalf("wrong BETWEEN tokens: %#v, %v", bounds, err)
	}
}

// These fixtures catch lost positions, quote decoding, keyword case sensitivity,
// operator splitting, and incorrect identifier normalization.
func TestLexFilterTokens(t *testing.T) {
	tests := []struct {
		name, input string
		want        []token
	}{
		{"empty", " \t\n\r\f\v", nil},
		{"keywords and comparisons", `(A=TRUE or b<+2) aNd c<=-3 AND d>4 AND e>=5 AND f EXCLUDES false`, []token{
			{kind: tokLParen, text: "(", pos: 1}, {kind: tokIdent, text: "a", pos: 2}, {kind: tokEq, text: "=", pos: 3}, {kind: tokBool, text: "TRUE", pos: 4},
			{kind: tokOr, text: "or", pos: 9}, {kind: tokIdent, text: "b", pos: 12}, {kind: tokLt, text: "<", pos: 13}, {kind: tokNumber, text: "+2", pos: 14}, {kind: tokRParen, text: ")", pos: 16},
			{kind: tokAnd, text: "aNd", pos: 18}, {kind: tokIdent, text: "c", pos: 22}, {kind: tokLte, text: "<=", pos: 23}, {kind: tokNumber, text: "-3", pos: 25},
			{kind: tokAnd, text: "AND", pos: 28}, {kind: tokIdent, text: "d", pos: 32}, {kind: tokGt, text: ">", pos: 33}, {kind: tokNumber, text: "4", pos: 34},
			{kind: tokAnd, text: "AND", pos: 36}, {kind: tokIdent, text: "e", pos: 40}, {kind: tokGte, text: ">=", pos: 41}, {kind: tokNumber, text: "5", pos: 43},
			{kind: tokAnd, text: "AND", pos: 45}, {kind: tokIdent, text: "f", pos: 49}, {kind: tokExcludes, text: "EXCLUDES", pos: 51}, {kind: tokBool, text: "false", pos: 60},
		}},
		{"quotes and byte positions", `"é" = 'a''b' AND "a""b" = ''`, []token{
			{kind: tokIdent, text: "é", pos: 1, quoted: true}, {kind: tokEq, text: "=", pos: 6}, {kind: tokString, text: "a'b", pos: 8},
			{kind: tokAnd, text: "AND", pos: 15}, {kind: tokIdent, text: `a"b`, pos: 19, quoted: true}, {kind: tokEq, text: "=", pos: 26}, {kind: tokString, text: "", pos: 28},
		}},
		{"identifier boundaries", `anderson _id2 cost$1 "AND" NaMe ÉTÉ`, []token{
			{kind: tokIdent, text: "anderson", pos: 1}, {kind: tokIdent, text: "_id2", pos: 10}, {kind: tokIdent, text: "cost$1", pos: 15}, {kind: tokIdent, text: "AND", pos: 22, quoted: true}, {kind: tokIdent, text: "name", pos: 28}, {kind: tokIdent, text: "ÉtÉ", pos: 33},
		}},
		{"decimals", `0 -0 +12 12.5 -0.25 +3.14`, []token{
			{kind: tokNumber, text: "0", pos: 1}, {kind: tokNumber, text: "-0", pos: 3}, {kind: tokNumber, text: "+12", pos: 6},
			{kind: tokNumber, text: "12.5", pos: 10}, {kind: tokNumber, text: "-0.25", pos: 15}, {kind: tokNumber, text: "+3.14", pos: 21},
		}},
		{"not equal", `a!=1 AND b != 'x' AND c!=true`, []token{
			{kind: tokIdent, text: "a", pos: 1}, {kind: tokNeq, text: "!=", pos: 2}, {kind: tokNumber, text: "1", pos: 4},
			{kind: tokAnd, text: "AND", pos: 6}, {kind: tokIdent, text: "b", pos: 10}, {kind: tokNeq, text: "!=", pos: 12}, {kind: tokString, text: "x", pos: 15},
			{kind: tokAnd, text: "AND", pos: 19}, {kind: tokIdent, text: "c", pos: 23}, {kind: tokNeq, text: "!=", pos: 24}, {kind: tokBool, text: "true", pos: 26},
		}},
		{"mixed keywords", `contains ExClUdEs between true FaLsE`, []token{
			{kind: tokContains, text: "contains", pos: 1}, {kind: tokExcludes, text: "ExClUdEs", pos: 10}, {kind: tokBetween, text: "between", pos: 19}, {kind: tokBool, text: "true", pos: 27}, {kind: tokBool, text: "FaLsE", pos: 32},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := lexFilter(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("tokens = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestLexFilterRejectsMalformedInput(t *testing.T) {
	for _, input := range []string{`name = 'open`, `"open`, `n = 12.3.4`, `n = @`} {
		if _, err := lexFilter(input); err == nil || !strings.Contains(err.Error(), "position") {
			t.Errorf("expected positioned error for %q, got %v", input, err)
		}
	}
}

func TestLexFilterErrorPositions(t *testing.T) {
	tests := []struct {
		input string
		pos   int
	}{
		{`name = 'open`, 8}, {`"open`, 1}, {`n = 12.3.4`, 9}, {`n = @`, 5},
		{`n = 1e3`, 6}, {`n = -`, 5}, {`n = +`, 5}, {`n = 1.`, 7},
		{`n = 12abc`, 7}, {`n = 1,2`, 6}, {`n ! 2`, 3}, {`n = [1]`, 5},
		{`n !`, 3}, {`n ! = 2`, 3}, {`n !x`, 3}, {`n !! 2`, 3},
		{`a.b = 2`, 2}, {`"é" = @`, 8}, {"n = \x00", 5},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := lexFilter(tt.input)
			if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("position %d", tt.pos)) {
				t.Fatalf("tokens %#v, error %v; want position %d", got, err, tt.pos)
			}
		})
	}
}
