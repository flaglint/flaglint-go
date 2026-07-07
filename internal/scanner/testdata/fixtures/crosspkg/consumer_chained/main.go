package consumerchained

import (
	"github.com/flaglint/flaglint-go/internal/scanner/testdata/fixtures/crosspkg/producer"
)

// Reproduces flaglint-go issue #20 exactly: a direct chained call on a
// cross-package factory function's result, with no intermediate variable
// — as opposed to crosspkg/consumer/main.go, which assigns the result to
// a variable first.
func Run() {
	_, _ = producer.GetLdClient().BoolVariation("chained-cross-package-flag", nil, false)
}
