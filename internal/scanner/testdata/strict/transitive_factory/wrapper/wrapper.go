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

// OuterWrapper adds a second hop (Middle.Inner, not just Inner) — proving
// resolveByStaticType's go/types.Info.TypeOf(rhs) generalizes to an
// arbitrarily deep field-selector chain for free, since it queries the
// expression's real type directly rather than manually walking one hop at
// a time the way Phase 1's syntax-only structFieldTypes chain resolution
// does.
type OuterWrapper struct {
	Middle *ClientWrapper
}

func NewOuterWrapper() *OuterWrapper {
	return &OuterWrapper{Middle: NewClientWrapper()}
}
