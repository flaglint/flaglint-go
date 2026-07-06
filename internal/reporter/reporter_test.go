package reporter

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/flaglint/flaglint-go/internal/types"
)

func sampleResult() types.ScanResult {
	return types.ScanResult{
		ScannedAt:      "2026-07-06T00:00:00Z",
		ScanRoot:       "/tmp/svc",
		ScannedFiles:   2,
		TotalUsages:    3,
		UniqueFlags:    []string{"checkout-v2"},
		ScanDurationMs: 42,
		Usages: []types.FlagUsage{
			{FlagKey: "checkout-v2", CallType: types.CallTypeBoolVariation, Risk: types.RiskLow, File: "flags.go", Line: 10},
			{FlagKey: "dynamic", IsDynamic: true, CallType: types.CallTypeStringVariation, Risk: types.RiskLow, File: "flags.go", Line: 20},
			{FlagKey: "*", CallType: types.CallTypeAllFlagsState, Risk: types.RiskHigh, File: "flags.go", Line: 30},
		},
		Warnings: []types.ScanWarning{{Kind: "parse-failure", File: "broken.go"}},
	}
}

func TestRender_json(t *testing.T) {
	out, err := Render(sampleResult(), Options{Format: FormatJSON})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if decoded["generatedAt"] != "2026-07-06T00:00:00Z" {
		t.Errorf("generatedAt = %v, want scannedAt value", decoded["generatedAt"])
	}
	if decoded["scannedAt"] != "2026-07-06T00:00:00Z" {
		t.Errorf("scannedAt = %v, want present at top level (spread, not nested)", decoded["scannedAt"])
	}
	if decoded["scannedFiles"] != float64(2) {
		t.Errorf("scannedFiles = %v, want 2", decoded["scannedFiles"])
	}
}

func TestRender_markdown(t *testing.T) {
	out, err := Render(sampleResult(), Options{Format: FormatMarkdown, Title: "Test Report"})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(out, "# Test Report") {
		t.Errorf("output missing title, got:\n%s", out)
	}
	if !strings.Contains(out, "checkout-v2") {
		t.Errorf("output missing static flag, got:\n%s", out)
	}
	if !strings.Contains(out, "Dynamic Flag Keys") {
		t.Errorf("output missing dynamic section, got:\n%s", out)
	}
	if !strings.Contains(out, "Bulk Evaluation Calls") {
		t.Errorf("output missing bulk section, got:\n%s", out)
	}
	if !strings.Contains(out, "parse-failure") {
		t.Errorf("output missing warnings section, got:\n%s", out)
	}
}

func TestRender_markdown_noUsages(t *testing.T) {
	out, err := Render(types.ScanResult{}, Options{Format: FormatMarkdown})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(out, "No LaunchDarkly Go SDK usage detected") {
		t.Errorf("output missing no-usage message, got:\n%s", out)
	}
}

func TestRender_unsupportedFormat(t *testing.T) {
	_, err := Render(sampleResult(), Options{Format: "sarif"})
	if err == nil {
		t.Fatal("Render() error = nil, want error for unsupported format")
	}
}

func TestRender_json_emptyArraysAreNotNull(t *testing.T) {
	out, err := Render(types.ScanResult{}, Options{Format: FormatJSON})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	// A raw string check, not just json.Unmarshal round-tripping: Go's
	// decoder would happily turn a `null` array back into an empty slice,
	// masking the exact bug this guards (nil slices marshaling as `null`
	// instead of `[]`, which breaks a consumer's `.map()`/`jq` pipeline on
	// the JSON *text* itself).
	for _, field := range []string{`"uniqueFlags":null`, `"usages":null`, `"warnings":null`} {
		if strings.Contains(out, field) {
			t.Errorf("output contains %q — empty slices must marshal as [], not null, to match flaglint-js's array semantics:\n%s", field, out)
		}
	}
}

func TestCodeSpan_embeddedBacktick(t *testing.T) {
	got := codeSpan("weird`key")
	if !strings.Contains(got, "``") {
		t.Errorf("codeSpan(%q) = %q, want a wider delimiter to correctly escape the embedded backtick", "weird`key", got)
	}
	if strings.Contains(got, "\n") {
		t.Errorf("codeSpan(%q) = %q, want no newline", "weird`key", got)
	}
}

func TestRender_markdown_flagKeyWithNewlineAndBacktickDoesNotBreakTable(t *testing.T) {
	result := types.ScanResult{
		Usages: []types.FlagUsage{
			{FlagKey: "weird`key\ninjected | row", CallType: types.CallTypeBoolVariation, Risk: types.RiskLow, File: "flags.go", Line: 1},
		},
	}
	out, err := Render(result, Options{Format: FormatMarkdown})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	var tableRow string
	for _, l := range lines {
		if strings.Contains(l, "BoolVariation") {
			tableRow = l
			break
		}
	}
	if tableRow == "" {
		t.Fatalf("no table row found containing the flag, got:\n%s", out)
	}
	// The whole row must stay on one line (no embedded newline) and must
	// have exactly the 4 expected columns (5 pipe characters), proving the
	// embedded "|" was escaped rather than read as a new column separator.
	if strings.Count(tableRow, "|") != 5 {
		t.Errorf("table row has %d '|' characters, want 5 (4 columns) — embedded pipe/newline corrupted the row:\n%q", strings.Count(tableRow, "|"), tableRow)
	}
}
