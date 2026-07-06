package producer

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

// Mirrors the official launchdarkly-labs/ld-sample-app-go's exact pattern:
// a package-level singleton getter whose declared return type is
// *ld.LDClient, called from a different package.
func GetLdClient() *ld.LDClient {
	client, _ := ld.MakeClient("sdk-key", 5*time.Second)
	return client
}
