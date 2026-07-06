package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/flaglint/flaglint-go/internal/baseline"
	"github.com/flaglint/flaglint-go/internal/scanner"
	"github.com/flaglint/flaglint-go/internal/types"
	"github.com/flaglint/flaglint-go/internal/validator"
)

func newValidateCommand() *cobra.Command {
	var (
		noDirectLD       bool
		bootstrapExclude []string
		format           string
		output           string
		configPath       string
		baselinePath     string
		failOnNew        bool
	)

	cmd := &cobra.Command{
		Use:   "validate [dir]",
		Short: "Validate that your codebase complies with feature flag usage policies",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if format != "text" && format != "sarif" {
				return invalidInput("invalid format %q: must be one of text, sarif", format)
			}

			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			if exitErr := validateDirectory(dir); exitErr != nil {
				return exitErr
			}
			cfg, exitErr := loadConfig(configPath)
			if exitErr != nil {
				return exitErr
			}

			result, err := scanner.Scan(dir, cfg)
			if err != nil {
				return internalError("scan failed: %v", err)
			}
			for _, w := range result.Warnings {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s: %s\n", w.Kind, w.File)
			}

			vOpts := validator.Options{NoDirectLaunchDarkly: noDirectLD, BootstrapExclude: bootstrapExclude}
			vResult := validator.Validate(result, vOpts)

			var report string
			if format == "sarif" {
				report = validator.FormatSARIF(vResult, result.ScanRoot, result.ScannedAt)
			} else {
				report = validator.FormatReport(vResult, vOpts)
			}
			// The report is written BEFORE the baseline check below runs —
			// so a report claiming the --no-direct-launchdarkly policy
			// passed can still be followed by an exit-1/exit-2 from a
			// baseline failure. This looks like a bug in isolation, but
			// it's deliberate parity with flaglint-js's validate.ts, which
			// has the identical ordering (write report -> baseline check ->
			// final exit decision). Reordering here would fix the Go
			// side's UX rough edge at the cost of making the two tools'
			// documented behavior diverge — worse for ADR 003's whole
			// purpose. If this changes, it should change in both repos.
			if writeErr := writeReport(cmd, strings.TrimRight(report, "\n"), output); writeErr != nil {
				return writeErr
			}

			// Baseline comparison runs independently of the policy check
			// above — a project can adopt --baseline/--fail-on-new without
			// ever turning on --no-direct-launchdarkly.
			if baselinePath != "" {
				if exitErr := checkBaseline(cmd, result, baselinePath, failOnNew); exitErr != nil {
					return exitErr
				}
			}

			if !vResult.Passed {
				return &ExitError{Code: ExitPolicyFailure, Message: fmt.Sprintf("%d violation(s) found", len(vResult.Violations))}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&noDirectLD, "no-direct-launchdarkly", false, "fail if any direct LaunchDarkly Go SDK evaluation calls are found")
	cmd.Flags().StringArrayVar(&bootstrapExclude, "bootstrap-exclude", nil, "glob pattern for files allowed to use the LaunchDarkly SDK directly (repeatable)")
	cmd.Flags().StringVarP(&format, "format", "f", "text", "output format: text | sarif")
	cmd.Flags().StringVarP(&output, "output", "o", "", "write report to file instead of stdout")
	cmd.Flags().StringVar(&configPath, "config", "", "path to config file")
	cmd.Flags().StringVar(&baselinePath, "baseline", "", "baseline file for comparing against known debt")
	cmd.Flags().BoolVar(&failOnNew, "fail-on-new", false, "exit 1 if any findings are not in the baseline")
	return cmd
}

func checkBaseline(cmd *cobra.Command, result types.ScanResult, baselinePath string, failOnNew bool) *ExitError {
	known, err := baseline.Read(baselinePath)
	if err != nil {
		return invalidInput("%v", err)
	}

	fingerprints := make([]string, 0, len(result.Usages))
	for _, u := range result.Usages {
		if u.Fingerprint != "" {
			fingerprints = append(fingerprints, u.Fingerprint)
		}
	}
	newFindings := baseline.New(fingerprints, known)

	if !failOnNew {
		if len(newFindings) > 0 {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d finding(s) not in baseline (use --fail-on-new to fail CI)\n", len(newFindings))
		}
		return nil
	}

	if len(newFindings) == 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), "✓ No new findings beyond baseline")
		return nil
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "Error: %d new finding(s) not in baseline:\n", len(newFindings))
	for _, fp := range newFindings {
		fmt.Fprintf(cmd.ErrOrStderr(), "  - %s\n", fp)
	}
	return &ExitError{Code: ExitPolicyFailure, Message: fmt.Sprintf("%d new finding(s) not in baseline", len(newFindings))}
}
