package fixtures

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

// A real client construction and method call, both inside an immediately-
// invoked function literal assigned to a package-level var. This must be
// scanned even though the initializer is a CallExpr wrapping the FuncLit,
// not the FuncLit directly.
var startupFlag = func() bool {
	client, _ := ld.MakeClient("sdk-key", 5*time.Second)
	v, _ := client.BoolVariation("startup-flag", nil, false)
	return v
}()
