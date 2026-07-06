package fixtures

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

func runParenthesized() {
	client, _ := (ld.MakeClient)("sdk-key", 5*time.Second)
	_, _ = (client.BoolVariation)("paren-flag", nil, false)
}
