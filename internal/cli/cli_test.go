package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
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
	defer os.RemoveAll(dir)

	binPath = filepath.Join(dir, "flaglint-go")
	build := exec.Command("go", "build", "-o", binPath, "./../../cmd/flaglint-go")
	build.Dir = mustGetwd()
	if out, err := build.CombinedOutput(); err != nil {
		panic("build failed: " + err.Error() + "\n" + string(out))
	}

	os.Exit(m.Run())
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
