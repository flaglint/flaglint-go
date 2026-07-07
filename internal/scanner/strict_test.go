package scanner

import (
	"testing"

	"github.com/flaglint/flaglint-go/internal/config"
)

func TestScanStrict_positiveInterfaceSatisfaction(t *testing.T) {
	dir := "testdata/strict/interface_satisfaction"
	cfg := config.Config{
		Include:  []string{"**/*.go"},
		Exclude:  []string{"**/vendor/**", "**/.git/**"},
		Provider: "launchdarkly",
	}

	phase1, err := Scan(dir, cfg)
	if err != nil {
		t.Fatalf("Scan error = %v", err)
	}
	if len(phase1.Usages) != 0 {
		t.Fatalf("Scan (Phase 1) found %d usage(s), want 0 — interface satisfaction is exactly what Phase 1 cannot see: %+v", len(phase1.Usages), phase1.Usages)
	}

	strict, err := ScanStrict(dir, cfg)
	if err != nil {
		t.Fatalf("ScanStrict error = %v", err)
	}
	if len(strict.Warnings) != 0 {
		t.Fatalf("ScanStrict warnings = %+v, want none — the fixture module builds cleanly", strict.Warnings)
	}
	if len(strict.Usages) != 1 {
		t.Fatalf("ScanStrict found %d usage(s), want exactly 1 — an unrelated type satisfying the same interface shape (not *ld.LDClient) must not be detected: %+v", len(strict.Usages), strict.Usages)
	}
	got := strict.Usages[0]
	if got.FlagKey != "interface-satisfaction-flag" {
		t.Errorf("usages[0].FlagKey = %q, want interface-satisfaction-flag (not should-not-be-detected)", got.FlagKey)
	}
	if got.DetectedBy != "strict-types" {
		t.Errorf("usages[0].DetectedBy = %q, want strict-types", got.DetectedBy)
	}
	if got.SDK != "go-server-sdk-v7" {
		t.Errorf("usages[0].SDK = %q, want go-server-sdk-v7", got.SDK)
	}
}

func TestScanStrict_failedLoadFallsBackToPhase1(t *testing.T) {
	// A directory with no go.mod at all (or above it) fails to load as a
	// module — ScanStrict must still return Phase 1's result plus a
	// warning, never a hard failure (ADR 005's fail-soft guarantee).
	dir := t.TempDir()
	cfg := config.Config{Include: []string{"**/*.go"}, Exclude: nil, Provider: "launchdarkly"}

	result, err := ScanStrict(dir, cfg)
	if err != nil {
		t.Fatalf("ScanStrict error = %v, want nil (fail soft, not fail hard)", err)
	}
	if len(result.Usages) != 0 {
		t.Errorf("usages = %+v, want none (empty dir)", result.Usages)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Kind != "typecheck-failure" {
		t.Errorf("warnings = %+v, want exactly one typecheck-failure warning", result.Warnings)
	}
}
