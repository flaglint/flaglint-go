package producer

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

// Reproduces flaglint-go issue #17: this file lives under its OWN nested
// go.mod (testdata/fixtures/monorepo/submodule/go.mod, declaring module
// github.com/example/monorepo-submodule) — a different, independent
// module from the outer flaglint-go repo this fixture lives inside.
func GetLdClient() *ld.LDClient {
	client, _ := ld.MakeClient("sdk-key", 5*time.Second)
	return client
}
