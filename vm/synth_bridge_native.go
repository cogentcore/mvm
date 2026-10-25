//go:build !goscript

package vm

import (
	"reflect"
	"unsafe"

	"github.com/mvm-sh/mvm/internal/runtype"
	"github.com/mvm-sh/mvm/mtype"
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
