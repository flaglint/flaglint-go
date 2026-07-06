package fixtures

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

// Package-level var bound in this file, used from a *different* file in
// the same package (positive_crossfile_pkgvar_b.go) — proves package-level
// bindings are resolved whole-scan, not just within the file that
// declares them.
var crossFilePkgClient, _ = ld.MakeClient("sdk-key", 5*time.Second)
