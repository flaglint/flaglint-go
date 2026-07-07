# ADR 005 — `--strict-types`: Type-Checked Identity Resolution (Phase 2)

Date: 2026-07
Status: Accepted

## Decision

Add an opt-in `--strict-types` pass that loads the target module with real
`go/types` information (via `golang.org/x/tools/go/packages`) and uses it
to prove client identity in cases Phase 1's syntactic tracing structurally
cannot — interface satisfaction and transitive factory wrapping. This pass
is strictly *additive*: it can only add findings Phase 1 missed, never
remove or contradict one Phase 1 already made. Phase 1 remains the
default; `flaglint-go audit` continues to work unconditionally, with no
build required.

This ADR splits what was originally described in
[ADR 002](002-client-identity-model.md) as one "Phase 2" into two
sub-phases with different footprints:

- **Phase 2a** (this ADR's actual scope): static type resolution. Closes
  issues [#15](https://github.com/flaglint/flaglint-go/issues/15)
  (interface satisfaction) and
  [#16](https://github.com/flaglint/flaglint-go/issues/16) (transitive
  factory wrapping). Both are pure "what is this expression's real type"
  questions — `go/types` answers them directly, no data-flow analysis
  needed.
- **Phase 2b** (explicitly deferred, tracked as its own future ADR):
  interprocedural data-flow. Closes
  [#26](https://github.com/flaglint/flaglint-go/issues/26) (a method
  value captured in one function, passed as an argument, and invoked from
  inside a *different* function). `go/types` alone gives you a
  parameter's *declared type*, not proof that a specific call site passed
  a specific real client's method into it — that requires tracing data
  flow across a call graph (`go/ssa`/`go/callgraph` or a bespoke
  lightweight tracer), a meaningfully larger and riskier undertaking than
  Phase 2a. Building it prematurely, bundled with Phase 2a, would risk
  either delaying real, contained value (#15/#16) or under-baking the
  harder problem. They ship separately.

## Context

Phase 1 (ADR 002) and its whole-scan extension (ADR 004) prove identity
entirely from syntax — import tracing, constructor-call binding, struct
fields, composite literals, factory functions, parameter types. This
works without a build and is fast, but it has a real ceiling: some
patterns are not expressible without knowing an expression's *actual*
static type, which only a type checker can answer.

- **Interface satisfaction** (#15): a client passed around only through
  an interface type it happens to satisfy. Phase 1 sees the interface
  type name, not the concrete `*ld.LDClient` underneath it — there is no
  syntactic proof available at all.
- **Transitive factory wrapping** (#16): a factory function returning a
  *wrapper* type (not `*ld.LDClient` directly) that itself has a client
  field, resolved through composite-literal/struct-field bindings
  elsewhere. Phase 1's factory registration (`returnsSDKClient`,
  factory.go) only recognizes a return type that is *literally*
  `*ld.LDClient` — one hop, by design (see ADR 004) — deliberately not
  chasing arbitrary wrapper chains without real type information to keep
  that mechanism provably safe.

Both are genuine, plausible real-world patterns, not contrived edge
cases — the same field-testing discipline that found #5/#17/#18/#20/#6
this project has followed throughout would very plausibly turn up live
examples of these too, the way it did for everything fixed so far.

## Package Loading

`--strict-types` loads the scanned directory as a Go module via
`golang.org/x/tools/go/packages` (`packages.Load` with `NeedTypes`,
`NeedTypesInfo`, `NeedSyntax`, `NeedImports` at minimum). This is a
materially different operating mode from Phase 1:

- It requires the module to actually type-check. Dependency resolution
  may need network access (or a warm module cache/vendor directory).
- It is meaningfully slower — type-checking a full dependency graph is a
  different order of magnitude of work than parsing `.go` files in
  isolation. Expect single-digit-to-tens of seconds on a large repo with
  a cold module cache, versus Phase 1's low seconds.
- It can fail — a mid-refactor branch, a partial checkout, or a repo with
  broken dependencies won't type-check. This is precisely the situation
  ADR 002 designed Phase 1 to keep working through; `--strict-types` does
  not get to break that guarantee for the *rest* of the tool.

**Failure mode: fail soft, per-package.** `packages.Load` reports errors
per-package, not just globally. A package that fails to load or
type-check is skipped for Phase 2a purposes — its Phase 1 findings stand
unchanged, and a warning is emitted (mirroring the existing
`ScanWarning` mechanism) naming the package and the reason. The scan
completes with whatever additional findings *were* provable, rather than
failing outright because one package elsewhere in a large repo doesn't
build. `validate`'s exit-code contract is unaffected: a `--strict-types`
package-load failure is a warning, not a policy violation, and never
promotes a `scan`/`audit` invocation to a nonzero exit on its own.

## Identity Resolution Model

Phase 2a's rule mirrors Phase 1's non-negotiable one (ADR 002): still
never inferred from a name, only now proven through the type checker
instead of syntax. Concretely, for each expression Phase 1 could not
already resolve:

- **Interface satisfaction**: `types.Info.TypeOf(expr)` on the receiver;
  if its underlying concrete type is exactly the SDK's `*LDClient` (or,
  for an interface-typed variable, if `go/types` can determine the
  *only* concrete type ever assigned to it through the package is
  `*LDClient` — the general "any possible concrete type" case is
  undecidable in general and out of scope; only the syntactically obvious
  single-assignment case is handled), the receiver is bound.
- **Transitive factory wrapping**: for a function whose declared return
  type is *not* literally `*ld.LDClient` (Phase 1's factory boundary),
  walk the returned type's fields via `go/types` structurally — if
  exactly one field (at any depth) resolves to `*ld.LDClient`, the
  factory is registered for that field path, extending Phase 1's
  one-hop-only factory model to arbitrary depth now that a real type
  graph is available to walk safely.

As in Phase 1, an unresolvable or ambiguous case is left unbound rather
than guessed — false negative over false positive remains non-negotiable
here too; `go/types` raises the ceiling of what can be *proven*, it does
not loosen the bar for what counts as proof.

## CLI and API Surface

`--strict-types` is a boolean flag on `scan`, `audit`, and `validate` —
not a `flaglint.config.json`/`.flaglintrc` field, unlike everything in
`internal/config.Config`. That struct exists specifically to mirror
flaglint-js's config schema for cross-tool parity (ADR 003); `go/types`
loading is a Go-only concept with no JS equivalent, so it doesn't belong
there.

At the API level, `scanner.Scan` (Phase 1, exported today) keeps its
exact current signature and behavior, unconditionally available with no
new dependency implications for callers that don't ask for Phase 2. A new
`scanner.ScanStrict(dir string, cfg config.Config) (types.ScanResult,
error)` runs Phase 1 first, then augments the result with Phase 2a
findings before returning — CLI commands call `ScanStrict` instead of
`Scan` only when `--strict-types` is passed. `types.ScanResult` gains
enough to distinguish a Phase 2a finding from a Phase 1 one (for
reporting/debugging), following the existing additive-fields discipline
already used for the JSON output contract (ADR 003).

## Consequences

- `golang.org/x/tools` becomes a real dependency (already what `gopls`
  and most Go tooling in this space is built on — a reasonable,
  well-established choice, not a niche one).
- Phase 1 remains fully independent of this pass at the call level;
  `flaglint-go audit` without `--strict-types` has zero behavior change
  and no new failure modes.
- CI/release tooling is unaffected — `--strict-types` is exercised by its
  own test fixtures (real, buildable Go modules under `testdata/`, since
  type-checking needs real buildable code, unlike Phase 1's testdata
  which is deliberately never built) but doesn't change how the rest of
  the suite runs.
- Phase 2b (interprocedural, closing #26) is explicitly not part of this
  ADR's scope — a follow-up ADR should make its own case for the
  data-flow approach once Phase 2a has shipped and proven out the package
  loading and fallback machinery in practice.

## What This ADR Does Not Decide

- Phase 2b's design (data-flow/call-graph approach for #26) — deferred to
  its own future ADR, as above.
- Whether `--strict-types` results should be cacheable across runs (a
  real performance question once package loading cost is measured against
  actual repos, not decided speculatively here).
- Whether `--strict-types` should ever become the default — no; Phase 1's
  no-build guarantee is a permanent product commitment (ADR 002), not a
  temporary bootstrapping state.
