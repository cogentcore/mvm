//go:build goscript

package mtype

import (
	"reflect"

	"github.com/mvm-sh/mvm/internal/gsreflect"
)

// patchRtype gives dst the struct layout of src in place, keeping its name.
func patchRtype(dst, src reflect.Type) { gsreflect.SetFields(dst, src) }

// typeIDs numbers the types in the keys of the type caches, which have no
// address under goscript. JavaScript runs one goroutine at a time, and
// typeKeyID does not yield, so it needs no lock.
var typeIDs = map[*Type]uint64{}

// typeKeyID returns the number that stands for p in the keys of the type
// caches.
func typeKeyID(p *Type) uint64 {
	if p == nil {
		return 0
	}
	id, ok := typeIDs[p]
	if !ok {
		id = uint64(len(typeIDs) + 1)
		typeIDs[p] = id
	}
	return id
}
