package validator

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/flaglint/flaglint-go/internal/types"
)

func sampleScanResult() types.ScanResult {
	return types.ScanResult{
		ScannedFiles: 2,
		TotalUsages:  2,
		Usages: []types.FlagUsage{
			{FlagKey: "checkout-v2", CallType: types.CallTypeBoolVariation, File: "flags.go", Line: 10, Column: 5},
			{FlagKey: "dynamic", IsDynamic: true, CallType: types.CallTypeStringVariation, File: "provider/bootstrap.go", Line: 20, Column: 3},
		},
	}
}

func TestValidate_flagOff_neverFails(t *testing.T) {
	result := Validate(sampleScanResult(), Options{})
	if !result.Passed {
		t.Errorf("Passed = false, want true — NoDirectLaunchDarkly off must never fail")
	}
	if len(result.Violations) != 0 {
		t.Errorf("Violations = %v, want none when the rule is off", result.Violations)
	}
}

func TestValidate_flagOn_everyUsageIsAViolation(t *testing.T) {
	result := Validate(sampleScanResult(), Options{NoDirectLaunchDarkly: true})
	if result.Passed {
		t.Error("Passed = true, want false — usages exist and the rule is on")
	}
	if len(result.Violations) != 2 {
		t.Fatalf("Violations = %v, want 2", result.Violations)
	}
}

func TestValidate_bootstrapExcludeFiltersMatchingFiles(t *testing.T) {
	result := Validate(sampleScanResult(), Options{
		NoDirectLaunchDarkly: true,
		BootstrapExclude:     []string{"provider/**"},
	})
	if len(result.Violations) != 1 {
		t.Fatalf("Violations = %v, want 1 (provider/bootstrap.go excluded)", result.Violations)
	}
	if result.Violations[0].File != "flags.go" {
		t.Errorf("Violations[0].File = %q, want flags.go", result.Violations[0].File)
	}
}

func TestValidate_passesWhenNoUsagesEvenWithRuleOn(t *testing.T) {
	result := Validate(types.ScanResult{}, Options{NoDirectLaunchDarkly: true})
	if !result.Passed {
		t.Error("Passed = false, want true — no usages means nothing to violate")
	}
}

func TestFormatReport_ruleOff(t *testing.T) {
	result := Validate(sampleScanResult(), Options{})
	out := FormatReport(result, Options{})
	if !strings.Contains(out, "Scanned 2 file(s)") {
		t.Errorf("output missing scan summary, got:\n%s", out)
	}
	if strings.Contains(out, "stale") {
		t.Errorf("output must never mention staleness, got:\n%s", out)
	}
}

func TestFormatReport_rulePassed(t *testing.T) {
	result := Validate(types.ScanResult{ScannedFiles: 3}, Options{NoDirectLaunchDarkly: true})
	out := FormatReport(result, Options{NoDirectLaunchDarkly: true})
	if !strings.Contains(out, "✓") {
		t.Errorf("output missing pass indicator, got:\n%s", out)
	}
}

func TestFormatReport_ruleFailed(t *testing.T) {
	result := Validate(sampleScanResult(), Options{NoDirectLaunchDarkly: true})
	out := FormatReport(result, Options{NoDirectLaunchDarkly: true})
	if !strings.Contains(out, "✗") {
		t.Errorf("output missing fail indicator, got:\n%s", out)
	}
	if !strings.Contains(out, "flags.go:10:5") {
		t.Errorf("output missing violation location, got:\n%s", out)
	}
}

func TestFormatSARIF_emptyViolationsIsValidDocument(t *testing.T) {
	result := Validate(types.ScanResult{}, Options{NoDirectLaunchDarkly: true})
	out := FormatSARIF(result, "/tmp/svc", "2026-07-06T00:00:00Z")

	var decoded map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if decoded["version"] != "2.1.0" {
		t.Errorf("version = %v, want 2.1.0", decoded["version"])
	}
}

func TestFormatSARIF_ruleIdIsGoNamespaced(t *testing.T) {
	result := Validate(sampleScanResult(), Options{NoDirectLaunchDarkly: true})
	out := FormatSARIF(result, "/tmp/svc", "2026-07-06T00:00:00Z")
	if !strings.Contains(out, `"flaglint.go.direct-launchdarkly"`) {
		t.Errorf("output missing Go-namespaced rule ID, got:\n%s", out)
	}
	if strings.Contains(out, `"flaglint.direct-launchdarkly"`) && !strings.Contains(out, `"flaglint.go.direct-launchdarkly"`) {
		t.Errorf("rule ID collides with flaglint-js's un-namespaced rule ID")
	}
}

func TestFormatSARIF_partialFingerprintsAndLocation(t *testing.T) {
	result := Validate(sampleScanResult(), Options{NoDirectLaunchDarkly: true})
	out := FormatSARIF(result, "/tmp/svc", "2026-07-06T00:00:00Z")

	var decoded struct {
		Runs []struct {
			Results []struct {
				PartialFingerprints map[string]string `json:"partialFingerprints"`
				Locations           []struct {
					PhysicalLocation struct {
						Region struct {
							StartLine   int `json:"startLine"`
							StartColumn int `json:"startColumn"`
						} `json:"region"`
					} `json:"physicalLocation"`
				} `json:"locations"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if len(decoded.Runs) != 1 || len(decoded.Runs[0].Results) != 2 {
		t.Fatalf("unexpected structure: %+v", decoded)
	}
	first := decoded.Runs[0].Results[0]
	if first.PartialFingerprints["flagKey/v1"] != "checkout-v2" {
		t.Errorf("partialFingerprints = %v, want flagKey/v1=checkout-v2", first.PartialFingerprints)
	}
	// Column 5 from go/token is already 1-based — SARIF must receive it
	// unmodified, not +1'd (that would be flaglint-js's convention, not Go's).
	if first.Locations[0].PhysicalLocation.Region.StartColumn != 5 {
		t.Errorf("StartColumn = %d, want 5 (no +1 adjustment for Go's already-1-based column)", first.Locations[0].PhysicalLocation.Region.StartColumn)
	}
}
