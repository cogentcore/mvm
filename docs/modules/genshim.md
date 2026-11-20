# genshim

> Registry of generic shims: interpreted source of the generic functions of
> a package whose other symbols are native bindings.

## Overview

A native binding is a `reflect.Value`, which a generic function cannot be:
it has no single value until it is instantiated. A generic shim provides
the source of the generic functions of a package (and of the constraint
interfaces that they use), which mvm parses against the context of the
package, so that `pkg.F[T](...)` is instantiated as interpreted code that
calls the native bindings of the package.

`genshim.Register(pkg, source, nativeRefs)` adds a shim; `nativeRefs` are
the names of the native bindings that the source refers to bare.
`goparser.RegisterGenericShim` is the same call. When a parser imports the
bindings of a package (`ImportPackageValues`), it installs the shims of the
package from `genshim.All()` (see `goparser/generic_shim.go`).

## Why a package of its own

The shims are registered by packages of bindings, such as those that
`cmd/extract -register` generates. A program can import such a package
only for its bindings, as a table of the functions of a package by name,
without interpreting code. genshim depends only on the standard library, so
that such a program does not link the parser.
