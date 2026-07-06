package validator

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// ruleNoDirectLD is the canonical rule definition for the no-direct-
// launchdarkly policy rule. The rule ID is namespaced with ".go." so it
// never collides with flaglint-js's "flaglint.direct-launchdarkly" in a
// shared GitHub Code Scanning dashboard — see ADR 003 and flaglint-js's
// ADR 006, which reserved this exact convention for a Go language engine.
var ruleNoDirectLD = sarifRule{
	ID:   "flaglint.go.direct-launchdarkly",
	Name: "DirectLaunchDarklyGoSDKUsage",
	ShortDescription: sarifText{
		Text: "Direct LaunchDarkly Go SDK evaluation call detected",
	},
	FullDescription: sarifText{
		Text: "A direct LaunchDarkly Go server SDK evaluation call was found. Migrate this call to OpenFeature so the codebase is provider-independent.",
	},
	HelpURI: "https://github.com/flaglint/flaglint-go#validate",
	Properties: sarifRuleProperties{
		Tags: []string{"openfeature", "migration", "launchdarkly", "go"},
	},
}

type sarifText struct {
	Text string `json:"text"`
}

type sarifRuleProperties struct {
	Tags []string `json:"tags"`
}

type sarifRule struct {
	ID               string              `json:"id"`
	Name             string              `json:"name"`
	ShortDescription sarifText           `json:"shortDescription"`
	FullDescription  sarifText           `json:"fullDescription"`
	HelpURI          string              `json:"helpUri"`
	Properties       sarifRuleProperties `json:"properties"`
}

type sarifDocument struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool               sarifTool               `json:"tool"`
	Invocations        []sarifInvocation       `json:"invocations"`
	OriginalUriBaseIds map[string]sarifBaseURI `json:"originalUriBaseIds"`
	Results            []sarifResult           `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifInvocation struct {
	ExecutionSuccessful bool                      `json:"executionSuccessful"`
	StartTimeUtc        string                    `json:"startTimeUtc"`
	Properties          sarifInvocationProperties `json:"properties"`
}

type sarifInvocationProperties struct {
	ScannedFiles int  `json:"scannedFiles"`
	TotalUsages  int  `json:"totalUsages"`
	Violations   int  `json:"violations"`
	Passed       bool `json:"passed"`
}

type sarifBaseURI struct {
	URI string `json:"uri"`
}

type sarifResult struct {
	RuleID              string                `json:"ruleId"`
	Level               string                `json:"level"`
	Message             sarifText             `json:"message"`
	Locations           []sarifLocation       `json:"locations"`
	PartialFingerprints map[string]string     `json:"partialFingerprints"`
	Properties          sarifResultProperties `json:"properties"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           sarifRegion           `json:"region"`
}

type sarifArtifactLocation struct {
	URI       string `json:"uri"`
	UriBaseId string `json:"uriBaseId"`
}

type sarifRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn"`
}

type sarifResultProperties struct {
	FlagKey   string `json:"flagKey"`
	CallType  string `json:"callType"`
	IsDynamic bool   `json:"isDynamic"`
}

// sarifURI normalizes path separators only — it does not percent-encode
// spaces or non-ASCII characters, unlike sarifScanRootURI below. That
// inconsistency is intentional parity with the TS reference's own
// sarifViolationUri/sarifUri (src/validator/index.ts, src/reporter/index.ts),
// which does the identical bare slash-normalization for per-result relative
// URIs while only escaping the base URI via pathToFileURL. Fixing it only
// here would make the two tools' SARIF output diverge for paths containing
// spaces — verified against the TS source before treating this as
// deliberate rather than a gap to close.
func sarifURI(file string) string {
	return strings.ReplaceAll(file, `\`, "/")
}

// sarifScanRootURI renders scanRoot as a file:// URI, matching
// pathToFileURL(resolve(scanRoot)).href in the TS reference.
func sarifScanRootURI(scanRoot string) string {
	abs, err := filepath.Abs(scanRoot)
	if err != nil {
		abs = scanRoot
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
	uri := u.String()
	if !strings.HasSuffix(uri, "/") {
		uri += "/"
	}
	return uri
}

func violationSarifMessage(v Violation) string {
	switch {
	case v.IsDynamic:
		return fmt.Sprintf("Direct LaunchDarkly Go SDK call %s() with a dynamic flag key at %s:%d. Migrate to OpenFeature.", v.CallType, v.File, v.Line)
	case v.FlagKey == "*":
		return fmt.Sprintf("Direct LaunchDarkly Go SDK bulk call %s() at %s:%d. Migrate to OpenFeature.", v.CallType, v.File, v.Line)
	default:
		return fmt.Sprintf("Direct LaunchDarkly Go SDK call %s(%q) at %s:%d. Migrate to OpenFeature.", v.CallType, v.FlagKey, v.File, v.Line)
	}
}

// FormatSARIF renders result as a SARIF 2.1.0 document. An empty
// Violations slice produces a valid document with zero results — GitHub
// Code Scanning interprets that as "all clear".
//
// Column convention: unlike flaglint-js (whose internal column field is
// 0-based, ESTree convention, requiring +1 for SARIF's 1-based columns),
// Go's go/token.Position.Column is already 1-based — so no +1 adjustment
// is applied here. This is a deliberate difference from the TS reference's
// arithmetic, not an oversight; see docs/adr/003-cross-tool-contract.md.
func FormatSARIF(result Result, scanRoot, scannedAt string) string {
	results := make([]sarifResult, 0, len(result.Violations))
	for _, v := range result.Violations {
		results = append(results, sarifResult{
			RuleID:  ruleNoDirectLD.ID,
			Level:   "error",
			Message: sarifText{Text: violationSarifMessage(v)},
			Locations: []sarifLocation{{
				PhysicalLocation: sarifPhysicalLocation{
					ArtifactLocation: sarifArtifactLocation{URI: sarifURI(v.File), UriBaseId: "%SRCROOT%"},
					Region:           sarifRegion{StartLine: max(v.Line, 1), StartColumn: max(v.Column, 1)},
				},
			}},
			PartialFingerprints: map[string]string{"flagKey/v1": v.FlagKey},
			Properties: sarifResultProperties{
				FlagKey:   v.FlagKey,
				CallType:  string(v.CallType),
				IsDynamic: v.IsDynamic,
			},
		})
	}

	doc := sarifDocument{
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Version: "2.1.0",
		Runs: []sarifRun{{
			Tool: sarifTool{Driver: sarifDriver{
				Name:           "flaglint-go",
				InformationURI: "https://github.com/flaglint/flaglint-go",
				Rules:          []sarifRule{ruleNoDirectLD},
			}},
			Invocations: []sarifInvocation{{
				ExecutionSuccessful: true,
				StartTimeUtc:        scannedAt,
				Properties: sarifInvocationProperties{
					ScannedFiles: result.ScannedFiles,
					TotalUsages:  result.TotalUsages,
					Violations:   len(result.Violations),
					Passed:       result.Passed,
				},
			}},
			OriginalUriBaseIds: map[string]sarifBaseURI{"%SRCROOT%": {URI: sarifScanRootURI(scanRoot)}},
			Results:            results,
		}},
	}

	b, _ := json.MarshalIndent(doc, "", "  ")
	return string(b)
}
