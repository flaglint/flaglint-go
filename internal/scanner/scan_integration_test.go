package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/flaglint/flaglint-go/internal/config"
)

func writeGoFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScan_endToEnd(t *testing.T) {
	root := t.TempDir()

	writeGoFile(t, filepath.Join(root, "flags.go"), `package svc

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

func run() {
	client, _ := ld.MakeClient("key", 5*time.Second)
	_, _ = client.BoolVariation("checkout-v2", nil, false)
}
`)
	writeGoFile(t, filepath.Join(root, "unrelated.go"), `package svc

func noop() {}
`)
	writeGoFile(t, filepath.Join(root, "vendor", "dep", "dep.go"), `package dep

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

func shouldNotBeScanned() {
	client, _ := ld.MakeClient("key", 5*time.Second)
	_, _ = client.BoolVariation("vendored-flag", nil, false)
}
`)

	cfg := config.Config{Include: []string{"**/*.go"}, Exclude: []string{"**/vendor/**"}}
	result, err := Scan(root, cfg)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	if result.ScannedFiles != 2 {
		t.Errorf("ScannedFiles = %d, want 2 (vendor/ must be excluded)", result.ScannedFiles)
	}
	if result.TotalUsages != 1 {
		t.Fatalf("TotalUsages = %d, want 1: %+v", result.TotalUsages, result.Usages)
	}
	if result.Usages[0].FlagKey != "checkout-v2" {
		t.Errorf("Usages[0].FlagKey = %q, want checkout-v2", result.Usages[0].FlagKey)
	}
	if len(result.UniqueFlags) != 1 || result.UniqueFlags[0] != "checkout-v2" {
		t.Errorf("UniqueFlags = %v, want [checkout-v2]", result.UniqueFlags)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("Warnings = %+v, want none", result.Warnings)
	}
}

func TestScan_parseFailureBecomesWarningNotFatal(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, filepath.Join(root, "broken.go"), `package svc

func broken( {{{ not valid go
`)
	writeGoFile(t, filepath.Join(root, "flags.go"), `package svc

import (
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
)

func run() {
	client, _ := ld.MakeClient("key", 5*time.Second)
	_, _ = client.BoolVariation("ok-flag", nil, false)
}
`)

	cfg := config.Config{Include: []string{"**/*.go"}, Exclude: nil}
	result, err := Scan(root, cfg)
	if err != nil {
		t.Fatalf("Scan() error = %v, want nil (parse failures are warnings, not fatal)", err)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Kind != "parse-failure" {
		t.Errorf("Warnings = %+v, want one parse-failure", result.Warnings)
	}
	if result.TotalUsages != 1 {
		t.Errorf("TotalUsages = %d, want 1 (the valid file should still be scanned)", result.TotalUsages)
	}
}
