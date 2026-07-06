# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Project scaffold: Go module, LICENSE, CONTRIBUTING, CI conventions.
- `flaglint-go scan` — inventory report (JSON/Markdown) of LaunchDarkly Go
  SDK usage, always exits 0 unless there's a tool error.
- `flaglint-go audit` — scan plus a risk-based migration-readiness score;
  `--write-baseline` captures current findings for CI adoption.
- `flaglint-go validate` — the only command that ever exits 1:
  `--no-direct-launchdarkly` policy enforcement, `--bootstrap-exclude` glob
  exceptions, `--baseline`/`--fail-on-new` for CI adoption of only new debt,
  text and SARIF (`flaglint.go.direct-launchdarkly`) output.
- Scanner: import-alias tracing + constructor-call binding for
  `github.com/launchdarkly/go-server-sdk` v6/v7 client identity — no
  name-based heuristics (ADR 002). Detects `BoolVariation`/`StringVariation`/
  `IntVariation`/`Float64Variation`/`JSONVariation`, their `*Ctx` and
  `*Detail(Ctx)` variants, and `AllFlagsState`. Local-variable bindings are
  scoped per function; struct-field bindings are type-qualified — both
  guard against distinct classes of false positives, each covered by a
  dedicated fixture.
- Baseline file format matching flaglint-js byte-for-byte (`version` as the
  JSON string `"1"`, sorted+deduped fingerprints).
- CI: cross-platform test matrix (ubuntu/macos/windows), golangci-lint,
  CodeQL, DCO enforcement, Conventional Commit PR titles.
- Release automation: goreleaser config for cross-platform binaries and an
  automatic Homebrew formula publish to `flaglint/homebrew-tap`.

### Cross-tool contract

See [docs/adr/003-cross-tool-contract.md](docs/adr/003-cross-tool-contract.md)
for the full, verified-against-source contract with flaglint-js (fingerprint
algorithm, exit codes, config field names, baseline schema, SARIF
conventions). Two discrepancies were found in flaglint-js's *current shipped
behavior* (not its documented contract) during this work and reported
upstream rather than replicated: directory-validation and `--output`
write-failure errors there currently exit 1 instead of the documented 2/3
(flaglint/flaglint-js#209).
