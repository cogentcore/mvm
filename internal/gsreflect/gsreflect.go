// Package gsreflect edits reflect types in place when mvm is compiled to
// JavaScript with goscript, where a reflect.Type is a JavaScript object
// rather than a runtime rtype. It is the goscript counterpart of the
// rtype memory edits that internal/runtype makes natively.
//
// The implementation is the goscript override in
// gsoverride/github.com/mvm-sh/mvm/internal/gsreflect, passed to goscript
// with --gs-path. The Go bodies here only panic: the package is used only
// by the goscript build of internal/runtype and mtype.
package gsreflect

import "reflect"

const errNative = "gsreflect: only available under goscript"

// Clone returns a new type with the same shape as t and its own identity.
func Clone(t reflect.Type) reflect.Type { panic(errNative) }

// SetName names t in place: name is the qualified name (pkg.Name) and
// pkgPath the import path reported by PkgPath.
func SetName(t reflect.Type, name, pkgPath string) { panic(errNative) }

// SetString sets the String text of t in place, leaving it unnamed.
func SetString(t reflect.Type, s string) { panic(errNative) }

// SetFields gives the struct type dst the fields of the struct type src in
// place, keeping the name of dst.
func SetFields(dst, src reflect.Type) { panic(errNative) }

// SetElem sets the element type of the pointer, slice, array, chan or map
// type t in place.
func SetElem(t, elem reflect.Type) { panic(errNative) }

// SetFieldType sets the type of field i of the struct type t in place.
func SetFieldType(t reflect.Type, i int, ft reflect.Type) { panic(errNative) }

// SetFieldEmbedded marks field i of the struct type t as embedded, with the
// given name, tag and export status.
func SetFieldEmbedded(t reflect.Type, i int, name, tag string, exported bool) {
	panic(errNative)
}

// SetMethod installs the method name, of signature sig (without receiver), on
// the values of the named type t, so that calling it calls fn with a pointer
// to the value and the arguments. ptrRecv is whether only *t has the method.
func SetMethod(t reflect.Type, name string, sig reflect.Type, ptrRecv bool,
	fn func(recv reflect.Value, args []reflect.Value) []reflect.Value) {
	panic(errNative)
}

// SetSynthMethods records t as a synthesized type with n methods.
func SetSynthMethods(t reflect.Type, n int) { panic(errNative) }

// SynthMethods returns the number of methods recorded for t by
// SetSynthMethods, and whether t is recorded.
func SynthMethods(t reflect.Type) (int, bool) { panic(errNative) }
