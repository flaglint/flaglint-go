package reporter

import (
	"encoding/json"

	"github.com/flaglint/flaglint-go/internal/types"
)

// scanResultSchemaVersion is the const value flaglint/spec's
// scan-result.v1.schema.json requires for the "schemaVersion" field —
// additive as of spec v1.1 (writers MUST emit, readers MUST tolerate its
// absence in documents produced before spec v1.1, which is exactly why
// this is safe to start emitting now without a version bump of our own).
const scanResultSchemaVersion = "scan-result.v1"

// jsonEnvelope wraps ScanResult with top-level generatedAt and
// schemaVersion fields, matching flaglint-js's `{ generatedAt:
// result.scannedAt, ...result }` spread — types.ScanResult is embedded
// without its own JSON tag, so encoding/json inlines its fields at the
// top level alongside GeneratedAt/SchemaVersion.
type jsonEnvelope struct {
	GeneratedAt   string `json:"generatedAt"`
	SchemaVersion string `json:"schemaVersion"`
	types.ScanResult
}

func formatJSON(result types.ScanResult) string {
	envelope := jsonEnvelope{GeneratedAt: result.ScannedAt, SchemaVersion: scanResultSchemaVersion, ScanResult: result}
	// Marshal cannot fail here: every field of ScanResult is a plain string,
	// int, bool, or slice/struct thereof — no channels, funcs, or cyclic
	// pointers that could produce an error.
	b, _ := json.MarshalIndent(envelope, "", "  ")
	return string(b)
}
