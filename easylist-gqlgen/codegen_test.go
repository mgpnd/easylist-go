package easylistgql

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	easylist "github.com/mgpnd/easylist-go/easylist-core"
)

// calcArrays returns a non-null and a nullable calculated array of type typ
// whose elements are T.
func calcArrays[T any](typ easylist.Type, values []string) []easylist.Field {
	name := "calc_" + typ.String()
	return []easylist.Field{
		easylist.Calc(name, typ, func(context.Context, easylist.Row) ([]T, error) { return nil, nil }).WithValues(values...),
		easylist.CalcNullable(name+"_opt", typ, func(context.Context, easylist.Row) (*[]T, error) { return nil, nil }).WithValues(values...),
	}
}

// arrayFieldsList declares, for every type, a non-null and a nullable SQL
// array and a non-null and a nullable calculated array.
func arrayFieldsList() easylist.List {
	fields := []easylist.Field{{Name: "id", Type: easylist.Integer}}
	for _, typ := range []easylist.Type{
		easylist.Text, easylist.Integer, easylist.Float, easylist.Numeric, easylist.Boolean,
		easylist.Date, easylist.Timestamp, easylist.UUID, easylist.Enum,
	} {
		var values []string
		if typ == easylist.Enum {
			values = []string{"a", "b"}
		}
		name := typ.String()
		fields = append(fields,
			easylist.Field{Name: name + "_list", Type: typ, Array: true, Values: values},
			easylist.Field{Name: name + "_opt", Type: typ, Array: true, Nullable: true, Values: values},
		)
		switch typ {
		case easylist.Integer:
			fields = append(fields, calcArrays[int64](typ, values)...)
		case easylist.Float:
			fields = append(fields, calcArrays[float64](typ, values)...)
		case easylist.Boolean:
			fields = append(fields, calcArrays[bool](typ, values)...)
		case easylist.Date, easylist.Timestamp:
			fields = append(fields, calcArrays[time.Time](typ, values)...)
		default:
			fields = append(fields, calcArrays[string](typ, values)...)
		}
	}
	return easylist.List{Name: "arrays", Table: "t", Alias: "t", PrimaryKey: "id", Fields: fields}
}

// TestGeneratedArrayCode generates bindings for array fields of every type,
// SQL and calculated, nullable or not, and checks that they compile.
func TestGeneratedArrayCode(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go build")
	}
	reg, err := easylist.NewRegistry(arrayFieldsList())
	if err != nil {
		t.Fatal(err)
	}
	// go build needs an import path, so the packages live inside this module.
	dir, err := os.MkdirTemp(filepath.Join("internal", "fixture"), "arrays")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	pkg := selfImport + "/internal/fixture/" + filepath.Base(dir)
	src, err := generateGo("gen", pkg+"/defs", "Registry", reg.Lists())
	if err != nil {
		t.Fatal(err)
	}
	// Generated code only references the registry variable, so the defs
	// package need not hold the lists to compile.
	defs := "package defs\n\nimport easylist \"" + coreImport + "\"\n\nvar Registry *easylist.Registry\n"
	for path, content := range map[string]string{"defs/defs.go": defs, "gen/gen.go": string(src)} {
		path = filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := exec.Command("go", "build", "./"+filepath.ToSlash(dir)+"/...").CombinedOutput(); err != nil {
		t.Fatalf("generated code does not compile: %v\n%s\n%s", err, out, src)
	}

	// Struct fields are aligned, so compare with runs of spaces collapsed.
	code := regexp.MustCompile(` +`).ReplaceAllString(string(src), " ")
	for _, want := range []string{
		"\tTextList []string\n", "\tTextOpt []string\n", "\tIntegerList []int64\n", "\tFloatOpt []float64\n",
		"\tBooleanList []bool\n", "\tDateList []time.Time\n", "\tTimestampOpt []time.Time\n", "\tEnumList []ArraysEnumList\n",
		`if v, ok := r.Int64s("integer_list"); ok {`, `if v, ok := r.Float64s("float_opt"); ok {`,
		`if v, ok := r.Bools("boolean_list"); ok {`, `if v, ok := r.Times("timestamp_opt"); ok {`,
		`if v, ok := r.Strings("numeric_list"); ok {`, "\t\tout.UUIDOpt = v\n",
		"\t\tout.EnumOpt = make([]ArraysEnumOpt, len(v))\n\t\tfor i, s := range v {\n\t\t\tout.EnumOpt[i] = ArraysEnumOpt(s)\n\t\t}\n",
		"func (r *ArraysRow) CalcDate(ctx context.Context) ([]time.Time, error) {",
		"func (r *ArraysRow) CalcTextOpt(ctx context.Context) ([]string, error) {",
		"func (r *ArraysRow) CalcEnumOpt(ctx context.Context) ([]ArraysCalcEnumOpt, error) {",
		"\tin := v.([]string)\n\tvar out []ArraysCalcEnum\n\tout = make([]ArraysCalcEnum, len(in))\n",
		`NewEnum[ArraysSortField]("ArraysSortField", "id", "ID")` + "\n",
	} {
		if !strings.Contains(code, want) {
			t.Errorf("generated code lacks %q", want)
		}
	}
}
