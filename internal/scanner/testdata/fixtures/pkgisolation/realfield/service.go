package realfield

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

// A genuinely bound struct field, in its own package.
type Service struct {
	Client *ld.LDClient
}

func newService() *Service {
	client, _ := ld.MakeClient("sdk-key", 5*time.Second)
	return &Service{Client: client}
}

func useService(s *Service) {
	_, _ = s.Client.BoolVariation("pkg-isolation-field-flag", nil, false)
}
