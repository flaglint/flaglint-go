package fixtures

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

// Two unrelated struct types both have a field named "Client", both
// constructed/used through a parameter named "s". Field bindings are
// type-qualified ("RealService.Client" vs "FakeService.Client"), so these
// must never be confused even though the raw receiver text ("s.Client") is
// identical in both cases.
type RealService struct {
	Client *ld.LDClient
}

type FakeService struct {
	Client *unrelatedFieldClient
}

type unrelatedFieldClient struct{}

func (c *unrelatedFieldClient) BoolVariation(key string, ctx interface{}, def bool) (bool, error) {
	return def, nil
}

func setupReal(s *RealService) {
	s.Client, _ = ld.MakeClient("sdk-key", 5*time.Second)
}

func useReal(s *RealService) {
	v, _ := s.Client.BoolVariation("real-field-flag", nil, false)
	_ = v
}

func setupFake(s *FakeService) {
	s.Client = &unrelatedFieldClient{}
}

func useFake(s *FakeService) {
	// Must NOT be detected: s here is *FakeService, never constructed via
	// ld.MakeClient, even though "s.Client" is textually identical to the
	// real binding above.
	_, _ = s.Client.BoolVariation("should-not-be-detected", nil, false)
}
