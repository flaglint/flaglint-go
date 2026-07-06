package fixtures

import (
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	ld "github.com/launchdarkly/go-server-sdk/v7"
)

func run() {
	client, _ := ld.MakeClient("sdk-key", 5*time.Second)
	ctx := ldcontext.New("user-key")

	enabled, _ := client.BoolVariation("checkout-v2", ctx, false)
	_ = enabled

	name, _ := client.StringVariation("greeting", ctx, "hi")
	_ = name
}
