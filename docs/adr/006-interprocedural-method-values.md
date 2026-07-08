# ADR 006 — Phase 2b: Interprocedural Method-Value Propagation

Date: 2026-07
Status: Accepted

## Decision

Ship approach A (the narrow "forwarding function" pattern, below) for
[issue #26](https://github.com/flaglint/flaglint-go/issues/26) — closing
the loop from [ADR 005](005-strict-types-pass.md), which deferred this
exact case as "Phase 2b". Approach B (general data-flow via `go/ssa`/
`go/callgraph`) remains available as a future, separately-justified
escalation, not pursued now.

## Context

[Issue #6](https://github.com/flaglint/flaglint-go/issues/6) closed the
same-scope method-value case (`f := client.BoolVariation; f(...)`, all
within one function). Issue #26 is the harder remainder: a method value
captured in one function, passed as an argument into a *different*
function, and invoked from inside that callee — a real, field-tested
pattern (e2b-dev/infra passes a bound client's method value through a
generic helper).

```go
func setup() {
	client, _ := ld.MakeClient("sdk-key", 5*time.Second)
	callGeneric(client.BoolVariation, "flag", false) // captured here
}

func callGeneric[T any](f func(string, ldcontext.Context, T) (T, error), key string, def T) T {
	v, _ := f(key, ctx, def) // invoked here — a different function
	return v
}
```

### The wrinkle found while scoping this: the flag key is *also* interprocedural

It's not just the method value's identity that crosses the function
boundary — the flag key does too. `key` inside `callGeneric` is a plain
parameter; its value ("flag") only exists as a literal at the *call
site*, `setup()`. This means detection can't simply "propagate a binding
for `f` into `callGeneric`'s body and treat `f(key, ctx, def)` as a
usage" — even with `f`'s identity resolved, `key` isn't a literal from
inside `callGeneric`, so no flag key could be extracted there. The
usage has to be attributed to the *call site* (`callGeneric(client.
BoolVariation, "flag", false)`), with the flag key read from that
call's own literal argument — which requires understanding what
`callGeneric`'s body actually *does* with `f`, not just what `f`
resolves to.

This is a materially different (and harder) shape than #15/#16's Phase
2a fixes, which only ever needed a single expression's static type —
never "does this function's body forward its parameter to a call using
another of its own parameters positionally as the key."

## Two candidate approaches

### A — Narrow "forwarding function" pattern (recommended first cut)

Recognize one specific, verifiable shape rather than general data flow:

1. A function `F` has a parameter `p` whose type (after generic
   instantiation) matches one of `methodSpecs`'s known method signatures.
2. `F`'s body calls `p` directly, with one of `F`'s *own* parameters in
   the position that signature's `keyArgIndex` expects.
3. At a call site `F(someExpr, someArg, ...)`, if `someExpr` resolves to
   a proven client's method value (reusing #6's existing same-scope
   method-value binding — `bindLocalValue`/`methodValueBinding` in
   identity.go) *and* the argument in `F`'s own key-parameter position is
   a static string literal, record *that call site* as the usage, with
   the flag key read from the literal argument.

This is a whole-scan pre-pass in the same spirit as factory.go's
existing "which functions produce a proven client" index (ADR 004) — a
new one answering "which functions forward a client method-value
parameter to a call, and at which of their own parameter positions is
the flag key." No `go/ssa`/`go/callgraph` needed; `go/types` alone
resolves the parameter's real (post-generic-instantiation) type, and the
rest is a structural check on `F`'s own body, done once per function
during the whole-scan pass, then consulted at each call site.

**What this catches**: exactly the e2b-dev/infra shape and close
variants (a generic or non-generic "invoke this callback with a key"
helper, called directly).

**What this does not catch** (explicitly, not silently): the method
value stored in a struct field and invoked from a much later, unrelated
call; passed through two or more layers of indirection before use;
returned from a function rather than called directly; invoked
conditionally in a way the structural check can't verify statically. All
of these remain false negatives, consistent with "false negative over
false positive" — but worth being explicit that this is a heuristic
matching a documented, real-world shape, not a general solution.

### B — General data-flow via `go/ssa` / `go/callgraph`

Build a real call graph (`golang.org/x/tools/go/callgraph`, likely via
`go/ssa`'s pointer analysis or a simpler CHA/RTA builder) and track method
values as an actual data-flow fact propagated through it. This would
generalize past approach A's forwarding-only shape to arbitrary
indirection depth, at real cost:

- A meaningfully larger, riskier engineering investment — `go/ssa`
  changes the program representation entirely (SSA form, not AST), and
  correctly mapping SSA values back to source positions for fingerprint/
  reporting purposes is real, fiddly work.
- Call graph construction has its own soundness/precision tradeoffs
  (CHA is fast but imprecise — many spurious edges; RTA/pointer analysis
  is more precise but slower and more complex to get right) that would
  need their own careful evaluation against this project's "prefer false
  negative over false positive" bar.
- Given `--strict-types` is already opt-in and already accepts real
  performance cost (ADR 005), a further "some real repos might time out
  or need excessive memory for pointer analysis" caveat compounds that
  tradeoff further.

## Recommendation

Ship A first. It directly closes the one real, field-tested case this
issue exists for, reuses infrastructure already in place (`typecheck.
Load`, `resolveByStaticType`, #6's method-value binding), and keeps
Phase 2b's initial footprint proportionate to its one confirmed
motivating example — matching how #16 turned out to need no new code at
all once #15 shipped: a well-scoped mechanism generalizing further than
expected is a better outcome than a broad mechanism that arrives late or
carries type-checker-scale risk. B remains available as a future,
separately-justified escalation if real-world field-testing turns up
cases A's narrower pattern doesn't reach — the same "ship the contained
fix, escalate only if the real world asks for it" discipline this
project has followed throughout (see #18, #17, #6→#26's own split).

## What This ADR Does Not Decide

- The exact data structures/file layout for approach A's implementation
  (left to the implementation PR, as with prior ADRs in this series).
- Whether approach B is ever pursued — deferred indefinitely unless a
  future field-testing round finds a real case A's pattern doesn't cover.
