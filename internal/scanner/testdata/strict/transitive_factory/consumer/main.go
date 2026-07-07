package main

import "flaglint-strict-fixture-transitive-factory/wrapper"

// Reproduces issue #16 exactly: w.Inner.BoolVariation(...) is not
// detected by Phase 1, since wrapper.NewClientWrapper isn't a registered
// factory function (its return type is *ClientWrapper, not *ld.LDClient
// directly) — the composite-literal binding establishing
// ClientWrapper.Inner also lives in a different package than this call
// site, so Phase 1's same-package composite-literal resolution doesn't
// reach it either.
func run() {
	w := wrapper.NewClientWrapper()
	_, _ = w.Inner.BoolVariation("transitive-factory-flag", nil, false)
}

func main() {
	run()
}
