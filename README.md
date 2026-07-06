# flaglint-go

Native Go binary for auditing [LaunchDarkly Go server SDK](https://github.com/launchdarkly/go-server-sdk)
usage — no Node.js required.

> **Status: early development.** This is the Go-native counterpart to
> [flaglint-js](https://github.com/flaglint/flaglint-js), built for teams that
> don't want a Node.js dependency in their toolchain. It shares the same
> fingerprint, exit-code, config, and SARIF contracts — see
> [docs/adr/003-cross-tool-contract.md](docs/adr/003-cross-tool-contract.md).

## Why a separate binary instead of the npm CLI's Go support?

flaglint-js's own architecture notes (ADR 006, deferred) originally proposed
adding Go support inside the npm CLI via `tree-sitter-go`. flaglint-go takes a
different path: since this ships as a standalone Go binary, it uses Go's
standard `go/parser`/`go/ast` directly — no tree-sitter, no Node runtime. See
[docs/adr/001-native-go-parser.md](docs/adr/001-native-go-parser.md) for the
full rationale.

Client identity is proven through import-alias tracing and constructor-call
binding — never by variable or function name alone (see
[docs/adr/002-client-identity-model.md](docs/adr/002-client-identity-model.md)).
An opt-in `go/types`-verified pass for deeper indirection (factory functions,
interface satisfaction) is designed but **not yet implemented** — today's
scanner is syntax-only, no build required.

## Scope (current)

- **Supported SDK:** `github.com/launchdarkly/go-server-sdk` v6 and v7
- **Detected methods:** `BoolVariation`, `StringVariation`, `IntVariation`,
  `Float64Variation`, `JSONVariation`, their `*Ctx` and `*Detail(Ctx)`
  variants, and `AllFlagsState`
- **Risk classification:** low (simple static-key variation) / medium
  (`JSONVariation`) / high (`*Detail` methods, `AllFlagsState`, dynamic keys)
- **Out of scope for now:** migration rewrites (`--apply`), browser/mobile
  SDKs, non-LaunchDarkly providers, `go/types`-verified identity (Phase 2)

## Install

```bash
go install github.com/flaglint/flaglint-go/cmd/flaglint-go@latest
```

Prebuilt release binaries publish automatically once a version tag is cut —
see [.goreleaser.yaml](.goreleaser.yaml). A Homebrew formula
(`flaglint/tap/flaglint-go`) is planned but not yet published — its
publishing step needs a cross-repo token that isn't configured yet.

## Usage

```bash
# Inventory report — always exits 0 unless there's a tool error
flaglint-go scan ./services
flaglint-go scan ./services --format json

# Inventory + migration-readiness score
flaglint-go audit ./services
flaglint-go audit ./services --write-baseline .flaglint-baseline.json

# Policy enforcement — the only command that ever exits 1
flaglint-go validate ./services --no-direct-launchdarkly
flaglint-go validate ./services --no-direct-launchdarkly \
  --bootstrap-exclude "internal/openfeature-bootstrap/**"
flaglint-go validate ./services --format sarif --output flaglint.sarif

# CI adoption: fail only on debt introduced after the baseline was captured
flaglint-go validate ./services --baseline .flaglint-baseline.json --fail-on-new
```

### Exit codes

| Code | Meaning |
|---|---|
| 0 | Success — `scan`/`audit` always end here unless something breaks |
| 1 | Policy failure — `validate` only, never `scan`/`audit` |
| 2 | Invalid input — bad directory, bad `--format`, malformed config/baseline |
| 3 | Internal error |
| 130 | Interrupted (Ctrl-C) |

Full cross-tool contract (fingerprint format, baseline schema, SARIF rule IDs):
[docs/adr/003-cross-tool-contract.md](docs/adr/003-cross-tool-contract.md).

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT
