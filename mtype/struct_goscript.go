//go:build goscript

package mtype

import (
	"reflect"

	"github.com/mvm-sh/mvm/internal/gsreflect"
)

// patchRtype gives dst the struct layout of src in place, keeping its name.
func patchRtype(dst, src reflect.Type) { gsreflect.SetFields(dst, src) }
