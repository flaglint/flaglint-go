# ADR 002 — Client Identity Model

Date: 2026-07
Status: Accepted

## Decision

A variable is treated as a LaunchDarkly Go SDK client only when its identity
can be *proven* — never by matching a method name in isolation. Phase 1 proves
identity through import-alias tracing plus constructor-call binding. Phase 2
(opt-in, `--strict-types`) additionally proves identity through `go/types`
static type resolution.

## Context

flaglint-js's scanner earned its "provably correct" reputation from one rule:
client identity is established only through `import/require → init() →
variable` — never from a variable or function *name* alone. That discipline
was violated in two places that shipped anyway (name-only detection of
`isFeatureEnabled` and of React hook names like `useFlags`), which produced
real false positives against unrelated user code with the same names. Both
were later fixed by requiring import verification.

flaglint-go adopts the same non-negotiable rule from day one, and goes one
step further: because it is a Go program analyzing Go source, it has access
to `go/types`, which can answer "is this variable's static type actually
`*ldclient.LDClient`?" directly — a stronger guarantee than syntactic
import-binding tracing alone, and something the JS scanner cannot get without
embedding a full TypeScript type checker.

## Phase 1 — Import + Constructor Tracing (default, no build required)

1. Walk import declarations for `github.com/launchdarkly/go-server-sdk/v6`
   or `v7`, recording whatever local package alias is used (default `ld`, but
   any alias — including dot-imports — must be handled).
2. A variable is bound as a client only when the right-hand side of its
   assignment is `<alias>.MakeClient(...)` or `<alias>.MakeCustomClient(...)`
   for a resolved SDK alias — package-level `var`, local `:=`, or struct-field
   assignment.
3. Method calls are only attributed to LaunchDarkly usage when the receiver
   resolves (through direct assignment, not name matching) to a bound client
   variable.

This phase works on any `.go` file in isolation — it does not require the
scanned module to type-check or its dependencies to be resolvable. This
matters because audit tools are frequently run against code that doesn't
currently build (mid-refactor branches, vendored snapshots, partial
checkouts).

## Phase 2 — Type-Verified Identity (opt-in via `--strict-types`)

Loads the target module via `golang.org/x/tools/go/packages` with full type
information and additionally treats a variable as a client when its static
type is exactly the SDK's client type — regardless of *how* it was
constructed (returned from a factory function, passed as a function
parameter, stored in a struct field, obtained from a package-level
`sync.Once`-guarded singleton, etc.). This closes the gap Phase 1's
syntactic tracing cannot: indirection through helper functions.

`--strict-types` is opt-in, not the default, because it requires the target
package graph to be loadable (network access for module downloads may be
needed, and code that doesn't currently compile cannot be analyzed this way).
Phase 1 remains the default so `flaglint-go audit` works unconditionally.

## False-Positive Guard (non-negotiable, tested)

Neither phase may ever match on:
- A function or method literally named `BoolVariation`, `MakeClient`, etc.
  that isn't reached through a verified LaunchDarkly import.
- A struct type that merely happens to expose a method with the same name as
  an SDK method.

Every detection pattern added to this scanner must ship with a corresponding
false-positive fixture (a non-LaunchDarkly type/function using the same
name) proving the pattern does not fire on it. See CONTRIBUTING.md.

## Detection Targets and Risk (Phase 1 scope)

| Method | Risk | Notes |
|---|---|---|
| `BoolVariation(key, ctx, default)` | low | static key only |
| `StringVariation(key, ctx, default)` | low | static key only |
| `IntVariation(key, ctx, default)` | low | static key only |
| `Float64Variation(key, ctx, default)` | low | static key only |
| `JSONVariation(key, ctx, &result)` | medium | pointer output, manual review |
| `BoolVariationDetail` / `*VariationDetail` | high | returns EvaluationDetail |
| `AllFlagsState(ctx, ...)` | high | bulk call, no OpenFeature equivalent |
| Dynamic key (identifier, `fmt.Sprintf`, string concat) | high | cannot resolve statically |

This table is carried over unchanged from flaglint-js's own Go-support design
notes (that repo's ADR 006) — it is the right classification independent of
which parser produces it.

## Dynamic Key Detection

A flag-key argument is `isDynamic: true` whenever it is not a string literal:
an identifier, a `fmt.Sprintf(...)` call, or a `+`-based string
concatenation. Dynamic call sites within a single file are numbered
sequentially in source-encounter order starting at `0`, matching
flaglint-js's `dynamicIndex` counter exactly — see
[ADR 003](003-cross-tool-contract.md) for why this must match byte-for-byte.

## Consequences

- Phase 1 ships first and is the default; `--strict-types` (Phase 2) ships
  once Phase 1's fixture suite is solid.
- Every new SDK method or wrapper pattern requires both a positive fixture
  and a negative (false-positive) fixture before merge.
- Struct-field and factory-function indirection is a known Phase 1 gap,
  explicitly deferred to Phase 2 rather than approximated with a heuristic
  that could reintroduce name-based matching.

## What This ADR Does Not Decide

- The migration (`--apply`) safety model — requires its own ADR.
- Whether Phase 2's `go/packages` dependency graph loading should be cached
  across runs (a performance question for later, once Phase 2 exists).
