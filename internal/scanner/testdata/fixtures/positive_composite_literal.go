package fixtures

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

// Mirrors the real-world pattern found in weaviate/weaviate and
// e2b-dev/infra: the client is constructed, then stored into a wrapper
// struct via composite literal rather than direct field assignment.
type CompositeIntegration struct {
	ldClient *ld.LDClient
}

func setupComposite() *CompositeIntegration {
	client, _ := ld.MakeClient("sdk-key", 5*time.Second)
	return &CompositeIntegration{ldClient: client}
}

func useComposite(i *CompositeIntegration) {
	_, _ = i.ldClient.BoolVariation("composite-literal-flag", nil, false)
}
