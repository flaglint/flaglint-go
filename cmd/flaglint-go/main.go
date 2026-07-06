// Command flaglint-go is the native Go binary for scanning LaunchDarkly Go SDK
// usage. It shares its JSON, SARIF, and baseline contracts with flaglint-js
// (github.com/flaglint/flaglint-js) but ships as a standalone Go binary with
// no Node.js dependency.
package main

import (
	"os"

	"github.com/flaglint/flaglint-go/internal/cli"
)

// version is stamped at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(cli.Execute(version))
}
