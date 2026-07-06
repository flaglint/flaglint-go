// Package config loads flaglint.config.json (or .flaglintrc[.json]),
// matching flaglint-js's search order and field names — see
// docs/adr/003-cross-tool-contract.md. Only the fields flaglint-go's Go
// scanner actually uses are implemented; flaglint-js-only fields
// (minFileCount, wrappers, openFeatureClientBindings) are intentionally
// omitted rather than stubbed, since they have no Go-side behavior yet.
package config

import (
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
		raw, err := os.ReadFile(full)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return Config{}, fmt.Errorf("error reading %s: %w", candidate, err)
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
