package interp_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mvm-sh/mvm/interp"
	"github.com/mvm-sh/mvm/lang/golang"
	"github.com/mvm-sh/mvm/stdlib"
	_ "github.com/mvm-sh/mvm/stdlib/core"
)

// caT is a host type with a method of one parameter.
type caT struct{ v float32 }

func (t *caT) Set(v float32) { t.v = v }

// TestCallArgCount checks that a call with the wrong number of arguments is
// a compile error, as in Go, for interpreted and host funcs and methods.
func TestCallArgCount(t *testing.T) {
	tests := []struct{ src, err string }{
		{"func f(x int) {}\nfunc main() { f() }", "not enough arguments in call to f"},
		{"func f(x int) {}\nfunc main() { f(1, 2) }", "too many arguments in call to f"},
		{"func main() { f := func(x int) {}; f() }", "not enough arguments in call to f"},
		{"import \"strings\"\nfunc main() { _ = strings.Repeat(\"a\") }", "not enough arguments in call to"},
		{"import \"ca\"\nfunc main() { var x ca.T; x.Set() }", "not enough arguments in call to"},
		{"import \"ca\"\nfunc main() { var x ca.T; x.Set(1, 2) }", "too many arguments in call to"},
		{"func f(x int, y ...int) {}\nfunc main() { f() }", "not enough arguments in call to f"},
		{"func f(x int) {}\nfunc main() { f([]int{1}...) }", "in call to non-variadic f"},
		{"func f(x int, y ...int) {}\nfunc main() { f(1); f(1, 2, 3); f(1, []int{2}...) }", ""},
		{"func g() (int, int) { return 1, 2 }\nfunc f(x, y int) {}\nfunc main() { f(g()) }", ""},
		{"import \"fmt\"\nfunc main() { fmt.Println(); fmt.Println(1, 2) }", ""},
	}
	for _, test := range tests {
		i := interp.NewInterpreter(golang.GoSpec)
		i.ImportPackageValues(stdlib.Values)
		i.ImportPackageValues(map[string]map[string]reflect.Value{"ca": {"T": reflect.ValueOf((*caT)(nil))}})
		_, err := i.Eval("test", test.src)
		switch {
		case test.err == "" && err != nil:
			t.Errorf("%q: unexpected error %v", test.src, err)
		case test.err != "" && (err == nil || !strings.Contains(err.Error(), test.err)):
			t.Errorf("%q: got error %v, want %q", test.src, err, test.err)
		}
	}
}
