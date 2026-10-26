package interp_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cogentcore/mvm/interp"
	"github.com/cogentcore/mvm/lang/golang"
)

// TestReplResult checks the value that Eval returns, which a REPL prints:
// that of an expression, a definition or an assignment, and none for a
// block-like statement, such as if or for, or another statement that has
// no value, such as go: a for loop leaves values of its own on the stack,
// such as its last index, and the last global was returned otherwise.
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
		{"if k > 1 { k = 7 }", false},
		{"if k > 100 { k = 7 } else { k + 1 }", false},
		{"switch { case k > 1: k = 8 }", false},
		{"switch k { case 8: k + 1 }", false},
		{"var a any = 1\nswitch v := a.(type) { case int: v }", false},
		{"{ k = 9 }", false},
		{"go func() {}()", false},
		{"if k > 1 { k = 7 }\nk", true},
		{"k = 9", true},
		{"k += 1", true},
		{"k + 1", true},
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
	if v, _ := i.Eval("test", "k + int(x)"); !v.IsValid() || v.Int() != 10+6 {
		t.Errorf("k + int(x): %v, want 16", v)
	}
}
