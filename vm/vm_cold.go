package vm

import (
	"fmt"
	"math"
	"math/bits"
	"reflect"

	"github.com/cogentcore/mvm/internal/derive"
	"github.com/cogentcore/mvm/internal/runtype"
	"github.com/cogentcore/mvm/mtype"
)

// runCold runs the rare opcodes of c, which neither runLoop nor runWarm
// runs (see runWarm). It shares runLoop's state as runWarm does.
func (m *Machine) runCold(c *Instruction, _ uint8, panicAddr, deferRetAddr int, _ word,
) (ctl coldCtl, done bool, err error) {
	mem, ip, fp, sp, safepointCtr := m.tierMem, m.tierIP, m.tierFP, m.tierSP, m.tierSafepointCtr
	switch c.Op {
	case HealSentinels:
		m.healSentinels()

	case Detach:
		// A multi-assign RHS may alias an LHS variable's storage (a callee
		// returning its param shares the caller's slot); assignSlot's in-place
		// write would clobber it before it is read.
		for i := sp - int(c.A) + 1; i <= sp; i++ {
			if r := mem[i].ref; r.IsValid() && r.CanSet() && !isNum(r.Kind()) {
				nr := reflect.New(r.Type()).Elem()
				nr.Set(r)
				mem[i].ref = nr
			}
		}

	case GetLocalSync:
		// GetLocal variant emitted after AddrLocal for the same slot:
		// the local was promoted to addressable storage, so a native
		// callee writing through the pushed pointer bypasses slot.num.
		// Re-read num from ref so subsequent reads see native writes
		// (e.g. flag.BoolVar(&b, ...) + Parse must update `b`).
		// Only when the slot was actually promoted (ref addressable): the
		// addressed flag is set at compile time for the whole function, but
		// the AddrLocal that promotes the slot is control-flow dependent. If
		// the &local sits in a not-taken sibling branch (e.g. another switch
		// case), the slot stays unpromoted with num authoritative and ref a
		// stale type-zero; syncing would clobber num to 0.
		sp++
		mem[sp] = mem[int(c.A)+fp-1]
		if mem[sp].ref.CanAddr() && isNum(mem[sp].ref.Kind()) {
			mem[sp].num = numBits(mem[sp].ref)
		}

	case GetLocalLowerUintImm:
		sp++
		mem[sp] = boolVal(uint(mem[int(c.A)+fp-1].num) < uint(int(c.B)))

	case GetLocalGreaterUintImm:
		sp++
		mem[sp] = boolVal(uint(mem[int(c.A)+fp-1].num) > uint(int(c.B)))

	case IndexSetBool:
		idx := int(mem[sp].num)
		reflect.Indirect(mem[sp-1].ref).Index(idx).SetBool(c.A != 0)
		sp -= 2

	case Get:
		if int(c.A) == Local {
			if sp+1 >= len(mem) {
				mem = growStack(mem, sp, 1)
			}
			sp++
			mem[sp] = mem[int(c.B)+fp-1]
		} else {
			v := m.globals[int(c.B)]
			if isNum(v.ref.Kind()) && v.ref.CanAddr() {
				v.num = numBits(v.ref)
			}
			if sp+1 >= len(mem) {
				mem = growStack(mem, sp, 1)
			}
			sp++
			mem[sp] = v
		}

	case Exit:
		m.tierMem, m.tierIP, m.tierFP, m.tierSP, m.tierSafepointCtr = mem, ip, fp, sp, safepointCtr
		return coldReturn, true, err

	case FnewE:
		if sp+1 >= len(mem) {
			mem = growStack(mem, sp, 1)
		}
		sp++
		mem[sp] = NewValue(m.globals[int(c.A)].ref.Type().Elem(), int(c.B))

	case JumpSetTrue:
		cond := mem[sp].num != 0
		if cond {
			ip += int(c.A)
			// Note that the stack is not modified if cond is true.
			m.tierMem, m.tierIP, m.tierFP, m.tierSP, m.tierSafepointCtr = mem, ip, fp, sp, safepointCtr
			return coldContinue, false, nil
		}
		sp--

	case JumpSetFalse:
		cond := mem[sp].num != 0
		if !cond {
			ip += int(c.A)
			// Note that the stack is not modified if cond is false.
			m.tierMem, m.tierIP, m.tierFP, m.tierSP, m.tierSafepointCtr = mem, ip, fp, sp, safepointCtr
			return coldContinue, false, nil
		}
		sp--

	case DeferPush:
		mem, sp = m.deferPush(c, ip, mem, fp, sp)

	case GoCall:
		narg := int(c.A)
		fval := mem[sp-narg]
		args := make([]Value, narg)
		for i := range args {
			args[i] = snapshotArg(mem[sp-narg+1+i])
		}
		sp -= narg + 1
		m.mem = mem[:sp+1]
		if m.newGoroutine(fval, args, c.B != 0) {
			ip = m.stageUnwind(ip, fp, mem)
			m.tierMem, m.tierIP, m.tierFP, m.tierSP, m.tierSafepointCtr = mem, ip, fp, sp, safepointCtr
			return coldContinue, false, nil
		}
		mem = m.mem[:cap(m.mem)]

	case GoCallImm:
		narg := int(c.B)
		fval := m.globals[int(c.A)]
		args := make([]Value, narg)
		for i := range args {
			args[i] = snapshotArg(mem[sp-narg+1+i])
		}
		sp -= narg
		m.mem = mem[:sp+1]
		// GoCallImm targets a known VM func: never reflect-spread.
		if m.newGoroutine(fval, args, false) {
			ip = m.stageUnwind(ip, fp, mem)
			m.tierMem, m.tierIP, m.tierFP, m.tierSP, m.tierSafepointCtr = mem, ip, fp, sp, safepointCtr
			return coldContinue, false, nil
		}
		mem = m.mem[:cap(m.mem)]

	case MkChan:
		var chanType reflect.Type
		if a := int(c.A); a < 0 {
			// make(NamedChan) or synth-iface elem: whole type passed, like MkSlice.
			chanType = m.globals[-a-1].ref.Type()
		} else {
			// Derive, not reflect.ChanOf: a synth elem must match materialize's chan identity.
			chanType = runtype.DeriveChanOf(reflect.BothDir, m.globals[a].ref.Type())
		}
		bufSize := int(c.B)
		if bufSize < 0 {
			bufSize = int(mem[sp].num)
			sp--
		}
		if sp+1 >= len(mem) {
			mem = growStack(mem, sp, 1)
		}
		sp++
		mem[sp] = Value{ref: reflect.MakeChan(chanType, bufSize)}

	case ChanSend:
		ch := mem[sp-1].ref
		m.chanSend(ch, m.reflectForSend(mem[sp], ch.Type().Elem()))
		sp -= 2

	case ChanRecv:
		ch := mem[sp]
		v, ok := m.chanRecv(ch.ref)
		mem[sp] = FromReflect(v)
		if int(c.A) == 1 {
			if sp+1 >= len(mem) {
				mem = growStack(mem, sp, 1)
			}
			sp++
			mem[sp] = boolVal(ok)
		}

	case ChanClose:
		mem[sp].ref.Close()
		sp--

	case SelectExec:
		meta := m.globals[int(c.A)].ref.Interface().(*SelectMeta)
		ncase := int(c.B)
		base := sp - meta.TotalPop + 1
		// A blocking select (no default) can deadlock if a sender/receiver
		// goroutine died; reserve a trailing slot to watch the fault and abort.
		hasDefault := false
		for _, ci := range meta.Cases {
			if ci.Dir == reflect.SelectDefault {
				hasDefault = true
				break
			}
		}
		abortIdx := -1
		n := ncase
		if !hasDefault && m.watchFault() {
			abortIdx = ncase
			n++
		}
		cases := make([]reflect.SelectCase, n)
		idx := base
		for i, ci := range meta.Cases {
			switch ci.Dir {
			case reflect.SelectRecv:
				cases[i] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: mem[idx].ref}
				idx++
			case reflect.SelectSend:
				ch := mem[idx].ref
				cases[i] = reflect.SelectCase{Dir: reflect.SelectSend, Chan: ch, Send: m.reflectForSend(mem[idx+1], ch.Type().Elem())}
				idx += 2
			case reflect.SelectDefault:
				cases[i] = reflect.SelectCase{Dir: reflect.SelectDefault}
			}
		}
		if abortIdx >= 0 {
			cases[abortIdx] = reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(m.fault.abort)}
		}
		chosen, recv, recvOK := reflect.Select(cases)
		if chosen == abortIdx {
			panic(ErrGoroutineFault)
		}
		sp = base
		ci := meta.Cases[chosen]
		if ci.Dir == reflect.SelectRecv {
			if ci.Slot >= 0 {
				v := FromReflect(recv)
				switch {
				case ci.Cell:
					setCell(mem[fp-1+ci.Slot].ref.Interface().(*Value), v)
				case ci.Local:
					// reflect.Select returns an unaddressable value.
					if dst, src := &mem[fp-1+ci.Slot], v; !assignSlotNum(dst, src) {
						m.assignSlot(dst, src)
					}
				default:
					if dst, src := &m.globals[ci.Slot], v; !assignSlotNum(dst, src) {
						m.assignSlot(dst, src)
					}
				}
			}
			if ci.OkSlot >= 0 {
				v := boolVal(recvOK)
				switch {
				case ci.OkCell:
					setCell(mem[fp-1+ci.OkSlot].ref.Interface().(*Value), v)
				case ci.Local:
					mem[fp-1+ci.OkSlot] = v
				default:
					if dst, src := &m.globals[ci.OkSlot], v; !assignSlotNum(dst, src) {
						m.assignSlot(dst, src)
					}
				}
			}
		}
		mem[sp] = Value{num: word(chosen), ref: zint}

	case Print:
		n := int(c.A)
		args := make([]any, n)
		for i := range n {
			args[i] = mem[sp-n+1+i].Interface()
		}
		_, _ = fmt.Fprint(m.out, args...)
		sp -= n

	case Println:
		n := int(c.A)
		args := make([]any, n)
		for i := range n {
			args[i] = mem[sp-n+1+i].Interface()
		}
		_, _ = fmt.Fprintln(m.out, args...)
		sp -= n

	case Complex:
		var (
			cnv = func(v Value) float64 {
				if v.CanInt() {
					return float64(v.Int())
				}
				return v.Float()
			}
			vimag = cnv(mem[sp])
			vreal = cnv(mem[sp-1])
			kind  = reflect.Kind(c.A)
			out   Value
		)
		switch kind {
		case reflect.Complex64:
			out = ValueOf(complex(
				float32(vreal),
				float32(vimag),
			))
		case reflect.Complex128:
			out = ValueOf(complex(
				float64(vreal),
				float64(vimag),
			))
		default:
			panic(fmt.Errorf("impossible: complex-kind: %v", out.ref.Kind()))
		}
		sp--
		mem[sp] = out

	case Real:
		var (
			ref  = mem[sp].ref
			kind = reflect.Kind(c.A)
			v    Value
		)
		switch kind {
		case reflect.Float32:
			v = ValueOf(float32(real(ref.Complex())))
		case reflect.Float64:
			v = ValueOf(float64(real(ref.Complex())))
		default:
			panic("impossible")
		}
		mem[sp] = v

	case Imag:
		var (
			ref  = mem[sp].ref
			kind = reflect.Kind(c.A)
			v    Value
		)
		switch kind {
		case reflect.Float32:
			v = ValueOf(float32(imag(ref.Complex())))
		case reflect.Float64:
			v = ValueOf(float64(imag(ref.Complex())))
		default:
			panic("impossible")
		}
		mem[sp] = v

	case AddComplex, SubComplex, MulComplex, DivComplex:
		x, y := mem[sp-1].ref.Complex(), mem[sp].ref.Complex()
		sp--
		if reflect.Kind(c.A) == reflect.Complex64 {
			// Compute in complex64 so precision matches gc.
			a, b := complex64(x), complex64(y)
			var r complex64
			switch c.Op {
			case AddComplex:
				r = a + b
			case SubComplex:
				r = a - b
			case MulComplex:
				r = a * b
			case DivComplex:
				r = a / b
			}
			mem[sp] = ValueOf(r)
			break
		}
		var r complex128
		switch c.Op {
		case AddComplex:
			r = x + y
		case SubComplex:
			r = x - y
		case MulComplex:
			r = x * y
		case DivComplex:
			r = x / y
		}
		mem[sp] = ValueOf(r)

	case NegComplex:
		if reflect.Kind(c.A) == reflect.Complex64 {
			mem[sp] = ValueOf(-complex64(mem[sp].ref.Complex()))
			break
		}
		mem[sp] = ValueOf(-mem[sp].ref.Complex())

	case Min:
		sp = minMax(mem, sp, int(c.A), reflect.Kind(c.B), false)

	case Max:
		sp = minMax(mem, sp, int(c.A), reflect.Kind(c.B), true)

	case WrapFunc:
		// Wrap the mvm func value on the stack in a reflect.MakeFunc for native Go callbacks.
		// The original mvm func is preserved in MvmFunc.Val for fast in-VM dispatch.
		// The trampoline dispatches each invocation on a pooled runner with pointer-shared
		// parent state, so concurrent native callers (e.g. parallel sort.Slice with an mvm
		// less func) execute safely; user-visible package vars follow Go's memory model.
		typ := m.globals[int(c.A)].ref.Interface().(*mtype.Type)
		fval := mem[sp-int(c.B)]
		gf := m.wrapForFunc(fval, typ.Rtype)
		if gf.Kind() == reflect.Func && gf.IsNil() {
			// A nil source func must stay nil-comparable: store the zero func,
			// not a (non-nil) MvmFunc box.
			// A method call on it still resolves via the named rtype and panics
			// on invocation, as in Go.
			mem[sp-int(c.B)] = Value{ref: gf}
		} else {
			mem[sp-int(c.B)] = Value{ref: reflect.ValueOf(MvmFunc{Val: fval, GF: gf})}
		}

	case MkMethodExpr:
		codeAddr := int(m.globals[int(c.A)].num)
		exprType := m.globals[int(c.B)].ref.Interface().(*mtype.Type)
		if sp+1 >= len(mem) {
			mem = growStack(mem, sp, 1)
		}
		sp++
		// Materialize lazily: *T may only resolve after generate (post-attach).
		mem[sp] = Value{ref: m.makeMethodExprFunc(codeAddr, derive.MaterializeRtype(exprType))}

	case Trap:
		ip, fp, sp, mem = m.handleTrap(ip, fp, sp, mem)
		m.tierMem, m.tierIP, m.tierFP, m.tierSP, m.tierSafepointCtr = mem, ip, fp, sp, safepointCtr
		return coldContinue, false, nil

	case Panic:
		m.panicking = true
		m.panicVal = m.effectivePanicVal(mem[sp])
		sp-- // pop the panic argument
		ip = m.stageUnwind(ip, fp, mem)
		m.tierMem, m.tierIP, m.tierFP, m.tierSP, m.tierSafepointCtr = mem, ip, fp, sp, safepointCtr
		return coldContinue, false, nil

	case Recover:
		sp, mem = m.handleRecover(fp, sp, mem, deferRetAddr)

	case MarkNamedRet:
		mem[fp-2].num = retSetFlag(mem[fp-2].num, hasNamedRetFlag)
		if c.A != 0 { // captured: normal Return finalizes too
			mem[fp-2].num = retSetFlag(mem[fp-2].num, namedRetFlag)
		}

	case BitComp:
		mem[sp].num = compWord(mem[sp].num, mem[sp].ref.Kind())
		resetNumRef(&mem[sp])

		// Bit manipulation.

	case Clz32:
		mem[sp].num = word(bits.LeadingZeros32(uint32(mem[sp].num)))
		mem[sp].ref = zint

	case Clz64:
		mem[sp].num = word(bits.LeadingZeros64(wordUint(mem[sp].num)))
		mem[sp].ref = zint

	case Ctz32:
		mem[sp].num = word(bits.TrailingZeros32(uint32(mem[sp].num)))
		mem[sp].ref = zint

	case Ctz64:
		mem[sp].num = word(bits.TrailingZeros64(wordUint(mem[sp].num)))
		mem[sp].ref = zint

	case Popcnt32:
		mem[sp].num = word(bits.OnesCount32(uint32(mem[sp].num)))
		mem[sp].ref = zint

	case Popcnt64:
		mem[sp].num = word(bits.OnesCount64(wordUint(mem[sp].num)))
		mem[sp].ref = zint

	case Rotl32:
		k := int(mem[sp].num)
		sp--
		mem[sp].num = word(bits.RotateLeft32(uint32(mem[sp].num), k))
		resetNumRef(&mem[sp])

	case Rotl64:
		k := int(mem[sp].num)
		sp--
		mem[sp].num = uintWord(bits.RotateLeft64(wordUint(mem[sp].num), k))
		resetNumRef(&mem[sp])

	case Rotr32:
		k := int(mem[sp].num)
		sp--
		mem[sp].num = word(bits.RotateLeft32(uint32(mem[sp].num), -k))
		resetNumRef(&mem[sp])

	case Rotr64:
		k := int(mem[sp].num)
		sp--
		mem[sp].num = uintWord(bits.RotateLeft64(wordUint(mem[sp].num), -k))
		resetNumRef(&mem[sp])

		// Float math (unary).

	case AbsFloat32:
		mem[sp].num = putf32(float32(math.Abs(float64(getf32(mem[sp].num)))))
		mem[sp].ref = zfloat32

	case AbsFloat64:
		mem[sp].num = floatWord(math.Abs(wordFloat(mem[sp].num)))
		mem[sp].ref = zfloat64

	case SqrtFloat32:
		mem[sp].num = putf32(float32(math.Sqrt(float64(getf32(mem[sp].num)))))
		mem[sp].ref = zfloat32

	case SqrtFloat64:
		mem[sp].num = floatWord(math.Sqrt(wordFloat(mem[sp].num)))
		mem[sp].ref = zfloat64

	case CeilFloat32:
		mem[sp].num = putf32(float32(math.Ceil(float64(getf32(mem[sp].num)))))
		mem[sp].ref = zfloat32

	case CeilFloat64:
		mem[sp].num = floatWord(math.Ceil(wordFloat(mem[sp].num)))
		mem[sp].ref = zfloat64

	case FloorFloat32:
		mem[sp].num = putf32(float32(math.Floor(float64(getf32(mem[sp].num)))))
		mem[sp].ref = zfloat32

	case FloorFloat64:
		mem[sp].num = floatWord(math.Floor(wordFloat(mem[sp].num)))
		mem[sp].ref = zfloat64

	case TruncFloat32:
		mem[sp].num = putf32(float32(math.Trunc(float64(getf32(mem[sp].num)))))
		mem[sp].ref = zfloat32

	case TruncFloat64:
		mem[sp].num = floatWord(math.Trunc(wordFloat(mem[sp].num)))
		mem[sp].ref = zfloat64

	case NearestFloat32:
		mem[sp].num = putf32(float32(math.RoundToEven(float64(getf32(mem[sp].num)))))
		mem[sp].ref = zfloat32

	case NearestFloat64:
		mem[sp].num = floatWord(math.RoundToEven(wordFloat(mem[sp].num)))
		mem[sp].ref = zfloat64

		// Float math (binary).

	case MinFloat32:
		mem[sp-1].num = putf32(float32(math.Min(float64(getf32(mem[sp-1].num)), float64(getf32(mem[sp].num)))))
		mem[sp-1].ref = zfloat32
		sp--

	case MinFloat64:
		mem[sp-1].num = floatWord(math.Min(wordFloat(mem[sp-1].num), wordFloat(mem[sp].num)))
		mem[sp-1].ref = zfloat64
		sp--

	case MaxFloat32:
		mem[sp-1].num = putf32(float32(math.Max(float64(getf32(mem[sp-1].num)), float64(getf32(mem[sp].num)))))
		mem[sp-1].ref = zfloat32
		sp--

	case MaxFloat64:
		mem[sp-1].num = floatWord(math.Max(wordFloat(mem[sp-1].num), wordFloat(mem[sp].num)))
		mem[sp-1].ref = zfloat64
		sp--

	case CopysignFloat32:
		mem[sp-1].num = putf32(float32(math.Copysign(float64(getf32(mem[sp-1].num)), float64(getf32(mem[sp].num)))))
		mem[sp-1].ref = zfloat32
		sp--

	case CopysignFloat64:
		mem[sp-1].num = floatWord(math.Copysign(wordFloat(mem[sp-1].num), wordFloat(mem[sp].num)))
		mem[sp-1].ref = zfloat64
		sp--

	case CellGet:
		sp++
		v := *mem[int(c.A)+fp-1].ref.Interface().(*Value)
		if isNum(v.ref.Kind()) && v.ref.CanAddr() {
			// setCell writes the cell storage in place, so a numeric snapshot
			// must detach its ref (num carries the payload) or a later write
			// leaks into it (defer incrementing a returned captured var).
			v.num = numBits(v.ref)
			v.ref = reflect.Zero(v.ref.Type())
		}
		mem[sp] = v

	case AddrCell:
		cell := mem[int(c.A)+fp-1].ref.Interface().(*Value)
		switch {
		case c.B != 0:
			m.promoteSlot(cell, m.globals[int(c.B)-1].ref.Type())
		case !cell.ref.CanAddr():
			rt := cell.ref.Type()
			rv := reflect.New(rt).Elem()
			if isNum(rt.Kind()) {
				setNumReflect(rv, cell.num)
			} else {
				rv.Set(cell.ref)
			}
			cell.ref = rv
		}
		if sp+1 >= len(mem) {
			mem = growStack(mem, sp, 1)
		}
		sp++
		mem[sp] = Value{ref: cell.ref.Addr()}

	case CellSet:
		setCell(mem[int(c.A)+fp-1].ref.Interface().(*Value), mem[sp])
		sp--

	case AddrHeap:
		cell := m.heap[int(c.A)]
		switch {
		case c.B != 0:
			m.promoteSlot(cell, m.globals[int(c.B)-1].ref.Type())
		case !cell.ref.CanAddr():
			rt := cell.ref.Type()
			rv := reflect.New(rt).Elem()
			if isNum(rt.Kind()) {
				setNumReflect(rv, cell.num)
			} else {
				rv.Set(cell.ref)
			}
			cell.ref = rv
		}
		if sp+1 >= len(mem) {
			mem = growStack(mem, sp, 1)
		}
		sp++
		mem[sp] = Value{ref: cell.ref.Addr()}

	case HeapPtr:
		if sp+1 >= len(mem) {
			mem = growStack(mem, sp, 1)
		}
		sp++
		mem[sp] = ValueOf(m.heap[int(c.A)])

	case Clear:
		clearValue(mem[sp].Reflect())
		sp--

	case Cap:
		if sp+1 >= len(mem) {
			mem = growStack(mem, sp, 1)
		}
		sp++
		mem[sp] = ValueOf(mem[sp-1-int(c.A)].ref.Cap())

	case PtrNew:
		typ := m.globals[int(c.A)].ref.Type()
		if sp+1 >= len(mem) {
			mem = growStack(mem, sp, 1)
		}
		sp++
		mem[sp] = Value{ref: reflect.New(typ)}

	case SetS:
		n := int(c.A)
		for i := range n {
			if dst, src := &mem[sp-2*n+1+i], mem[sp-n+1+i]; !assignSlotNum(dst, src) {
				m.assignSlot(dst, src)
			}
		}
		sp -= 2 * n

		// Per-type Add.

	case AddStr:
		mem[sp-1] = Value{ref: reflect.ValueOf(mem[sp-1].ref.String() + mem[sp].ref.String())}
		sp--

	case AddInt8:
		mem[sp-1].num = addInt8(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zint8
		sp--

	case AddInt16:
		mem[sp-1].num = addInt16(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zint16
		sp--

	case AddInt32:
		mem[sp-1].num = addInt32(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zint32
		sp--

	case AddInt64:
		mem[sp-1].num = addInt64(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zint64
		sp--

	case AddUint:
		mem[sp-1].num = addUint(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = uintOrUintptr(mem[sp-1].ref)
		sp--

	case AddUint8:
		mem[sp-1].num = addUint8(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zuint8
		sp--

	case AddUint16:
		mem[sp-1].num = addUint16(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zuint16
		sp--

	case AddUint32:
		mem[sp-1].num = addUint32(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zuint32
		sp--

	case AddUint64:
		mem[sp-1].num = addUint64(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zuint64
		sp--

	case AddFloat32:
		mem[sp-1].num = addfFloat32(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zfloat32
		sp--

	case SubInt8:
		mem[sp-1].num = subInt8(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zint8
		sp--

	case SubInt16:
		mem[sp-1].num = subInt16(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zint16
		sp--

	case SubInt32:
		mem[sp-1].num = subInt32(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zint32
		sp--

	case SubInt64:
		mem[sp-1].num = subInt64(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zint64
		sp--

	case SubUint:
		mem[sp-1].num = subUint(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = uintOrUintptr(mem[sp-1].ref)
		sp--

	case SubUint8:
		mem[sp-1].num = subUint8(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zuint8
		sp--

	case SubUint16:
		mem[sp-1].num = subUint16(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zuint16
		sp--

	case SubUint32:
		mem[sp-1].num = subUint32(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zuint32
		sp--

	case SubUint64:
		mem[sp-1].num = subUint64(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zuint64
		sp--

	case SubFloat32:
		mem[sp-1].num = subfFloat32(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zfloat32
		sp--

	case MulInt8:
		mem[sp-1].num = mulInt8(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zint8
		sp--

	case MulInt16:
		mem[sp-1].num = mulInt16(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zint16
		sp--

	case MulInt32:
		mem[sp-1].num = mulInt32(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zint32
		sp--

	case MulInt64:
		mem[sp-1].num = mulInt64(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zint64
		sp--

	case MulUint:
		mem[sp-1].num = mulUint(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = uintOrUintptr(mem[sp-1].ref)
		sp--

	case MulUint8:
		mem[sp-1].num = mulUint8(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zuint8
		sp--

	case MulUint16:
		mem[sp-1].num = mulUint16(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zuint16
		sp--

	case MulUint32:
		mem[sp-1].num = mulUint32(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zuint32
		sp--

	case MulUint64:
		mem[sp-1].num = mulUint64(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zuint64
		sp--

	case MulFloat32:
		mem[sp-1].num = mulfFloat32(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zfloat32
		sp--

	case DivInt8:
		mem[sp-1].num = divInt8(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zint8
		sp--

	case DivInt16:
		mem[sp-1].num = divInt16(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zint16
		sp--

	case DivInt32:
		mem[sp-1].num = divInt32(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zint32
		sp--

	case DivInt64:
		mem[sp-1].num = divInt64(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zint64
		sp--

	case DivUint:
		mem[sp-1].num = divUint(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = uintOrUintptr(mem[sp-1].ref)
		sp--

	case DivUint8:
		mem[sp-1].num = divUint8(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zuint8
		sp--

	case DivUint16:
		mem[sp-1].num = divUint16(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zuint16
		sp--

	case DivUint32:
		mem[sp-1].num = divUint32(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zuint32
		sp--

	case DivUint64:
		mem[sp-1].num = divUint64(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zuint64
		sp--

	case DivFloat32:
		mem[sp-1].num = divfFloat32(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zfloat32
		sp--

	case RemInt8:
		mem[sp-1].num = remInt8(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zint8
		sp--

	case RemInt16:
		mem[sp-1].num = remInt16(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zint16
		sp--

	case RemInt32:
		mem[sp-1].num = remInt32(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zint32
		sp--

	case RemInt64:
		mem[sp-1].num = remInt64(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zint64
		sp--

	case RemUint:
		mem[sp-1].num = remUint(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = uintOrUintptr(mem[sp-1].ref)
		sp--

	case RemUint8:
		mem[sp-1].num = remUint8(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zuint8
		sp--

	case RemUint16:
		mem[sp-1].num = remUint16(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zuint16
		sp--

	case RemUint32:
		mem[sp-1].num = remUint32(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zuint32
		sp--

	case RemUint64:
		mem[sp-1].num = remUint64(mem[sp-1].num, mem[sp].num)
		mem[sp-1].ref = zuint64
		sp--

		// Per-type Neg.

	case NegInt8:
		mem[sp].num = negInt8(mem[sp].num)
		mem[sp].ref = zint8

	case NegInt16:
		mem[sp].num = negInt16(mem[sp].num)
		mem[sp].ref = zint16

	case NegInt32:
		mem[sp].num = negInt32(mem[sp].num)
		mem[sp].ref = zint32

	case NegInt64:
		mem[sp].num = negInt64(mem[sp].num)
		mem[sp].ref = zint64

	case NegUint:
		mem[sp].num = negUint(mem[sp].num)
		mem[sp].ref = uintOrUintptr(mem[sp].ref)

	case NegUint8:
		mem[sp].num = negUint8(mem[sp].num)
		mem[sp].ref = zuint8

	case NegUint16:
		mem[sp].num = negUint16(mem[sp].num)
		mem[sp].ref = zuint16

	case NegUint32:
		mem[sp].num = negUint32(mem[sp].num)
		mem[sp].ref = zuint32

	case NegUint64:
		mem[sp].num = negUint64(mem[sp].num)
		mem[sp].ref = zuint64

	case NegFloat32:
		mem[sp].num = negfFloat32(mem[sp].num)
		mem[sp].ref = zfloat32

	case NegFloat64:
		mem[sp].num = negfFloat64(mem[sp].num)
		mem[sp].ref = zfloat64

		// String Greater / Lower.

	case GreaterUint, GreaterUint8, GreaterUint16, GreaterUint32, GreaterUint64:
		mem[sp-1] = boolVal(mem[sp-1].num > mem[sp].num)
		sp--

	case LowerUint, LowerUint8, LowerUint16, LowerUint32, LowerUint64:
		mem[sp-1] = boolVal(mem[sp-1].num < mem[sp].num)
		sp--

	case GreaterEqualFloat:
		mem[sp-1] = boolVal(wordFloat(mem[sp-1].num) >= wordFloat(mem[sp].num))
		sp--

	case LowerEqualFloat:
		mem[sp-1] = boolVal(wordFloat(mem[sp-1].num) <= wordFloat(mem[sp].num))
		sp--

		// Immediate operand ops: right-hand constant is in Arg[0].

	case GreaterUintImm:
		mem[sp] = boolVal(uint(mem[sp].num) > uint(int(c.A)))

	case LowerUintImm:
		mem[sp] = boolVal(uint(mem[sp].num) < uint(int(c.A)))

	case DeferRet:
		mem, sp, ip = m.deferRet(mem, fp, sp)
		m.tierMem, m.tierIP, m.tierFP, m.tierSP, m.tierSafepointCtr = mem, ip, fp, sp, safepointCtr
		return coldContinue, false, nil

	case PanicUnwind:
		if done, uerr := m.panicUnwind(&mem, &fp, &sp, &ip, panicAddr); done {
			m.tierMem, m.tierIP, m.tierFP, m.tierSP, m.tierSafepointCtr = mem, ip, fp, sp, safepointCtr
			return coldReturn, true, uerr
		}
		m.tierMem, m.tierIP, m.tierFP, m.tierSP, m.tierSafepointCtr = mem, ip, fp, sp, safepointCtr
		return coldContinue, false, nil

	}
	m.tierMem, m.tierIP, m.tierFP, m.tierSP, m.tierSafepointCtr = mem, ip, fp, sp, safepointCtr
	return coldNext, false, nil
}
