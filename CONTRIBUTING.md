# Contributing to flaglint-go

Thank you for your interest in contributing.

## Prerequisites

- Go 1.22+

## Setup

```bash
git clone https://github.com/flaglint/flaglint-go.git
cd flaglint-go
go build ./...
go test ./...
```

## Development

```bash
go run ./cmd/flaglint-go audit ./testdata
go vet ./...
golangci-lint run
```

## Testing

```bash
go test ./...              # unit + fixture tests
go test ./... -cover       # with coverage
```

All tests must pass and `go vet` / `golangci-lint` must be clean before a PR will be merged.

## Code style

- Standard `gofmt`/`goimports` formatting — CI rejects unformatted diffs
- No name-based heuristics for LaunchDarkly client detection — identity must be
  traced through import aliases and `go/types` verification, matching the
  provably-correct model documented in [ADR 002](docs/adr/002-client-identity-model.md)
- Keep functions small and single-purpose; prefer explicit error returns over panics
- No comments that restate what the code already says

## Cross-tool contract

flaglint-go must stay behaviorally compatible with
[flaglint-js](https://github.com/flaglint/flaglint-js) — same fingerprint
schema, exit codes, config field names, baseline file format, and SARIF rule
ID conventions. See [ADR 003](docs/adr/003-cross-tool-contract.md) before
changing any output format. A change to output shape needs a version bump on
both sides.

## Adding a new detection pattern

1. Add the detection logic in `internal/scanner/`
2. Add a corresponding fixture in `internal/scanner/testdata/fixtures/`
   (including a false-positive fixture if the pattern could ever match
   non-LaunchDarkly code)
3. Add test cases exercising the fixture
4. Update the README detection/scope table

## PR process

1. Create a feature branch: `git checkout -b feat/my-feature`
2. Make your changes with tests
3. Ensure `go test ./...`, `go vet ./...`, and `golangci-lint run` all pass
4. Open a PR with a clear description of the change and why

## Pull request requirements

Before a PR can be merged:

- The PR title should follow Conventional Commit style, for example `fix: handle aliased LaunchDarkly imports`.
- Commits must be DCO signed (`git commit -s`) — the `Signed-off-by` trailer is required on every commit.
- Tests, vet, and lint must pass in CI.
- A maintainer review is required before merge.

## Commit format

Use [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add JSONVariation detection
fix: handle aliased LaunchDarkly go-server-sdk imports
docs: update README scope table
test: add AllFlagsState fixture
chore: bump golangci-lint version
```

Types: `feat`, `fix`, `docs`, `test`, `chore`, `refactor`, `perf`, `ci`, `build`
