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

## Consequences

**Easier:**
- Native calls with arguments of native types, such as the widgets of a GUI,
  no longer walk their object graph. The 150-widget example: 90 to under
  10 ms natively, 12 s to 0.37 s under goscript.
- The walk covers the values it must by their type, not by a depth limit: an
  earlier fix limited it to 4 pointer hops, which was arbitrary.
- The same 17 boxes are found over the `interptest` suite.

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
  can go. This is the better end state, and the 7 tests above show the store
  paths that still need it; until they all unbox, the walk stays as the
  safety net.
