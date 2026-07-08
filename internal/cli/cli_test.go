package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// binPath is built once in TestMain and reused by every test — spawning the
// real compiled binary (not calling package internals) catches issues unit
// tests miss: os.Exit paths, stdout/stderr separation, flag parsing.
var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "flaglint-go-cli-test-*")
	if err != nil {
		panic(err)
	}
	// NOT a `defer os.RemoveAll(dir)` — os.Exit below bypasses every
	// deferred function in this goroutine, so a deferred cleanup here
	// would silently never run on any test invocation. Clean up
	// explicitly before exiting instead.

	binName := "flaglint-go"
	if runtime.GOOS == "windows" {
		// go build appends .exe to the output file on Windows regardless
		// of the -o path given — the binary on disk is actually named
		// "flaglint-go.exe" there, so binPath must match or every
		// exec.Command call below fails with "executable file not found".
		binName += ".exe"
	}
	binPath = filepath.Join(dir, binName)
	build := exec.Command("go", "build", "-o", binPath, "./../../cmd/flaglint-go")
	build.Dir = mustGetwd()
	if out, err := build.CombinedOutput(); err != nil {
		panic("build failed: " + err.Error() + "\n" + string(out))
	}

	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return wd
}

func writeGoFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const sampleService = `package svc

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

func Run() {
	client, _ := ld.MakeClient("sdk-key", 5*time.Second)
	_, _ = client.BoolVariation("checkout-v2", nil, false)
}
`

func TestCLI_scanMarkdown(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, filepath.Join(dir, "flags.go"), sampleService)

	cmd := exec.Command(binPath, "scan", dir)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if !strings.Contains(string(out), "checkout-v2") {
		t.Errorf("stdout missing flag, got:\n%s", out)
	}
	if cmd.ProcessState.ExitCode() != 0 {
		t.Errorf("exit code = %d, want 0", cmd.ProcessState.ExitCode())
	}
}

func TestCLI_scanJSON_isValidAndOnStdoutOnly(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, filepath.Join(dir, "flags.go"), sampleService)

	cmd := exec.Command(binPath, "scan", dir, "--format", "json")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("scan failed: %v, stderr=%s", err, stderr.String())
	}

	if !strings.HasPrefix(strings.TrimSpace(stdout.String()), "{") {
		t.Errorf("stdout is not JSON (progress output leaked into stdout?), got:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Scan complete") {
		t.Errorf("stderr missing progress summary, got:\n%s", stderr.String())
	}
}

func TestCLI_auditShowsReadiness(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, filepath.Join(dir, "flags.go"), sampleService)

	cmd := exec.Command(binPath, "audit", dir)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("audit failed: %v, stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stderr.String(), "Migration readiness") {
		t.Errorf("stderr missing readiness summary, got:\n%s", stderr.String())
	}
}

func TestCLI_nonexistentDirectoryExits2(t *testing.T) {
	cmd := exec.Command(binPath, "scan", "/no/such/directory-xyz")
	err := cmd.Run()
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected *exec.ExitError, got %v", err)
	}
	if exitErr.ExitCode() != 2 {
		t.Errorf("exit code = %d, want 2", exitErr.ExitCode())
	}
}

func TestCLI_invalidFormatExits2(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, filepath.Join(dir, "flags.go"), sampleService)

	cmd := exec.Command(binPath, "scan", dir, "--format", "sarif")
	err := cmd.Run()
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected *exec.ExitError, got %v", err)
	}
	if exitErr.ExitCode() != 2 {
		t.Errorf("exit code = %d, want 2", exitErr.ExitCode())
	}
}

func TestCLI_scanExitsZeroEvenWithHighRiskFindings(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, filepath.Join(dir, "flags.go"), `package svc

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

func Run() {
	client, _ := ld.MakeClient("sdk-key", 5*time.Second)
	_ = client.AllFlagsState(nil)
}
`)
	cmd := exec.Command(binPath, "scan", dir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("scan is an inventory command and must exit 0 even with high-risk findings, got error: %v", err)
	}
	if cmd.ProcessState.ExitCode() != 0 {
		t.Errorf("exit code = %d, want 0 (scan never enforces policy)", cmd.ProcessState.ExitCode())
	}
}

func TestCLI_customConfigPath(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, filepath.Join(dir, "flags.go"), sampleService)
	configPath := filepath.Join(dir, "custom-config.json")
	writeGoFile(t, configPath, `{"reportTitle": "Custom Title From Config"}`)

	out, err := exec.Command(binPath, "scan", dir, "--config", configPath).Output()
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if !strings.Contains(string(out), "Custom Title From Config") {
		t.Errorf("output missing custom reportTitle from --config, got:\n%s", out)
	}
}

func TestCLI_malformedConfigExits2(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, filepath.Join(dir, "flags.go"), sampleService)
	configPath := filepath.Join(dir, "bad-config.json")
	writeGoFile(t, configPath, `{not valid json`)

	cmd := exec.Command(binPath, "scan", dir, "--config", configPath)
	err := cmd.Run()
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected *exec.ExitError, got %v", err)
	}
	if exitErr.ExitCode() != 2 {
		t.Errorf("exit code = %d, want 2", exitErr.ExitCode())
	}
}

func TestCLI_emptyScanProducesEmptyArraysNotNull(t *testing.T) {
	dir := t.TempDir() // no .go files at all

	out, err := exec.Command(binPath, "scan", dir, "--format", "json").Output()
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	for _, field := range []string{`"uniqueFlags":null`, `"usages":null`, `"warnings":null`} {
		if strings.Contains(string(out), field) {
			t.Errorf("output contains %q — empty results must serialize as [], not null:\n%s", field, out)
		}
	}
}

func TestCLI_version(t *testing.T) {
	out, err := exec.Command(binPath, "--version").Output()
	if err != nil {
		t.Fatalf("--version failed: %v", err)
	}
	if !strings.Contains(string(out), "flaglint-go") {
		t.Errorf("--version output = %q, want it to mention flaglint-go", out)
	}
}

func TestCLI_writeOutputToFile(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, filepath.Join(dir, "flags.go"), sampleService)
	outFile := filepath.Join(dir, "report.md")

	cmd := exec.Command(binPath, "scan", dir, "--output", outFile)
	if err := cmd.Run(); err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	content, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("output file not written: %v", err)
	}
	if !strings.Contains(string(content), "checkout-v2") {
		t.Errorf("output file missing flag, got:\n%s", content)
	}
}

func TestCLI_validate_reportOnlyByDefault(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, filepath.Join(dir, "flags.go"), sampleService)

	cmd := exec.Command(binPath, "validate", dir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("validate without --no-direct-launchdarkly must never fail, got: %v", err)
	}
}

func TestCLI_validate_noDirectLaunchDarklyFailsWithFindings(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, filepath.Join(dir, "flags.go"), sampleService)

	cmd := exec.Command(binPath, "validate", dir, "--no-direct-launchdarkly")
	err := cmd.Run()
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected *exec.ExitError, got %v", err)
	}
	if exitErr.ExitCode() != 1 {
		t.Errorf("exit code = %d, want 1 (policy failure)", exitErr.ExitCode())
	}
}

func TestCLI_validate_bootstrapExcludeAllowsFile(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, filepath.Join(dir, "provider", "bootstrap.go"), sampleService)

	cmd := exec.Command(binPath, "validate", dir, "--no-direct-launchdarkly", "--bootstrap-exclude", "provider/**")
	if err := cmd.Run(); err != nil {
		t.Fatalf("validate with matching --bootstrap-exclude must pass, got: %v", err)
	}
}

func TestCLI_validate_sarifFormat(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, filepath.Join(dir, "flags.go"), sampleService)

	out, err := exec.Command(binPath, "validate", dir, "--no-direct-launchdarkly", "--format", "sarif").Output()
	// Exit code 1 is expected here (violations exist) — .Output() returns an
	// error in that case, but stdout is still captured and must be valid SARIF.
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if !strings.Contains(string(out), `"flaglint.go.direct-launchdarkly"`) {
		t.Errorf("SARIF output missing Go-namespaced rule ID, got:\n%s", out)
	}
}

// TestCLI_baselineRoundTrip exercises the primary CI-adoption workflow end
// to end: write a baseline, add a new finding, confirm --fail-on-new only
// fails on the new one. This is exactly the round-trip flaglint-js's own
// architectural review flagged as an untested gap when it audited that
// project — verified here from day one rather than retrofitted later.
func TestCLI_baselineRoundTrip(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, filepath.Join(dir, "flags.go"), sampleService)
	baselinePath := filepath.Join(dir, "baseline.json")

	if err := exec.Command(binPath, "audit", dir, "--write-baseline", baselinePath).Run(); err != nil {
		t.Fatalf("audit --write-baseline failed: %v", err)
	}
	if _, err := os.Stat(baselinePath); err != nil {
		t.Fatalf("baseline file not written: %v", err)
	}

	// No new findings yet — --fail-on-new must pass.
	if err := exec.Command(binPath, "validate", dir, "--baseline", baselinePath, "--fail-on-new").Run(); err != nil {
		t.Fatalf("validate --baseline --fail-on-new should pass with no new findings, got: %v", err)
	}

	// Add a new finding.
	writeGoFile(t, filepath.Join(dir, "more.go"), `package svc

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

func RunMore() {
	client, _ := ld.MakeClient("sdk-key", 5*time.Second)
	_, _ = client.StringVariation("brand-new-flag", nil, "x")
}
`)

	cmd := exec.Command(binPath, "validate", dir, "--baseline", baselinePath, "--fail-on-new")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected *exec.ExitError after adding a new finding, got %v", err)
	}
	if exitErr.ExitCode() != 1 {
		t.Errorf("exit code = %d, want 1", exitErr.ExitCode())
	}
	if !strings.Contains(stderr.String(), "brand-new-flag") {
		t.Errorf("stderr missing the new finding's fingerprint, got:\n%s", stderr.String())
	}
	if strings.Contains(stderr.String(), "checkout-v2") {
		t.Errorf("stderr must not list the already-baselined finding as new, got:\n%s", stderr.String())
	}
}

func TestCLI_baselineRatchetHole_duplicateCallExceedsBaselinedCount(t *testing.T) {
	// The v1 fingerprint's known static-collision limitation
	// (spec/fingerprint.md in flaglint/spec): two call sites sharing
	// (callType, flagKey, file) share one fingerprint string. Before the
	// baseline "counts" extension, a brand-new *duplicate* of an
	// already-baselined call was invisible to --fail-on-new, since the
	// fingerprint was already in the known set — copy-paste is exactly
	// how flag debt spreads, so this was a real ratchet hole, not a
	// theoretical one. This test proves it's closed.
	dir := t.TempDir()
	writeGoFile(t, filepath.Join(dir, "flags.go"), sampleService)
	baselinePath := filepath.Join(dir, "baseline.json")

	if err := exec.Command(binPath, "audit", dir, "--write-baseline", baselinePath).Run(); err != nil {
		t.Fatalf("audit --write-baseline failed: %v", err)
	}

	// A second, identically-shaped call in the SAME file — same callType,
	// flagKey, and file, so it shares checkout-v2's exact v1 fingerprint
	// with the one already baselined at count 1.
	writeGoFile(t, filepath.Join(dir, "flags.go"), `package svc

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

func Run() {
	client, _ := ld.MakeClient("sdk-key", 5*time.Second)
	_, _ = client.BoolVariation("checkout-v2", nil, false)
	_, _ = client.BoolVariation("checkout-v2", nil, false)
}
`)

	cmd := exec.Command(binPath, "validate", dir, "--baseline", baselinePath, "--fail-on-new")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected *exec.ExitError for a new duplicate call exceeding its baselined count, got %v (stderr: %s)", err, stderr.String())
	}
	if exitErr.ExitCode() != 1 {
		t.Errorf("exit code = %d, want 1 (stderr: %s)", exitErr.ExitCode(), stderr.String())
	}
}

func TestCLI_validate_malformedBaselineExits2(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, filepath.Join(dir, "flags.go"), sampleService)
	baselinePath := filepath.Join(dir, "bad-baseline.json")
	writeGoFile(t, baselinePath, `{not valid json`)

	cmd := exec.Command(binPath, "validate", dir, "--baseline", baselinePath, "--fail-on-new")
	err := cmd.Run()
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected *exec.ExitError, got %v", err)
	}
	if exitErr.ExitCode() != 2 {
		t.Errorf("exit code = %d, want 2", exitErr.ExitCode())
	}
}
