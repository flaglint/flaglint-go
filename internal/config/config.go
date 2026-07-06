// Package config loads flaglint.config.json (or .flaglintrc[.json]),
// matching flaglint-js's search order and field names — see
// docs/adr/003-cross-tool-contract.md. Only the fields flaglint-go's Go
// scanner actually uses are implemented; flaglint-js-only fields
// (minFileCount, wrappers, openFeatureClientBindings) are intentionally
// omitted rather than stubbed, since they have no Go-side behavior yet.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config is the subset of FlagLintConfig that flaglint-go currently wires up.
type Config struct {
	Include     []string `json:"include"`
	Exclude     []string `json:"exclude"`
	Provider    string   `json:"provider"`
	ReportTitle string   `json:"reportTitle,omitempty"`
	OutputDir   string   `json:"outputDir"`
}

func defaultConfig() Config {
	return Config{
		Include:   []string{"**/*.go"},
		Exclude:   []string{"**/vendor/**", "**/testdata/**", "**/.git/**"},
		Provider:  "launchdarkly",
		OutputDir: ".",
	}
}

var searchPaths = []string{".flaglintrc", ".flaglintrc.json", "flaglint.config.json"}

// Load searches the current directory for a config file in flaglint-js's
// search order, or reads configPath directly if non-empty. Returns defaults
// if no config file is found. Returns an error (the caller should exit 2)
// for malformed JSON or an unsupported provider.
func Load(configPath string) (Config, error) {
	candidates := searchPaths
	if configPath != "" {
		candidates = []string{configPath}
	}

	for _, candidate := range candidates {
		full, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		// Mirrors loadConfig()'s bare `catch { continue }` in config.ts: any
		// read failure (not just "not found" — permission denied, EISDIR,
		// etc.) skips this candidate and tries the next one.
		raw, err := os.ReadFile(full)
		if err != nil {
			continue
		}

		if err := rejectExplicitNulls(raw, candidate); err != nil {
			return Config{}, err
		}

		cfg := defaultConfig()
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return Config{}, fmt.Errorf("error in %s: %w", candidate, err)
		}

		if err := assertSupportedProvider(cfg.Provider, candidate); err != nil {
			return Config{}, err
		}
		return cfg, nil
	}

	return defaultConfig(), nil
}

// rejectExplicitNulls errors on any top-level field whose JSON value is the
// literal `null`. This matches Zod's behavior in the TS reference: `.default()`
// only fires when a key is *absent* — a key explicitly set to `null` still
// fails schema validation there. Without this check, Go's json.Unmarshal
// would silently reset slice-typed fields (include/exclude) to nil on
// explicit null while leaving scalar fields untouched, an asymmetry that
// has no equivalent in the TS reference and would otherwise pass silently.
func rejectExplicitNulls(raw []byte, candidate string) error {
	trimmed := bytes.TrimSpace(raw)
	if bytes.Equal(trimmed, []byte("null")) {
		return fmt.Errorf("error in %s: config must be a JSON object, not null", candidate)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fmt.Errorf("error in %s: %w", candidate, err)
	}
	for key, val := range fields {
		if bytes.Equal(bytes.TrimSpace(val), []byte("null")) {
			return fmt.Errorf("error in %s: field %q must not be null", candidate, key)
		}
	}
	return nil
}

func assertSupportedProvider(provider, configPath string) error {
	if provider != "launchdarkly" {
		return fmt.Errorf(
			"error in %s: provider %q is not supported in this version. "+
				"Only \"launchdarkly\" is currently wired. Remove the provider field or set it to \"launchdarkly\"",
			configPath, provider,
		)
	}
	return nil
}
