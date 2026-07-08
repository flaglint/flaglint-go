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
	// DetectedBy is a Go-specific additive field (see ADR 005): "strict-types"
	// for a finding only provable with real go/types information (an
	// interface-satisfaction case ordinary syntactic tracing can't see),
	// omitted entirely for an ordinary Phase 1 finding — so Scan's JSON
	// output is byte-for-byte unchanged from before this field existed.
	DetectedBy string `json:"detectedBy,omitempty"`
}

// IsStale reports whether a usage carries any staleness signal. Always false
// for Go findings in Phase 1 (see StalenessSignal doc comment).
func IsStale(u FlagUsage) bool {
	return len(u.StalenessSignals) > 0
}

// ScanWarning records a non-fatal problem encountered while scanning.
type ScanWarning struct {
	Kind   string `json:"kind"`             // "read-failure" | "parse-failure" | "typecheck-failure"
	File   string `json:"file"`             // for "typecheck-failure", the package path that failed to load/type-check, not a file
	FsCode string `json:"fsCode,omitempty"` // e.g. "ENOENT", "EACCES" — only set for "read-failure"
	Reason string `json:"reason,omitempty"` // human-readable cause — only set for "typecheck-failure"
}

// MigrationValueType is the OpenFeature evaluation-method category a call's
// return value maps to — which getXxxValue method a future migrate command
// would rewrite the call to use. Unlike flaglint-js (whose generic
// variation()/isFeatureEnabled() calls require inferring this from the
// fallback argument's literal type), every Go SDK method name is already
// type-specific, so this is always derived directly from CallType, never
// from a fallback expression.
type MigrationValueType string

const (
	MigrationValueBoolean MigrationValueType = "boolean"
	MigrationValueString  MigrationValueType = "string"
	MigrationValueNumber  MigrationValueType = "number"
	MigrationValueObject  MigrationValueType = "object"
	MigrationValueUnknown MigrationValueType = "unknown"
)

// MigrationManualReviewReason is why a call site isn't safely automatable by
// a future migrate command. Matches flaglint-js's set exactly (see ADR 003);
// "unknown-fallback" is never produced by flaglint-go itself (Go's method
// names always determine ValueType outright, see MigrationValueType) but is
// kept for cross-tool consistency — the same reason vocabulary either tool's
// migrationInventory can carry.
type MigrationManualReviewReason string

const (
	MigrationReasonDynamicKey        MigrationManualReviewReason = "dynamic-key"
	MigrationReasonUnknownFallback   MigrationManualReviewReason = "unknown-fallback"
	MigrationReasonDetailMethod      MigrationManualReviewReason = "detail-method"
	MigrationReasonBulkInventoryCall MigrationManualReviewReason = "bulk-inventory-call"
)

// MigrationInventoryItem is a richer, migration-focused record of one call
// site than FlagUsage — the additional detail a future `migrate` command
// would need to safely rewrite the call, not just report it. Mirrors
// flaglint-js's MigrationInventoryItem field-for-field (see ADR 003), with
// one deliberate omission: flaglint-js's `isAwaited?: boolean` has no Go
// equivalent (no async/await) and is never emitted.
type MigrationInventoryItem struct {
	File               string   `json:"file"`
	Line               int      `json:"line"`
	Column             int      `json:"column"`
	LaunchDarklyMethod CallType `json:"launchDarklyMethod"`
	CallExpression     string   `json:"callExpression,omitempty"`
	RangeStart         int      `json:"rangeStart,omitempty"`
	RangeEnd           int      `json:"rangeEnd,omitempty"`
	FlagKeyExpression  string   `json:"flagKeyExpression,omitempty"`
	// StaticFlagKey is only set when !IsDynamic — matches flaglint-js's
	// `staticFlagKey?: string`.
	StaticFlagKey               string                      `json:"staticFlagKey,omitempty"`
	IsDynamic                   bool                        `json:"isDynamic"`
	ValueType                   MigrationValueType          `json:"valueType"`
	FallbackExpression          string                      `json:"fallbackExpression,omitempty"`
	EvaluationContextExpression string                      `json:"evaluationContextExpression,omitempty"`
	SafelyAutomatable           bool                        `json:"safelyAutomatable"`
	ManualReviewReason          MigrationManualReviewReason `json:"manualReviewReason,omitempty"`
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
	// MigrationInventory carries a MigrationInventoryItem for every Phase 1
	// (pure-syntax) usage — see docs/adr/003-cross-tool-contract.md. A
	// --strict-types-only usage (FlagUsage.DetectedBy == "strict-types",
	// e.g. a forwarding-function call) is deliberately excluded: its call
	// site doesn't directly show the LD method's own (key, context,
	// fallback) arguments the way a migrate rewrite would need, so a
	// MigrationInventoryItem for it would misrepresent what's actually safe
	// to rewrite.
	MigrationInventory []MigrationInventoryItem `json:"migrationInventory"`
}
