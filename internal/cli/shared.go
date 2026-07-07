package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/flaglint/flaglint-go/internal/config"
	"github.com/flaglint/flaglint-go/internal/scanner"
	"github.com/flaglint/flaglint-go/internal/types"
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

// runScan calls scanner.Scan, or scanner.ScanStrict when strictTypes is
// set — the single place all three commands (scan/audit/validate) decide
// which to run, so the --strict-types flag behaves identically everywhere
// it's exposed. See docs/adr/005-strict-types-pass.md.
func runScan(dir string, cfg config.Config, strictTypes bool) (types.ScanResult, error) {
	if strictTypes {
		return scanner.ScanStrict(dir, cfg)
	}
	return scanner.Scan(dir, cfg)
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
		// The report is the command's actual output, not a discardable
		// progress line — unlike stderrInfo below, a real write failure
		// here (stdout closed, broken pipe) is worth surfacing rather than
		// silently swallowing, even though there's little a CLI can
		// usefully do about a broken stdout beyond reporting it.
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), report); err != nil {
			return internalError("failed to write report to stdout: %v", err)
		}
		return nil
	}
	if err := os.WriteFile(outputPath, []byte(report+"\n"), 0o644); err != nil {
		return internalError("failed to write report to %s: %v", outputPath, err)
	}
	stderrInfo(cmd, "Report written to %s\n", outputPath)
	return nil
}

// printWarning writes one scan warning to cmd's stderr, appending w's
// Reason (only ever set for a "typecheck-failure" warning — see
// types.ScanWarning) when present — without it, a --strict-types package
// load failure would print only a bare package path with no clue why it
// failed.
func printWarning(cmd *cobra.Command, w types.ScanWarning) {
	if w.Reason != "" {
		stderrInfo(cmd, "warning: %s: %s: %s\n", w.Kind, w.File, w.Reason)
		return
	}
	stderrInfo(cmd, "warning: %s: %s\n", w.Kind, w.File)
}

// stderrInfo writes a formatted progress/summary line to cmd's stderr,
// mirroring flaglint-js's own stderrInfo helper (src/commands/shared.ts).
// The write error is deliberately discarded: a progress line failing to
// reach a closed or broken stderr isn't something any command can
// meaningfully react to, unlike writeReport's actual output above.
func stderrInfo(cmd *cobra.Command, format string, args ...any) {
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), format, args...)
}

// formatDuration renders a millisecond duration as a short human string
// (e.g. "42ms", "1.3s"), matching flaglint-js's terminal summary style.
func formatDuration(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000)
}
