package consumerofunrelated

import (
	"github.com/flaglint/flaglint-go/internal/scanner/testdata/fixtures/crosspkg/unrelated_producer"
)

// Calls a same-named GetLdClient() from a package that returns an
// unrelated type — must never be detected, proving factory resolution
// isn't fooled by the coincidence of a matching function name.
func Run() {
	client := unrelatedproducer.GetLdClient()
	_, _ = client.BoolVariation("should-not-be-detected", nil, false)
}
