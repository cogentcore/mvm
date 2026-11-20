package interp_test

import (
	"reflect"
	"testing"

	"github.com/mvm-sh/mvm/interp"
	"github.com/mvm-sh/mvm/lang/golang"
)

// vfmT has a method with a variadic func parameter, as such setters
// commonly have an optional event handler.
type vfmT struct{ calls int }

func (t *vfmT) Set(name string, onClick ...func(n int)) *vfmT {
	for _, f := range onClick {
		f(len(name))
	}
	t.calls++
	return t
}

// TestVariadicFuncMethod calls a native method that has a variadic func
// parameter, with and without func arguments: the arguments of a method
// call through its receiver were offset from the interpreted ones.
func TestVariadicFuncMethod(t *testing.T) {
	i := interp.NewInterpreter(golang.GoSpec)
	i.ImportPackageValues(map[string]map[string]reflect.Value{
		"vfm": {"T": reflect.ValueOf((*vfmT)(nil)), "New": reflect.ValueOf(func() *vfmT { return &vfmT{} })},
	})
	r, err := i.Eval("test", `import "vfm"
func main() {}
func run() int {
	n := 0
	x := vfm.New()
	x.Set("a")
	x.Set("abc", func(k int) { n += k })
	x.Set("de").Set("f", func(k int) { n += 10 * k })
	return n
}`)
	if err != nil {
		t.Fatal(err)
	}
	_ = r
	r, err = i.Eval("test", "run()")
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Int(); got != 13 {
		t.Errorf("got %d, want 13", got)
	}
}
