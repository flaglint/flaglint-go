package fixtures

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

// runA binds "client" to a real LaunchDarkly client — this usage IS real
// and must be detected.
func runA() {
	client, _ := ld.MakeClient("sdk-key", 5*time.Second)
	_, _ = client.BoolVariation("real-flag", nil, false)
}

// runB reuses the name "client" for something completely unrelated in a
// different function scope. Because bindings are scoped per top-level
// function (not tracked in one flat file-wide map), this must NOT be
// detected even though the variable name and call shape are identical to
// runA's real usage.
func runB() {
	client := newUnrelatedClient()
	_, _ = client.BoolVariation("should-not-be-detected", nil, false)
}

type unrelatedClientB struct{}

func (c *unrelatedClientB) BoolVariation(key string, ctx interface{}, def bool) (bool, error) {
	return def, nil
}

func newUnrelatedClient() *unrelatedClientB { return &unrelatedClientB{} }
