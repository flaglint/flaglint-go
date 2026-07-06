package fixtures

import (
	"context"
	"fmt"
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

func runVariety(flagName string) {
	client, _ := ld.MakeClient("sdk-key", 5*time.Second)

	// Dynamic: identifier argument.
	_, _ = client.BoolVariation(flagName, nil, false)

	// Dynamic: non-literal expression (fmt.Sprintf call).
	_, _ = client.StringVariation(fmt.Sprintf("flag-%d", 1), nil, "x")

	// High risk: *Detail method.
	_, _, _ = client.BoolVariationDetail("detail-flag", nil, false)

	// Bulk call: no single flag key.
	_ = client.AllFlagsState(nil)

	// *Ctx variant: flag key is argument index 1, not 0.
	_, _ = client.BoolVariationCtx(context.Background(), "ctx-flag", nil, false)
}
