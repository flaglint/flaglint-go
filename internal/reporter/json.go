package reporter

import (
	"encoding/json"

	"github.com/flaglint/flaglint-go/internal/types"
)

// jsonEnvelope wraps ScanResult with a top-level generatedAt field, matching
// flaglint-js's `{ generatedAt: result.scannedAt, ...result }` spread —
// types.ScanResult is embedded without its own JSON tag, so encoding/json
// inlines its fields at the top level alongside GeneratedAt.
type jsonEnvelope struct {
	GeneratedAt string `json:"generatedAt"`
	types.ScanResult
}

func formatJSON(result types.ScanResult) string {
	envelope := jsonEnvelope{GeneratedAt: result.ScannedAt, ScanResult: result}
	// Marshal cannot fail here: every field of ScanResult is a plain string,
	// int, bool, or slice/struct thereof — no channels, funcs, or cyclic
	// pointers that could produce an error.
	b, _ := json.MarshalIndent(envelope, "", "  ")
	return string(b)
}
