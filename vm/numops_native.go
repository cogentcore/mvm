//go:build !goscript

package vm

import "math"

// Per-kind numeric ops on words, which the VM opcodes call; natively they
// are the generic ops below, instantiated.

func addInt(a, b word) word    { return add[int](a, b) }
func addInt8(a, b word) word   { return add[int8](a, b) }
func addInt16(a, b word) word  { return add[int16](a, b) }
func addInt32(a, b word) word  { return add[int32](a, b) }
func addInt64(a, b word) word  { return add[int64](a, b) }
func addUint(a, b word) word   { return add[uint](a, b) }
func addUint8(a, b word) word  { return add[uint8](a, b) }
func addUint16(a, b word) word { return add[uint16](a, b) }
func addUint32(a, b word) word { return add[uint32](a, b) }
func addUint64(a, b word) word { return add[uint64](a, b) }

func subInt(a, b word) word    { return sub[int](a, b) }
func subInt8(a, b word) word   { return sub[int8](a, b) }
func subInt16(a, b word) word  { return sub[int16](a, b) }
func subInt32(a, b word) word  { return sub[int32](a, b) }
func subInt64(a, b word) word  { return sub[int64](a, b) }
func subUint(a, b word) word   { return sub[uint](a, b) }
func subUint8(a, b word) word  { return sub[uint8](a, b) }
func subUint16(a, b word) word { return sub[uint16](a, b) }
func subUint32(a, b word) word { return sub[uint32](a, b) }
func subUint64(a, b word) word { return sub[uint64](a, b) }

func mulInt(a, b word) word    { return mul[int](a, b) }
func mulInt8(a, b word) word   { return mul[int8](a, b) }
func mulInt16(a, b word) word  { return mul[int16](a, b) }
func mulInt32(a, b word) word  { return mul[int32](a, b) }
func mulInt64(a, b word) word  { return mul[int64](a, b) }
func mulUint(a, b word) word   { return mul[uint](a, b) }
func mulUint8(a, b word) word  { return mul[uint8](a, b) }
func mulUint16(a, b word) word { return mul[uint16](a, b) }
func mulUint32(a, b word) word { return mul[uint32](a, b) }
func mulUint64(a, b word) word { return mul[uint64](a, b) }

func divInt(a, b word) word    { return div[int](a, b) }
func divInt8(a, b word) word   { return div[int8](a, b) }
func divInt16(a, b word) word  { return div[int16](a, b) }
func divInt32(a, b word) word  { return div[int32](a, b) }
func divInt64(a, b word) word  { return div[int64](a, b) }
func divUint(a, b word) word   { return div[uint](a, b) }
func divUint8(a, b word) word  { return div[uint8](a, b) }
func divUint16(a, b word) word { return div[uint16](a, b) }
func divUint32(a, b word) word { return div[uint32](a, b) }
func divUint64(a, b word) word { return div[uint64](a, b) }

func remInt(a, b word) word    { return rem[int](a, b) }
func remInt8(a, b word) word   { return rem[int8](a, b) }
func remInt16(a, b word) word  { return rem[int16](a, b) }
func remInt32(a, b word) word  { return rem[int32](a, b) }
func remInt64(a, b word) word  { return rem[int64](a, b) }
func remUint(a, b word) word   { return rem[uint](a, b) }
func remUint8(a, b word) word  { return rem[uint8](a, b) }
func remUint16(a, b word) word { return rem[uint16](a, b) }
func remUint32(a, b word) word { return rem[uint32](a, b) }
func remUint64(a, b word) word { return rem[uint64](a, b) }

func negInt(a word) word    { return neg[int](a) }
func negInt8(a word) word   { return neg[int8](a) }
func negInt16(a word) word  { return neg[int16](a) }
func negInt32(a word) word  { return neg[int32](a) }
func negInt64(a word) word  { return neg[int64](a) }
func negUint(a word) word   { return neg[uint](a) }
func negUint8(a word) word  { return neg[uint8](a) }
func negUint16(a word) word { return neg[uint16](a) }
func negUint32(a word) word { return neg[uint32](a) }
func negUint64(a word) word { return neg[uint64](a) }

func addfFloat32(a, b word) word { return addf[float32](a, b) }
func addfFloat64(a, b word) word { return addf[float64](a, b) }

func subfFloat32(a, b word) word { return subf[float32](a, b) }
func subfFloat64(a, b word) word { return subf[float64](a, b) }

func mulfFloat32(a, b word) word { return mulf[float32](a, b) }
func mulfFloat64(a, b word) word { return mulf[float64](a, b) }

func divfFloat32(a, b word) word { return divf[float32](a, b) }
func divfFloat64(a, b word) word { return divf[float64](a, b) }

func negfFloat32(a word) word { return negf[float32](a) }
func negfFloat64(a word) word { return negf[float64](a) }

type integer interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

type float interface{ ~float32 | ~float64 }

func add[T integer](a, b uint64) uint64 { return uint64(T(a) + T(b)) }
func sub[T integer](a, b uint64) uint64 { return uint64(T(a) - T(b)) }
func mul[T integer](a, b uint64) uint64 { return uint64(T(a) * T(b)) }
func div[T integer](a, b uint64) uint64 { return uint64(T(a) / T(b)) }
func rem[T integer](a, b uint64) uint64 { return uint64(T(a) % T(b)) }
func neg[T integer](a uint64) uint64    { return uint64(-T(a)) }

func addf[T float](a, b uint64) uint64 {
	return math.Float64bits(float64(T(math.Float64frombits(a)) + T(math.Float64frombits(b))))
}

func subf[T float](a, b uint64) uint64 {
	return math.Float64bits(float64(T(math.Float64frombits(a)) - T(math.Float64frombits(b))))
}

func mulf[T float](a, b uint64) uint64 {
	return math.Float64bits(float64(T(math.Float64frombits(a)) * T(math.Float64frombits(b))))
}

func divf[T float](a, b uint64) uint64 {
	return math.Float64bits(float64(T(math.Float64frombits(a)) / T(math.Float64frombits(b))))
}

func negf[T float](a uint64) uint64 {
	return math.Float64bits(float64(-T(math.Float64frombits(a))))
}
