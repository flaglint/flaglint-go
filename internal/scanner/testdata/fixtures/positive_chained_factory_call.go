package fixtures

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

// Same-package variant of issue #20: a direct chained call on a factory
// function's result, no intermediate variable.
func chainedFactory() *ld.LDClient {
	client, _ := ld.MakeClient("sdk-key", 5*time.Second)
	return client
}

func useChainedFactory() {
	_, _ = chainedFactory().BoolVariation("chained-factory-flag", nil, false)
}
