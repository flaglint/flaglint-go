package fixtures

import (
	"time"

	. "github.com/launchdarkly/go-server-sdk/v7"
)

func runDotImport() {
	client, _ := MakeClient("sdk-key", 5*time.Second)
	_, _ = client.BoolVariation("dot-import-flag", nil, false)
}
