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
		`genshim.Register("cmp",`,
		"func Or[T comparable](vals ...T) T {",
		"type Ordered interface {",
		`add(map[string]map[string]reflect.Value{`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("gen_cmp.go does not contain %q:\n%s", want, src)
		}
	}
}

// TestRegisterGenericTypes checks that -register binds an alias of an
// instance of a generic type, and leaves out a generic function that refers
// to a generic type, which is not bound, and one that refers to it in turn.
func TestRegisterGenericTypes(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	files := map[string]string{
		"go.mod": "module example.com/gt\n\ngo 1.24\n",
		"gt.go": `package gt

type Box[T any] struct{ V T }

type IntBox = Box[int]

func NewBox[T any](v T) *Box[T] { return &Box[T]{v} }

func Wrap[T any](v T) any { return NewBox(v) }

func Id[T any](v T) T { return v }
`,
	}
	for name, src := range files {
		if err := os.WriteFile(name, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := runRegister("symbols", []string{"example.com/gt"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile("gen_example_com_gt.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	for _, want := range []string{`"IntBox":`, "func Id[T any](v T) T"} {
		if !strings.Contains(src, want) {
			t.Errorf("does not contain %q:\n%s", want, src)
		}
	}
	for _, not := range []string{`"Box":`, "func NewBox", "func Wrap"} {
		if strings.Contains(src, not) {
			t.Errorf("contains %q:\n%s", not, src)
		}
	}
}
