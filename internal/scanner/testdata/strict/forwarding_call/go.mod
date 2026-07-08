module flaglint-strict-fixture-forwarding-call

go 1.22

require github.com/launchdarkly/go-server-sdk/v7 v7.0.0

replace github.com/launchdarkly/go-server-sdk/v7 => ./stubsdk
