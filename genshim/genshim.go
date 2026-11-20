// Package genshim is the registry of generic shims: interpreted source of
// the generic functions of a package whose other symbols are native
// bindings, which goparser installs when it imports the bindings of the
// package (see goparser.RegisterGenericShim). It depends only on the
// standard library, so that a package of bindings can register shims
// without linking the parser into the programs that import it only for its
// bindings.
package genshim

import (
	"slices"
	"sync"
)

// Shim is the source of the generic functions of a package.
type Shim struct {
	// Source is the entire shim source ("package <pkg>" + declarations).
	Source string

	// NativeRefs are the names of the native bindings of the package that
	// Source refers to bare.
	NativeRefs []string
}

var (
	mu       sync.Mutex
	registry = map[string][]Shim{}
)

// Register adds a shim of the package of import path pkg, of the given
// source, which refers to the given names of its native bindings. It is
// safe to call from package init; the registry persists for the process
// lifetime.
func Register(pkg, source string, nativeRefs []string) {
	mu.Lock()
	defer mu.Unlock()
	registry[pkg] = append(registry[pkg], Shim{Source: source, NativeRefs: nativeRefs})
}

// All returns a copy of the registry: the shims of each package, by import
// path.
func All() map[string][]Shim {
	mu.Lock()
	defer mu.Unlock()
	all := make(map[string][]Shim, len(registry))
	for pkg, shims := range registry {
		all[pkg] = slices.Clone(shims)
	}
	return all
}
