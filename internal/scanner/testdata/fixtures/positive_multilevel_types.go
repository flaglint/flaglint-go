package fixtures

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

// Mirrors weaviate/weaviate's actual shape: an inner wrapper struct holding
// the client (LDIntegration), and an outer struct (FeatureFlag) holding a
// pointer to the inner one — so real usage is a two-level field chain
// (f.integ.ldClient.Method(...)), not a direct field access.
type MultiLevelIntegration struct {
	ldClient *ld.LDClient
}

func newMultiLevelIntegration() *MultiLevelIntegration {
	client, _ := ld.MakeClient("sdk-key", 5*time.Second)
	return &MultiLevelIntegration{ldClient: client}
}

type MultiLevelFeatureFlag struct {
	integ *MultiLevelIntegration
}
