package fixtures

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

type unrelatedHelper struct{}

func (c *unrelatedHelper) BoolVariation(key string, ctx interface{}, def bool) (bool, error) {
	return def, nil
}

func newUnrelatedHelper(id string) *unrelatedHelper { return &unrelatedHelper{} }

// blockScopedShadowing reproduces flaglint-go issue #5 exactly: a
// deliberate re-`:=` of "client" inside a nested block (the for-loop
// body), shadowing the outer real client with an unrelated type. The
// shadowed usage inside the loop must NOT be detected, but the real
// client must still be correctly detected both before the loop and after
// it ends — proving the fix is properly scoped (the shadow doesn't
// permanently destroy the outer binding once the block exits).
func blockScopedShadowing() {
	client, _ := ld.MakeClient("sdk-key", 5*time.Second)
	_, _ = client.BoolVariation("before-loop-flag", nil, false)

	for _, id := range []string{"a", "b"} {
		client := newUnrelatedHelper(id)
		_, _ = client.BoolVariation("should-not-be-detected", nil, false)
	}

	_, _ = client.BoolVariation("after-loop-flag", nil, false)
}
