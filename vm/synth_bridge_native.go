//go:build !goscript

package vm

import (
	"reflect"
	"unsafe"

	"github.com/cogentcore/mvm/internal/runtype"
	"github.com/cogentcore/mvm/mtype"
)

// ifaceEfaceWord is the eface type word of an any boxing vm.Iface, the form
// storeIfaceFromReflect writes.
// An itab can never alias an rtype allocation, so it discriminates that form
// from a genuine native iface in the same slot.
var ifaceEfaceWord = func() unsafe.Pointer {
	var a any = Iface{}
	return (*[2]unsafe.Pointer)(unsafe.Pointer(&a))[0]
}()

// mvmEfaceInSynthSlot decodes the mvm eface form out of an addressable
// synth-iface slot (see storeIfaceFromReflect); native reads (v.Elem) would
// misinterpret its rtype word as an itab.
func mvmEfaceInSynthSlot(v reflect.Value, t reflect.Type) (Iface, bool) {
	if !v.CanAddr() || !runtype.IsSynth(t) {
		return Iface{}, false
	}
	p := v.Addr().UnsafePointer()
	if (*[2]unsafe.Pointer)(p)[0] != ifaceEfaceWord {
		return Iface{}, false
	}
	ifc, ok := reflect.NewAt(mtype.AnyRtype, p).Elem().Interface().(Iface)
	return ifc, ok
}

// synthJSMethods reports that native code dispatches the methods of synth
// types directly, as JavaScript methods of their values (goscript only).
const synthJSMethods = false

// synthMethodHandler returns nil: native targets dispatch interpreted methods
// through stub pools, and wasm does not dispatch them natively.
func synthMethodHandler(m *Machine, t *mtype.Type, s synthMethodSpec) any { return nil }
