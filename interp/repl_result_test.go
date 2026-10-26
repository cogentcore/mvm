package interp_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mvm-sh/mvm/interp"
	"github.com/mvm-sh/mvm/lang/golang"
)

// TestReplResult checks the value that Eval returns, which a REPL prints:
// that of an expression, and none for a for loop, which leaves values of
// its own on the stack, such as its last index.
func TestReplResult(t *testing.T) {
	i := interp.NewInterpreter(golang.GoSpec)
	var out bytes.Buffer
	i.SetIO(strings.NewReader(""), &out, &out)
	tests := []struct {
		src   string
		value bool // whether Eval returns a value
	}{
		{"x := 0.5", true},
		{"x * 2", true},
		{"for i := range 3 { x += 1 }", false},
		{"k := 0", true},
		{"for i := range 3 { k += i }", false},
		{"for i := 0; i < 3; i++ {\n k++\n}", false},
		{"for _, v := range []float64{1, 2} { x += v }", false},
		{"outer: for i := range 2 { if i > 0 { break outer } }", false},
		{"k = 9", true},
		{"k", true},
		{"y := 3\nfor i := range 2 { y += i }", false},
		{"for i := range 2 { y += i }\ny", true},
	}
	for _, test := range tests {
		v, err := i.Eval("test", test.src)
		if err != nil {
			t.Fatalf("%q: %v", test.src, err)
		}
		if v.IsValid() != test.value {
			t.Errorf("%q: value %v, want a value: %v", test.src, v, test.value)
		}
	}
	if v, _ := i.Eval("test", "k + int(x)"); !v.IsValid() || v.Int() != 9+6 {
		t.Errorf("k + int(x): %v, want 15", v)
	}
}
