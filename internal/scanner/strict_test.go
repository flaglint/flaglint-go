package scanner

import (
	"testing"

	"github.com/flaglint/flaglint-go/internal/config"
	"github.com/flaglint/flaglint-go/internal/types"
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

	// Interface-satisfaction usages go through the same detect() call-
	// argument extraction as any Phase 1 usage (only how the receiver's
	// identity gets proven differs — the call site itself is an ordinary
	// e.BoolVariation(key, ctx, default)), so they get full
	// MigrationInventoryItems too, not just FlagUsages.
	if len(strict.MigrationInventory) != len(strict.Usages) {
		t.Fatalf("MigrationInventory has %d item(s), want %d (one per usage): %+v", len(strict.MigrationInventory), len(strict.Usages), strict.MigrationInventory)
	}
	for _, item := range strict.MigrationInventory {
		if item.CallExpression == "" {
			t.Errorf("item %+v has no CallExpression", item)
		}
		if item.ValueType != types.MigrationValueBoolean {
			t.Errorf("item %+v ValueType = %q, want boolean", item, item.ValueType)
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
	// 2: the one-hop repro (w.Inner.BoolVariation) and a two-hop variant
	// (o.Middle.Inner.BoolVariation) — proving resolveByStaticType
	// generalizes past a single field-selector hop for free, since it
	// queries go/types for the whole expression's real type directly
	// rather than manually walking one hop at a time.
	if len(strict.Usages) != 2 {
		t.Fatalf("ScanStrict found %d usage(s), want 2: %+v", len(strict.Usages), strict.Usages)
	}
	wantFlagKeys := map[string]bool{"transitive-factory-flag": false, "two-hop-transitive-factory-flag": false}
	for _, got := range strict.Usages {
		if _, ok := wantFlagKeys[got.FlagKey]; !ok {
			t.Errorf("unexpected usage %+v", got)
			continue
		}
		wantFlagKeys[got.FlagKey] = true
		if got.DetectedBy != "strict-types" {
			t.Errorf("usage %q DetectedBy = %q, want strict-types", got.FlagKey, got.DetectedBy)
		}
		if got.SDK != "go-server-sdk-v7" {
			t.Errorf("usage %q SDK = %q, want go-server-sdk-v7", got.FlagKey, got.SDK)
		}
	}
	for k, found := range wantFlagKeys {
		if !found {
			t.Errorf("missing expected usage for flag key %q", k)
		}
	}
}

func TestScanStrict_positiveForwardingCall(t *testing.T) {
	// Issue #26 (ADR 006): a method value captured in one function,
	// passed as an argument, and invoked from inside a different callee
	// ("forwarding function") — the harder remainder after #6 fixed the
	// same-scope case. Covers: a non-generic forwarding function, a
	// generic one matching the issue's exact repro (proving
	// findEvalSummaries resolves the pre-instantiation *types.Func, not a
	// per-call-site instantiated wrapper), a one-hop "pass-through"
	// function (fixes the SDK method internally, forwards only its own
	// key parameter — the simplified version of the real e2b-dev/infra
	// shape), a false-positive guard (unrelated same-shaped method
	// value), and a genuinely dynamic (non-literal) key guard.
	dir := "testdata/strict/forwarding_call"
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
		t.Fatalf("Scan (Phase 1) found %d usage(s), want 0 — a method value crossing a function boundary is exactly what Phase 1 cannot see: %+v", len(phase1.Usages), phase1.Usages)
	}

	strict, err := ScanStrict(dir, cfg)
	if err != nil {
		t.Fatalf("ScanStrict error = %v", err)
	}
	if len(strict.Warnings) != 0 {
		t.Fatalf("ScanStrict warnings = %+v, want none — the fixture module builds cleanly", strict.Warnings)
	}
	if len(strict.Usages) != 3 {
		t.Fatalf("ScanStrict found %d usage(s), want exactly 3 (should-not-be-detected and the genuinely dynamic-key call must both be absent): %+v", len(strict.Usages), strict.Usages)
	}

	wantFlagKeys := map[string]bool{"forwarding-direct-flag": false, "forwarding-generic-flag": false, "pass-through-flag": false}
	for _, got := range strict.Usages {
		if _, ok := wantFlagKeys[got.FlagKey]; !ok {
			t.Errorf("unexpected usage %+v", got)
			continue
		}
		wantFlagKeys[got.FlagKey] = true
		if got.DetectedBy != "strict-types" {
			t.Errorf("usage %q DetectedBy = %q, want strict-types", got.FlagKey, got.DetectedBy)
		}
		if got.IsDynamic {
			t.Errorf("usage %q IsDynamic = true, want false", got.FlagKey)
		}
		if got.SDK != "go-server-sdk-v7" {
			t.Errorf("usage %q SDK = %q, want go-server-sdk-v7", got.FlagKey, got.SDK)
		}
	}
	for k, found := range wantFlagKeys {
		if !found {
			t.Errorf("missing expected usage for flag key %q", k)
		}
	}

	// A forwarding-function call site (e.g. callDirect(client.BoolVariation,
	// key, def)) doesn't directly show the LD method's own (key, context,
	// fallback) arguments the way a migrate rewrite would need — deliberately
	// excluded from MigrationInventory (see its doc comment, types.go), so
	// none of these 3 strict-types-only usages should have produced one.
	if len(strict.MigrationInventory) != 0 {
		t.Errorf("MigrationInventory = %+v, want empty — forwarding-call usages have no migration item", strict.MigrationInventory)
	}
}

func TestScanStrict_positiveFlagDescriptorChain(t *testing.T) {
	// The real e2b-dev/infra shape found during --strict-types
	// verification against that repo (issue #26's actual motivating
	// case) — a package-level var (SnapshotFeatureFlag) built by a
	// factory (NewBoolFlag) that stores a literal into a struct field,
	// read back via a trivial accessor (Key), forwarded through TWO
	// levels of function wrapping (Client.BoolFlag, a pass-through, then
	// getFlag, a direct forwarding function) before reaching the SDK
	// call. See testdata/strict/flag_descriptor/main.go and
	// forwarding.go's package doc comment.
	dir := "testdata/strict/flag_descriptor"
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
		t.Fatalf("Scan (Phase 1) found %d usage(s), want 0: %+v", len(phase1.Usages), phase1.Usages)
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
	if got.FlagKey != "use-nfs-for-snapshots" {
		t.Errorf("usages[0].FlagKey = %q, want use-nfs-for-snapshots", got.FlagKey)
	}
	if got.CallType != types.CallType("BoolVariationCtx") {
		t.Errorf("usages[0].CallType = %q, want BoolVariationCtx", got.CallType)
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
