# cmd/mvmlint

> Project-specific source linter for mvm's own Go code, built on mvm's
> scanner rather than the Go AST.

## Overview

`cmd/mvmlint` checks mvm's source for a small set of mvm-specific
mistakes that `go vet` and `golangci-lint` cannot express.
It parses with mvm's own scanner (`github.com/cogentcore/mvm/scan`), not
`go/ast` or `go/analysis`, so the tool stays dependency-free (no
`x/tools`) and dogfoods the scanner across the whole repository as a
side effect.
`make lint` runs it after `golangci-lint`.

## Usage

```
go run ./cmd/mvmlint [dir ...]   # default: the whole module
mvm github.com/cogentcore/mvm/cmd/mvmlint .   # same, run through mvm itself
```

It exits non-zero when any check fires, printing `file:line:col: message`
diagnostics.

## Checks

| Check | What it flags | Suppress with |
|-------|---------------|---------------|
| `symkey` | a write to a `.Symbols` (`symbol.SymMap`) table whose key is a bare name -- the root of the cross-package symbol-collision class. Keys must be package-qualified (`pkgKey`/`QualifyName`/...), lexically scoped (a `"/"` concat), a predeclared/builtin name, or forwarded from an enclosing parameter. | `// mvm:symkey-ok` |
| `posbase` | re-adding `PosBase` outside `Compiler.emit`, which double-applies the position base (see [ADR-015](../decisions/ADR-015-absolute-token-positions.md)). | `// mvm:posbase-ok` |

The `symkey` check exists because bare symbol-table keys were the cause
of an entire class of cross-package collision bugs (sibling imports with
same-named types/funcs/vars clobbering each other); the canonical
package-qualified-key refactor closed them, and this check guards the
invariant going forward.

## Internal design

The checks are text/token heuristics, not a type-checked analysis.
Key classification resolves locals through best-effort reaching
definitions that are *not* block-scoped, so a safe definition in one
branch can clear a same-named bare key in a sibling branch -- a possible
false negative.
This is acceptable for a dogfooding guard whose green output is sanity,
not proof.

## Dependencies

- `scan/` -- the scanner used to tokenize each source file.

## Open questions / TODOs

- Block-scoped reaching definitions would remove the `symkey` false
  negatives noted above.
