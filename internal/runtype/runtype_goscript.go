//go:build goscript

package runtype

// This file is the goscript backend of runtype. Under goscript a reflect.Type
// is a JavaScript object, not a runtime rtype, so the synthesized types are
// clones of goscript reflect types, named and patched in place through
// internal/gsreflect. Sizes and layouts are computed by goscript reflect on
// demand, so there is no layout or geometry to maintain.
//
// The methods of a named type are installed as JavaScript methods of its
// values, which call their MethodSpec.Handler, so native code dispatches them
// directly (see gsreflect.SetMethod).

import (
	"errors"
	"fmt"
	"reflect"
	"unsafe"

	"github.com/mvm-sh/mvm/internal/gsreflect"
)

// MethodSpec describes one method to install on a synthesized rtype.
// StubPC is unused under goscript.
type MethodSpec struct {
	Name     string
	Exported bool
	PkgPath  string // declaring package for an unexported method
	Sig      reflect.Type
	StubPC   uintptr

	// Handler, a MethodFunc, runs the method for native callers.
	Handler any
}

// MethodFunc runs an interpreted method with the receiver recv, a pointer
// to the value, and the arguments args, returning its results.
type MethodFunc = func(recv reflect.Value, args []reflect.Value) []reflect.Value

// Imethod is one interface method: name, export status, and no-receiver
// signature (e.g. func() bool for Timeout).
type Imethod struct {
	Name     string
	Exported bool
	Sig      reflect.Type
}

// ErrKindUnsupported is returned by ReserveMethods for layouts whose Kind is
// not in the synth catalog.
var ErrKindUnsupported = errors.New(
	"runtype: ReserveMethods: layout kind not supported")

var errNoMethods = errors.New("runtype: methods slice is empty")

// maxMethods caps the methods installable per synth attach call.
const maxMethods = 256

// MaxMethods is the per-synth-rtype method-table capacity.
const MaxMethods = maxMethods

// markSynth records t as a synthesized type and returns it. The types and
// the number of methods Fill records for each are kept on the types (see
// gsreflect.SetSynthMethods): a Go map keyed by reflect.Type hashes the type
// under goscript, which is slow for a lookup as frequent as IsSynth.
func markSynth(t reflect.Type) reflect.Type {
	if _, ok := gsreflect.SynthMethods(t); !ok {
		gsreflect.SetSynthMethods(t, 0)
	}
	return t
}

// Reservation is a synthesized named type whose methods are recorded by Fill.
type Reservation struct {
	rt reflect.Type
}

// Type returns the reserved type.
func (r *Reservation) Type() reflect.Type { return r.rt }

// Fill records the methods of the reserved type.
func (r *Reservation) Fill(methods []MethodSpec) error {
	if r == nil {
		return errors.New("runtype: Fill on nil Reservation")
	}
	switch {
	case len(methods) == 0:
		return errNoMethods
	case len(methods) > maxMethods:
		return fmt.Errorf("runtype: too many methods (%d > %d)", len(methods), maxMethods)
	}
	gsreflect.SetSynthMethods(r.rt, len(methods))

	// The methods of a named type, or of a pointer to one, are installed on
	// the named type (see gsreflect.SetMethod). The pointer reservation is
	// filled after the value one, so its methods that the value already has
	// keep their value receiver.
	st := r.rt
	ptrRecv := st.Kind() == reflect.Pointer
	if ptrRecv {
		st = st.Elem()
	}
	if st.Name() == "" {
		return nil
	}
	for _, m := range methods {
		if fn, ok := m.Handler.(MethodFunc); ok && m.Exported {
			gsreflect.SetMethod(st, m.Name, m.Sig, ptrRecv, fn)
		}
	}
	return nil
}

// isPrimitiveKind reports whether k is a basic kind.
func isPrimitiveKind(k reflect.Kind) bool {
	return k >= reflect.Bool && k <= reflect.Complex128 || k == reflect.String
}

// SupportedKind reports whether a layout of kind k can be given a reserved identity.
func SupportedKind(k reflect.Kind) bool {
	switch k {
	case reflect.Struct, reflect.Slice, reflect.Array, reflect.Map, reflect.Chan,
		reflect.Pointer, reflect.Func:
		return true
	}
	return isPrimitiveKind(k)
}

// ReserveMethods returns a new named type with the layout of layout.
func ReserveMethods(layout reflect.Type, name, pkgPath string) (*Reservation, error) {
	if !SupportedKind(layout.Kind()) {
		return nil, ErrKindUnsupported
	}
	rt := gsreflect.Clone(layout)
	gsreflect.SetName(rt, name, pkgPath)
	return &Reservation{rt: markSynth(rt)}, nil
}

// ReservePtrMethods returns the reservation of *elem.
func ReservePtrMethods(elem reflect.Type, name, pkgPath string) (*Reservation, error) {
	if elem == nil {
		return nil, errors.New("runtype: ReservePtrMethods: nil elem type")
	}
	return &Reservation{rt: markSynth(reflect.PointerTo(elem))}, nil
}

// SetElem sets the element type of the composite type t in place.
func SetElem(t, elem reflect.Type) { gsreflect.SetElem(t, elem) }

// ClearUncommon is a no-op: goscript types have no uncommon section.
func ClearUncommon(t reflect.Type) {}

// EmbedTripsStructOf reports false: synthesized types carry no native
// methods, so StructOf accepts them as embedded fields.
func EmbedTripsStructOf(t reflect.Type) bool { return false }

// CloneStructLayoutWithFields returns a new unnamed struct type with the fields
// of src, the listed fields getting the given types.
func CloneStructLayoutWithFields(src reflect.Type, fieldTypes map[int]reflect.Type) reflect.Type {
	rt := gsreflect.Clone(src)
	gsreflect.SetString(rt, src.String())
	for i, ft := range fieldTypes {
		if ft != nil && i >= 0 && i < rt.NumField() {
			gsreflect.SetFieldType(rt, i, ft)
		}
	}
	return markSynth(rt)
}

// SetFieldEmbedded marks field idx of the struct type rt as embedded.
func SetFieldEmbedded(rt reflect.Type, idx int, name, tag string, exported bool) {
	if idx < 0 || idx >= rt.NumField() {
		return
	}
	gsreflect.SetFieldEmbedded(rt, idx, name, tag, exported)
}

// HasMethods reports whether t declares any methods.
func HasMethods(t reflect.Type) bool {
	if n, ok := gsreflect.SynthMethods(t); ok {
		return n > 0
	}
	return t.NumMethod() > 0
}

// FillStructLayout gives the reserved struct type the fields of realLayout
// in place.
func FillStructLayout(reserved, realLayout reflect.Type) {
	gsreflect.SetFields(reserved, realLayout)
}

// StampName names t in place.
func StampName(t reflect.Type, name string) {
	if t != nil {
		gsreflect.SetName(t, name, "")
	}
}

// StampNamePkg names t in place, with an import path.
func StampNamePkg(t reflect.Type, name, pkgPath string) {
	if t != nil {
		gsreflect.SetName(t, name, pkgPath)
	}
}

// StampString sets the String text of t in place, leaving it unnamed.
func StampString(t reflect.Type, s string) {
	if t != nil {
		gsreflect.SetString(t, s)
	}
}

// IsSynth reports whether t is a synthesized type.
func IsSynth(t reflect.Type) bool {
	if t == nil {
		return false
	}
	_, ok := gsreflect.SynthMethods(t)
	return ok
}

// HasPtrToThis reports false: reflect.PointerTo always gives the *T of a
// synthesized type under goscript.
func HasPtrToThis(t reflect.Type) bool { return false }

// PointerTo returns *elem.
func PointerTo(elem reflect.Type) reflect.Type { return markSynth(reflect.PointerTo(elem)) }

// SliceOf returns []elem.
func SliceOf(elem reflect.Type) reflect.Type { return markSynth(reflect.SliceOf(elem)) }

// ChanOf returns the chan type of elem with the given direction.
func ChanOf(dir reflect.ChanDir, elem reflect.Type) reflect.Type {
	return markSynth(reflect.ChanOf(dir, elem))
}

// ArrayOf returns [n]elem.
func ArrayOf(n int, elem reflect.Type) reflect.Type { return markSynth(reflect.ArrayOf(n, elem)) }

// MapOf returns map[key]elem.
func MapOf(key, elem reflect.Type) reflect.Type { return markSynth(reflect.MapOf(key, elem)) }

// DerivePointerTo returns *elem, synthesized when elem is.
func DerivePointerTo(elem reflect.Type) reflect.Type {
	if IsSynth(elem) {
		return PointerTo(elem)
	}
	return reflect.PointerTo(elem)
}

// DeriveSliceOf returns []elem, synthesized when elem is.
func DeriveSliceOf(elem reflect.Type) reflect.Type {
	if IsSynth(elem) {
		return SliceOf(elem)
	}
	return reflect.SliceOf(elem)
}

// DeriveArrayOf returns [n]elem, synthesized when elem is.
func DeriveArrayOf(n int, elem reflect.Type) reflect.Type {
	if IsSynth(elem) {
		return ArrayOf(n, elem)
	}
	return reflect.ArrayOf(n, elem)
}

// DeriveChanOf returns the chan type of elem, synthesized when elem is.
func DeriveChanOf(dir reflect.ChanDir, elem reflect.Type) reflect.Type {
	if IsSynth(elem) {
		return ChanOf(dir, elem)
	}
	return reflect.ChanOf(dir, elem)
}

// DeriveMapOf returns map[key]elem, synthesized when key or elem is.
func DeriveMapOf(key, elem reflect.Type) reflect.Type {
	if IsSynth(key) || IsSynth(elem) {
		return MapOf(key, elem)
	}
	return reflect.MapOf(key, elem)
}

// RebuildMapGeometry is a no-op: goscript maps have no geometry.
func RebuildMapGeometry(mapRT reflect.Type) {}

// RebuildArrayGeometry reports false: goscript computes array sizes from
// the element type on demand.
func RebuildArrayGeometry(arrRT reflect.Type) bool { return false }

// InterfaceOf returns a named interface type. Its method set is recorded
// only: under goscript it behaves as any for native code.
func InterfaceOf(name, pkgPath string, methods []Imethod) reflect.Type {
	if len(methods) == 0 {
		return reflect.TypeFor[any]()
	}
	h := ReserveInterface(name, pkgPath)
	h.FillMethods(methods)
	return h.Type()
}

// IfaceHandle is a reserved interface type.
type IfaceHandle struct{ t reflect.Type }

// Type returns the reserved interface type.
func (h *IfaceHandle) Type() reflect.Type { return h.t }

// ReserveInterface returns a new named interface type.
func ReserveInterface(name, pkgPath string) *IfaceHandle {
	t := gsreflect.Clone(reflect.TypeFor[any]())
	gsreflect.SetName(t, name, pkgPath)
	return &IfaceHandle{t: markSynth(t)}
}

// FillMethods records the method set of the reserved interface.
func (h *IfaceHandle) FillMethods(methods []Imethod) {}

// FuncPC returns 0: goscript functions have no entry PC.
func FuncPC(fn any) uintptr { return 0 }

// PointerFromUintptr returns nil: goscript pointers are not addresses.
func PointerFromUintptr(p uintptr) unsafe.Pointer { return nil }

// Exportable returns rv: goscript reflect has no read-only flag.
func Exportable(rv reflect.Value) reflect.Value { return rv }

// TypeMethodByName is reflect.Type.MethodByName.
func TypeMethodByName(t reflect.Type, name string) (reflect.Method, bool) {
	return t.MethodByName(name)
}

// TypeHasMethodByName reports whether t has an exported method named name.
func TypeHasMethodByName(t reflect.Type, name string) bool {
	_, ok := t.MethodByName(name)
	return ok
}

// TypeMethodNames lists t's exported method names.
func TypeMethodNames(t reflect.Type) []string {
	names := make([]string, t.NumMethod())
	for i := range names {
		names[i] = t.Method(i).Name
	}
	return names
}

// TypeMethods lists t's exported methods.
func TypeMethods(t reflect.Type) []reflect.Method {
	out := make([]reflect.Method, t.NumMethod())
	for i := range out {
		out[i] = t.Method(i)
	}
	return out
}

// ValueMethodByName is reflect.Value.MethodByName.
func ValueMethodByName(v reflect.Value, name string) reflect.Value {
	return v.MethodByName(name)
}

// ValueMethod is reflect.Value.Method.
func ValueMethod(v reflect.Value, i int) reflect.Value { return v.Method(i) }
