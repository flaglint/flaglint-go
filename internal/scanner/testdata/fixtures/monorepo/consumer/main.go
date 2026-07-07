package consumer

import (
	"github.com/example/monorepo-submodule/producer"
)

// Lives under the OUTER module (flaglint-go's own go.mod — this file has
// no nested go.mod between it and flaglint-go's root), importing a
// factory function from a sibling directory that's actually an
// independent nested submodule (../submodule/go.mod). Correctly
// resolving this cross-package call proves each file resolves against
// its own *nearest* go.mod, not a single go.mod for the whole scan.
func Run() {
	client := producer.GetLdClient()
	_, _ = client.BoolVariation("monorepo-submodule-flag", nil, false)
}
