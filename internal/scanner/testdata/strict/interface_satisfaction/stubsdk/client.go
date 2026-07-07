// Package ldclient is a minimal local stand-in for the real
// github.com/launchdarkly/go-server-sdk/v7 — just enough of its real API
// surface (package name, LDClient type, MakeClient, BoolVariation) for
// go/types to type-check against, without this fixture module needing
// network access or the real (much larger) SDK dependency in CI.
package ldclient

import "time"

type LDClient struct{}

func MakeClient(sdkKey string, waitFor time.Duration) (*LDClient, error) {
	return &LDClient{}, nil
}

func (c *LDClient) BoolVariation(key string, ctx interface{}, defaultVal bool) (bool, error) {
	return defaultVal, nil
}
