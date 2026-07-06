package fixtures

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v6"
)

func runV6() {
	client, _ := ld.MakeClient("sdk-key", 5*time.Second)
	_, _ = client.StringVariation("v6-flag", nil, "default")
}
