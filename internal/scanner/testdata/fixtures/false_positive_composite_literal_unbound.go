package fixtures

// A struct with the same field name pattern as positive_composite_literal.go
// (ldClient), constructed via composite literal, but the value is never a
// bound SDK client — proving composite-literal binding requires the value
// to actually resolve to a client (constructor call or already-bound
// identifier), not just any value assigned to a field named "ldClient".
type unrelatedComposite struct{}

func (c *unrelatedComposite) BoolVariation(key string, ctx interface{}, def bool) (bool, error) {
	return def, nil
}

type FakeIntegration struct {
	ldClient *unrelatedComposite
}

func setupFakeComposite() *FakeIntegration {
	return &FakeIntegration{ldClient: &unrelatedComposite{}}
}

func useFakeComposite(i *FakeIntegration) {
	_, _ = i.ldClient.BoolVariation("should-not-be-detected", nil, false)
}
