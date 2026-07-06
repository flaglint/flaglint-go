package fixtures

import ld "github.com/launchdarkly/go-server-sdk/v7"

// Mirrors CMS-Enterprise/mint-app's actual pattern: an already-constructed
// client passed in as a plain function parameter — there's no assignment
// to trace at all here, only the parameter's declared type proves
// identity.
func useParamTypedClient(client *ld.LDClient) {
	_, _ = client.BoolVariation("param-typed-flag", nil, false)
}
