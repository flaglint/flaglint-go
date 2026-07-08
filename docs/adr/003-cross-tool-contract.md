# ADR 003 — Cross-Tool Contract with flaglint-js

Date: 2026-07
Status: Accepted

## Decision

flaglint-go must produce byte-identical fingerprints, matching exit codes,
matching config field names, and matching SARIF/baseline conventions to
flaglint-js, so that a monorepo containing both JS/TS and Go services can mix
both tools' output — same baseline file, same CI gate, same Code Scanning
dashboard — without special-casing which language produced a finding.

This document is the single source of truth for that contract, verified
directly against flaglint-js `v1.1.0` (`origin/main` @ `6eefb35`). If
flaglint-js changes any of these, this ADR must be updated in the same PR
that changes the Go side — a drift between the two is a compatibility bug,
not a style choice.

## Fingerprint algorithm (must match exactly)

flaglint-js (`src/scanner/fingerprint.ts`):

```
normalizePath(path) = path.replace(/\\/g, "/").replace(/^\.\//, "")
key = (flagKey == "*" || flagKey == "") ? "*" : flagKey
fingerprint = "launchdarkly:" + callType + ":" + key + ":" + normalizePath(file)
if dynamic: fingerprint += ":" + dynamicIndex
```

flaglint-go implements the identical function in
`internal/fingerprint`, with one necessary difference: `callType` values are
drawn from the Go SDK's method names (`BoolVariation`, `AllFlagsState`, ...)
rather than the JS scanner's generic vocabulary (`variation`, `hoc`,
`provider`, ...), because the two SDKs expose different call shapes. This is
not a contract violation — `callType` was never meant to be a shared enum
across languages, only a stable string within one finding's fingerprint.

Rules that **do** carry over exactly, with no exceptions:

- Provider segment is always the literal `launchdarkly`.
- Path separator is always `/` — normalize backslashes on any platform.
- No leading `./` in the path segment.
- Empty or wildcard flag keys (bulk calls like `AllFlagsState`) collapse to
  the literal `*`.
- `dynamicIndex` is a single counter **per file**, starting at `0`,
  incremented in source-encounter order across *all* dynamic call sites in
  that file regardless of call type — not reset per call type, not reset per
  flag key. This must match flaglint-js's `let dynamicIndex = 0` /
  `dynamicIndex++` behavior in `src/scanner/index.ts` exactly, because a
  baseline written against one tool's numbering and read by the other must
  agree on which finding is which.

## Exit code contract (must match exactly)

| Code | Meaning |
|---|---|
| `0` | Success, no violations |
| `1` | Policy/staleness failure (validate found violations, `--fail-on-new` triggered) |
| `2` | Invalid user input (bad `--format`, missing/malformed `--baseline` file, invalid config) |
| `3` | Internal/unexpected error |
| `130` | SIGINT |

`audit` and `scan` are inventory commands and must never set exit code 1 for
findings — only `validate` enforces policy. (flaglint-js itself shipped this
wrong in an earlier version — `scan` exiting 1 on stale-flag heuristics — and
fixed it in v1.1.0. flaglint-go should not repeat that mistake.)

## Config contract

- File search order: `.flaglintrc` → `.flaglintrc.json` → `flaglint.config.json`
- Field names are camelCase and must match flaglint-js exactly where the
  concept is shared: `include`, `exclude`, `provider`, `reportTitle`,
  `outputDir`. Go-specific fields (e.g. a future `goModules` allowlist) are
  additive and must not collide with JS field names.
- `provider` must be validated the same way flaglint-js validates it as of
  v1.1.0: an unsupported value is a hard error (exit 2), never silently
  ignored.

## Baseline file format (must match exactly)

```json
{
  "version": "1",
  "createdAt": "<ISO 8601>",
  "flaglintVersion": "<semver of the tool that wrote it>",
  "fingerprints": ["...sorted, deduplicated..."]
}
```

- `version` is the **string** `"1"`, not the number `1` — this was itself a
  documentation/implementation mismatch in flaglint-js (ADR 008) that was
  resolved in v1.1.0 by fixing the docs to match the shipped string type.
  flaglint-go must use the string form to stay compatible.
- `fingerprints` must be sorted and deduplicated on write, matching
  flaglint-js's `writeBaseline`.
- A baseline is valid input to either tool's `validate --baseline` regardless
  of which tool wrote it, provided the fingerprints inside were generated
  against files that still exist at those paths.

## SARIF contract

- `$schema`: `https://json.schemastore.org/sarif-2.1.0.json`, `version: "2.1.0"`
- `uriBaseId`: literal `%SRCROOT%`
- `partialFingerprints` key is `"<ruleId-suffix>/v1"`, matching flaglint-js's
  `partialFingerprints["flagKey/v1"]` pattern, so GitHub Code Scanning
  deduplicates correctly across re-runs.
- Rule IDs are namespaced by language so JS and Go findings never collide in
  a shared Code Scanning dashboard: flaglint-js uses
  `flaglint.direct-launchdarkly`; flaglint-go uses
  `flaglint.go.direct-launchdarkly` (and equivalents), per the original
  language-support design notes in flaglint-js's ADR 006.

## stdout/stderr contract

Reports (the actual JSON/Markdown/SARIF/HTML payload) go to stdout, or to a
file when `--output` is given. Progress, spinners, warnings, and summaries go
to stderr. This is what lets `flaglint-go scan ./svc | jq .` work, matching
flaglint-js.

## FlagUsage / ScanResult shape

flaglint-go's `FlagUsage` mirrors flaglint-js's current shape
(`flagKey`, `isDynamic`, `file`, `line`, `column`, `callType`, `fingerprint`)
and adds two fields, additively, that flaglint-js's schema does not currently
have:

- `language`: `"go"` — per flaglint-js ADR 006's schema sketch, this field is
  additive on the JS side too (existing JS findings would gain
  `"language": "typescript"` if/when that ADR is picked back up).
- `sdk`: e.g. `"go-server-sdk-v7"` — which SDK major version produced the
  finding, since v6 and v7 differ in a few call signatures.
- `risk`: `"low" | "medium" | "high"` — the table from
  [ADR 002](002-client-identity-model.md). flaglint-js does not currently
  expose a `risk` field on `FlagUsage` (it expresses risk indirectly through
  `stalenessSignals` and command-specific logic); this is a Go-side addition
  that a future flaglint-js version could adopt but is not required to.

`stalenessSignals` is always an empty array for Go findings in Phase 1 — Go
audit does not yet implement staleness heuristics (keyword/path/minFileCount).
This is a placeholder for parity, not a promise it will be populated soon.

`ScanResult.migrationInventory` mirrors flaglint-js's `MigrationInventoryItem`
field-for-field (`file`, `line`, `column`, `launchDarklyMethod`,
`callExpression`, `rangeStart`, `rangeEnd`, `flagKeyExpression`,
`staticFlagKey`, `isDynamic`, `valueType`, `fallbackExpression`,
`evaluationContextExpression`, `safelyAutomatable`, `manualReviewReason`) —
richer, migration-focused detail a future `migrate` command would need to
safely rewrite a call, not just report it (flaglint-go has no `migrate`
command yet — see ADR 002's Phase 1 scope). One deliberate omission:
flaglint-js's `isAwaited?: boolean` has no Go equivalent (no async/await) and
is never emitted. Unlike flaglint-js (whose generic `variation()`/
`isFeatureEnabled()` calls require inferring `valueType` from the fallback
argument's runtime type), every Go SDK method name is already type-specific,
so `valueType` is always derived directly from the method name — flaglint-go
never actually produces `"unknown-fallback"` as a `manualReviewReason` as a
result (kept in the type for cross-tool consistency regardless). A
`--strict-types`-only usage detected via the "forwarding function" pattern
(ADR 006) has no corresponding `migrationInventory` item: its call site
doesn't directly show the LD method's own `(key, context, fallback)`
arguments, so an item for it would misrepresent what's actually safe to
rewrite. Every other usage — Phase 1 and interface-satisfaction/transitive-
factory-wrapping `--strict-types` findings alike — gets one, since their call
sites are all ordinary `client.Method(key, context, fallback)` shapes
regardless of how the receiver's identity was proven.

## Consequences

- Any PR to flaglint-go that changes fingerprint format, exit codes, config
  field names, baseline shape, or SARIF conventions must update this ADR in
  the same PR.
- Any PR to flaglint-js that changes these must be checked against this ADR;
  if it drifts, flaglint-go needs a corresponding fix, tracked as an issue
  against this repo.
- No code sharing between the two scanners is attempted — the contract is
  enforced by this document and by fixtures, not by a shared implementation.

## What This ADR Does Not Decide

- Whether a shared JSON Schema package should be published and consumed by
  both tools (flaglint-js issue #123 proposes publishing one for its own
  config; if that ships, flaglint-go should consume it rather than
  hand-maintain a duplicate — tracked as a follow-up, not decided here).
