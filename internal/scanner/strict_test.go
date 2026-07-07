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
	// Phase 1 sees runDirect's collision-flag call (a plain, syntactically
	// resolvable client.BoolVariation) but neither interface-satisfaction
	// call site (run's interface-satisfaction-flag, runInterfaceCollision's
	// collision-flag).
	if len(phase1.Usages) != 1 || phase1.Usages[0].FlagKey != "collision-flag" {
		t.Fatalf("Scan (Phase 1) found %+v, want exactly one collision-flag usage", phase1.Usages)
	}

	strict, err := ScanStrict(dir, cfg)
	if err != nil {
		t.Fatalf("ScanStrict error = %v", err)
	}
	if len(strict.Warnings) != 0 {
		t.Fatalf("ScanStrict warnings = %+v, want none — the fixture module builds cleanly", strict.Warnings)
	}
	// 3 total: interface-satisfaction-flag (strict-types only) + two
	// collision-flag call sites (runDirect, Phase 1; runInterfaceCollision,
	// strict-types only) — NOT collapsed into one despite sharing a
	// fingerprint (same callType/flagKey/file — fingerprint.Generate
	// deliberately omits line/column). should-not-be-detected must still
	// be absent.
	if len(strict.Usages) != 3 {
		t.Fatalf("ScanStrict found %d usage(s), want exactly 3: %+v", len(strict.Usages), strict.Usages)
	}

	byFlagKeyAndSource := map[string]int{}
	for _, u := range strict.Usages {
		if u.FlagKey == "should-not-be-detected" {
			t.Errorf("usages contains should-not-be-detected: %+v", u)
		}
		byFlagKeyAndSource[u.FlagKey+"/"+u.DetectedBy]++
	}
	want := map[string]int{
		"interface-satisfaction-flag/strict-types": 1,
		"collision-flag/":                          1, // Phase 1's own copy, DetectedBy left unchanged
		"collision-flag/strict-types":              1,
	}
	if len(byFlagKeyAndSource) != len(want) {
		t.Fatalf("usages = %+v, want %+v", byFlagKeyAndSource, want)
	}
	for k, wantCount := range want {
		if byFlagKeyAndSource[k] != wantCount {
			t.Errorf("count[%q] = %d, want %d (full: %+v)", k, byFlagKeyAndSource[k], wantCount, strict.Usages)
		}
	}

	for _, u := range strict.Usages {
		if u.SDK != "go-server-sdk-v7" {
			t.Errorf("usage %+v SDK = %q, want go-server-sdk-v7", u, u.SDK)
		}
	}
}

func TestScanStrict_positiveTransitiveFactoryWrapping(t *testing.T) {
	// Issue #16's exact repro: a cross-package factory function
	// (wrapper.NewClientWrapper) whose declared return type isn't
	// *ld.LDClient directly but a wrapper struct with a client field,
	// called from a different package than the one that constructs it.
	// Turns out this needed no new resolution logic at all — found while
	// building this regression test, not before: resolveByStaticType
	// (identity.go, added for issue #15's interface-satisfaction fix)
	// checks *any* expression's real static type against the SDK client
	// type, not just an assignment's RHS — so `w.Inner` in
	// `w.Inner.BoolVariation(...)` resolves correctly as a byproduct,
	// since go/types reports a field selector's type exactly like any
	// other expression's, regardless of how the struct it's read from was
	// itself constructed.
	dir := "testdata/strict/transitive_factory"
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
		t.Fatalf("Scan (Phase 1) found %d usage(s), want 0 — transitive factory wrapping is exactly what Phase 1 cannot see: %+v", len(phase1.Usages), phase1.Usages)
	}

	strict, err := ScanStrict(dir, cfg)
	if err != nil {
		t.Fatalf("ScanStrict error = %v", err)
	}
	if len(strict.Warnings) != 0 {
		t.Fatalf("ScanStrict warnings = %+v, want none — the fixture module builds cleanly", strict.Warnings)
	}
	if len(strict.Usages) != 1 {
		t.Fatalf("ScanStrict found %d usage(s), want 1: %+v", len(strict.Usages), strict.Usages)
	}
	got := strict.Usages[0]
	if got.FlagKey != "transitive-factory-flag" {
		t.Errorf("usages[0].FlagKey = %q, want transitive-factory-flag", got.FlagKey)
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
