package fixtures

// No LaunchDarkly SDK import anywhere in this file. A custom type exposing
// a method literally named BoolVariation, with the exact same call shape
// as the real SDK, must never be detected — there is no import to trace.
type PermissionChecker struct{}

func (p *PermissionChecker) BoolVariation(key string, ctx interface{}, def bool) (bool, error) {
	return def, nil
}

func runNoSDKImport() {
	p := &PermissionChecker{}
	_, _ = p.BoolVariation("not-a-real-flag", nil, false)
}
