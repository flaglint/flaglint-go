package realvar

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

// A genuinely bound package-level var, in its own package.
var client, _ = ld.MakeClient("sdk-key", 5*time.Second)

func useClient() {
	_, _ = client.BoolVariation("pkg-isolation-var-flag", nil, false)
}
