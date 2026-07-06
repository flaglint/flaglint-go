// Command flaglint-go is the native Go binary for scanning LaunchDarkly Go SDK
// usage. It shares its JSON, SARIF, and baseline contracts with flaglint-js
// (github.com/flaglint/flaglint-js) but ships as a standalone Go binary with
// no Node.js dependency.
package main

import (
	"fmt"
	"os"
)

// version is stamped at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println(version)
		return
	}
	fmt.Fprintln(os.Stderr, "flaglint-go: no commands implemented yet")
	os.Exit(3)
}
