package fixtures

// A method value taken from an unrelated type with a coincidentally
// matching method name — must never be detected, proving method-value
// binding requires the receiver to actually be a proven client at the
// point of capture, not just any identifier with a same-named method.
type unrelatedMethodValueClient struct{}

func (c *unrelatedMethodValueClient) BoolVariation(key string, ctx interface{}, def bool) (bool, error) {
	return def, nil
}

func useUnrelatedMethodValue() {
	unrelated := &unrelatedMethodValueClient{}
	f := unrelated.BoolVariation
	_, _ = f("should-not-be-detected", nil, false)
}
