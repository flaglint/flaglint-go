package wrapper

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

// ClientWrapper is issue #16's exact repro shape: a factory function
// (NewClientWrapper) whose *declared return type* is not *ld.LDClient
// directly, but a wrapper struct that itself has a client field.
type ClientWrapper struct {
	Inner *ld.LDClient
}

// NewClientWrapper is NOT a registered Phase 1 factory function (its
// return type is *ClientWrapper, not *ld.LDClient) — closing this
// generally requires real go/types information (--strict-types), per
// issue #16 and ADR 005.
func NewClientWrapper() *ClientWrapper {
	client, _ := ld.MakeClient("sdk-key", 5*time.Second)
	return &ClientWrapper{Inner: client}
}
