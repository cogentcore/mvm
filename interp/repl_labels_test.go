package interp_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cogentcore/mvm/interp"
	"github.com/cogentcore/mvm/lang/golang"
)

// TestReplLoopLabels checks loops in successive Evals, whose labels have
// the same names, such as for0#e: a jump to the end of a loop went to that
// of the loop of an earlier Eval, into its code.
func TestReplLoopLabels(t *testing.T) {
	i := interp.NewInterpreter(golang.GoSpec)
	var out bytes.Buffer
	i.SetIO(strings.NewReader(""), &out, &out)
	tests := []struct {
		src  string
		want int64 // k after src
	}{
		{"k := 0", 0},
		{"for i := range 3 { k += i }", 3},
		{"for j := 0; j < 3; j++ { k++ }", 6},
		{"for k < 10 { k += 2 }", 10},
		{"for _, v := range []int{1, 2} { k += v }", 13},
		{"for i := 0; i < 2; i++ { if i == 1 { break }; k += 10 }", 23},
	}
	for _, test := range tests {
		if _, err := i.Eval("test", test.src); err != nil {
			t.Fatalf("%q: %v", test.src, err)
		}
		v, err := i.Eval("test", "k")
		if err != nil || v.Int() != test.want {
			t.Errorf("%q: k = %v (%v), want %d", test.src, v, err, test.want)
		}
	}
}
