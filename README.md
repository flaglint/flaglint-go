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
standard `go/parser` and `go/types` directly — no tree-sitter, no Node runtime,
and access to real type-checking for client identity verification. See
[docs/adr/001-native-go-parser.md](docs/adr/001-native-go-parser.md) for the
full rationale.

## Scope (current)

- **Supported:** `github.com/launchdarkly/go-server-sdk` v6 and v7
- **Detected:** `BoolVariation`, `StringVariation`, `IntVariation`,
  `Float64Variation`, `JSONVariation`, `*VariationDetail`, `AllFlagsState`
- **Out of scope for now:** migration rewrites (`--apply`), browser/mobile SDKs,
  non-LaunchDarkly providers

## Install

```bash
go install github.com/flaglint/flaglint-go/cmd/flaglint-go@latest
```

## Usage

```bash
flaglint-go audit ./services
flaglint-go scan ./services --format json
flaglint-go validate ./services --baseline .flaglint-baseline.json --fail-on-new
```

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT
