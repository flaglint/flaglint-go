package fixtures

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

// Mirrors flaglint/corpus's composite-literal-binding fixture: the client
// is bound via a package-level composite literal — never inside any
// function body at all — which runWholeScanAnalysis's Pass B previously
// never visited (it only walked function scopes). svcClient itself is
// bound via Pass A's ordinary package-level constructor-call handling;
// what's new is svcWrapper's own composite literal, and resolving
// svcWrapper.Client's chain from a bare package-level identifier (not a
// function parameter/receiver) at the call site in useSvcWrapper.
type SvcWrapper struct {
	Client *ld.LDClient
}

var svcClient, _ = ld.MakeClient("sdk-key", 5*time.Second)
var svcWrapper = &SvcWrapper{Client: svcClient}

func useSvcWrapper() {
	_, _ = svcWrapper.Client.BoolVariation("package-level-composite-literal-flag", nil, false)
}

// unrelatedWrapper is a package-level composite literal too, but its
// field is never bound from a proven client — proving this mechanism
// requires the field's *value* to resolve, not just any package-level
// composite literal existing.
type unrelatedWrapper struct {
	Label string
}

var unrelatedGlobal = &unrelatedWrapper{Label: "not-a-flag-client"}

func useUnrelatedWrapper() string {
	return unrelatedGlobal.Label
}
