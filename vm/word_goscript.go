//go:build goscript

package vm

import "reflect"

// Under goscript, Value.num is a float64, a JavaScript number: a numeric
// Value holds its value itself (an integer exactly up to 2^53), rather than
// its bits in a uint64, which goscript represents as a BigInt. Integer
// conversions of a word wrap as goscript wraps them for compiled code.
// Frame bookkeeping words pack their fields arithmetically within 53 bits.

// word is the storage of Value.num: the value of a numeric Value, or a frame
// bookkeeping word.
type word = float64

// wordFloat returns the float64 that w holds.
func wordFloat(w word) float64 { return w }

// floatWord returns the word that holds f.
func floatWord(f float64) word { return f }

// wordUint returns the uint64 that w holds.
func wordUint(w word) uint64 { return uint64(w) }

// uintWord returns the word that holds u.
func uintWord(u uint64) word { return word(u) }

// getf32 returns the float32 that n holds.
func getf32(n word) float32 { return float32(n) }

// putf32 returns the word that holds f.
func putf32(f float32) word { return word(f) }

// Factors of the retIPInfo word fields: retIP (24 bits, sign-extended),
// nret (14 bits), the two named-return flags and frameBase (13 bits).
const (
	retIPBase       = 1 << 24
	retNretMask     = 1<<14 - 1
	hasNamedRetFlag = 1 << 38
	namedRetFlag    = 1 << 39
	retFrameFactor  = 1 << 40
	retFrameMax     = 1<<13 - 1
)

func packRetIP(retIP, nret, frameBase int) word {
	if retIP >= retIPBase/2 || retIP < -retIPBase/2 || frameBase > retFrameMax {
		panic("vm: frame layout out of range under goscript")
	}
	if retIP < 0 {
		retIP += retIPBase
	}
	return word(retIP + (nret&retNretMask)*retIPBase + frameBase*retFrameFactor)
}

// retIPOf, retNret and retFrameBase unpack a retIPInfo word.
func retIPOf(w word) int {
	ip := int(w) % retIPBase
	if ip >= retIPBase/2 {
		ip -= retIPBase
	}
	return ip
}

func retNret(w word) int      { return int(w) / retIPBase % (retNretMask + 1) }
func retFrameBase(w word) int { return int(w) / retFrameFactor }

// retHasFlag reports whether the retIPInfo word w has flag set.
func retHasFlag(w, flag word) bool { return int(w)/int(flag)%2 == 1 }

// retSetFlag returns w with flag set.
func retSetFlag(w, flag word) word {
	if retHasFlag(w, flag) {
		return w
	}
	return w + flag
}

// heapSavedFlag marks a saved frame pointer whose caller closure heap was
// saved to heapFrames.
const heapSavedFlag = 1 << 52

// fpWord packs a saved frame pointer, flagging a caller closure heap saved
// to heapFrames.
func fpWord(fp int, heapSaved bool) word {
	if heapSaved {
		return word(fp + heapSavedFlag)
	}
	return word(fp)
}

// fpOf and fpHeapSaved unpack a saved frame pointer word.
func fpOf(w word) int         { return int(w) % heapSavedFlag }
func fpHeapSaved(w word) bool { return w >= heapSavedFlag }

// A defer entry's mem[dh-2] slot packs isX (0 VM, 1 native, 2 builtin) plus
// 4 for spread plus 8 times narg, and deferStartedFlag.
const deferStartedFlag = 1 << 52

func packDefer(narg, isX, spread int) word { return word(narg*8 + spread*4 + isX) }
func deferNarg(packed word) int            { return int(packed) % deferStartedFlag / 8 }
func deferIsX(packed word) int             { return int(packed) % 4 }
func deferSpread(packed word) bool         { return int(packed)/4%2 == 1 }

// deferStarted reports whether the defer header word w is marked started.
func deferStarted(w word) bool { return w >= deferStartedFlag }

// deferSetStarted returns the defer header word w marked started.
func deferSetStarted(w word) word {
	if deferStarted(w) {
		return w
	}
	return w + deferStartedFlag
}

// convertNum converts the numeric word w of kind src to kind dst: a float
// to an integer truncates, and an integer wraps to the width of dst.
func convertNum(w word, src, dst reflect.Kind) word {
	switch dst {
	case reflect.Int, reflect.Int64:
		return word(int(w))
	case reflect.Int8:
		return word(int8(int(w)))
	case reflect.Int16:
		return word(int16(int(w)))
	case reflect.Int32:
		return word(int32(int(w)))
	case reflect.Uint, reflect.Uint64, reflect.Uintptr:
		if isFloat(src) {
			return word(uint(w))
		}
		return word(uint(int(w)))
	case reflect.Uint8:
		return word(uint8(int(w)))
	case reflect.Uint16:
		return word(uint16(int(w)))
	case reflect.Uint32:
		return word(uint32(int(w)))
	case reflect.Float32:
		return word(float32(w))
	}
	return w
}

// A running VM defer's mem[dh] slot packs its return address and result
// count as a retIPInfo word does.
func packDeferRet(ip, nret int) word { return packRetIP(ip, nret, 0) }
func deferRetIP(w word) int          { return retIPOf(w) }
func deferRetNret(w word) int        { return retNret(w) }

// isInt32 reports whether w is in the range of int32, where JavaScript
// bitwise ops apply directly.
func isInt32(w word) bool { return w >= -1<<31 && w < 1<<31 }

// bitAnd, bitOr, bitXor and bitAndNot are the generic bitwise ops: on int32
// numbers when both operands are in range, and on int64s otherwise.
func bitAnd(a, b word) word {
	if isInt32(a) && isInt32(b) {
		return word(int32(a) & int32(b))
	}
	return word(int64(a) & int64(b))
}

func bitOr(a, b word) word {
	if isInt32(a) && isInt32(b) {
		return word(int32(a) | int32(b))
	}
	return word(int64(a) | int64(b))
}

func bitXor(a, b word) word {
	if isInt32(a) && isInt32(b) {
		return word(int32(a) ^ int32(b))
	}
	return word(int64(a) ^ int64(b))
}

func bitAndNot(a, b word) word {
	if isInt32(a) && isInt32(b) {
		return word(int32(a) &^ int32(b))
	}
	return word(int64(a) &^ int64(b))
}

// isUnsigned reports whether k is an unsigned integer kind.
func isUnsigned(k reflect.Kind) bool { return k >= reflect.Uint && k <= reflect.Uintptr }

// shlWord shifts a of kind k left by n, wrapped to the width of k.
func shlWord(a, n word, k reflect.Kind) word {
	if isUnsigned(k) {
		return convertNum(word(uint64(a)<<uint64(n)), k, k)
	}
	return convertNum(word(int64(a)<<uint64(n)), k, k)
}

// shrWord shifts a of kind k right by n, arithmetically for signed kinds.
func shrWord(a, n word, k reflect.Kind) word {
	if isUnsigned(k) {
		return word(uint64(a) >> uint64(n))
	}
	return word(int64(a) >> uint64(n))
}

// compWord is the bitwise complement of a of kind k.
func compWord(a word, k reflect.Kind) word {
	if isUnsigned(k) {
		return convertNum(word(^uint64(a)), k, k)
	}
	return -a - 1
}
