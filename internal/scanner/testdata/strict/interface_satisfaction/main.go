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

func main() {
	run()
	runUnrelated()
}
