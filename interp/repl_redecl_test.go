package interp_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mvm-sh/mvm/interp"
	"github.com/mvm-sh/mvm/lang/golang"
	"github.com/mvm-sh/mvm/stdlib"
)

// TestReplRedeclare checks that a package var declared again in a later
// Eval, as in a REPL, with another type, is a new var of that type: it
// reused the var of the earlier Eval, of the earlier type. A func of the
// earlier Eval keeps the earlier var.
func TestReplRedeclare(t *testing.T) {
	i := interp.NewInterpreter(golang.GoSpec)
	var out bytes.Buffer
	i.SetIO(strings.NewReader(""), &out, &out)
	i.ImportPackageValues(stdlib.Values)
	i.AutoImportPackages()
	for _, src := range []string{
		`x := 1`,
		`f := func() int { return x }`,
		`x := "a"`,
		`fmt.Println(x, f())`,
		`var y = 2.5`,
		`var y = "b"`,
		`var z int`,
		`var z string`,
		`fmt.Printf("%T %T\n", y, z)`,
	} {
		if _, err := i.Eval("test", src); err != nil {
			t.Fatalf("%s: %v", src, err)
		}
	}
	if got, want := out.String(), "a 1\nstring string\n"; got != want {
		t.Errorf("output %q, want %q", got, want)
	}
}
