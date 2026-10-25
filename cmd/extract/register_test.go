package main

import (
	"os"
	"strings"
	"testing"
)

// TestRegister checks the file that -register writes for cmp: the values of
// its exported names, and its generic functions, with the constraint that
// they use, as a generic shim.
func TestRegister(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := runRegister("symbols", []string{"cmp"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile("gen_cmp.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	for _, want := range []string{
		"package symbols",
		"var _ = register(func() {",
		`goparser.RegisterGenericShim("cmp",`,
		"func Or[T comparable](vals ...T) T {",
		"type Ordered interface {",
		`add(map[string]map[string]reflect.Value{`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("gen_cmp.go does not contain %q:\n%s", want, src)
		}
	}
}
