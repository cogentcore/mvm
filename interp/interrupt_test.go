package interp_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cogentcore/mvm/interp"
	"github.com/cogentcore/mvm/lang/golang"
	"github.com/cogentcore/mvm/vm"
)

// TestInterrupt checks that Interrupt stops running code, as Ctrl-C does
// in a REPL: loops without calls, loops with calls, and recursion, with an
// error at the position where it stopped.
func TestInterrupt(t *testing.T) {
	i := interp.NewInterpreter(golang.GoSpec)
	var out bytes.Buffer
	i.SetIO(strings.NewReader(""), &out, &out)
	for _, src := range []string{
		"for i := range 10000000000 { }",
		"n := 0\nfor i := 0; i < 10000000000; i++ { n += i }",
		"func f(x int) int { return x + 1 }\nfor i := range 10000000000 { f(i) }",
		"func r(x int) int { if x == 0 { return 0 }; return r(x-1) + r(x-1) }\nr(100)",
	} {
		timer := time.AfterFunc(50*time.Millisecond, i.Interrupt)
		start := time.Now()
		_, err := i.Eval("test", src)
		timer.Stop()
		if !errors.Is(err, vm.ErrInterrupted) {
			t.Fatalf("%q: error %v, want %v", src, err, vm.ErrInterrupted)
		}
		if d, ok := i.Diagnose(err); !ok || d.Line == 0 {
			t.Errorf("%q: no position: %+v", src, d)
		}
		if el := time.Since(start); el > 2*time.Second {
			t.Errorf("%q: interrupted after %v", src, el)
		}
	}
	// the interpreter still works after an interrupt
	if v, err := i.Eval("test", "1 + 2"); err != nil || v.Int() != 3 {
		t.Errorf("after interrupts: %v, %v", v, err)
	}
	// an interrupt when no code is running does not stop the next code
	i.Interrupt()
	if v, err := i.Eval("test", "n := 0\nfor i := range 1000 { n += i }\nn"); err != nil || v.Int() != 499500 {
		t.Errorf("after an interrupt without code: %v, %v", v, err)
	}
}
