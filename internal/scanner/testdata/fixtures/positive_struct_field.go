package fixtures

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

type Service struct {
	Client *ld.LDClient
}

func setup(s *Service) {
	s.Client, _ = ld.MakeClient("sdk-key", 5*time.Second)
}

func useStructField(s *Service) {
	v, _ := s.Client.BoolVariation("struct-field-flag", nil, false)
	_ = v
}
