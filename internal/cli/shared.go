package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/flaglint/flaglint-go/internal/config"
)

// validateDirectory checks that dir exists and is a directory, returning an
// *ExitError (exit 2) with a clear message otherwise. Centralized here from
// the start — flaglint-js had to retrofit this after the same check was
// copy-pasted across four commands.
//
// Deliberate deviation from flaglint-js's current shipped behavior: its
// validateDirectory (src/commands/shared.ts) actually calls process.exit(1)
// for both "not found" and "not a directory" — verified directly against
// the built binary, not just the source. That contradicts the documented
// exit-code contract (ADR 010) and reopens an exit-1 path for scan/audit,
// which are supposed to never produce it. Filed as
// github.com/flaglint/flaglint-js#209 rather than replicated here — this
// implements the documented contract (exit 2), not the current bug.
func validateDirectory(dir string) *ExitError {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return invalidInput("directory not found: %s", dir)
		}
		return invalidInput("cannot access %s: %v", dir, err)
	}
	if !info.IsDir() {
		return invalidInput("not a directory: %s", dir)
	}
	return nil
}

// loadConfig loads the config at configPath (or the default search order
// when configPath is ""), wrapping any error as an *ExitError (exit 2).
func loadConfig(configPath string) (config.Config, *ExitError) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return config.Config{}, invalidInput("%v", err)
	}
	return cfg, nil
}

// writeReport writes report to outputPath if non-empty, otherwise to the
// command's stdout. A file-write failure is an internal error (exit 3) —
// NOT matching flaglint-js's current shipped behavior, which calls
// process.exit(1) here (verified against source; same root-cause pattern
// as validateDirectory above, reported alongside it in
// github.com/flaglint/flaglint-js#209). This implements the documented
// contract rather than the bug.
func writeReport(cmd *cobra.Command, report, outputPath string) error {
	if outputPath == "" {
		fmt.Fprintln(cmd.OutOrStdout(), report)
		return nil
	}
	if err := os.WriteFile(outputPath, []byte(report+"\n"), 0o644); err != nil {
		return internalError("failed to write report to %s: %v", outputPath, err)
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Report written to %s\n", outputPath)
	return nil
}

// formatDuration renders a millisecond duration as a short human string
// (e.g. "42ms", "1.3s"), matching flaglint-js's terminal summary style.
func formatDuration(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000)
}
