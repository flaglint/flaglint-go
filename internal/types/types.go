// Package types defines the shared data shapes flaglint-go produces. Field
// names and JSON tags are chosen to match flaglint-js's FlagUsage/ScanResult
// contract wherever the concept is shared — see docs/adr/003-cross-tool-contract.md.
package types

// CallType identifies which LaunchDarkly Go SDK method produced a finding.
// Unlike flaglint-js (which collapses same-shape calls into a generic
// "variation" callType), Go SDK methods are distinct per return type, so
// callType values here are the SDK method names themselves.
type CallType string

const (
	CallTypeBoolVariation          CallType = "BoolVariation"
	CallTypeStringVariation        CallType = "StringVariation"
	CallTypeIntVariation           CallType = "IntVariation"
	CallTypeFloat64Variation       CallType = "Float64Variation"
	CallTypeJSONVariation          CallType = "JSONVariation"
	CallTypeBoolVariationDetail    CallType = "BoolVariationDetail"
	CallTypeStringVariationDetail  CallType = "StringVariationDetail"
	CallTypeIntVariationDetail     CallType = "IntVariationDetail"
	CallTypeFloat64VariationDetail CallType = "Float64VariationDetail"
	CallTypeJSONVariationDetail    CallType = "JSONVariationDetail"
	CallTypeAllFlagsState          CallType = "AllFlagsState"
)

// Risk is a Go-specific additive field — see ADR 002 for the classification
// table and ADR 003 for why flaglint-js does not (yet) carry this field.
type Risk string

const (
	RiskLow    Risk = "low"
	RiskMedium Risk = "medium"
	RiskHigh   Risk = "high"
)

// StalenessSignal is a placeholder for future staleness-detection parity
// with flaglint-js, which uses a discriminated union with variant-specific
// payloads ({source: "keyword"; keyword} | {source: "path"; pattern} |
// {source: "minFileCount"; fileCount; threshold}). Go audit does not
// implement staleness heuristics in Phase 1 and never populates this field —
// see docs/adr/003-cross-tool-contract.md. The shape below is intentionally
// minimal rather than a premature copy of the TS union; it will be revisited
// (likely as a proper sum type) if and when Phase 2 implements staleness.
type StalenessSignal struct {
	Source string `json:"source"`
}

// FlagUsage is one detected LaunchDarkly Go SDK call site.
type FlagUsage struct {
	FlagKey          string            `json:"flagKey"`
	IsDynamic        bool              `json:"isDynamic"`
	File             string            `json:"file"` // always relative to scan root
	Line             int               `json:"line"`
	Column           int               `json:"column"`
	CallType         CallType          `json:"callType"`
	Fingerprint      string            `json:"fingerprint"`
	StalenessSignals []StalenessSignal `json:"stalenessSignals"`
	Language         string            `json:"language"` // additive: always "go"
	SDK              string            `json:"sdk"`      // e.g. "go-server-sdk-v7"
	Risk             Risk              `json:"risk"`
}

// IsStale reports whether a usage carries any staleness signal. Always false
// for Go findings in Phase 1 (see StalenessSignal doc comment).
func IsStale(u FlagUsage) bool {
	return len(u.StalenessSignals) > 0
}

// ScanWarning records a non-fatal problem encountered while scanning.
type ScanWarning struct {
	Kind   string `json:"kind"` // "read-failure" | "parse-failure"
	File   string `json:"file"`
	FsCode string `json:"fsCode,omitempty"` // e.g. "ENOENT", "EACCES" — only set for "read-failure"
}

// ScanResult is the top-level output of a scan, shared by audit/scan/validate.
type ScanResult struct {
	ScannedAt      string        `json:"scannedAt"`
	ScanRoot       string        `json:"scanRoot"`
	ScannedFiles   int           `json:"scannedFiles"`
	TotalUsages    int           `json:"totalUsages"`
	UniqueFlags    []string      `json:"uniqueFlags"`
	Usages         []FlagUsage   `json:"usages"`
	ScanDurationMs int64         `json:"scanDurationMs"`
	Warnings       []ScanWarning `json:"warnings"`
}
