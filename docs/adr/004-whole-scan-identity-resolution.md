# ADR 004 — Whole-Scan Identity Resolution (Phase 1.5)

Date: 2026-07
Status: Accepted

## Decision

Extend Phase 1 identity resolution ([ADR 002](002-client-identity-model.md))
from per-file analysis to whole-scan (multi-file, cross-package) analysis,
closing several real-world indirection patterns without requiring
`go/types` or a buildable module — Phase 1's "works on any `.go` file,
build or no build" guarantee is preserved. `--strict-types` (true Phase 2)
remains for the cases this still cannot resolve.

## Context

Phase 1 shipped and was validated against synthetic fixtures, then
field-tested against real, unmodified open-source Go repositories known or
suspected to use the LaunchDarkly Go SDK. Result: zero false positives, but
also zero recall on every repository with genuine usage. Every one of them
wrapped the client behind indirection Phase 1's per-file, same-scope
analysis could not see:

- **launchdarkly-labs/ld-sample-app-go** (the official sample app) — a
  package-level singleton getter (`func GetLdClient() *ld.LDClient`),
  called from a *different package*.
- **weaviate/weaviate** — the client stored into a wrapper struct via
  composite literal (`&LDIntegration{ldClient: client}`), then reached
  through a **two-level field chain** (`f.integ.ldClient.Method(...)`)
  from a **different file in the same package**, on a **generic struct**
  (`FeatureFlag[T]`).
- **CMS-Enterprise/mint-app** — the client passed into a constructor as a
  **plain parameter** (no assignment to trace at all — the parameter's
  declared type is the only place identity is established), plus a
  constructor function whose declared return type is the client type.
- **e2b-dev/infra** — a method *value* taken from a bound client and
  passed through a generic helper function; still unresolved (see "Not
  Covered" below) — the one real-world case this ADR does not close.

None of these require real type-checking to resolve: in every case, the
proof of identity is available directly from the AST — a struct's
declared field type, a function's declared parameter or return type, a
package-level `var`'s initializer — the same "trust the syntax, no build
required" spirit as the rest of Phase 1. What they *do* require is seeing
more than one file at a time, since the struct declaring a field, the
function declaring a factory return type, and the code using either are
routinely in a different file — sometimes a different package — than the
code that first establishes the binding.

## What Changed

**Architecture**: `Scan` now parses every file up front (previously:
read → parse → detect → discard, one file at a time) and runs a whole-scan
pre-pass before any per-file detection:

1. **Struct field types** (`structtypes.go`) — every `type X struct {...}`
   declaration across every parsed file, merged into one
   `"TypeName.Field" -> declared field type` index. Used to walk
   multi-level field-selector chains one hop at a time
   (`qualifiedFieldKey`/`resolveChainType` in `identity.go`).
2. **Factory functions** (`factory.go`) — every free function whose
   *declared* return type is exactly `*<traced-alias>.LDClient`, keyed by
   the declaring package's import path (see below). A call site binds its
   assigned variable when it invokes a registered factory function, either
   cross-package (`pkgAlias.FuncName()`) or same-package (bare
   `FuncName()`).
3. **Parameter-typed bindings** (`paramClientBindings`, `factory.go`) — a
   function or closure parameter (or receiver) whose *declared* type is
   directly `*<traced-alias>.LDClient` is bound from the start of that
   scope — no assignment to trace at all.
4. **Package-level vars and direct struct-field assignments**
   (`collectPackageLevelBindings`, `collectFieldBindings`) — now merged
   across every file in the scan, not just within one file, matching real
   Go semantics (neither is file-scoped).
5. **Composite-literal field bindings**
   (`collectCompositeLiteralFieldBindings`) — `&LDIntegration{ldClient:
   client}` binds `"LDIntegration.ldClient"` when `client` is itself
   already bound (by any of the above), not just a direct constructor
   call.

Detection then runs as a final pass with the complete, whole-scan binding
set — a composite literal or factory call in one file can make a binding
visible to a different file's detection pass.

**Cross-package resolution requires a real import path**, computed from
`go.mod` (`module.go`): the nearest `go.mod` found by searching upward from
the scan root gives the module path, from which every scanned file's real
Go import path is computed (`module path + directory relative to module
root`). A qualified call `pkgAlias.FuncName()` is only resolved against the
factory index by matching the *calling file's own import path string* — a
raw string literal, one import declaration in that same file — against a
package we've actually parsed and registered a factory function for. No
go.mod means cross-package resolution is silently skipped (same-file and
same-package-by-directory resolution still work; only a specific string
match fails to happen) — a false negative, never a false positive.

**An explicitly rejected alternative**: resolving imports by matching an
import path's *last segment* against a package name, avoiding the need to
parse `go.mod` at all. This was rejected during design — it is a
name-based heuristic wearing an import-path costume, exactly what
[ADR 002](002-client-identity-model.md) forbids. A package's declared name
can legitimately differ from its import path's last segment; resolving
only via real, computed import paths keeps this at zero false-positive
risk.

**Incidental fixes found only by testing against real code**, not part of
the original plan but required to make the above work on the actual repos
that motivated it:
- `simpleTypeName` didn't handle Go generics (`*ast.IndexExpr` /
  `*ast.IndexListExpr`) — a method receiver on any generic struct
  (`func (f *FeatureFlag[T]) M()`) silently failed to resolve its own
  type, breaking every chain rooted at one. weaviate's `FeatureFlag[T]` is
  exactly this shape.

## Not Covered (still deferred)

- **Method values** (`f := client.BoolVariation; f(...)`, or a method
  value passed through a generic helper function and invoked from inside
  it) — e2b-dev/infra's remaining gap. Tracked as issue #6.
- **Block-scoped shadowing** — a deliberate re-`:=` of the same name
  inside a nested block within one function. Tracked as issue #5.
- **Interface satisfaction** — a value known only through an interface
  type, never a concrete client-typed variable, parameter, or field.
- **A factory function returning a wrapper type** (not `*ld.LDClient`
  itself) that itself wraps a further factory call — factory resolution
  is one hop (declared return type must be the client type directly), not
  transitive.
- **Multiple `go.mod` files within one scanned tree** (independent
  submodules in a monorepo) — files under a submodule get an import path
  computed relative to the *outer* module, which won't match real import
  strings. Same failure mode as "no go.mod": a missed cross-package
  resolution, never an incorrect one.

All of the above remain candidates for `--strict-types` (true Phase 2,
`go/types`-backed), which resolves every one of them uniformly rather than
as one-off syntactic special cases.

## Consequences

- `Scan` now holds every parsed file in memory for the duration of one
  scan (previously: one file at a time, discarded after detection). For
  the real repositories field-tested against (up to ~4,500 files), this
  cost is unmeasurable against parse time itself.
- Whole-scan analysis is measurably more work than the original per-file
  model (multiple passes over every parsed file rather than one): ~2.3s
  for a 4,500-file real repository, up from ~1s. Still well within
  interactive use; not yet a concern.
- Every capability above shipped with both a positive fixture (proving it
  fires) and a false-positive fixture (proving it doesn't fire on a
  same-named-but-unrelated type, function, or import) — see
  `internal/scanner/testdata/fixtures/`.
