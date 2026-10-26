package interp_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cogentcore/mvm/interp"
	"github.com/cogentcore/mvm/lang/golang"
)

// TestDiagnose checks the source positions and messages of the errors of
// compiling and running code.
func TestDiagnose(t *testing.T) {
	tests := []struct {
		src       string
		line, col int
		msg       string
		panic     bool
	}{
		{"x := 1\ny := x + undefinedName", 2, 8, "undefined: undefinedName", false},
		{"a := 10\nb := 12.5\nc := a + b", 3, 8, "invalid operation: mismatched types int and float64", false},
		{"s := []int{1, 2}\ni := 5\nx := s[i]", 3, 7, "panic: runtime error: index out of range", true},
		{"x := 1\ny := (x", 2, 6, "", false},
		{"x := 1\ny := x +", 2, 8, "missing operand after +", false},
		{"x := 1\nx(2)", 2, 2, "invalid operation: cannot call non-function x (type int)", false},
		{"var x any = \"a\"\nn := x.(int)", 2, 7, "panic: interface conversion: interface {} is string, not int", true},
	}
	for _, test := range tests {
		i := interp.NewInterpreter(golang.GoSpec)
		var out bytes.Buffer
		i.SetIO(strings.NewReader(""), &out, &out)
		_, err := i.Eval("test", test.src)
		if err == nil {
			t.Errorf("%q: no error", test.src)
			continue
		}
		d, ok := i.Diagnose(err)
		t.Logf("%q: %+v", test.src, d)
		if !ok || d.Source != "test" || d.Line != test.line || !strings.HasPrefix(d.Msg, test.msg) || d.Msg == "" || d.Panic != test.panic {
			t.Errorf("%q: %+v, %v; want line %d: %q (panic %v)\nerror: %v", test.src, d, ok, test.line, test.msg, test.panic, err)
		}
		if d.Col != test.col {
			t.Errorf("%q: col %d, want %d", test.src, d.Col, test.col)
		}
	}
}
