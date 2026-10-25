package interp_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/cogentcore/mvm/interp"
	"github.com/cogentcore/mvm/lang/golang"
	"github.com/cogentcore/mvm/stdlib"
	_ "github.com/cogentcore/mvm/stdlib/core"
)

// suHolder is a host type, whose memory the deep unbox walk of native call
// arguments does not enter, so what interpreted code stores into it is what
// native code sees.
type suHolder struct{ S []any }

// suKinds returns the dynamic types of the elements of h.S.
func suKinds(h *suHolder) string {
	var b strings.Builder
	for _, v := range h.S {
		b.WriteString(reflect.TypeOf(v).String() + " ")
	}
	return b.String()
}

// TestStoreUnboxes checks that interface values that interpreted code
// stores into Go memory, by composite literal, append and assignment, are
// real Go values, not the interpreter's boxes.
func TestStoreUnboxes(t *testing.T) {
	src := `import "su"
type Pair[K comparable, V any] struct { K K; V V }
func main() {}
func run() string {
	var a string
	h := &su.Holder{}
	h.S = []any{&a, 1}
	h.S = append(h.S, map[string]int{"x": 1}, Pair[string, int]{"k", 1})
	h.S[1] = &a
	return su.Kinds(h)
}`
	i := interp.NewInterpreter(golang.GoSpec)
	i.ImportPackageValues(stdlib.Values)
	i.ImportPackageValues(map[string]map[string]reflect.Value{"su": {
		"Holder": reflect.ValueOf((*suHolder)(nil)),
		"Kinds":  reflect.ValueOf(suKinds),
	}})
	if _, err := i.Eval("test", src); err != nil {
		t.Fatal(err)
	}
	r, err := i.Eval("test", "run()")
	if err != nil {
		t.Fatal(err)
	}
	got := r.String()
	if strings.Contains(got, "vm.Iface") {
		t.Errorf("boxes stored: %s", got)
	}
	if want := "*string *string map[string]int "; !strings.HasPrefix(got, want) {
		t.Errorf("got %q, want prefix %q", got, want)
	}
}
