// Package gsreflect edits goscript reflect types in place: the goscript
// counterpart of the rtype memory edits of mvm's internal/runtype.
//
// goscript reflect types are instances of private classes of
// @goscript/reflect, whose state is in underscore fields (_name, _fields,
// _elemType, _typeName, ...). These functions read and write those fields.
import * as $ from '@goscript/builtin/index.js'
import type * as reflect from '@goscript/reflect/index.js'

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
}
