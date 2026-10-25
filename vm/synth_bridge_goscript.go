//go:build goscript

package vm

import (
	"reflect"

	"github.com/mvm-sh/mvm/internal/runtype"
	"github.com/mvm-sh/mvm/mtype"
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

// synthJSMethods reports that native code dispatches the methods of synth
// types directly, as JavaScript methods of their values, so they need no
// forwarding shims (see wrapSynthIfaceForNative).
const synthJSMethods = true

// synthMethodHandler returns the runtype.MethodFunc that runs the interpreted
// method s of t for native callers, which goscript installs as a JavaScript
// method of the values of t. The receiver it gets is a pointer to the value.
func synthMethodHandler(m *Machine, t *mtype.Type, s synthMethodSpec) any {
	return runtype.MethodFunc(func(recv reflect.Value, args []reflect.Value) []reflect.Value {
		// A value receiver is a copy, so writes in the method stay local.
		rv := recv
		if s.form != recvPtr {
			rv = reflect.New(t.Rtype).Elem()
			rv.Set(recv.Elem())
		}
		out, err := callMethod(m, t, s.name, rv, s.method, s.method.Rtype, args)
		if err != nil {
			raiseMethodErr(err)
		}
		return out
	})
}
