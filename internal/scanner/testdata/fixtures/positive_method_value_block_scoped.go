package fixtures

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

type unrelatedMethodValueHelper struct{}

func (h *unrelatedMethodValueHelper) BoolVariation(key string, ctx interface{}, def bool) (bool, error) {
	return false, nil
}

func newUnrelatedMethodValueHelper() *unrelatedMethodValueHelper {
	return &unrelatedMethodValueHelper{}
}

// Proves method-value bindings follow the same block-scoped shadowing
// rules as client bindings (issue #5) — a re-`:=` of "f" inside a nested
// block, shadowing the outer real method value with an unrelated one,
// must not leak into the block, and the outer method value must still
// resolve correctly both before and after the block.
func methodValueBlockScopedShadowing() {
	client, _ := ld.MakeClient("sdk-key", 5*time.Second)
	f := client.BoolVariation
	_, _ = f("before-loop-method-value-flag", nil, false)

	for _, id := range []string{"a", "b"} {
		unrelatedClient := newUnrelatedMethodValueHelper()
		f := unrelatedClient.BoolVariation
		_, _ = f(id, nil, false)
	}

	_, _ = f("after-loop-method-value-flag", nil, false)
}
