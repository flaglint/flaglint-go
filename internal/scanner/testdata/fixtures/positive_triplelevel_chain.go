package fixtures

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

// A three-level field chain (o.middle.integ.ldClient), proving
// resolveChainType's recursion isn't only correct for the two-level case
// the weaviate-shaped fixtures (positive_multilevel_*.go) happen to
// exercise.
type TripleLevelIntegration struct {
	ldClient *ld.LDClient
}

func newTripleLevelIntegration() *TripleLevelIntegration {
	client, _ := ld.MakeClient("sdk-key", 5*time.Second)
	return &TripleLevelIntegration{ldClient: client}
}

type TripleLevelMiddle struct {
	integ *TripleLevelIntegration
}

type TripleLevelOuter struct {
	middle *TripleLevelMiddle
}

func (o *TripleLevelOuter) Evaluate() bool {
	v, _ := o.middle.integ.ldClient.BoolVariation("triplelevel-flag", nil, false)
	return v
}
