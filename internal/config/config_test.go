package config

import (
	"os"
	"path/filepath"
	"testing"
)

// withTempDir chdirs into a fresh temp dir for the duration of the test and
// restores the original working directory afterward.
func withTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(orig)
	})
	return dir
}

func TestLoad_defaultsWhenNoConfigFile(t *testing.T) {
	withTempDir(t)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := defaultConfig()
	if cfg.Provider != want.Provider || cfg.OutputDir != want.OutputDir {
		t.Errorf("Load() = %+v, want defaults %+v", cfg, want)
	}
	if len(cfg.Include) != 1 || cfg.Include[0] != "**/*.go" {
		t.Errorf("Load().Include = %v, want [**/*.go]", cfg.Include)
	}
}

func TestLoad_searchOrder(t *testing.T) {
	dir := withTempDir(t)

	// .flaglintrc.json should be picked up even though flaglint.config.json
	// also exists — .flaglintrc.json is earlier in the search order.
	writeFile(t, filepath.Join(dir, "flaglint.config.json"), `{"outputDir": "from-lower-priority"}`)
	writeFile(t, filepath.Join(dir, ".flaglintrc.json"), `{"outputDir": "from-higher-priority"}`)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.OutputDir != "from-higher-priority" {
		t.Errorf("Load().OutputDir = %q, want %q", cfg.OutputDir, "from-higher-priority")
	}
}

func TestLoad_customPath(t *testing.T) {
	dir := withTempDir(t)
	custom := filepath.Join(dir, "custom.json")
	writeFile(t, custom, `{"outputDir": "custom-dir"}`)

	cfg, err := Load(custom)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.OutputDir != "custom-dir" {
		t.Errorf("Load().OutputDir = %q, want custom-dir", cfg.OutputDir)
	}
}

func TestLoad_malformedJSON(t *testing.T) {
	dir := withTempDir(t)
	writeFile(t, filepath.Join(dir, ".flaglintrc"), `{not valid json`)

	_, err := Load("")
	if err == nil {
		t.Fatal("Load() error = nil, want error for malformed JSON")
	}
}

func TestLoad_unsupportedProvider(t *testing.T) {
	dir := withTempDir(t)
	writeFile(t, filepath.Join(dir, ".flaglintrc"), `{"provider": "unleash"}`)

	_, err := Load("")
	if err == nil {
		t.Fatal("Load() error = nil, want error for unsupported provider")
	}
}

func TestLoad_explicitLaunchDarklyProviderOK(t *testing.T) {
	dir := withTempDir(t)
	writeFile(t, filepath.Join(dir, ".flaglintrc"), `{"provider": "launchdarkly"}`)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Provider != "launchdarkly" {
		t.Errorf("Load().Provider = %q, want launchdarkly", cfg.Provider)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
