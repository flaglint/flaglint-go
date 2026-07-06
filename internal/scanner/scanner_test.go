package scanner

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"

	"github.com/flaglint/flaglint-go/internal/types"
)

func parseFixture(t *testing.T, name string) []types.FlagUsage {
	t.Helper()
	path := filepath.Join("testdata", "fixtures", name)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("ParseFile(%s) error = %v", name, err)
	}
	return scanFile(fset, file, name)
}

func TestScanFile_positiveBasic(t *testing.T) {
	usages := parseFixture(t, "positive_basic.go")
	if len(usages) != 2 {
		t.Fatalf("got %d usages, want 2: %+v", len(usages), usages)
	}

	if got := usages[0]; got.FlagKey != "checkout-v2" || got.CallType != types.CallTypeBoolVariation ||
		got.Risk != types.RiskLow || got.IsDynamic || got.SDK != "go-server-sdk-v7" || got.Language != "go" {
		t.Errorf("usages[0] = %+v, want checkout-v2/BoolVariation/low/go-server-sdk-v7", got)
	}
	if got := usages[1]; got.FlagKey != "greeting" || got.CallType != types.CallTypeStringVariation || got.Risk != types.RiskLow {
		t.Errorf("usages[1] = %+v, want greeting/StringVariation/low", got)
	}

	wantFP := "launchdarkly:BoolVariation:checkout-v2:positive_basic.go"
	if usages[0].Fingerprint != wantFP {
		t.Errorf("usages[0].Fingerprint = %q, want %q", usages[0].Fingerprint, wantFP)
	}
}

func TestScanFile_positiveDefaultAlias(t *testing.T) {
	usages := parseFixture(t, "positive_default_alias.go")
	if len(usages) != 1 {
		t.Fatalf("got %d usages, want 1: %+v", len(usages), usages)
	}
	if got := usages[0]; got.FlagKey != "limit" || got.CallType != types.CallTypeIntVariation || got.SDK != "go-server-sdk-v7" {
		t.Errorf("usages[0] = %+v, want limit/IntVariation/go-server-sdk-v7 (unaliased import)", got)
	}
}

func TestScanFile_positiveV6(t *testing.T) {
	usages := parseFixture(t, "positive_v6.go")
	if len(usages) != 1 {
		t.Fatalf("got %d usages, want 1: %+v", len(usages), usages)
	}
	if got := usages[0]; got.FlagKey != "v6-flag" || got.SDK != "go-server-sdk-v6" {
		t.Errorf("usages[0] = %+v, want v6-flag/go-server-sdk-v6", got)
	}
}

func TestScanFile_positiveStructField(t *testing.T) {
	usages := parseFixture(t, "positive_struct_field.go")
	if len(usages) != 1 {
		t.Fatalf("got %d usages, want 1: %+v", len(usages), usages)
	}
	if got := usages[0]; got.FlagKey != "struct-field-flag" {
		t.Errorf("usages[0] = %+v, want struct-field-flag (via s.Client binding)", got)
	}
}

func TestScanFile_positiveDotImport(t *testing.T) {
	usages := parseFixture(t, "positive_dot_import.go")
	if len(usages) != 1 {
		t.Fatalf("got %d usages, want 1: %+v", len(usages), usages)
	}
	if got := usages[0]; got.FlagKey != "dot-import-flag" {
		t.Errorf("usages[0] = %+v, want dot-import-flag", got)
	}
}

func TestScanFile_positiveVariety(t *testing.T) {
	usages := parseFixture(t, "positive_variety.go")
	if len(usages) != 5 {
		t.Fatalf("got %d usages, want 5: %+v", len(usages), usages)
	}

	// 1: dynamic identifier argument — high risk regardless of the
	// underlying method's own risk (BoolVariation is otherwise low risk;
	// ADR 002's dynamic-key rule overrides that, since an unresolvable key
	// "cannot resolve statically" no matter how simple the method is).
	if got := usages[0]; !got.IsDynamic || got.FlagKey != "dynamic" || got.Risk != types.RiskHigh {
		t.Errorf("usages[0] = %+v, want dynamic placeholder at high risk", got)
	}
	if want := "launchdarkly:BoolVariation:dynamic:positive_variety.go:0"; usages[0].Fingerprint != want {
		t.Errorf("usages[0].Fingerprint = %q, want %q", usages[0].Fingerprint, want)
	}

	// 2: dynamic fmt.Sprintf argument — second dynamic index in this file
	if got := usages[1]; !got.IsDynamic || got.FlagKey != "dynamic" || got.Risk != types.RiskHigh {
		t.Errorf("usages[1] = %+v, want dynamic placeholder at high risk", got)
	}
	if want := "launchdarkly:StringVariation:dynamic:positive_variety.go:1"; usages[1].Fingerprint != want {
		t.Errorf("usages[1].Fingerprint = %q, want %q", usages[1].Fingerprint, want)
	}

	// 3: *Detail method is high risk
	if got := usages[2]; got.FlagKey != "detail-flag" || got.Risk != types.RiskHigh || got.CallType != types.CallTypeBoolVariationDetail {
		t.Errorf("usages[2] = %+v, want detail-flag/BoolVariationDetail/high", got)
	}

	// 4: bulk call collapses to wildcard
	if got := usages[3]; got.FlagKey != "*" || got.CallType != types.CallTypeAllFlagsState || got.Risk != types.RiskHigh {
		t.Errorf("usages[3] = %+v, want */AllFlagsState/high", got)
	}

	// 5: *Ctx variant — flag key is argument index 1, not 0
	if got := usages[4]; got.FlagKey != "ctx-flag" || got.IsDynamic || got.CallType != types.CallType("BoolVariationCtx") {
		t.Errorf("usages[4] = %+v, want ctx-flag (static, from arg index 1)", got)
	}
}

func TestScanFile_positivePackageVarIIFE(t *testing.T) {
	usages := parseFixture(t, "positive_package_var_iife.go")
	if len(usages) != 1 {
		t.Fatalf("got %d usages, want 1 — a real client construction inside an IIFE assigned to a package var must be scanned: %+v", len(usages), usages)
	}
	if usages[0].FlagKey != "startup-flag" {
		t.Errorf("usages[0].FlagKey = %q, want startup-flag", usages[0].FlagKey)
	}
}

func TestScanFile_positiveParenthesizedCalls(t *testing.T) {
	usages := parseFixture(t, "positive_parenthesized_calls.go")
	if len(usages) != 1 {
		t.Fatalf("got %d usages, want 1 — parenthesized constructor/method calls must still be recognized: %+v", len(usages), usages)
	}
	if usages[0].FlagKey != "paren-flag" {
		t.Errorf("usages[0].FlagKey = %q, want paren-flag", usages[0].FlagKey)
	}
}

// ── False-positive guards (ADR 002's non-negotiable rule) ──────────────────

func TestScanFile_falsePositiveNoSDKImport(t *testing.T) {
	usages := parseFixture(t, "false_positive_no_sdk_import.go")
	if len(usages) != 0 {
		t.Errorf("got %d usages, want 0 — no SDK import means no identity to trace: %+v", len(usages), usages)
	}
}

func TestScanFile_falsePositiveUnboundVariable(t *testing.T) {
	usages := parseFixture(t, "false_positive_unbound_variable.go")
	if len(usages) != 0 {
		t.Errorf("got %d usages, want 0 — variable never bound via MakeClient must not be detected by name alone: %+v", len(usages), usages)
	}
}

func TestScanFile_falsePositiveUnrelatedMakeClient(t *testing.T) {
	usages := parseFixture(t, "false_positive_unrelated_makeclient.go")
	if len(usages) != 0 {
		t.Errorf("got %d usages, want 0 — a local MakeClient function unrelated to the SDK import must not be detected: %+v", len(usages), usages)
	}
}

func TestScanFile_falsePositiveCrossFunctionShadowing(t *testing.T) {
	usages := parseFixture(t, "false_positive_cross_function_shadowing.go")
	if len(usages) != 1 {
		t.Fatalf("got %d usages, want exactly 1 (only runA's real client) — bindings must be scoped per function, not shared across sibling functions: %+v", len(usages), usages)
	}
	if usages[0].FlagKey != "real-flag" {
		t.Errorf("usages[0].FlagKey = %q, want real-flag (runB's unrelated \"client\" must not leak in)", usages[0].FlagKey)
	}
}

func TestScanFile_falsePositiveCrossTypeFieldCollision(t *testing.T) {
	usages := parseFixture(t, "false_positive_cross_type_field_collision.go")
	if len(usages) != 1 {
		t.Fatalf("got %d usages, want exactly 1 (only RealService.Client) — field bindings must be type-qualified, not keyed on receiver variable name alone: %+v", len(usages), usages)
	}
	if usages[0].FlagKey != "real-field-flag" {
		t.Errorf("usages[0].FlagKey = %q, want real-field-flag (FakeService's unrelated \"s.Client\" must not leak in)", usages[0].FlagKey)
	}
}
