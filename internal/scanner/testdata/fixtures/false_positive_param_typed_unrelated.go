package fixtures

// A parameter type named exactly "LDClient" — the same bare name the SDK
// uses — but declared locally in this file, with no SDK import anywhere
// (not even a dot-import). Must not be detected: proves paramClientBindings
// requires an actual traced SDK import to resolve against, not just a
// matching type name text.
type LDClient struct{}

func (c *LDClient) BoolVariation(key string, ctx interface{}, def bool) (bool, error) {
	return def, nil
}

func useUnrelatedLDClientParam(client *LDClient) {
	_, _ = client.BoolVariation("should-not-be-detected", nil, false)
}
