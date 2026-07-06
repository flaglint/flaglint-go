package unrelatedfield

// An unrelated package that happens to declare a same-named struct
// ("Service") and same-named field ("Client") as
// pkgisolation/realfield/service.go, with a same-named method
// (BoolVariation) — but has no connection to the LaunchDarkly SDK at all.
// Must never be detected: proves whole-scan struct-field binding is
// partitioned per-package, not a single flat cross-package map keyed by
// bare "TypeName.Field" text.
type FakeClient struct{}

func (c *FakeClient) BoolVariation(key string, ctx interface{}, def bool) (bool, error) {
	return def, nil
}

type Service struct {
	Client *FakeClient
}

func newService() *Service {
	return &Service{Client: &FakeClient{}}
}

func useService(s *Service) {
	_, _ = s.Client.BoolVariation("should-not-be-detected", nil, false)
}
