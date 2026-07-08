package main

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

// callDirect and callGeneric are issue #26's "forwarding function"
// shape: each calls its own function-typed parameter f directly, using
// its own "key" parameter positionally as f's flag-key argument.
func callDirect(f func(string, interface{}, bool) (bool, error), key string, def bool) bool {
	v, _ := f(key, nil, def)
	return v
}

// callGeneric matches issue #26's exact repro, generic-instantiated —
// proving findForwardingFunctions registers the pre-instantiation
// *types.Func (from the generic declaration itself), not a per-call-site
// instantiated wrapper object, since it's keyed by whatever Info.Uses
// resolves a later call site's identifier to.
func callGeneric[T any](f func(string, interface{}, T) (T, error), key string, def T) T {
	v, _ := f(key, nil, def)
	return v
}

func useDirect() {
	client, _ := ld.MakeClient("sdk-key", 5*time.Second)
	_ = callDirect(client.BoolVariation, "forwarding-direct-flag", false)
}

func useGeneric() {
	client, _ := ld.MakeClient("sdk-key", 5*time.Second)
	_ = callGeneric(client.BoolVariation, "forwarding-generic-flag", false)
}

// unrelatedForwardingClient satisfies callDirect's callback shape but is
// not, and never wraps, a real client — proving forwarding-call
// resolution requires the callback argument to actually resolve to
// *ld.LDClient, not just any same-shaped method value.
type unrelatedForwardingClient struct{}

func (c *unrelatedForwardingClient) BoolVariation(key string, ctx interface{}, def bool) (bool, error) {
	return def, nil
}

func useUnrelatedForwarding() {
	unrelated := &unrelatedForwardingClient{}
	_ = callDirect(unrelated.BoolVariation, "should-not-be-detected", false)
}

// passThroughWrapper is itself a "pass-through" function (not a direct
// forwarding function): it fixes the SDK method internally
// (client.BoolVariation, resolved once within its own body, not varying
// per call site) and simply forwards its own "key" parameter straight
// into callDirect's key position — the one-hop version of the real
// e2b-dev/infra shape (BoolFlag wrapping getFlag), proving
// findEvalSummaries' pass-through discovery works even without the
// accessor-method/factory-var machinery the full two-hop case needs.
func passThroughWrapper(key string) bool {
	client, _ := ld.MakeClient("sdk-key", 5*time.Second)
	return callDirect(client.BoolVariation, key, false)
}

func usePassThrough() {
	_ = passThroughWrapper("pass-through-flag")
}

// useDynamicKey proves a genuinely non-literal key argument (runtimeKey
// is computed, not a literal at this call site) is a deliberate miss
// (ADR 006's literal-only requirement), not detected — unlike
// usePassThrough above, which passes a real literal all the way through.
func useDynamicKey() {
	runtimeKey := computeKeyAtRuntime()
	_ = passThroughWrapper(runtimeKey)
}

func computeKeyAtRuntime() string {
	return "computed-" + time.Now().String()
}

func main() {
	useDirect()
	useGeneric()
	useUnrelatedForwarding()
	usePassThrough()
	useDynamicKey()
}
