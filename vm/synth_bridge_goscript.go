//go:build goscript

package vm

import (
	"reflect"

	"github.com/mvm-sh/mvm/internal/runtype"
)

// mvmEfaceInSynthSlot decodes the mvm eface form out of an addressable
// synth-iface slot (see storeIfaceFromReflect). Under goscript a synth
// interface holds its value as any does, so the slot holds the Iface itself.
func mvmEfaceInSynthSlot(v reflect.Value, t reflect.Type) (Iface, bool) {
	if !v.CanAddr() || !runtype.IsSynth(t) || v.IsNil() {
		return Iface{}, false
	}
	ifc, ok := v.Interface().(Iface)
	return ifc, ok
}
