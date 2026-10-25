package interp_test

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/cogentcore/mvm/interp"
	"github.com/cogentcore/mvm/lang/golang"
	"github.com/cogentcore/mvm/stdlib"
)

// npV is a host variable of a pointer type, which is nil when its package
// is imported, and set later.
var npV *ieT

// TestNilPointerVar checks that a host variable of a pointer type that is
// nil when its package is imported is a variable, not a type: types are
// bound as nil pointers, (*T)(nil), and it was taken for its element type.
func TestNilPointerVar(t *testing.T) {
	for _, src := range [][2]string{
		{`import . "np"`, `fmt.Printf("%T %d\n", V, Get(V))`},
		{`import "np"`, `fmt.Printf("%T %d\n", np.V, np.Get(np.V))`},
	} {
		npV = nil
		i := interp.NewInterpreter(golang.GoSpec)
		var out bytes.Buffer
		i.SetIO(strings.NewReader(""), &out, &out)
		i.ImportPackageValues(stdlib.Values)
		i.ImportPackageValues(map[string]map[string]reflect.Value{"np": {
			"V":   reflect.ValueOf(&npV),
			"Get": reflect.ValueOf(func(t *ieT) int { return t.n }),
		}})
		i.AutoImportPackages()
		if _, err := i.Eval("test", src[0]); err != nil {
			t.Fatal(err)
		}
		npV = &ieT{n: 4}
		if _, err := i.Eval("test", src[1]); err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		if got, want := out.String(), "*interp_test.ieT 4\n"; got != want {
			t.Errorf("%q: output %q, want %q", src, got, want)
		}
	}
}
