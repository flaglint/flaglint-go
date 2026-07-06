package consumer

import (
	"github.com/flaglint/flaglint-go/internal/scanner/testdata/fixtures/crosspkg/producer"
)

func Run() {
	client := producer.GetLdClient()
	_, _ = client.BoolVariation("cross-package-flag", nil, false)
}
