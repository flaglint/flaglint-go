// Package validator implements policy enforcement over a scan result. This
// is the only flaglint-go command that ever produces exit code 1 — see
// docs/adr/003-cross-tool-contract.md.
package validator

import (
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/flaglint/flaglint-go/internal/types"
)

// Violation is one call site that violated an active policy rule.
type Violation struct {
	File      string         `json:"file"`
	Line      int            `json:"line"`
	Column    int            `json:"column"`
	CallType  types.CallType `json:"callType"`
	FlagKey   string         `json:"flagKey"`
	IsDynamic bool           `json:"isDynamic"`
}

// Result is the outcome of validating a scan. Passed maps directly to the
// CLI's exit code: true -> 0, false -> 1.
type Result struct {
	ScannedFiles int         `json:"scannedFiles"`
	TotalUsages  int         `json:"totalUsages"`
	Violations   []Violation `json:"violations"`
	Passed       bool        `json:"passed"`
}

// Options controls which policy rules are active.
type Options struct {
	// NoDirectLaunchDarkly, when true, makes every detected LaunchDarkly Go
	// SDK call site a violation. When false, Validate never fails — it only
	// reports. This mirrors flaglint-js's --no-direct-launchdarkly flag
	// exactly (same name, same opt-in-to-enforce semantics) even though the
	// double negative reads oddly in isolation — see ADR 003.
	NoDirectLaunchDarkly bool
	// BootstrapExclude is a set of glob patterns (relative to the scan
	// root) for files legitimately allowed to use the SDK directly — e.g.
	// an OpenFeature provider bootstrap file. Usages in matching files are
	// excluded from violations.
	BootstrapExclude []string
}

// Validate evaluates result against opts. The only rule implemented today
// is NoDirectLaunchDarkly — since Go Phase 1 has no OpenFeature-equivalent
// detection to distinguish from, every finding is, by construction, a
// direct LaunchDarkly SDK call.
func Validate(result types.ScanResult, opts Options) Result {
	violations := []Violation{}

	if opts.NoDirectLaunchDarkly {
		for _, u := range result.Usages {
			if matchesBootstrapPattern(u.File, opts.BootstrapExclude) {
				continue
			}
			violations = append(violations, Violation{
				File:      u.File,
				Line:      u.Line,
				Column:    u.Column,
				CallType:  u.CallType,
				FlagKey:   u.FlagKey,
				IsDynamic: u.IsDynamic,
			})
		}
	}

	return Result{
		ScannedFiles: result.ScannedFiles,
		TotalUsages:  result.TotalUsages,
		Violations:   violations,
		Passed:       len(violations) == 0,
	}
}

func matchesBootstrapPattern(file string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	clean := strings.TrimPrefix(file, "./")
	for _, p := range patterns {
		if ok, _ := doublestar.Match(strings.TrimPrefix(p, "./"), clean); ok {
			return true
		}
	}
	return false
}
