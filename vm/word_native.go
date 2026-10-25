//go:build !goscript

package vm

import (
	"math"
	"reflect"
)

// word is the storage of Value.num: the typed value of a numeric Value in
// uint64 form (floats as their float64 bits), or a frame bookkeeping word.
type word = uint64

// wordFloat returns the float64 that w holds.
func wordFloat(w word) float64 { return math.Float64frombits(w) }

// floatWord returns the word that holds f.
func floatWord(f float64) word { return math.Float64bits(f) }

// wordUint returns the uint64 that w holds.
func wordUint(w word) uint64 { return w }

// uintWord returns the word that holds u.
func uintWord(u uint64) word { return u }

// getf32 extracts a float32 from a Value's uint64 storage (float64-bits encoding).
func getf32(n word) float32 { return float32(math.Float64frombits(n)) }

// putf32 stores a float32 into a Value's uint64 storage (float64-bits encoding).
func putf32(f float32) word { return math.Float64bits(float64(f)) }

const heapSavedFlag = uint64(1) << 63

// retIPInfo word layout: retIP bits 0..31, nret bits 32..45 (retNretMask),
// hasNamedRetFlag bit 46, namedRetFlag bit 47, frameBase bits 48..63.
// namedRetFlag: captured returns, finalized on the normal Return path.
// hasNamedRetFlag: any named returns; a recover returns their current values, so
// panicUnwind finalizes from the slots rather than zeroing.
const (
	retNretMask            = 0x3FFF
	hasNamedRetFlag uint64 = 1 << 46
	namedRetFlag    uint64 = 1 << 47
)

func packRetIP(retIP, nret, frameBase int) uint64 {
	return uint64(uint32(retIP)) | uint64(nret&retNretMask)<<32 | uint64(frameBase)<<48
}

// retIPOf, retNret and retFrameBase unpack a retIPInfo word.
func retIPOf(w word) int      { return int(int32(w)) }
func retNret(w word) int      { return int((w >> 32) & retNretMask) }
func retFrameBase(w word) int { return int(w >> 48) }

// retHasFlag reports whether the retIPInfo word w has flag set.
func retHasFlag(w, flag word) bool { return w&flag != 0 }

// retSetFlag returns w with flag set.
func retSetFlag(w, flag word) word { return w | flag }

// fpWord packs a saved frame pointer, flagging a caller closure heap saved
// to heapFrames.
func fpWord(fp int, heapSaved bool) word {
	if heapSaved {
		return uint64(fp) | heapSavedFlag
	}
	return uint64(fp)
}

// fpOf and fpHeapSaved unpack a saved frame pointer word.
func fpOf(w word) int         { return int(w &^ heapSavedFlag) }
func fpHeapSaved(w word) bool { return w&heapSavedFlag != 0 }

// A defer entry's mem[dh-2] slot packs isX (bits 0..1: 0 VM, 1 native, 2
// builtin), narg (bits 2..62), and deferStartedFlag (bit 63), via the helpers
// below. The flag lets panicUnwind skip an already-dispatched defer (Go's
// _defer.started) rather than re-run one whose body panicked before deferRet.
const deferStartedFlag = uint64(1) << 63

func packDefer(narg, isX, spread int) uint64 { return uint64(narg<<3 | spread<<2 | isX) }
func deferNarg(packed uint64) int            { return int((packed &^ deferStartedFlag) >> 3) }
func deferIsX(packed uint64) int             { return int(packed & 3) }
func deferSpread(packed uint64) bool         { return packed&DeferSpreadFlag != 0 }

// deferStarted reports whether the defer header word w is marked started.
func deferStarted(w word) bool { return w&deferStartedFlag != 0 }

// deferSetStarted returns the defer header word w marked started.
func deferSetStarted(w word) word { return w | deferStartedFlag }

// convertNum converts the numeric word w of kind src to kind dst, truncating
// to the width of dst.
func convertNum(w word, src, dst reflect.Kind) word {
	bits := w
	switch {
	case isFloat(src) && isFloat(dst):
		if src != dst {
			f := math.Float64frombits(bits)
			if dst == reflect.Float32 {
				bits = math.Float64bits(float64(float32(f)))
			}
		}
	case isFloat(src):
		f := math.Float64frombits(bits)
		if dst >= reflect.Uint && dst <= reflect.Uintptr {
			bits = uint64(f)
		} else {
			bits = uint64(int64(f))
		}
	case isFloat(dst):
		// int -> float.
		if src >= reflect.Uint && src <= reflect.Uintptr {
			bits = math.Float64bits(float64(bits))
		} else {
			bits = math.Float64bits(float64(int64(bits)))
		}
	}
	// Truncate to target width for sub-word types.
	switch dst {
	case reflect.Int8:
		return uint64(int8(bits))
	case reflect.Int16:
		return uint64(int16(bits))
	case reflect.Int32:
		return uint64(int32(bits))
	case reflect.Uint8:
		return uint64(uint8(bits))
	case reflect.Uint16:
		return uint64(uint16(bits))
	case reflect.Uint32:
		return uint64(uint32(bits))
	case reflect.Float32:
		return math.Float64bits(float64(float32(math.Float64frombits(bits))))
	}
	return bits
}

// A running VM defer's mem[dh] slot packs its return address (bits 0..31)
// and result count (bits 32..63).
func packDeferRet(ip, nret int) word { return uint64(uint32(ip)) | uint64(nret)<<32 }
func deferRetIP(w word) int          { return int(int32(w & 0xFFFFFFFF)) }
func deferRetNret(w word) int        { return int(w >> 32) }

// truncToKind narrows n to k's value width, sign-extending for signed kinds.
// Restores the invariant that Value.num holds the typed value in uint64 form
// after ops (BitShl, BitComp) that operate on uint64 without respecting width.
func truncToKind(n uint64, k reflect.Kind) uint64 {
	switch k {
	case reflect.Int8:
		return uint64(int64(int8(n)))
	case reflect.Int16:
		return uint64(int64(int16(n)))
	case reflect.Int32:
		return uint64(int64(int32(n)))
	case reflect.Uint8:
		return uint64(uint8(n))
	case reflect.Uint16:
		return uint64(uint16(n))
	case reflect.Uint32:
		return uint64(uint32(n))
	}
	return n
}

// bitAnd, bitOr, bitXor and bitAndNot are the generic bitwise ops.
func bitAnd(a, b word) word    { return a & b }
func bitOr(a, b word) word     { return a | b }
func bitXor(a, b word) word    { return a ^ b }
func bitAndNot(a, b word) word { return a &^ b }

// shlWord shifts a of kind k left by n, truncated to the width of k.
func shlWord(a, n word, k reflect.Kind) word { return truncToKind(a<<n, k) }

// shrWord shifts a of kind k right by n, arithmetically for signed kinds.
func shrWord(a, n word, k reflect.Kind) word {
	switch {
	case k >= reflect.Uint && k <= reflect.Uintptr:
		return truncToKind(a, k) >> n
	case k == reflect.Int8:
		return uint64(int64(int8(a)) >> n)
	case k == reflect.Int16:
		return uint64(int64(int16(a)) >> n)
	case k == reflect.Int32:
		return uint64(int64(int32(a)) >> n)
	}
	return uint64(int64(a) >> n)
}

// compWord is the bitwise complement of a of kind k.
func compWord(a word, k reflect.Kind) word { return truncToKind(^a, k) }
