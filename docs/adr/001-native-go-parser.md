# ADR 001 — Native Go Parser Instead of tree-sitter-go

Date: 2026-07
Status: Accepted

## Decision

flaglint-go parses Go source using the standard library (`go/parser`, `go/ast`,
`go/token`) and, where deeper verification is needed, `go/types` /
`golang.org/x/tools/go/packages`. It does not use tree-sitter in any form.

## Context

flaglint-js's own architecture notes (ADR 006 in that repo, status PROPOSED,
later removed from public ADR tracking pending a working spike) sketched Go
support as a *second language engine embedded inside the npm CLI*, parsed via
`tree-sitter-go` through `node-tree-sitter` bindings. That plan explicitly
required a pre-implementation spike to validate that a native Node module
bundles acceptably across npm/Node 20/22/macOS/Linux/Windows, because
tree-sitter's Node bindings are a compiled native addon — exactly the kind of
cross-platform packaging risk that motivated the spike gate in the first
place.

flaglint-go does not have this problem, because it never has this problem:
it is a standalone Go binary, not a language engine embedded in a Node
runtime. Since the tool itself is written in Go, it has direct, zero-dependency
access to the same parser and type-checker the Go compiler uses. There is
nothing to spike — `go/parser` is part of every Go toolchain that will ever
run this binary.

## Rationale

1. **No cross-platform native-module risk.** `go/parser` is pure Go, part of
   the standard library. No CGo, no prebuilt grammar binaries, no per-platform
   packaging matrix to validate in CI — the exact category of risk ADR 006's
   Phase 0 spike existed to de-risk.
2. **No Node.js runtime dependency.** The entire reason to build a separate
   `flaglint-go` binary (per flaglint-js's own distribution gap analysis) is
   that Go/platform teams should not need `npx` in their CI to audit Go code.
   Using tree-sitter through Node bindings would reintroduce exactly the
   dependency this project exists to remove.
3. **Real type information is available for free.** `go/types` (or
   `golang.org/x/tools/go/packages` when a full build is desired) lets the
   scanner verify a variable's *static type* is the LaunchDarkly SDK's client
   type, rather than inferring identity purely from syntactic import-alias
   tracing. This is a stronger correctness guarantee than flaglint-js's
   TypeScript scanner can get without embedding a full type checker — see
   [ADR 002](002-client-identity-model.md).
4. **Grammar currency is guaranteed.** tree-sitter-go's grammar can lag
   language releases (generics support, new syntax). `go/parser` is maintained
   in lockstep with the language by definition.
5. **Single-pass audit tool, no incremental-parse requirement.** tree-sitter's
   headline strength — incremental re-parsing for editors — is irrelevant to a
   CLI that parses each file once per run.

## Consequences

- flaglint-go depends only on the Go standard library for parsing; `go/types`
  / `golang.org/x/tools` are added only if and when Phase 2 (type-verified
  client identity) is implemented.
- Distribution is a prebuilt binary (via `go install`, Homebrew, or a release
  archive) — no npm package, no `node-gyp`, no grammar bundling ever needed.
- Parsing correctness is bounded by whatever Go language version the *host
  toolchain* that built flaglint-go supports — in practice this tracks
  current Go closely since releases are frequent and this is a small binary.
- flaglint-go will not attempt to reuse any parsing code from flaglint-js;
  the two scanners are independent implementations that agree only on output
  contract (see [ADR 003](003-cross-tool-contract.md)).

## What This ADR Does Not Decide

- Whether flaglint-js still eventually implements ADR 006's embedded
  `tree-sitter-go` engine inside the npm CLI. That is a separate, independent
  effort — this ADR only explains why flaglint-go, a different binary aimed
  at a different distribution problem, made a different choice.
- The migration (`--apply`) safety model for Go, which requires its own ADR
  before it ships, matching the deferral pattern flaglint-js used for its own
  migrator.
