package vm

import (
	"fmt"
	"iter"
	"reflect"

	"github.com/mvm-sh/mvm/internal/derive"
	"github.com/mvm-sh/mvm/internal/runtype"
	"github.com/mvm-sh/mvm/mtype"
)

// coldCtl tells runLoop what to do after runWarm or runCold ran an opcode.
type coldCtl int

const (
	coldNext     coldCtl = iota // advance to the next instruction
	coldContinue                // resume the loop at ip, already set
	coldReturn                  // return from runLoop with done and err
)

// runWarm runs the opcodes of c that are common but not hot enough for
// runLoop, which must stay small enough for the JavaScript JIT to optimize
// it under goscript (V8 does not optimize a function of more than 60 KB of
// bytecode): calls through interfaces and the opcodes of maps, slices and
// type assertions. runWarm is kept under that size too, apart from the
// rare opcodes in runCold. It shares runLoop's state through pointers,
// written back at each return. An opcode that panics leaves runLoop the
// state from before it, from which runLoop's recovery unwinds as from the
// faulting instruction; a deferred write-back would cost each call under
// goscript. An opcode moves between the functions with its case.
func (m *Machine) runWarm(c *Instruction, pmem *[]Value, pip, pfp, psp *int, psafe *uint,
	traceFlags uint8, panicAddr, deferRetAddr int, deferRetBits word,
) (ctl coldCtl, done bool, err error) {
	mem, ip, fp, sp, safepointCtr := *pmem, *pip, *pfp, *psp, *psafe
	switch c.Op {
	case Addr:
		v := mem[sp]
		switch {
		case v.ref.IsValid() && v.ref.Type() == ifaceRtype:
			// An mvm Iface struct is not a Go eface, so &iface for unsafe access
			// (protobuf's pointerOfIface) must alias a real *interface{}. Before
			// CanAddr since an addressable Iface struct has the same wrong layout.
			// Struct form only: a real interface{} var keeps CanAddr below so writes
			// through &iface propagate.
			// bridgeIface (not raw Reflect) so a boxed func materializes as its
			// native MakeFunc wrapper: the ccgo __ccgo_fp idiom reads &f as
			// [2]uintptr and calls word[1] as a *funcval, which must be a real
			// trampoline (registered in funcFields), not a bare code address.
			r := reflect.New(mtype.AnyRtype)
			if cv := m.bridgeIface(v.IfaceVal(), mtype.AnyRtype); cv.IsValid() {
				r.Elem().Set(cv)
			}
			mem[sp] = Value{ref: r}
		case v.ref.CanAddr():
			mem[sp] = Value{ref: v.ref.Addr()}
		case isNum(v.ref.Kind()):
			// Materialize via Reflect() to get an addressable value, then take its address.
			mem[sp] = Value{ref: v.Reflect().Addr()}
		case v.IsIface():
			// Non-addressable interface{} holding an Iface: no slot to alias.
			// bridgeIface so a boxed func materializes as its native wrapper
			// (see the ifaceRtype branch above).
			r := reflect.New(mtype.AnyRtype)
			if cv := m.bridgeIface(v.IfaceVal(), mtype.AnyRtype); cv.IsValid() {
				r.Elem().Set(cv)
			}
			mem[sp] = Value{ref: r}
		case !v.ref.IsValid():
			// Nil interface parameter: allocate *interface{} with zero value.
			mem[sp] = Value{ref: reflect.New(mtype.AnyRtype)}
		default:
			// Non-numeric, non-addressable composite (e.g. string parameter):
			// allocate addressable storage and copy.
			r := reflect.New(v.ref.Type())
			r.Elem().Set(v.ref)
			mem[sp] = Value{ref: r}
		}

	case Deref:
		r := mem[sp].ref.Elem()
		if !r.IsValid() {
			// *nil: recoverable nil deref, not a raw reflect panic.
			m.raiseNilDeref()
			ip = m.stageUnwind(ip, fp, mem)
			*pmem, *pip, *pfp, *psp, *psafe = mem, ip, fp, sp, safepointCtr
			return coldContinue, false, nil
		}
		v := Value{ref: r}
		if isNum(r.Kind()) {
			v.num = numBits(r)
		}
		mem[sp] = v

	case DerefSet:
		ptr := mem[sp-1]
		val := mem[sp]
		elem := ptr.ref.Elem()
		sp -= 2
		if !elem.IsValid() {
			// *nil = v: recoverable nil deref, not a raw reflect panic.
			m.raiseNilDeref()
			ip = m.stageUnwind(ip, fp, mem)
			*pmem, *pip, *pfp, *psp, *psafe = mem, ip, fp, sp, safepointCtr
			return coldContinue, false, nil
		}
		if elem.Kind() == reflect.Func && elem.CanAddr() {
			// *p = f: wrap the mvm func value into a Go func (reflect.Set
			// rejects the Closure/code-address representation).
			m.setFuncField(elem, val)
			break
		}
		if elem.Kind() == reflect.Interface {
			// *p = iface into a narrow interface (*error): an interface{} box is
			// not assignable, so reuse assignSlot's bridge/unwrap (elem is settable).
			cell := Value{ref: elem}
			if dst, src := &cell, val; !assignSlotNum(dst, src) {
				m.assignSlot(dst, src)
			}
			break
		}
		numSet(elem, val)
		// Update the .num cache of any stack slot whose ref shares the
		// same underlying address, so fused GetLocal*Imm instructions and
		// num-first reads see the updated value. Scan ALL frames on the
		// stack, not just the current one: a pointer-receiver method that
		// mutates `*v` must propagate to the caller's slot (the typical
		// `(*X).Set` pattern on `type X int`).
		if isNum(elem.Kind()) {
			addr := elem.UnsafeAddr()
			n := numBits(elem)
			for i := 0; i <= sp; i++ {
				if mem[i].ref.IsValid() && mem[i].ref.CanAddr() && mem[i].ref.UnsafeAddr() == addr {
					mem[i].num = n
				}
			}
		}

	case AddrLocal:
		slot := &mem[int(c.A)+fp-1]
		switch {
		case c.B != 0:
			// Func/interface slots are interface{} boxes; retype to the declared
			// type ($2-1) so &f is *func(...) and &iface a *T into heap storage,
			// not a *interface{} aliasing the volatile frame slot.
			m.promoteSlot(slot, m.globals[int(c.B)-1].ref.Type())
		case !slot.ref.CanAddr():
			// Promote to addressable storage so the pushed pointer aliases
			// the slot. DerefSet keeps slot.num in sync on writes.
			rt := slot.ref.Type()
			rv := reflect.New(rt).Elem()
			if isNum(rt.Kind()) {
				setNumReflect(rv, slot.num)
			} else {
				rv.Set(slot.ref)
			}
			slot.ref = rv
		}
		sp++
		mem[sp] = Value{ref: slot.ref.Addr()}

	case AddrGlobal:
		slot := &m.globals[int(c.A)]
		switch {
		case c.B != 0:
			m.promoteSlot(slot, m.globals[int(c.B)-1].ref.Type())
		case !slot.ref.CanAddr():
			rt := slot.ref.Type()
			rv := reflect.New(rt).Elem()
			if isNum(rt.Kind()) {
				setNumReflect(rv, slot.num)
			} else {
				rv.Set(slot.ref)
			}
			slot.ref = rv
		}
		if sp+1 >= len(mem) {
			mem = growStack(mem, sp, 1)
		}
		sp++
		mem[sp] = Value{ref: slot.ref.Addr()}

	case EqualSet:
		if mem[sp-1].Equal(mem[sp]) {
			// If equal then lhs and rhs are popped, replaced by test result, as in Equal.
			mem[sp-1] = boolVal(true)
			sp--
		} else {
			// If not equal then the lhs is let on stack for further processing.
			// This is used to simplify bytecode in case clauses of switch statments.
			mem[sp] = boolVal(false)
		}

	case IfaceWrap:
		typ := m.globals[int(c.A)].ref.Interface().(*mtype.Type)
		idx := sp - int(c.B)
		v := mem[idx]
		// Assigning a value to an interface copies it (Go spec).
		// Without this clone, the wrapped Iface.Val.ref would alias the source
		// slot's storage and later mutations to that slot would leak through
		// the interface (e.g. compact.Make stores `tag.full = t` and a caller
		// mutating t later would see the change inside tag.full).
		// Struct/array always copy; other kinds (map/slice headers in an
		// addressable local cell) copy when the slot is writable, else a
		// later reassignment of the local mutates the boxed value.
		// Numeric kinds are exempt: their payload is snapshotted in v.num
		// (ref is type metadata), so cloning would only waste an alloc.
		if v.ref.IsValid() {
			if k := v.ref.Kind(); k == reflect.Struct || k == reflect.Array || (!isNum(k) && v.ref.CanSet()) {
				nv := reflect.New(v.ref.Type()).Elem()
				nv.Set(runtype.Exportable(v.ref))
				v.ref = nv
			}
		}
		mem[idx] = Value{ref: reflect.ValueOf(Iface{Typ: typ, Val: v})}

	case IfaceCall:
		methodID := int(c.A)
		if !mem[sp].IsIface() {
			// A native value whose concrete rtype maps back to an
			// interpreted *Type carrying a compiled method (e.g. an
			// interpreted struct round-tripped through encoding/gob into an
			// interface) must dispatch through that type's method -- the
			// StructOf rtype carries no native methods. Gate on the rtype
			// having no native method for this call: defined types over a
			// bridged stdlib struct (type ipNetValue net.IPNet) share that
			// struct's rtype, and re-wrapping a genuine native receiver
			// there would hijack the call back into the interpreted method
			// and recurse forever.
			if rv := mem[sp].Reflect(); rv.IsValid() {
				if rv.Kind() == reflect.Interface && !rv.IsNil() {
					rv = rv.Elem()
				}
				rt := rv.Type()
				// A synth rtype's native methods are stubs back into the
				// interpreter; prefer direct compiled-method dispatch. It
				// keeps an interface-typed result in mvm Iface form rather
				// than losing it in the stub's result marshaling (a synth
				// iface the concrete synth rtype does not implement).
				if !hasNativeMethod(rt, m.MethodNames[methodID]) || isSynthOrSynthPtr(rt) ||
					embeddedIfaceStub(rt, m.MethodNames[methodID]) {
					if t := m.typeByRtype(rt); t != nil && t.ResolveMethodType(methodID) != nil {
						// FromReflect, not Value{ref: rv}: a numeric concrete (rv from
						// iface.Elem()) is non-addressable, so its data must go in num
						// or the value-receiver body reads a stale zero.
						mem[sp] = Value{ref: reflect.ValueOf(Iface{Typ: t, Val: FromReflect(rv)})}
					}
				}
			}
		}
		if !mem[sp].IsIface() {
			// Native interface value: use reflect to get the method.
			methodName := m.MethodNames[methodID]
			recvHint := c.B &^ IfaceCallDetachBit
			recvRV := mem[sp].Reflect()
			if isNilReceiver(recvRV) {
				m.raiseNilDeref()
				ip = m.stageUnwind(ip, fp, mem)
				*pmem, *pip, *pfp, *psp, *psafe = mem, ip, fp, sp, safepointCtr
				return coldContinue, false, nil
			}
			// defer/go: capture the receiver by value, not its slot.
			if c.B&IfaceCallDetachBit != 0 && recvRV.CanAddr() {
				cp := reflect.New(recvRV.Type()).Elem()
				cp.Set(runtype.Exportable(recvRV))
				recvRV = cp
			}
			// Native method table (ADR-023): reuse a cached unbound func.
			if nativeMethodTables && recvHint == 0 && c.B&IfaceCallDetachBit == 0 &&
				recvRV.IsValid() && recvRV.Kind() != reflect.Interface {
				if d := m.nativeMethodDesc(recvRV.Type(), methodID, methodName); d.ok && (!d.needAddr || recvRV.CanAddr()) {
					recv := recvRV
					if d.needAddr {
						recv = recvRV.Addr()
					}
					mem[sp] = Value{ref: reflect.ValueOf(cachedNativeCall{Unbound: d.fn, Recv: runtype.Exportable(recv)})}
					break
				}
			}
			if mv, ok := m.bindPromotedNative(recvRV, methodName); ok {
				mem[sp] = Value{ref: mv}
				break
			}
			rv := nativeMethodLookup(m, recvRV, methodName)
			if !rv.IsValid() && recvHint != 0 {
				// Numeric value lost its named type (e.g. time.Duration stored as int64).
				// Convert to the named type encoded in B-1 and retry the method lookup.
				namedType := m.globals[int(recvHint)-1].ref.Type()
				rv = runtype.ValueMethodByName(recvRV.Convert(namedType), methodName)
			}
			if rv.IsValid() && recvRV.IsValid() &&
				hasNativeMethodHook(recvRV.Type(), methodName) {
				mem[sp] = Value{ref: reflect.ValueOf(boundHookCall{Fn: rv, RecvType: recvRV.Type(), Method: methodName, Recv: recvRV})}
				break
			}
			mem[sp] = Value{ref: rv}
			break
		}
		ifc := mem[sp].IfaceVal()
		methodTyp := ifc.Typ.ResolveMethodType(methodID)
		if methodTyp == nil {
			if ct := m.canonicalDispatchType(ifc.Val, methodID); ct != nil {
				ifc.Typ = ct
				methodTyp = ct.ResolveMethodType(methodID)
			}
		}
		if methodTyp == nil {
			rv := ifc.Val.Reflect()
			if !rv.IsValid() {
				m.raiseNilDeref()
				ip = m.stageUnwind(ip, fp, mem)
				*pmem, *pip, *pfp, *psp, *psafe = mem, ip, fp, sp, safepointCtr
				return coldContinue, false, nil
			}
			if mv, ok := m.bindPromotedNative(rv, m.MethodNames[methodID]); ok {
				mem[sp] = Value{ref: mv}
				break
			}
			mem[sp] = Value{ref: nativeMethodLookup(m, rv, m.MethodNames[methodID])}
			break
		}
		method := methodTyp.Methods[methodID]
		const (
			outChain = iota
			outNative
			outNilRcv
		)
		outcome := outChain
		for method.EmbedIface {
			rv := ifc.Val.Reflect()
			if rv.Kind() == reflect.Pointer {
				rv = rv.Elem()
			}
			nilHop := false
			for _, fi := range method.Path {
				if !rv.IsValid() {
					// nil pointer receiver: promoted access derefs nil.
					nilHop = true
					break
				}
				rv = rv.Field(fi)
			}
			if nilHop {
				outcome = outNilRcv
				break
			}
			embedded := FromReflect(rv)
			if !embedded.IsIface() {
				// Embedded field native-boxed (lost its Iface): re-wrap to the
				// interpreted *Type and continue, as the IfaceCall re-wrap above.
				erv := rv
				if erv.Kind() == reflect.Interface && !erv.IsNil() {
					erv = erv.Elem()
				}
				if erv.IsValid() &&
					(!hasNativeMethod(erv.Type(), m.MethodNames[methodID]) || isSynthOrSynthPtr(erv.Type()) ||
						embeddedIfaceStub(erv.Type(), m.MethodNames[methodID])) {
					if t := m.typeByRtype(erv.Type()); t != nil {
						if mt := t.ResolveMethodType(methodID); mt != nil {
							methodTyp = mt
							ifc = Iface{Typ: t, Val: FromReflect(erv)}
							method = mt.Methods[methodID]
							continue
						}
					}
				}
				if isNilReceiver(rv) {
					outcome = outNilRcv
				} else if mv, ok := m.bindPromotedNative(rv, m.MethodNames[methodID]); ok {
					mem[sp] = Value{ref: mv}
					outcome = outNative
				} else {
					mem[sp] = Value{ref: nativeMethodLookup(m, rv, m.MethodNames[methodID])}
					outcome = outNative
				}
				break
			}
			ifc = embedded.IfaceVal()
			method = ifc.Typ.Methods[methodID]
		}
		if outcome == outNilRcv {
			m.raiseNilDeref()
			ip = m.stageUnwind(ip, fp, mem)
			*pmem, *pip, *pfp, *psp, *psafe = mem, ip, fp, sp, safepointCtr
			return coldContinue, false, nil
		}
		if outcome == outNative {
			break
		}
		codeAddr := int(m.globals[method.Index].num)
		// Build a closure with the concrete receiver as Heap[0], replacing the
		// interface value on the stack. Same result as HeapAlloc+Get+Swap+MkClosure.
		// For promoted methods, extract the embedded field as receiver.
		// The fused path packs the cell + 1-entry heap in one alloc (see methodFrame).
		var mf *methodFrame
		var cell *Value
		if fusedMethodFrame {
			mf = &methodFrame{code: codeAddr}
			cell = &mf.cell
		} else {
			cell = new(Value)
		}
		*cell = ifc.Val
		if path := method.Path; path != nil {
			rv := reflect.Indirect(ifc.Val.Reflect())
			nilHop := false
			for _, idx := range path {
				if rv.Kind() == reflect.Pointer {
					rv = rv.Elem()
				}
				if !rv.IsValid() {
					// Promoted method on a nil pointer: navigating to the
					// embedded field derefs nil, a recoverable nil-pointer
					// dereference in Go, not a raw reflect Field panic.
					nilHop = true
					break
				}
				rv = rv.Field(idx)
			}
			if nilHop {
				m.raiseNilDeref()
				ip = m.stageUnwind(ip, fp, mem)
				*pmem, *pip, *pfp, *psp, *psafe = mem, ip, fp, sp, safepointCtr
				return coldContinue, false, nil
			}
			// A ptr-recv method needs the pointer in the cell: with the bare
			// struct, passing the receiver to an inner call detaches a copy.
			if method.PtrRecv && rv.Kind() != reflect.Pointer && rv.CanAddr() {
				rv = rv.Addr()
			}
			*cell = FromReflect(rv)
		} else if methodTyp == ifc.Typ.ElemType && !method.PtrRecv && ifc.Val.ref.Kind() == reflect.Pointer {
			// Value-receiver method found by walking *T -> T (ResolveMethodType).
			// The iface holds *T but the method body expects T; deref so the body's
			// receiver storage is the value, not the pointer. PtrRecv is reliable
			// here because the method was registered directly on the value type
			// (comp/compiler.go where it sets PtrRecv from the receiver token).
			*cell = FromReflect(ifc.Val.Reflect().Elem())
		}
		// A value receiver is a copy: detach struct/array cells so the body
		// gets an addressable receiver (an iface unbox is read-only) and the
		// caller stays unmutated, matching detachByValueArgs at Call.
		if !method.PtrRecv && cell.ref.IsValid() {
			if k := cell.ref.Kind(); k == reflect.Struct || k == reflect.Array {
				nv := reflect.New(cell.ref.Type()).Elem()
				nv.Set(runtype.Exportable(cell.ref))
				cell.ref = nv
			}
		} else if cell.ref.IsValid() && !cell.ref.CanInterface() {
			// forceSettable rescues writes through a flagRO ptr receiver but not reads of it.
			cell.ref = runtype.Exportable(cell.ref)
		}
		if mf != nil {
			mf.slot[0] = cell
			mem[sp] = Value{ref: reflect.ValueOf(mf)}
		} else {
			mem[sp] = Value{ref: reflect.ValueOf(Closure{Code: codeAddr, Heap: []*Value{cell}})}
		}

	case TypeAssert:
		dstTyp := m.globals[int(c.A)].ref.Interface().(*mtype.Type)
		okForm := int(c.B) == 1
		ifc := mem[sp]
		if !ifc.IsIface() {
			// Native interface value: use reflect for type assertion.
			rv := ifc.Reflect()
			isNil := !rv.IsValid()
			ifaceTyp := mtype.AnyRtype
			if !isNil && rv.Kind() == reflect.Interface {
				ifaceTyp = rv.Type()
				isNil = rv.IsNil()
				if !isNil {
					rv = rv.Elem()
				}
			}
			// For interface targets, check the method set directly. AssignableTo
			// would yield a false positive when dstTyp.Rtype is AnyRtype (the
			// reflect placeholder for user-defined interfaces) since every type
			// is assignable to `interface{}`.
			matched := false
			var wrapTyp *mtype.Type // non-nil => wrap result as Iface for interpreted-method dispatch
			if !isNil {
				if dstTyp.IsInterface() {
					matched = dstTyp.NativeImplements(rv.Type())
				} else {
					dstRT := derive.MaterializeRtype(dstTyp)
					matched = (dstRT != nil && rv.Type().AssignableTo(dstRT)) || dstTyp.NativeImplements(rv.Type())
				}
				// Interpreted concrete type round-tripped through native reflect (e.g.
				// reflect.Value.Interface()): its synthetic rtype carries no native
				// methods, so the checks above miss interpreted methods. Recover the
				// *Type and consult mvm's method tables (mirrors IfaceCall above).
				if !matched {
					if ct := m.typeByRtype(rv.Type()); ct != nil {
						if dstTyp.IsInterface() {
							if ct.Implements(dstTyp) {
								matched, wrapTyp = true, ct
							}
						} else if ct.SameAs(dstTyp) {
							matched = true
						}
					}
				}
				// Wrap an interface-target match in an Iface so a bare typed-nil
				// pointer keeps its type instead of collapsing to a nil interface.
				if matched && wrapTyp == nil && dstTyp.IsInterface() {
					if ct := m.typeByRtype(rv.Type()); ct != nil {
						wrapTyp = ct
					}
				}
			}
			if matched {
				if wrapTyp != nil {
					mem[sp] = Value{ref: reflect.ValueOf(Iface{Typ: wrapTyp, Val: FromReflect(rv)})}
				} else {
					mem[sp] = FromReflect(rv)
				}
				if okForm {
					if sp+1 >= len(mem) {
						mem = growStack(mem, sp, 1)
					}
					sp++
					mem[sp] = boolVal(true)
				}
				break
			}
			if !okForm {
				var msg string
				switch {
				case isNil:
					msg = fmt.Sprintf("interface conversion: %s is nil, not %s", ifaceTyp, dstTyp)
				case dstTyp.IsInterface():
					missing := dstTyp.MissingMethod(rv.Type())
					msg = fmt.Sprintf("interface conversion: %s is not %s: missing method %s", rv.Type(), dstTyp, missing)
				default:
					msg = fmt.Sprintf("interface conversion: %s is %s, not %s", ifaceTyp, rv.Type(), dstTyp)
				}
				m.panicking = true
				m.panicVal = Value{ref: reflect.ValueOf(m.posPrefix(c.Pos) + msg)}
				sp--
				ip = m.stageUnwind(ip, fp, mem)
				*pmem, *pip, *pfp, *psp, *psafe = mem, ip, fp, sp, safepointCtr
				return coldContinue, false, nil
			}
			mem[sp] = NewValue(dstTyp.Rtype)
			if sp+1 >= len(mem) {
				mem = growStack(mem, sp, 1)
			}
			sp++
			mem[sp] = boolVal(false)
			break
		}
		concrete := ifc.IfaceVal()
		var matched bool
		dstIsIface := dstTyp.IsInterface()
		if dstIsIface {
			matched = concrete.Typ.Implements(dstTyp)
		} else {
			matched = concrete.Typ.SameAs(dstTyp)
		}
		if matched {
			// For interface targets, keep the Iface wrapping so IfaceCall still works.
			result := concrete.Val
			if dstIsIface {
				result = ifc
			}
			if okForm {
				mem[sp] = result
				if sp+1 >= len(mem) {
					mem = growStack(mem, sp, 1)
				}
				sp++
				mem[sp] = boolVal(true)
			} else {
				mem[sp] = result
			}
		} else {
			if !okForm {
				var msg string
				if dstIsIface {
					missing := dstTyp.MissingMethod(concrete.Typ.Rtype)
					msg = fmt.Sprintf("interface conversion: %s is not %s: missing method %s", concrete.Typ, dstTyp, missing)
				} else {
					msg = fmt.Sprintf("interface conversion: %s is %s, not %s", mtype.AnyRtype, concrete.Typ, dstTyp)
				}
				m.panicking = true
				m.panicVal = Value{ref: reflect.ValueOf(m.posPrefix(c.Pos) + msg)}
				sp--
				ip = m.stageUnwind(ip, fp, mem)
				*pmem, *pip, *pfp, *psp, *psafe = mem, ip, fp, sp, safepointCtr
				return coldContinue, false, nil
			}
			mem[sp] = NewValue(dstTyp.Rtype)
			if sp+1 >= len(mem) {
				mem = growStack(mem, sp, 1)
			}
			sp++
			mem[sp] = boolVal(false)
		}

	case TypeBranch: // Arg[0]=offset, Arg[1]=typeIdx (-1 for nil case)
		ifc := mem[sp]
		sp--
		var dtyp *mtype.Type
		if int(c.B) != -1 {
			dtyp = m.globals[int(c.B)].ref.Interface().(*mtype.Type)
		}
		var matched bool
		if ifc.IsIface() {
			if dtyp != nil {
				ctyp := ifc.IfaceVal().Typ
				if dtyp.IsInterface() {
					matched = ctyp.Implements(dtyp)
				} else {
					matched = ctyp.SameAs(dtyp)
				}
			}
		} else if rv := ifc.Reflect(); rv.IsValid() && rv.Kind() == reflect.Interface && !rv.IsNil() {
			// Native interface value (e.g. from json.Unmarshal map).
			if dtyp != nil {
				concrete := rv.Elem()
				switch {
				case dtyp.IsInterface() && dtyp.Rtype.NumMethod() > 0:
					// Genuine native interface: its rtype carries the method set.
					matched = concrete.Type().Implements(dtyp.Rtype)
				case dtyp.IsInterface():
					// Interpreted (or empty) interface target: dtyp.Rtype is the
					// methodless interface{} placeholder, so reflect.Implements would
					// false-positive. Recover the concrete *Type and consult mvm's
					// method tables; fall back to a name-based check for native concretes.
					if ct := m.typeByRtype(concrete.Type()); ct != nil {
						matched = ct.Implements(dtyp)
					} else {
						matched = dtyp.NativeImplements(concrete.Type())
					}
				default:
					// A type-switch case matches the exact dynamic type, not an
					// assignable one: net.IP (underlying []byte) must not match a
					// `case []byte`. Use identity-aware SameAs, falling back to rtype
					// equality for pure-native concretes.
					if ct := m.typeByRtype(concrete.Type()); ct != nil {
						matched = ct.SameAs(dtyp)
					} else {
						dtypRT := derive.MaterializeRtype(dtyp)
						matched = dtypRT != nil && concrete.Type() == dtypRT
					}
				}
			}
		} else {
			// Nil or invalid value: only matches the nil case.
			matched = dtyp == nil
		}
		if !matched {
			ip += int(c.A)
			*pmem, *pip, *pfp, *psp, *psafe = mem, ip, fp, sp, safepointCtr
			return coldContinue, false, nil
		}

	case Next:
		if e := &m.iterStack[len(m.iterStack)-1]; e.indexed {
			k, ok := e.nextIndexed()
			if !ok {
				ip += int(c.A)
				*pmem, *pip, *pfp, *psp, *psafe = mem, ip, fp, sp, safepointCtr
				return coldContinue, false, nil
			}
			assignSlotNum(&m.globals[int(c.B)], k)
			break
		}
		if k, ok := m.iterStack[len(m.iterStack)-1].next.ref.Interface().(func() (reflect.Value, bool))(); ok {
			if dst, src := &m.globals[int(c.B)], FromReflect(k); !assignSlotNum(dst, src) {
				m.assignSlot(dst, src)
			}
		} else {
			ip += int(c.A)
			*pmem, *pip, *pfp, *psp, *psafe = mem, ip, fp, sp, safepointCtr
			return coldContinue, false, nil
		}

	case Next2:
		if e := &m.iterStack[len(m.iterStack)-1]; e.indexed {
			i := e.i
			k, ok := e.nextIndexed()
			if !ok {
				ip += int(c.A)
				*pmem, *pip, *pfp, *psp, *psafe = mem, ip, fp, sp, safepointCtr
				return coldContinue, false, nil
			}
			kAddr, vAddr := int(int16(c.B)), int(int16(c.B>>16))
			assignSlotNum(&m.globals[kAddr], k)
			if dst, src := &m.globals[vAddr], FromReflect(e.seq.Index(i)); !assignSlotNum(dst, src) {
				m.assignSlot(dst, src)
			}
			break
		}
		if k, v, ok := m.iterStack[len(m.iterStack)-1].next.ref.Interface().(func() (reflect.Value, reflect.Value, bool))(); ok {
			kAddr, vAddr := int(int16(c.B)), int(int16(c.B>>16))
			if dst, src := &m.globals[kAddr], FromReflect(k); !assignSlotNum(dst, src) {
				m.assignSlot(dst, src)
			}
			if dst, src := &m.globals[vAddr], FromReflect(v); !assignSlotNum(dst, src) {
				m.assignSlot(dst, src)
			}
		} else {
			ip += int(c.A)
			*pmem, *pip, *pfp, *psp, *psafe = mem, ip, fp, sp, safepointCtr
			return coldContinue, false, nil
		}

	case Pull:
		v := mem[sp]
		if v.IsIface() {
			v = v.IfaceVal().Val
		}
		seq := emptySeq // invalid (nil slice/map/string) -> empty range, as in Go
		if v.ref.IsValid() {
			if c.A&1 != 0 {
				v = v.CopyArray()
			}
			if e, ok := indexedIter(fp, v, false); ok && c.B == 0 {
				m.iterStack = append(m.iterStack, e)
				sp -= int(c.A>>1) + 1 // drop the n dead loop-var values + the subject
				break
			}
			fromSeq2 := false
			if c.B != 0 {
				// Range-over-func: wrap a mvm Closure into a native Go func.
				funcType := m.globals[int(c.B)-1].ref.Type()
				v = Value{ref: m.wrapForFunc(v, funcType)}
				fromSeq2 = funcType.In(0).NumIn() == 2 // yield takes (K, V)
			}
			if fromSeq2 {
				// iter.Seq2 ranged with fewer than two iteration variables:
				// iterate as a Seq2 but bind only the first yielded value
				// (Go semantics for `for k := range seq2`).
				seq2 := v.Seq2()
				seq = func(yield func(reflect.Value) bool) {
					seq2(func(k, _ reflect.Value) bool { return yield(k) })
				}
			} else {
				seq = v.Seq()
			}
		}
		next, stop := iter.Pull(seq)
		m.iterStack = append(m.iterStack, iterEntry{fp: fp, next: ValueOf(next), stop: ValueOf(stop)})
		sp -= int(c.A>>1) + 1 // drop the n dead loop-var values + the subject

	case Pull2:
		v := mem[sp]
		if v.IsIface() {
			v = v.IfaceVal().Val
		}
		seq2 := emptySeq2 // invalid (nil slice/map) -> empty range, as in Go
		if v.ref.IsValid() {
			if c.A&1 != 0 {
				v = v.CopyArray()
			}
			if e, ok := indexedIter(fp, v, true); ok && c.B == 0 {
				m.iterStack = append(m.iterStack, e)
				sp -= int(c.A>>1) + 1 // drop the n dead loop-var values + the subject
				break
			}
			if c.B != 0 {
				funcType := m.globals[int(c.B)-1].ref.Type()
				v = Value{ref: m.wrapForFunc(v, funcType)}
			}
			// No Seq-vs-Seq2 adapter needed here (unlike Pull): two iteration
			// variables require a yield arity of 2, so the func is always a Seq2.
			seq2 = v.Seq2()
		}
		next, stop := iter.Pull2(seq2)
		m.iterStack = append(m.iterStack, iterEntry{fp: fp, next: ValueOf(next), stop: ValueOf(stop)})
		sp -= int(c.A>>1) + 1 // drop the n dead loop-var values + the subject

	case Slice:
		low := int(mem[sp-1].num)
		high := int(mem[sp].num)
		mem[sp-2] = Value{ref: derefArray(mem[sp-2].ref).Slice(low, high)}
		sp -= 2

	case Slice3:
		low := int(mem[sp-2].num)
		high := int(mem[sp-1].num)
		hi := int(mem[sp].num)
		mem[sp-3] = Value{ref: derefArray(mem[sp-3].ref).Slice3(low, high, hi)}
		sp -= 3

	case Stop:
		m.popIterStop()
		// Generic bitwise.

	case BitAnd:
		mem[sp-1].num = bitAnd(mem[sp-1].num, mem[sp].num)
		resetNumRef(&mem[sp-1])
		sp--

	case BitOr:
		mem[sp-1].num = bitOr(mem[sp-1].num, mem[sp].num)
		resetNumRef(&mem[sp-1])
		sp--

	case BitXor:
		mem[sp-1].num = bitXor(mem[sp-1].num, mem[sp].num)
		resetNumRef(&mem[sp-1])
		sp--

	case BitAndNot:
		mem[sp-1].num = bitAndNot(mem[sp-1].num, mem[sp].num)
		resetNumRef(&mem[sp-1])
		sp--

	case BitShl:
		// Truncate to LHS width: Go's `<<` yields the LHS type (see shlWord).
		mem[sp-1].num = shlWord(mem[sp-1].num, mem[sp].num, mem[sp-1].ref.Kind())
		resetNumRef(&mem[sp-1])
		sp--

	case BitShr:
		mem[sp-1].num = shrWord(mem[sp-1].num, mem[sp].num, mem[sp-1].ref.Kind())
		resetNumRef(&mem[sp-1])
		sp--

	case Swap:
		a, b := sp-int(c.A), sp-int(c.B)
		mem[a], mem[b] = mem[b], mem[a]

	case MkSlice:
		n := int(c.A)
		var sliceType reflect.Type
		if b := int(c.B); b < 0 {
			// make(NamedSlice, ...): whole type passed, read live like MkMap.
			sliceType = m.globals[-b-1].ref.Type()
		} else {
			// Derive, not reflect.SliceOf: a synth elem must match materialize's slice identity.
			sliceType = runtype.DeriveSliceOf(m.globals[b].ref.Type())
		}
		switch {
		case n < 0:
			// make([]T, len[, cap]): size args are on the stack.
			nSizeArgs := -n
			sLen := int(mem[sp-nSizeArgs+1].num)
			sCap := sLen
			if nSizeArgs == 2 {
				sCap = int(mem[sp].num)
			}
			sp -= nSizeArgs - 1
			mem[sp] = Value{ref: reflect.MakeSlice(sliceType, sLen, sCap)}
		case n == 0:
			if sp+1 >= len(mem) {
				mem = growStack(mem, sp, 1)
			}
			sp++
			mem[sp] = Value{ref: reflect.Zero(sliceType)}
		default:
			slice := reflect.MakeSlice(sliceType, n, n)
			for i := range n {
				m.setFuncField(slice.Index(i), mem[sp-n+1+i])
			}
			mem[sp-n+1] = Value{ref: slice}
			sp -= n - 1
		}

	case MkMap:
		mapIdx, size := int(c.A), 0
		if mapIdx < 0 {
			// make(map, size): negated type index flags a size hint on the stack.
			mapIdx = -mapIdx - 1
			size = int(mem[sp].num)
			sp--
		}
		mapType := m.globals[mapIdx].ref.Type()
		if sp+1 >= len(mem) {
			mem = growStack(mem, sp, 1)
		}
		sp++
		mem[sp] = Value{ref: reflect.MakeMapWithSize(mapType, size)}

	case Append:
		n := int(c.A)
		m.appendValues(mem, sp, n)
		sp -= n

	case AppendSlice:
		n := int(c.A)
		if n == 0 {
			// Spread mode: append(a, b...)
			src := mem[sp].ref
			if src.Kind() == reflect.String {
				// append([]byte, string...) special case.
				src = reflect.ValueOf([]byte(src.String()))
			}
			dst := mem[sp-1].ref
			var result reflect.Value
			de, se := dst.Type().Elem(), src.Type().Elem()
			if de != se && de.Kind() == reflect.Interface && se.Kind() == reflect.Interface {
				// Erased vs synth iface elem split (an erased variadic pack
				// spread onto a synth-elem slice): element-wise, like copySlice.
				result = dst
				for i := range src.Len() {
					result = reflect.Append(result, m.reflectForSend(FromReflect(src.Index(i)), de))
				}
			} else {
				result = reflect.AppendSlice(dst, src)
			}
			sp--
			mem[sp] = Value{ref: result}
			break
		}
		m.appendValues(mem, sp, n)
		sp -= n

	case CopySlice:
		n := m.copySlice(mem[sp-1].ref, mem[sp].ref)
		mem[sp-1] = ValueOf(n)
		sp--

	case DeleteMap:
		mapVal := mem[sp-1].ref
		mapVal.SetMapIndex(m.mapKey(mapVal.Type().Key(), mem[sp]), reflect.Value{})
		sp--

	case IndexAddr:
		idx := int(mem[sp].num)
		ref := reflect.Indirect(mem[sp-1].ref)
		mem[sp-1] = Value{ref: ref.Index(idx).Addr()}
		sp--

	case MapIndex:
		mapVal := mem[sp-1].ref
		rv := mapVal.MapIndex(m.mapKey(mapVal.Type().Key(), mem[sp]))
		if !rv.IsValid() {
			rv = reflect.Zero(mapVal.Type().Elem())
		}
		mem[sp-1] = FromReflect(rv)
		sp--

	case MapIndexOk:
		mapVal := mem[sp-1].ref
		rv := mapVal.MapIndex(m.mapKey(mapVal.Type().Key(), mem[sp]))
		ok := rv.IsValid()
		if !ok {
			rv = reflect.Zero(mapVal.Type().Elem())
		}
		mem[sp-1] = FromReflect(rv)
		mem[sp] = boolVal(ok)

	case MapSet:
		mapVal := mem[sp-2].ref
		if mapVal.Kind() == reflect.Map && mapVal.IsNil() {
			// Writing to a nil map is a recoverable runtime panic in Go.
			m.raiseNilMapAssign()
			ip = m.stageUnwind(ip, fp, mem)
			*pmem, *pip, *pfp, *psp, *psafe = mem, ip, fp, sp, safepointCtr
			return coldContinue, false, nil
		}
		mt := mapVal.Type()
		// adoptNamedType: an untyped constant value keeps its base type. Adopt it.
		mapVal.SetMapIndex(runtype.Exportable(m.mapKey(mt.Key(), mem[sp-1])),
			runtype.Exportable(adoptNamedType(m.wrapForFunc(mem[sp], mt.Elem()), mt.Elem())))
		sp -= 2

	case GreaterStr:
		mem[sp-1] = boolVal(mem[sp-1].ref.String() > mem[sp].ref.String())
		sp--

	case LowerStr:
		mem[sp-1] = boolVal(mem[sp-1].ref.String() < mem[sp].ref.String())
		sp--

		// Per-type Greater.

	}
	*pmem, *pip, *pfp, *psp, *psafe = mem, ip, fp, sp, safepointCtr
	return coldNext, false, nil
}
