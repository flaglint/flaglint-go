package unrelatedvar

// An unrelated package with its own same-named package-level var
// ("client") and same-named method (BoolVariation) as
// pkgisolation/realvar/client.go — but no connection to the LaunchDarkly
// SDK at all. Must never be detected: proves whole-scan package-level var
// binding is partitioned per-package, not a single flat cross-package map
// keyed by bare identifier text.
type fakeClient struct{}

func (c *fakeClient) BoolVariation(key string, ctx interface{}, def bool) (bool, error) {
	return def, nil
}

var client = &fakeClient{}

func useClient() {
	_, _ = client.BoolVariation("should-not-be-detected", nil, false)
}
