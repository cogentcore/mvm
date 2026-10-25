package interp_test

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/cogentcore/mvm/goparser"
	"github.com/cogentcore/mvm/interp"
	"github.com/cogentcore/mvm/lang/golang"
	"github.com/cogentcore/mvm/stdlib"
	_ "github.com/cogentcore/mvm/stdlib/core"
)

// ieT is a host type, of which ieV is a variable.
type ieT struct{ n int }

var ieV = &ieT{n: 3}

// TestImportOnlyEval checks that an Eval of an import alone, which adds data
// but no code, does not break the next Eval, which uses what it imported:
// the next Eval copied all the data to the VM again, as if it were the
// first. The import adds data as it installs the generic shim of the
// package, which imports slices, an interpreted package.
func TestImportOnlyEval(t *testing.T) {
	i := interp.NewInterpreter(golang.GoSpec)
	var out bytes.Buffer
	i.SetIO(strings.NewReader(""), &out, &out)
	i.ImportPackageValues(stdlib.Values)
	goparser.RegisterGenericShim("ie", "package ie\n\nimport \"slices\"\n\nfunc Has[E comparable](s []E, v E) bool { return slices.Contains(s, v) }\n", nil)
	i.ImportPackageValues(map[string]map[string]reflect.Value{"ie": {
		"V":   reflect.ValueOf(&ieV),
		"Get": reflect.ValueOf(func(t *ieT) int { return t.n }),
	}})
	i.AutoImportPackages()
	for _, src := range []string{`import . "ie"`, `x := Get(V)`, `import "strings"`, `println(x, strings.Repeat("a", x))`} {
		if _, err := i.Eval("test", src); err != nil {
			t.Fatalf("%q: %v", src, err)
		}
	}
	if got := out.String(); got != "3 aaa\n" {
		t.Errorf("output %q, want %q", got, "3 aaa\n")
	}
}
