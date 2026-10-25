//go:build goscript

package vm

// Per-kind numeric ops on words, which the VM opcodes call. Under goscript
// they are plain arithmetic on the values in the words: goscript wraps the
// result of each conversion and operation as Go does. int64 and uint64 use
// the arithmetic of int and uint, which are numbers too, rather than that of
// BigInts: a word holds an integer exactly only up to 2^53 anyway.

func addInt(a, b word) word    { return word(int(a) + int(b)) }
func addInt8(a, b word) word   { return word(int8(a) + int8(b)) }
func addInt16(a, b word) word  { return word(int16(a) + int16(b)) }
func addInt32(a, b word) word  { return word(int32(a) + int32(b)) }
func addInt64(a, b word) word  { return word(int(a) + int(b)) }
func addUint(a, b word) word   { return word(uint(a) + uint(b)) }
func addUint8(a, b word) word  { return word(uint8(a) + uint8(b)) }
func addUint16(a, b word) word { return word(uint16(a) + uint16(b)) }
func addUint32(a, b word) word { return word(uint32(a) + uint32(b)) }
func addUint64(a, b word) word { return word(uint(a) + uint(b)) }

func subInt(a, b word) word    { return word(int(a) - int(b)) }
func subInt8(a, b word) word   { return word(int8(a) - int8(b)) }
func subInt16(a, b word) word  { return word(int16(a) - int16(b)) }
func subInt32(a, b word) word  { return word(int32(a) - int32(b)) }
func subInt64(a, b word) word  { return word(int(a) - int(b)) }
func subUint(a, b word) word   { return word(uint(a) - uint(b)) }
func subUint8(a, b word) word  { return word(uint8(a) - uint8(b)) }
func subUint16(a, b word) word { return word(uint16(a) - uint16(b)) }
func subUint32(a, b word) word { return word(uint32(a) - uint32(b)) }
func subUint64(a, b word) word { return word(uint(a) - uint(b)) }

func mulInt(a, b word) word    { return word(int(a) * int(b)) }
func mulInt8(a, b word) word   { return word(int8(a) * int8(b)) }
func mulInt16(a, b word) word  { return word(int16(a) * int16(b)) }
func mulInt32(a, b word) word  { return word(int32(a) * int32(b)) }
func mulInt64(a, b word) word  { return word(int(a) * int(b)) }
func mulUint(a, b word) word   { return word(uint(a) * uint(b)) }
func mulUint8(a, b word) word  { return word(uint8(a) * uint8(b)) }
func mulUint16(a, b word) word { return word(uint16(a) * uint16(b)) }
func mulUint32(a, b word) word { return word(uint32(a) * uint32(b)) }
func mulUint64(a, b word) word { return word(uint(a) * uint(b)) }

func divInt(a, b word) word    { return word(int(a) / int(b)) }
func divInt8(a, b word) word   { return word(int8(a) / int8(b)) }
func divInt16(a, b word) word  { return word(int16(a) / int16(b)) }
func divInt32(a, b word) word  { return word(int32(a) / int32(b)) }
func divInt64(a, b word) word  { return word(int(a) / int(b)) }
func divUint(a, b word) word   { return word(uint(a) / uint(b)) }
func divUint8(a, b word) word  { return word(uint8(a) / uint8(b)) }
func divUint16(a, b word) word { return word(uint16(a) / uint16(b)) }
func divUint32(a, b word) word { return word(uint32(a) / uint32(b)) }
func divUint64(a, b word) word { return word(uint(a) / uint(b)) }

func remInt(a, b word) word    { return word(int(a) % int(b)) }
func remInt8(a, b word) word   { return word(int8(a) % int8(b)) }
func remInt16(a, b word) word  { return word(int16(a) % int16(b)) }
func remInt32(a, b word) word  { return word(int32(a) % int32(b)) }
func remInt64(a, b word) word  { return word(int(a) % int(b)) }
func remUint(a, b word) word   { return word(uint(a) % uint(b)) }
func remUint8(a, b word) word  { return word(uint8(a) % uint8(b)) }
func remUint16(a, b word) word { return word(uint16(a) % uint16(b)) }
func remUint32(a, b word) word { return word(uint32(a) % uint32(b)) }
func remUint64(a, b word) word { return word(uint(a) % uint(b)) }

func negInt(a word) word    { return word(-int(a)) }
func negInt8(a word) word   { return word(-int8(a)) }
func negInt16(a word) word  { return word(-int16(a)) }
func negInt32(a word) word  { return word(-int32(a)) }
func negInt64(a word) word  { return word(-int(a)) }
func negUint(a word) word   { return word(-uint(a)) }
func negUint8(a word) word  { return word(-uint8(a)) }
func negUint16(a word) word { return word(-uint16(a)) }
func negUint32(a word) word { return word(-uint32(a)) }
func negUint64(a word) word { return word(-uint(a)) }

func addfFloat32(a, b word) word { return word(float32(a) + float32(b)) }
func addfFloat64(a, b word) word { return a + b }

func subfFloat32(a, b word) word { return word(float32(a) - float32(b)) }
func subfFloat64(a, b word) word { return a - b }

func mulfFloat32(a, b word) word { return word(float32(a) * float32(b)) }
func mulfFloat64(a, b word) word { return a * b }

func divfFloat32(a, b word) word { return word(float32(a) / float32(b)) }
func divfFloat64(a, b word) word { return a / b }

func negfFloat32(a word) word { return word(-float32(a)) }
func negfFloat64(a word) word { return word(-a) }
