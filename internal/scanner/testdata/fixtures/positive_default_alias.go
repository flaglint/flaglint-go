package fixtures

import (
	"time"

	"github.com/launchdarkly/go-server-sdk/v7"
)

// No explicit import alias — the local identifier is the SDK's declared
// package name, "ldclient", not a path-derived guess.
func runDefaultAlias() {
	client, _ := ldclient.MakeClient("sdk-key", 5*time.Second)
	_, _ = client.IntVariation("limit", nil, 10)
}
