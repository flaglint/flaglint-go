package fixtures

import (
	ld "github.com/launchdarkly/go-server-sdk/v7"
)

// The SDK IS imported in this file — but the variable calling BoolVariation
// is never constructed from ld.MakeClient/MakeCustomClient. Identity must
// be proven by the constructor binding, not by the fact that the SDK
// happens to be imported somewhere in the file, and not by the variable's
// name (it is deliberately named "client" to try to trick a name-based
// heuristic).
type FakeClient struct{}

func (f *FakeClient) BoolVariation(key string, ctx interface{}, def bool) (bool, error) {
	return def, nil
}

func newFakeClient() *FakeClient { return &FakeClient{} }

func runUnboundVariable() {
	client := newFakeClient()
	_, _ = client.BoolVariation("should-not-be-detected", nil, false)

	// The SDK is used for something unrelated to client construction, to
	// prove mere presence of a traced import isn't sufficient either.
	_ = ld.Config{}
}
