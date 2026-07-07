package fixtures

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

// Reproduces flaglint-go issue #6's exact same-scope repro: a method
// value taken from a bound client (no call parens), then invoked via a
// bare call at the identifier — not a selector expression at the call
// site itself.
func useMethodValue() {
	client, _ := ld.MakeClient("sdk-key", 5*time.Second)
	f := client.BoolVariation
	_, _ = f("method-value-flag", nil, false)
}
