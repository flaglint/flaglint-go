package scanner

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"

	"github.com/flaglint/flaglint-go/internal/types"
)

// parseFixture parses a single fixture file in isolation and runs it
// through the same whole-scan analysis Scan() uses, with a "scan" of
// exactly one file — the cross-file passes (struct field types, factory
// functions, package-level bindings) degrade naturally to single-file
// behavior when there's only one file to analyze, so this remains a valid
// way to test a self-contained fixture. Fixtures that specifically
// exercise cross-file resolution use parseFixtures (plural) instead.
func parseFixture(t *testing.T, name string) []types.FlagUsage {
	t.Helper()
	return parseFixtures(t, name)
}

func parseFixtures(t *testing.T, names ...string) []types.FlagUsage {
	t.Helper()
	fset := token.NewFileSet()
	var parsed []parsedFile
	for _, name := range names {
		path := filepath.Join("testdata", "fixtures", name)
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("ParseFile(%s) error = %v", name, err)
		}
		absDir, err := filepath.Abs(filepath.Dir(path))
		if err != nil {
			t.Fatalf("Abs(%s) error = %v", path, err)
		}
		parsed = append(parsed, parsedFile{
			relPath: name,
			dir:     absDir,
			file:    file,
			imports: traceSDKImports(file),
		})
	}
	absRoot, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("Abs(.) error = %v", err)
	}
	return runWholeScanAnalysis(fset, absRoot, parsed)
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

func TestScanFile_positiveCompositeLiteral(t *testing.T) {
	usages := parseFixture(t, "positive_composite_literal.go")
	if len(usages) != 1 {
		t.Fatalf("got %d usages, want 1: %+v", len(usages), usages)
	}
	if got := usages[0]; got.FlagKey != "composite-literal-flag" {
		t.Errorf("usages[0] = %+v, want composite-literal-flag (via &CompositeIntegration{ldClient: client})", got)
	}
}

func TestScanFile_falsePositiveCompositeLiteralUnbound(t *testing.T) {
	usages := parseFixture(t, "false_positive_composite_literal_unbound.go")
	if len(usages) != 0 {
		t.Errorf("got %d usages, want 0 — composite-literal field value that never resolves to a bound client must not be detected: %+v", len(usages), usages)
	}
}

func TestScanFiles_positiveMultiLevelChain(t *testing.T) {
	usages := parseFixtures(t, "positive_multilevel_types.go", "positive_multilevel_usage.go")
	if len(usages) != 1 {
		t.Fatalf("got %d usages, want 1: %+v", len(usages), usages)
	}
	if got := usages[0]; got.FlagKey != "multilevel-flag" {
		t.Errorf("usages[0] = %+v, want multilevel-flag (via f.integ.ldClient, a two-level chain resolved across files)", got)
	}
}

func TestScanFile_positiveParamTypedClient(t *testing.T) {
	usages := parseFixture(t, "positive_param_typed_client.go")
	if len(usages) != 1 {
		t.Fatalf("got %d usages, want 1: %+v", len(usages), usages)
	}
	if got := usages[0]; got.FlagKey != "param-typed-flag" {
		t.Errorf("usages[0] = %+v, want param-typed-flag (client passed in as a plain *ld.LDClient parameter, no assignment to trace)", got)
	}
}

func TestScanFile_falsePositiveParamTypedUnrelated(t *testing.T) {
	usages := parseFixture(t, "false_positive_param_typed_unrelated.go")
	if len(usages) != 0 {
		t.Errorf("got %d usages, want 0 — a same-named *LDClient parameter type with no traced SDK import must not be detected: %+v", len(usages), usages)
	}
}

// TestScanFiles_crossPackageFieldIsolation reproduces a real bug found in
// review: whole-scan struct-field binding was originally keyed by bare
// "TypeName.Field" text in one flat map across the entire scan, so a
// genuinely bound "Service.Client" in one package would incorrectly also
// match an unrelated, unconnected "Service.Client" in a completely
// different package that happens to share both names. Struct-field
// bindings must be partitioned per-package.
func TestScanFiles_crossPackageFieldIsolation(t *testing.T) {
	usages := parseFixtures(t,
		"pkgisolation/realfield/service.go",
		"pkgisolation/unrelatedfield/service.go",
	)
	if len(usages) != 1 {
		t.Fatalf("got %d usages, want 1: %+v", len(usages), usages)
	}
	if got := usages[0]; got.FlagKey != "pkg-isolation-field-flag" || got.File != "pkgisolation/realfield/service.go" {
		t.Errorf("usages[0] = %+v, want pkg-isolation-field-flag from realfield/service.go only", got)
	}
}

// TestScanFiles_crossPackageVarIsolation is the same class of bug as
// TestScanFiles_crossPackageFieldIsolation, for package-level vars instead
// of struct fields.
func TestScanFiles_crossPackageVarIsolation(t *testing.T) {
	usages := parseFixtures(t,
		"pkgisolation/realvar/client.go",
		"pkgisolation/unrelatedvar/client.go",
	)
	if len(usages) != 1 {
		t.Fatalf("got %d usages, want 1: %+v", len(usages), usages)
	}
	if got := usages[0]; got.FlagKey != "pkg-isolation-var-flag" || got.File != "pkgisolation/realvar/client.go" {
		t.Errorf("usages[0] = %+v, want pkg-isolation-var-flag from realvar/client.go only", got)
	}
}

func TestScanFile_positiveTripleLevelChain(t *testing.T) {
	usages := parseFixture(t, "positive_triplelevel_chain.go")
	if len(usages) != 1 {
		t.Fatalf("got %d usages, want 1: %+v", len(usages), usages)
	}
	if got := usages[0]; got.FlagKey != "triplelevel-flag" {
		t.Errorf("usages[0] = %+v, want triplelevel-flag (three-level chain o.middle.integ.ldClient)", got)
	}
}

func TestScanFile_positiveGenericReceiver(t *testing.T) {
	usages := parseFixture(t, "positive_generic_receiver.go")
	if len(usages) != 1 {
		t.Fatalf("got %d usages, want 1: %+v", len(usages), usages)
	}
	if got := usages[0]; got.FlagKey != "generic-receiver-flag" {
		t.Errorf("usages[0] = %+v, want generic-receiver-flag (method on a generic struct, receiver type *ast.IndexExpr)", got)
	}
}

func TestScanFiles_positiveCrossFilePackageVar(t *testing.T) {
	usages := parseFixtures(t, "positive_crossfile_pkgvar_a.go", "positive_crossfile_pkgvar_b.go")
	if len(usages) != 1 {
		t.Fatalf("got %d usages, want 1: %+v", len(usages), usages)
	}
	if got := usages[0]; got.FlagKey != "crossfile-pkgvar-flag" {
		t.Errorf("usages[0] = %+v, want crossfile-pkgvar-flag (package var bound in one file, used in another)", got)
	}
}

func TestScanFiles_positiveCrossPackageFactoryFunction(t *testing.T) {
	usages := parseFixtures(t,
		"crosspkg/producer/client.go",
		"crosspkg/consumer/main.go",
	)
	if len(usages) != 1 {
		t.Fatalf("got %d usages, want 1: %+v", len(usages), usages)
	}
	if got := usages[0]; got.FlagKey != "cross-package-flag" {
		t.Errorf("usages[0] = %+v, want cross-package-flag (via producer.GetLdClient(), a cross-package factory function)", got)
	}
}

func TestScanFiles_falsePositiveCrossPackageFactoryFunctionUnrelated(t *testing.T) {
	usages := parseFixtures(t,
		"crosspkg/unrelated_producer/client.go",
		"crosspkg/consumer_of_unrelated/main.go",
	)
	if len(usages) != 0 {
		t.Errorf("got %d usages, want 0 — a same-named factory function returning an unrelated type must not be detected: %+v", len(usages), usages)
	}
}

func TestScanFile_positiveChainedFactoryCall(t *testing.T) {
	usages := parseFixture(t, "positive_chained_factory_call.go")
	if len(usages) != 1 {
		t.Fatalf("got %d usages, want 1: %+v", len(usages), usages)
	}
	if got := usages[0]; got.FlagKey != "chained-factory-flag" {
		t.Errorf("usages[0] = %+v, want chained-factory-flag (chainedFactory().BoolVariation(...) — no intermediate variable, issue #20)", got)
	}
}

func TestScanFiles_positiveChainedCrossPackageFactoryCall(t *testing.T) {
	usages := parseFixtures(t,
		"crosspkg/producer/client.go",
		"crosspkg/consumer_chained/main.go",
	)
	if len(usages) != 1 {
		t.Fatalf("got %d usages, want 1: %+v", len(usages), usages)
	}
	if got := usages[0]; got.FlagKey != "chained-cross-package-flag" {
		t.Errorf("usages[0] = %+v, want chained-cross-package-flag (producer.GetLdClient().BoolVariation(...) — no intermediate variable)", got)
	}
}

func TestScanFile_falsePositiveChainedCallUnrelated(t *testing.T) {
	usages := parseFixture(t, "false_positive_chained_call_unrelated.go")
	if len(usages) != 0 {
		t.Errorf("got %d usages, want 0 — a chained call on a non-factory function must not be detected: %+v", len(usages), usages)
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

func TestScanFile_falsePositiveBlockScopedShadowing(t *testing.T) {
	usages := parseFixture(t, "false_positive_block_scoped_shadowing.go")
	if len(usages) != 2 {
		t.Fatalf("got %d usages, want exactly 2 (before-loop-flag and after-loop-flag) — a re-`:=` of the same name inside a nested block must shadow the outer binding only within that block, not leak the shadow's absence back out: %+v", len(usages), usages)
	}
	got := map[string]bool{}
	for _, u := range usages {
		got[u.FlagKey] = true
	}
	if !got["before-loop-flag"] || !got["after-loop-flag"] {
		t.Errorf("usages = %+v, want before-loop-flag and after-loop-flag, neither should-not-be-detected", usages)
	}
	if got["should-not-be-detected"] {
		t.Errorf("usages = %+v, want should-not-be-detected absent — the nested block's unrelated \"client\" must shadow the outer real one", usages)
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
