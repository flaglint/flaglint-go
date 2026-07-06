package fixtures

import "time"

// A local MakeClient function with the exact same name as the SDK
// constructor, but reached through no LaunchDarkly import whatsoever. Proves
// detection keys off the traced import, never off the constructor name in
// isolation.
func MakeClient(key string, timeout time.Duration) (*unrelatedClient, error) {
	return &unrelatedClient{}, nil
}

type unrelatedClient struct{}

func (c *unrelatedClient) BoolVariation(key string, ctx interface{}, def bool) (bool, error) {
	return def, nil
}

func runUnrelatedMakeClient() {
	client, _ := MakeClient("key", 5*time.Second)
	_, _ = client.BoolVariation("cache-warm-flag", nil, false)
}
