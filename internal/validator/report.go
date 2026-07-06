package validator

import (
	"fmt"
	"strings"
)

func violationLabel(v Violation) string {
	switch {
	case v.IsDynamic:
		return fmt.Sprintf("%s(\"(dynamic key)\")", v.CallType)
	case v.FlagKey == "*":
		return fmt.Sprintf("%s(bulk inventory)", v.CallType)
	default:
		return fmt.Sprintf("%s(%q)", v.CallType, v.FlagKey)
	}
}

// FormatReport renders result as a human-readable string for stdout.
// Never mentions staleness or "safe to delete" — this command enforces one
// thing (direct LaunchDarkly SDK usage), nothing more.
func FormatReport(result Result, opts Options) string {
	var b strings.Builder

	if !opts.NoDirectLaunchDarkly {
		fmt.Fprintf(&b, "Scanned %d file(s). Found %d LaunchDarkly Go SDK usage(s).\n", result.ScannedFiles, result.TotalUsages)
		return b.String()
	}

	if result.Passed {
		b.WriteString("✓ validate --no-direct-launchdarkly: no direct LaunchDarkly Go SDK calls found.\n")
		fmt.Fprintf(&b, "  Scanned %d file(s).\n", result.ScannedFiles)
		return b.String()
	}

	fmt.Fprintf(&b, "✗ validate --no-direct-launchdarkly: %d direct LaunchDarkly Go SDK call(s) found.\n\n", len(result.Violations))
	for _, v := range result.Violations {
		fmt.Fprintf(&b, "  %s:%d:%d — %s\n", v.File, v.Line, v.Column, violationLabel(v))
	}
	b.WriteString("\nThese call sites must migrate to OpenFeature before this rule passes.\n")
	return b.String()
}
