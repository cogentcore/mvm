//go:build !goscript

package mtype

import (
	"reflect"
	"sync"
	"unsafe"
)

var (
	// Byte size of reflect's internal structType, probed at init.
	structTypeSize uintptr

	// Pins every source rtype whose internal arrays patchRtype aliases by pointer
	// (the raw copy bypasses the GC write barrier), so the collector can't free
	// them while a placeholder still points in -- a flaky go1.26 "bad pointer".
	patchKeepAliveMu sync.Mutex
	patchKeepAlive   []reflect.Type
)

func init() {
	// Probe structType's size by scanning a built struct for its Fields slice
	// header; the header is fixed-size, so the result is field-count-independent.
	const nfields = 7
	sf := make([]reflect.StructField, nfields)
	for i := range sf {
		sf[i] = reflect.StructField{Name: string(rune('A' + i)), Type: intRtype}
	}
	rt := reflect.StructOf(sf)
	data := rtypeData(rt)
	ws := unsafe.Sizeof(uintptr(0))
	for off := ws; off < 256; off += ws {
		lenp := (*int)(unsafe.Add(data, off+ws))
		capp := (*int)(unsafe.Add(data, off+2*ws))
		if *lenp == nfields && *capp >= nfields {
			structTypeSize = off + 3*ws
			return
		}
	}
	panic("mtype: cannot determine reflect structType size")
}

// rtypeData extracts the *rtype data pointer from a reflect.Type interface.
func rtypeData(t reflect.Type) unsafe.Pointer {
	return (*[2]unsafe.Pointer)(unsafe.Pointer(&t))[1]
}

// patchRtype overwrites dst's rtype bytes with src's, skipping Str/PtrToThis
// (the nameOff/typeOff at offsets 40-47): those are registered per heap address,
// so copying or zeroing them crashes rtype.String(); dst's originals stay valid.
func patchRtype(dst, src reflect.Type) {
	// Pin src; its arrays are aliased below without a write barrier.
	patchKeepAliveMu.Lock()
	patchKeepAlive = append(patchKeepAlive, src)
	patchKeepAliveMu.Unlock()

	d := rtypeData(dst)
	s := rtypeData(src)
	for i := range uintptr(40) {
		*(*byte)(unsafe.Add(d, i)) = *(*byte)(unsafe.Add(s, i))
	}
	for i := uintptr(48); i < structTypeSize; i++ {
		*(*byte)(unsafe.Add(d, i)) = *(*byte)(unsafe.Add(s, i))
	}
}
