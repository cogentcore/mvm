package interp_test

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/mvm-sh/mvm/interp"
	"github.com/mvm-sh/mvm/lang/golang"
	"github.com/mvm-sh/mvm/stdlib"
)

// TestVariadicSliceElem checks calls of a host func whose variadic
// parameter has elements of a slice type, which are passed as a slice of
// slices: it was converted to the element type.
func TestVariadicSliceElem(t *testing.T) {
	tests := []struct{ src, out string }{
		{`fmt.Println(vs.F(1))`, "0\n"},
		{`fmt.Println(vs.F(1, []int{1}, []int{2, 3}))`, "2\n"},
		{`fmt.Println(vs.F(1, [][]int{{1}}...))`, "1\n"},
		{`func main() { defer vs.F(1, []int{1}) }`, ""},
	}
	for _, test := range tests {
		i := interp.NewInterpreter(golang.GoSpec)
		var out bytes.Buffer
		i.SetIO(strings.NewReader(""), &out, &out)
		i.ImportPackageValues(stdlib.Values)
		i.ImportPackageValues(map[string]map[string]reflect.Value{"vs": {
			"F": reflect.ValueOf(func(x int, s ...[]int) int { return len(s) }),
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
