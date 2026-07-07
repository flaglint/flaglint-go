package fixtures

// A direct chained call on a function that is NOT a registered factory
// (it returns an unrelated type) — must never be detected, proving the
// chained-call resolution (issue #20) keys off the inner call actually
// resolving to a real client, not just "the receiver is some call
// expression".
type unrelatedChainedClient struct{}

func (c *unrelatedChainedClient) BoolVariation(key string, ctx interface{}, def bool) (bool, error) {
	return def, nil
}

func newUnrelatedChainedClient() *unrelatedChainedClient {
	return &unrelatedChainedClient{}
}

func useUnrelatedChainedCall() {
	_, _ = newUnrelatedChainedClient().BoolVariation("should-not-be-detected", nil, false)
}
