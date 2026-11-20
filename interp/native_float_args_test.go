package interp_test

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/mvm-sh/mvm/interp"
	"github.com/mvm-sh/mvm/lang/golang"
	"github.com/mvm-sh/mvm/stdlib"
)

// TestNativeFloatArgs checks calls of host funcs with float parameters,
// variadic or not, passing untyped integer constants: they were passed as
// the bits of the integer instead of the converted float value.
func TestNativeFloatArgs(t *testing.T) {
	tests := []struct{ src, out string }{
		{`fmt.Println(nf.F64(1, 2, 3))`, "[1 2 3]\n"},
		{`fmt.Println(nf.F64(1.0, 2, 0.5))`, "[1 2 0.5]\n"},
		{`fmt.Println(nf.F64(0.1, 0.9))`, "[0.1 0.9]\n"},
		{`fmt.Println(nf.F64())`, "[]\n"},
		{`fmt.Println(nf.F32(1, 2.5, 3))`, "[1 2.5 3]\n"},
		{`fmt.Println(nf.One(6))`, "6\n"},
		{`fmt.Println(nf.Two(6, 7))`, "6 7\n"},
		{`fmt.Println(nf.Mixed(2, 3, 4))`, "2 [3 4]\n"},
		{`const c = 5; fmt.Println(nf.F64(c, c+1))`, "[5 6]\n"},
		{`x := 2; fmt.Println(nf.F64(float64(x), 3))`, "[2 3]\n"},
		{`fmt.Println(nf.C128(1, 2))`, "[(1+0i) (2+0i)]\n"},
		{`f := nf.F64; fmt.Println(f(1, 2))`, "[1 2]\n"},
		{`func main() { defer func() { fmt.Println(nf.F64(7, 8)) }() }`, "[7 8]\n"},
		{`func main() { defer fmt.Println(nf.Mixed(1, 2)) }`, "1 [2]\n"},
		{`fmt.Println(nf.Nil(nil, nil))`, "true true\n"},
		// Interpreted variadic funcs had the same problem.
		{`func f(v ...float64) []float64 { return v }; fmt.Println(f(1, 2.5, 3))`, "[1 2.5 3]\n"},
		{`func f(n int, v ...float32) []float32 { return v }; fmt.Println(f(1, 2, 3))`, "[2 3]\n"},
		{`func f(v ...any) []any { return v }; fmt.Println(f(1, 2.5))`, "[1 2.5]\n"},
		{`func f(v ...float64) []float64 { return v }; g := f; fmt.Println(g(4, 5))`, "[4 5]\n"},
		{`func f(v ...float64) { fmt.Println(v) }; func main() { defer f(6, 7) }`, "[6 7]\n"},
		{`func f(x float64, v ...float64) { fmt.Println(x, v) }; func main() { defer f(5, 6, 7) }`, "5 [6 7]\n"},
		{`func f(v ...float64) { fmt.Println(v) }; func main() { d := make(chan bool); go func() { f(8, 9); d <- true }(); <-d }`, "[8 9]\n"},
		{`func f(x float64) { fmt.Println(x); d <- true }; var d = make(chan bool, 2); func main() { defer f(5); go f(4); <-d }`, "4\n5\n"},
		{`func main() { defer nf.Mixed(1, 2); defer fmt.Println(nf.One(3)) }`, "3\n"},
		{`func f(v ...float64) { fmt.Println(v); d <- true }; var d = make(chan bool); func main() { go f(8, 9); <-d }`, "[8 9]\n"},
	}
	for _, test := range tests {
		i := interp.NewInterpreter(golang.GoSpec)
		var out bytes.Buffer
		i.SetIO(strings.NewReader(""), &out, &out)
		i.ImportPackageValues(stdlib.Values)
		i.ImportPackageValues(map[string]map[string]reflect.Value{"nf": {
			"F64":   reflect.ValueOf(func(v ...float64) []float64 { return v }),
			"F32":   reflect.ValueOf(func(v ...float32) []float32 { return v }),
			"C128":  reflect.ValueOf(func(v ...complex128) []complex128 { return v }),
			"One":   reflect.ValueOf(func(v float64) float64 { return v }),
			"Two":   reflect.ValueOf(func(a float32, b float64) string { return fmt.Sprint(a, b) }),
			"Nil":   reflect.ValueOf(func(v ...[]int) string { return fmt.Sprint(v[0] == nil, v[1] == nil) }),
			"Mixed": reflect.ValueOf(func(n int, v ...float64) string { return fmt.Sprint(n, v) }),
		}})
		i.AutoImportPackages()
		if _, err := i.Eval("test", test.src); err != nil {
			t.Errorf("%s: %v", test.src, err)
			continue
		}
		if got := out.String(); got != test.out {
			t.Errorf("%s: output %q, want %q", test.src, got, test.out)
		}
	}
}
