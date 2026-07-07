package cli

import (
	"github.com/spf13/cobra"

	"github.com/flaglint/flaglint-go/internal/baseline"
	"github.com/flaglint/flaglint-go/internal/readiness"
	"github.com/flaglint/flaglint-go/internal/reporter"
)

func newAuditCommand(version string) *cobra.Command {
	var format, output, configPath, writeBaselinePath string
	var strictTypes bool

	cmd := &cobra.Command{
		Use:   "audit [dir]",
		Short: "Scan and report migration readiness for LaunchDarkly Go SDK usage",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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

			result, err := runScan(dir, cfg, strictTypes)
			if err != nil {
				return internalError("scan failed: %v", err)
			}

			report, err := reporter.Render(result, reporter.Options{Format: reporter.Format(format), Title: cfg.ReportTitle})
			if err != nil {
				return invalidInput("%v", err)
			}

			r := readiness.Compute(result.Usages)
			stderrInfo(cmd, "Scan complete — %d unique flag(s) across %d call site(s) (%s, %d file(s))\n",
				len(result.UniqueFlags), result.TotalUsages, formatDuration(result.ScanDurationMs), result.ScannedFiles)
			if r.Grade == readiness.GradeNotApplicable {
				stderrInfo(cmd, "Migration readiness: N/A — no direct LaunchDarkly Go SDK calls detected.\n")
			} else {
				stderrInfo(cmd, "Migration readiness: %d/100 · %s\n", *r.Score, r.Grade)
				stderrInfo(cmd, "  %d low risk · %d medium risk · %d high risk\n",
					r.LowRiskCalls, r.MediumRiskCalls, r.HighRiskCalls)
			}
			for _, w := range result.Warnings {
				printWarning(cmd, w)
			}

			if writeBaselinePath != "" {
				fingerprints := make([]string, 0, len(result.Usages))
				for _, u := range result.Usages {
					if u.Fingerprint != "" {
						fingerprints = append(fingerprints, u.Fingerprint)
					}
				}
				// invalidInput (exit 2), even for an OS-level write failure
				// (disk full, permission denied), not internalError (exit
				// 3) — this looks arguably wrong in isolation (exit 3 reads
				// like the better fit for an unexpected I/O failure), but
				// it's deliberate parity with flaglint-js's BaselineError,
				// whose default exitCode is 2 for every failure mode
				// including write failures (src/baseline.ts's writeBaseline
				// throws BaselineError with no explicit exitCode override).
				if err := baseline.Write(writeBaselinePath, fingerprints, version); err != nil {
					return invalidInput("%v", err)
				}
				uniqueCount := len(uniqueStrings(fingerprints))
				stderrInfo(cmd, "✓ Baseline written to %s (%d fingerprints)\n", writeBaselinePath, uniqueCount)
			}

			// audit, like scan, is an inventory/reporting command — always
			// exits 0 unless there's a tool error.
			return writeReport(cmd, report, output)
		},
	}

	cmd.Flags().StringVarP(&format, "format", "f", "markdown", "output format: json | markdown")
	cmd.Flags().StringVarP(&output, "output", "o", "", "write report to file instead of stdout")
	cmd.Flags().StringVar(&configPath, "config", "", "path to config file")
	cmd.Flags().StringVar(&writeBaselinePath, "write-baseline", "", "write current finding fingerprints to a baseline file")
	cmd.Flags().BoolVar(&strictTypes, "strict-types", false, "additionally resolve findings only provable with real go/types information (requires the module to build; see docs/adr/005-strict-types-pass.md)")
	return cmd
}

func uniqueStrings(s []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(s))
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
