package main

import (
	"context"
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

// This fixture mirrors the real e2b-dev/infra shape that motivated issue
// #26 (found during --strict-types verification against that repo,
// beyond ADR 006's original "direct forwarding function" scope):
//
//   - BoolFlag is a "flag descriptor" struct — a factory constructor
//     (NewBoolFlag) stores a literal name into its "name" field, read
//     back through a trivial accessor method (Key).
//   - A package-level var (SnapshotFeatureFlag) is constructed via that
//     factory with a literal flag name.
//   - getFlag is a "direct forwarding" function: it calls its own
//     function-typed parameter, using flag.Key() (a method call on
//     another of its own parameters) as the key argument — critically,
//     ctx (ALSO one of getFlag's own parameters) is passed at an EARLIER
//     argument position than flag.Key(), exactly like the real code. The
//     first implementation of this fixture put the flag descriptor
//     first and had no ctx parameter at all, which accidentally passed
//     even though the underlying bug (assuming the *first* argument
//     referencing one of the function's own parameters is always the
//     key) was real — only caught by re-verifying against e2b-dev/infra
//     itself, where ctx genuinely does come first.
//   - Client.BoolFlag is a "pass-through" function: it doesn't call a
//     callback parameter of its own at all — it calls getFlag with an
//     already-resolved, concrete client method value, forwarding only
//     its own "flag" parameter into getFlag's flag-descriptor position.
//
// Closing the loop requires chaining accessor-method recognition,
// factory-constructor field tracking, package-var literal resolution,
// and pass-through discovery — see forwarding.go's package doc comment.
type typedFlag[T any] interface {
	Key() string
	Fallback() T
}

type BoolFlag struct {
	name     string
	fallback bool
}

func (f BoolFlag) Key() string    { return f.name }
func (f BoolFlag) Fallback() bool { return f.fallback }
func (f BoolFlag) String() string { return f.name }

func NewBoolFlag(name string, fallback bool) BoolFlag {
	flag := BoolFlag{name: name, fallback: fallback}
	return flag
}

var SnapshotFeatureFlag = NewBoolFlag("use-nfs-for-snapshots", false)

type Client struct {
	ld *ld.LDClient
}

func NewClient() (*Client, error) {
	ldClient, err := ld.MakeClient("sdk-key", 5*time.Second)
	if err != nil {
		return nil, err
	}
	return &Client{ld: ldClient}, nil
}

func getFlag[T any](
	ctx context.Context,
	ldClient *ld.LDClient,
	getFromLaunchDarkly func(ctx context.Context, key string, defaultVal T) (T, error),
	flag typedFlag[T],
) T {
	value, _ := getFromLaunchDarkly(ctx, flag.Key(), flag.Fallback())
	return value
}

func (c *Client) BoolFlag(ctx context.Context, flag BoolFlag) bool {
	return getFlag(ctx, c.ld, c.ld.BoolVariationCtx, flag)
}

func run() {
	client, _ := NewClient()
	_ = client.BoolFlag(context.Background(), SnapshotFeatureFlag)
}

func main() {
	run()
}
