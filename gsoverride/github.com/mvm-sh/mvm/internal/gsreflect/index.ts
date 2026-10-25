// Package gsreflect edits goscript reflect types in place: the goscript
// counterpart of the rtype memory edits of mvm's internal/runtype.
//
// goscript reflect types are instances of private classes of
// @goscript/reflect, whose state is in underscore fields (_name, _fields,
// _elemType, _typeName, ...). These functions read and write those fields.
import * as $ from '@goscript/builtin/index.js'
import * as reflect from '@goscript/reflect/index.js'
import { typeInfoFromReflectType } from '@goscript/reflect/type.js'

// TypeState is the private state of the goscript reflect type classes.
type TypeState = {
  _name?: string
  _string?: string
  _pkgPath?: string
  _typeName?: string
  _registeredName?: string
  _methods?: $.MethodSignature[]
  _fields?: FieldState[]
  _elemType?: reflect.Type
}

// FieldState is the descriptor of a struct field.
type FieldState = {
  name: string
  key: string
  type: reflect.Type
  tag?: string
  pkgPath: string
  anonymous: boolean
  index: number[]
  offset: number
  exported: boolean
}

function state(t: reflect.Type | null): TypeState {
  if (t === null) {
    throw new Error('gsreflect: nil type')
  }
  return t as unknown as TypeState
}

// shortName returns the name of a qualified type name without its package.
function shortName(name: string): string {
  const bracket = name.indexOf('[')
  const base = bracket >= 0 ? name.substring(0, bracket) : name
  const dot = base.lastIndexOf('.')
  return dot >= 0 ? name.substring(dot + 1) : name
}

// Clone returns a new type with the same shape as t and its own identity.
export function Clone(t: reflect.Type | null): reflect.Type {
  const src = state(t)
  const c = Object.assign(Object.create(Object.getPrototypeOf(src)), src)
  if (src._fields !== undefined) {
    c._fields = src._fields.map((f) => ({ ...f }))
  }
  if (src._methods !== undefined) {
    c._methods = [...src._methods]
  }
  return c as reflect.Type
}

// SetName names t in place.
export function SetName(
  t: reflect.Type | null,
  name: string,
  pkgPath: string,
): void {
  const s = state(t) as TypeState & Record<string, unknown>
  if (s._fields !== undefined) {
    // StructType: the name is also its identity key.
    s._name = name
    s._string = name
    s._pkgPath = pkgPath
    registerStruct(t as reflect.Type)
    return
  }
  if ('_typeName' in s) {
    s._typeName = name
  }
  if ((t as reflect.Type).Kind() === 20) {
    // InterfaceType spells its name in _name.
    s._name = name
    s._registeredName = undefined
  }
  s.runtimeString = () => name
  s.Name = () => shortName(name)
  s.PkgPath = () => pkgPath
}

// SetString sets the String text of t in place, leaving it unnamed.
export function SetString(t: reflect.Type | null, str: string): void {
  const s = state(t) as TypeState & Record<string, unknown>
  if (s._fields !== undefined) {
    s._name = ''
    s._string = str
    return
  }
  s.runtimeString = () => str
  s.Name = () => ''
}

// SetFields gives the struct type dst the fields of src in place.
export function SetFields(
  dst: reflect.Type | null,
  src: reflect.Type | null,
): void {
  state(dst)._fields = (state(src)._fields ?? []).map((f) => ({ ...f }))
  registerStruct(dst as reflect.Type)
}

// SetElem sets the element type of t in place.
export function SetElem(
  t: reflect.Type | null,
  elem: reflect.Type | null,
): void {
  state(t)._elemType = elem as reflect.Type
}

// SetFieldType sets the type of field i of the struct type t in place.
export function SetFieldType(
  t: reflect.Type | null,
  i: number,
  ft: reflect.Type | null,
): void {
  const fields = state(t)._fields!
  fields[i] = { ...fields[i], type: ft as reflect.Type }
  registerStruct(t as reflect.Type)
}

// SetFieldEmbedded marks field i of the struct type t as embedded.
export function SetFieldEmbedded(
  t: reflect.Type | null,
  i: number,
  name: string,
  tag: string,
  exported: boolean,
): void {
  const fields = state(t)._fields!
  fields[i] = {
    ...fields[i],
    name,
    ...(tag !== '' ? { tag } : {}),
    anonymous: true,
    exported,
  }
  registerStruct(t as reflect.Type)
}

// MethodFunc runs a method with a pointer to the receiver and the arguments.
type MethodFunc = (
  recv: reflect.Value,
  args: $.Slice<reflect.Value>,
) => $.Slice<reflect.Value> | Promise<$.Slice<reflect.Value>>

// SetMethod installs the method name of signature sig on the values of the
// named type t. For a struct type, it is a JavaScript method of their class, with one parameter per
// Go parameter (so interface checks see its arity), that calls fn with a
// pointer to the value and the arguments as reflect Values. Its result is
// the Go result: nothing, one value or an array, or a Promise of it when fn
// suspends. ptrRecv is whether only *t has the method; the method set of t
// is registered with the type, so a t boxed in an interface lacks it. For
// another type, it is an entry of the method tables that goscript boxes the
// values of t and *t with (see setValueMethod).
export function SetMethod(
  t: reflect.Type | null,
  name: string,
  sig: reflect.Type | null,
  ptrRecv: boolean,
  fn: MethodFunc | null,
): void {
  const typ = t as reflect.Type
  const ptrType = reflect.PointerTo(typ)!
  const msig = sig as reflect.Type
  const nin = msig.NumIn()
  const inTypes: reflect.Type[] = []
  for (let i = 0; i < nin; i++) {
    inTypes.push(msig.In(i))
  }

  // A named non-struct type has method tables for its values and pointers.
  if (typ.Kind() !== reflect.Struct) {
    setValueMethod(typ, ptrType, name, msig, inTypes, ptrRecv, fn!)
    return
  }

  // The values of a struct type without a registered constructor share a
  // class, whose prototype gets the method.
  const sample = reflect.New(typ).Interface() as object
  const proto = Object.getPrototypeOf(sample) as Record<string, unknown>

  const method = function (this: object, ...args: unknown[]): unknown {
    const recv = new reflect.Value(this as never, ptrType)
    const argv = inTypes.map((at, i) => new reflect.Value(args[i] as never, at))
    const out = fn!(recv, $.arrayToSlice(argv))
    if (out instanceof Promise) {
      return out.then(goResults)
    }
    return goResults(out)
  }
  Object.defineProperty(method, 'length', { value: nin })
  Object.defineProperty(proto, name, {
    value: method,
    writable: true,
    configurable: true,
  })
  registerMethod(typ, name, msig, ptrRecv)
}

// methodSets holds the method signatures installed on each struct type.
const methodSets = new WeakMap<reflect.Type, $.MethodSignature[]>()

// registerMethod adds the method name to the method set of t, registered
// with the runtime type system under the name of t. A method that a value
// already has keeps it when it is added again for the pointer.
function registerMethod(
  t: reflect.Type,
  name: string,
  sig: reflect.Type,
  ptrRecv: boolean,
): void {
  let methods = methodSets.get(t)
  if (methods === undefined) {
    methods = []
    methodSets.set(t, methods)
  }
  const prev = methods.findIndex((m) => m.name === name)
  const pointerReceiver =
    ptrRecv && (prev < 0 || methods[prev].pointerReceiver === true)
  setSignature(methods, methodSignature(name, sig, pointerReceiver))

  registerStruct(t)
}

// registerStruct registers the named struct type t with the runtime type
// system, as the compiler registers a struct type: by name, with its class
// as constructor, its fields and the methods installed on it. It is called
// again whenever t changes, so the registration follows t.
function registerStruct(t: reflect.Type): void {
  const s = state(t)
  if (s._fields === undefined || !s._name) {
    return
  }
  const ctor = Object.getPrototypeOf(reflect.New(t).Interface() as object)
    .constructor as new () => object
  const fields = (): $.StructFieldInfo[] =>
    s._fields!.map((f) => ({
      name: f.name,
      key: f.key,
      type: typeInfoFromReflectType(f.type),
      ...(f.tag !== undefined ? { tag: f.tag } : {}),
      pkgPath: f.pkgPath,
      anonymous: f.anonymous,
      index: f.index,
      offset: f.offset,
      exported: f.exported,
    }))
  $.registerStructType(s._name, null, methodSets.get(t) ?? [], ctor, fields)

  // t is made the canonical type of its name, so that reflection gives t
  // for a value boxed by name, rather than a type built from its type info.
  reflect.TypeOf(new ctor() as never)
}

// goResults returns the results of a Go call as JavaScript returns them.
function goResults(out: $.Slice<reflect.Value>): unknown {
  const values = $.asArray(out) as reflect.Value[]
  switch (values.length) {
    case 0:
      return undefined
    case 1:
      return values[0].Interface()
    default:
      return values.map((v) => v.Interface())
  }
}

// valueMethodTables holds the method tables of the values of each named
// non-struct type t and of its pointers, as registerNamedValueType takes
// them, with their signatures.
const valueMethodTables = new WeakMap<
  reflect.Type,
  { value: $.NamedValueMethods; ptr: $.NamedValueMethods }
>()

// setValueMethod adds the method name to the method tables of the named
// non-struct type t, whose values are plain JavaScript values: the methods of
// a boxed t get the value, those of a boxed *t a pointer to it. fn gets a
// pointer either way. ptrRecv is whether only *t has the method.
function setValueMethod(
  t: reflect.Type,
  ptrType: reflect.Type,
  name: string,
  sig: reflect.Type,
  inTypes: reflect.Type[],
  ptrRecv: boolean,
  fn: MethodFunc,
): void {
  let tables = valueMethodTables.get(t)
  if (tables === undefined) {
    tables = {
      value: { methods: {}, signatures: [] },
      ptr: { methods: {}, signatures: [] },
    }
    valueMethodTables.set(t, tables)
  }

  // call runs fn with a pointer to the receiver and the arguments.
  const call = (ptr: unknown, args: unknown[]): unknown => {
    const recv = new reflect.Value(ptr as never, ptrType)
    const argv = inTypes.map((at, i) => new reflect.Value(args[i] as never, at))
    const out = fn(recv, $.arrayToSlice(argv))
    return out instanceof Promise ? out.then(goResults) : goResults(out)
  }
  const msig = methodSignature(name, sig, ptrRecv)

  // A value method is in the method sets of t and *t; a pointer method, or
  // a value method added again for the pointer, only in that of *t.
  const hasValue = name in tables.value.methods
  if (!ptrRecv || hasValue) {
    tables.value.methods[name] = (recv: unknown, ...args: unknown[]) =>
      call($.varRef(recv), args)
    setSignature(tables.value.signatures, { ...msig, pointerReceiver: false })
  }
  tables.ptr.methods[name] = (recv: unknown, ...args: unknown[]) =>
    call(recv, args)
  setSignature(tables.ptr.signatures, {
    ...msig,
    pointerReceiver: ptrRecv && !hasValue,
  })
  $.registerNamedValueType(
    t.String(),
    tables.value.methods,
    tables.value.signatures,
  )
  $.registerNamedValueType(
    '*' + t.String(),
    tables.ptr.methods,
    tables.ptr.signatures,
  )
}

// setSignature adds sig to signatures, replacing one of the same name.
function setSignature(
  signatures: $.MethodSignature[],
  sig: $.MethodSignature,
): void {
  const i = signatures.findIndex((m) => m.name === sig.name)
  if (i < 0) {
    signatures.push(sig)
  } else {
    signatures[i] = sig
  }
}

// methodSignature returns the runtime signature of the method name, of
// signature sig without receiver.
function methodSignature(
  name: string,
  sig: reflect.Type,
  ptrRecv: boolean,
): $.MethodSignature {
  const args: $.MethodArg[] = []
  for (let i = 0; i < sig.NumIn(); i++) {
    args.push({ type: typeInfoFromReflectType(sig.In(i)) })
  }
  const returns: $.MethodArg[] = []
  for (let i = 0; i < sig.NumOut(); i++) {
    returns.push({ type: typeInfoFromReflectType(sig.Out(i)) })
  }
  return { name, args, returns, pointerReceiver: ptrRecv }
}
