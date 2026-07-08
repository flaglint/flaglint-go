// Package ldclient is a minimal local stand-in for the real
// github.com/launchdarkly/go-server-sdk/v7 — see the sibling
// interface_satisfaction fixture's stubsdk for why.
package ldclient

import "time"

type LDClient struct{}

func MakeClient(sdkKey string, waitFor time.Duration) (*LDClient, error) {
	return &LDClient{}, nil
}

func (c *LDClient) BoolVariation(key string, ctx interface{}, defaultVal bool) (bool, error) {
	return defaultVal, nil
}
