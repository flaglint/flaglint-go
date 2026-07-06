// Package fingerprint generates stable finding fingerprints. The algorithm
// must match flaglint-js's src/scanner/fingerprint.ts exactly — see
// docs/adr/003-cross-tool-contract.md.
package fingerprint

import (
	"fmt"
	"strings"

	"github.com/flaglint/flaglint-go/internal/types"
)

func normalizePath(filePath string) string {
	normalized := strings.ReplaceAll(filePath, "\\", "/")
	return strings.TrimPrefix(normalized, "./")
}

// Generate produces "launchdarkly:<callType>:<flagKey>:<normalizedPath>",
// with ":<dynamicIndex>" appended when dynamicIndex is non-nil.
//
// flagKey of "*" or "" collapses to the literal "*" (bulk calls like
// AllFlagsState have no single flag key).
func Generate(flagKey string, callType types.CallType, filePath string, dynamicIndex *int) string {
	normalized := normalizePath(filePath)
	key := flagKey
	if key == "*" || key == "" {
		key = "*"
	}
	base := fmt.Sprintf("launchdarkly:%s:%s:%s", callType, key, normalized)
	if dynamicIndex != nil {
		return fmt.Sprintf("%s:%d", base, *dynamicIndex)
	}
	return base
}
