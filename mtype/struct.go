package mtype

import (
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
)

var (
	// Unique placeholder field names keep reflect.StructOf from returning a
	// cached (shared) rtype.
	placeholderSeq atomic.Uint64

	intRtype = reflect.TypeFor[int]()
)

// NewStructType creates a forward-declared struct placeholder named name (empty
// for anonymous). Register it, then call SetFields to finalize. The placeholder
// is symbolic (Rtype nil); comp materializes the rtype once the fields are set.
func NewStructType(name string) *Type {
	return &Type{Name: name, kind: reflect.Struct, Placeholder: true}
}

// NewPlaceholderRtype builds a fresh, uniquely-shaped struct rtype used to break
// pointer cycles while materializing a named struct: install it as the type's
// Rtype before materializing fields (so a *T built mid-recursion resolves), then
// PatchRtype it in place with the real layout.
func NewPlaceholderRtype(name string) reflect.Type {
	n := placeholderSeq.Add(1)
	sf := []reflect.StructField{{Name: placeholderFieldName(name, n), Type: intRtype}}
	return reflect.StructOf(sf)
}

// PatchRtype overwrites dst's struct-rtype bytes with src's, preserving dst's
// identity (so derived/pointer rtypes captured against dst stay valid).
func PatchRtype(dst, src reflect.Type) { patchRtype(dst, src) }

// placeholderFieldName builds a unique exported identifier for the placeholder's
// sole field; the leading "P" guarantees export regardless of name's first rune.
func placeholderFieldName(name string, n uint64) string {
	var b strings.Builder
	b.WriteByte('P')
	for _, r := range name {
		if r == '_' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
			b.WriteRune(r)
		}
	}
	b.WriteByte('_')
	b.WriteString(strconv.FormatUint(n, 10))
	return b.String()
}

// SetFields finalizes a forward-declared struct from src, patching the rtype in
// place so derived types (e.g. PointerTo) see the real layout.
func (t *Type) SetFields(src *Type) {
	if t.Rtype == nil {
		// Symbolic placeholder (post-flip): adopt src's symbolic shape; the rtype
		// is materialized later at comp. Identity (*Type pointer) is preserved so
		// forward references stay bound.
		t.kind = reflect.Struct
		t.Fields = src.Fields
		t.Embedded = src.Embedded
		t.Tags = src.Tags
		t.Placeholder = false
		return
	}
	if t.Rtype.Kind() != reflect.Struct || !t.Placeholder {
		// Shared read-only rtype (e.g. a bare-name collision): patchRtype would
		// memcpy onto read-only memory, so adopt src's layout by reference.
		t.Rtype = src.Rtype
		t.Fields = src.Fields
		t.Embedded = src.Embedded
		t.Placeholder = false
		return
	}
	patchRtype(t.Rtype, src.Rtype)
	t.Fields = src.Fields
	t.Embedded = src.Embedded
	t.Placeholder = false
}
