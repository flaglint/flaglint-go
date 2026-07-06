package config

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestLoad_explicitNullFieldRejected(t *testing.T) {
	dir := withTempDir(t)
	writeFile(t, filepath.Join(dir, ".flaglintrc"), `{"include": null}`)

	_, err := Load("")
	if err == nil {
		t.Fatal("Load() error = nil, want error for explicit null on a slice field (must not silently drop the default)")
	}
}

func TestLoad_explicitNullScalarRejected(t *testing.T) {
	dir := withTempDir(t)
	writeFile(t, filepath.Join(dir, ".flaglintrc"), `{"provider": null}`)

	_, err := Load("")
	if err == nil {
		t.Fatal("Load() error = nil, want error for explicit null on a scalar field")
	}
}

func TestLoad_topLevelNullRejected(t *testing.T) {
	dir := withTempDir(t)
	writeFile(t, filepath.Join(dir, ".flaglintrc"), `null`)

	_, err := Load("")
	if err == nil {
		t.Fatal("Load() error = nil, want error for a config file that is just `null`")
	}
}

func TestLoad_unreadableCandidateSkipsToNext(t *testing.T) {
	if runtime.GOOS == "windows" {
		// os.Chmod on Windows only toggles the read-only attribute, not a
		// POSIX-style permission bitmask — 0o000 does not actually make
		// the file unreadable by the current user there, so this test's
		// premise doesn't hold. Verified by this suite's own Windows CI
		// run, not assumed.
		t.Skip("chmod-based unreadable-file simulation is POSIX-only")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root — file permissions are not enforced")
	}
	dir := withTempDir(t)
	unreadable := filepath.Join(dir, ".flaglintrc")
	writeFile(t, unreadable, `{"provider": "launchdarkly"}`)
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o644) })

	writeFile(t, filepath.Join(dir, ".flaglintrc.json"), `{"outputDir": "from-second-candidate"}`)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v, want it to skip the unreadable candidate and use the next one", err)
	}
	if cfg.OutputDir != "from-second-candidate" {
		t.Errorf("Load().OutputDir = %q, want from-second-candidate", cfg.OutputDir)
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
