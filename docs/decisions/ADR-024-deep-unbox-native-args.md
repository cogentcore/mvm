# ADR-024: Deep unbox of native call arguments

**Status:** accepted
**Date:** 2026-10-25

## Context

In interpreted code, a value of an interface type is an `Iface{Typ, Val}`
box: the concrete value and its mvm `*Type`, which keeps the interpreted
type identity (see [vm](../modules/vm.md)). Native code must never see a
box: it would read an `Iface` struct where it expects a Go interface value.

Since [ADR-021](ADR-021-synthesized-rtypes.md), interpreted types have
synthesized rtypes that carry their methods, so a box can be replaced by a
real Go interface value holding its concrete value. This happens in two
places:

- **At store time.** An interpreted store of an interface value into an
  interface-typed field or element of Go memory goes through
  `setFuncField`, which unboxes it (`unboxIfaceFor`).
- **At call time.** `bridgeArgs` unboxes the interface-typed arguments of a
  native call (`bridgeIface`), and then `deepUnboxIface` walks each argument
  for boxes nested inside it, replacing them in place for reference types.

The call-time walk is a safety net for the store paths that do not unbox.
Over the `interptest` suite it ran 8,933 times, and replaced 17 boxes, from
7 tests:
`TestInterface/box_range_append`, a `TestVariadic` case,
`TestFile/errors_custom_as.go`, `TestFile/generic_forward.go`,
`TestGobBinaryMarshaler`, `TestReflectSetSynthIfaceElemThenNativeSort` and
`TestSynthDatabaseSQL`. They come from appends of interface values, slices of
interpreted interface types set through reflect, and variadic `...any`
arguments. All of them were within 3 heap hops of the argument.

The walk followed every pointer and slice that an argument reaches. Passing
one widget of a GUI to a native func, as each call that builds the GUI does,
walked the widget tree that the widget reaches, at each call, so building a
tree of n widgets took time quadratic in n. Its visited set also did not
work: it was made on first use in the call that needed it, and passed by
value, so each branch of a walk made its own, and a node reached through
several paths was walked once for each; and its key, a struct with a
`reflect.Type`, made each lookup scan the whole map under goscript, where
such keys are compared field by field. A docs example making 150 widgets
took 90 ms natively and 12 seconds in a browser under goscript.

## Decision

The walk does not enter values of named types that the interpreter did not
make (`nativeNamed`: a named rtype that is neither synthesized nor known to
`typeByRtype`), through a pointer or by value. Such memory holds no boxes:
native code only writes values that were unboxed when they reached it, and
interpreted code writes its fields and elements through `setFuncField`,
which unboxes.

The walk still enters everything else: interface values, slices, arrays and
maps of any type, and pointers to and values of interpreted types and of
unnamed types. These are what interpreted code makes and may hold boxes
from the store paths that do not unbox, such as a `[]any` argument, an
interpreted struct, or an interface slot wherever it is.

The visited set is made once per walk and shared through a pointer, keyed by
address in a map of the types seen there. The one-hop limit on maps
(`maxMapHops`) is kept: it guards against iterating a live map of the wider
object graph that another goroutine writes.

The store paths that left boxes by mistake now unbox: 16 of the 17 boxes
above came from them.
- `unboxIfaceFor` kept the box of a pointer, as `&a` in `[]any{&a}`,
  because the `*Type` of a pointer has the name of its element, which its
  rtype does not carry, and so looked like an interpreted named type whose
  rtype does not identify it. It kept the box of a generic instance too,
  whose name is spelled differently in its `*Type` and its rtype
  (`Map#string#int`, `Map[string,int]`). It now keeps a box only for a named
  non-pointer type whose rtype neither is synthesized nor maps back to it
  (`typeByRtype`).
- `reflectForSend`, which `append` and channel sends use, stored the box of
  a value sent to an `any` element. It now unboxes as `setFuncField` does.

Two forms of boxes remain by design, and the walk is kept for them:
- A slot of an interpreted (synthesized) interface type holds the
  interpreter's interface form (`storeIfaceFromReflect`), since the
  interpreter cannot decode a native itab for a synthesized interface.
- An interpreted named type whose rtype does not identify it, such as a
  methodless `type S string`, keeps its box in an `any` slot, for its
  identity.

## Consequences

**Easier:**
- Native calls with arguments of native types, such as the widgets of a GUI,
  no longer walk their object graph. The 150-widget example: 90 to under
  10 ms natively, 12 s to 0.37 s under goscript.
- The walk covers the values it must by their type, not by a depth limit: an
  earlier fix limited it to 4 pointer hops, which was arbitrary.
- The same 17 boxes were found over the `interptest` suite; after the store
  path fixes, the walk finds only the one of the designed forms.

**Harder / limitations:**
- The rule relies on the store invariant: every interpreted store into a
  native type's memory unboxes. A store path that does not would leave a box
  in a native value that the walk no longer finds; the walk of an argument of
  that native type used to find it.
- The walk still costs a traversal of the interpreted values reachable from
  each argument.

## Alternatives considered

- **A depth limit** on pointers and slices (4 hops). It bounded the cost but
  is arbitrary: a box deeper than the limit is missed, and a walk within the
  limit can still reach much of a large graph.
- **Caching on `Value`.** The boxes are in Go memory, not in the `Value`s of
  the interpreter, so a cache there does not tell which memory holds boxes.
- **Unboxing at every store**, so that no box reaches Go memory and the walk
  can go. The store paths that left boxes by mistake now do; the two forms
  that remain are needed for the interpreter's own reads and type identity,
  so the walk stays, for them and as the safety net.
