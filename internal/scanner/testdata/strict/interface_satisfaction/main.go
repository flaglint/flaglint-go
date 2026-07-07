package main

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

// FlagEvaluator is an interface the real client happens to satisfy — Phase
// 1's syntactic tracing sees only this interface type, never the concrete
// *ld.LDClient underneath (issue #15). Only real go/types information
// (--strict-types) can prove evaluator really is a client here.
type FlagEvaluator interface {
	BoolVariation(key string, ctx interface{}, defaultVal bool) (bool, error)
}

func run() {
	client, _ := ld.MakeClient("sdk-key", 5*time.Second)
	var evaluator FlagEvaluator = client
	_, _ = evaluator.BoolVariation("interface-satisfaction-flag", nil, false)
}

// unrelatedEvaluator satisfies FlagEvaluator's shape but is not, and never
// wraps, a real client — proving interface-satisfaction resolution
// requires the concrete type to actually *be* *ld.LDClient, not just any
// value satisfying the same interface method set.
type unrelatedEvaluator struct{}

func (u *unrelatedEvaluator) BoolVariation(key string, ctx interface{}, defaultVal bool) (bool, error) {
	return defaultVal, nil
}

func runUnrelated() {
	var evaluator FlagEvaluator = &unrelatedEvaluator{}
	_, _ = evaluator.BoolVariation("should-not-be-detected", nil, false)
}

// runDirect and runInterfaceCollision both evaluate the exact same flag
// key with the exact same call type in this same file — a fingerprint
// (internal/fingerprint) is keyed on exactly that (callType, flagKey,
// file), deliberately omitting line/column, so these two distinct real
// call sites produce identical fingerprints. This reproduces a real bug
// caught during independent review: deduping ScanStrict's merge on
// fingerprint alone silently dropped runInterfaceCollision's genuinely
// new finding because it collided with runDirect's Phase-1-visible one —
// the merge must dedupe on the call site's actual position instead.
func runDirect(client *ld.LDClient) {
	_, _ = client.BoolVariation("collision-flag", nil, false)
}

func runInterfaceCollision() {
	client, _ := ld.MakeClient("sdk-key", 5*time.Second)
	var evaluator FlagEvaluator = client
	_, _ = evaluator.BoolVariation("collision-flag", nil, false)
}

func main() {
	run()
	runUnrelated()
	runDirect(nil)
	runInterfaceCollision()
}
