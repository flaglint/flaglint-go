package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/flaglint/flaglint-go/internal/reporter"
	"github.com/flaglint/flaglint-go/internal/scanner"
)

func newScanCommand() *cobra.Command {
	var format, output, configPath string

	cmd := &cobra.Command{
		Use:   "scan [dir]",
		Short: "Scan a directory for LaunchDarkly Go SDK usage — a structured inventory, not a policy gate",
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

			result, err := scanner.Scan(dir, cfg)
			if err != nil {
				return internalError("scan failed: %v", err)
			}

			report, err := reporter.Render(result, reporter.Options{Format: reporter.Format(format), Title: cfg.ReportTitle})
			if err != nil {
				return invalidInput("%v", err)
			}

			fmt.Fprintf(cmd.ErrOrStderr(), "Scan complete — %d unique flag(s) across %d call site(s) (%s, %d file(s))\n",
				len(result.UniqueFlags), result.TotalUsages, formatDuration(result.ScanDurationMs), result.ScannedFiles)
			for _, w := range result.Warnings {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s: %s\n", w.Kind, w.File)
			}

			// scan is an inventory command — enforcement exit codes belong
			// only in `validate`. It always exits 0 unless there's a tool
			// error (returned above as an *ExitError).
			return writeReport(cmd, report, output)
		},
	}

	cmd.Flags().StringVarP(&format, "format", "f", "markdown", "output format: json | markdown")
	cmd.Flags().StringVarP(&output, "output", "o", "", "write report to file instead of stdout")
	cmd.Flags().StringVar(&configPath, "config", "", "path to config file")
	return cmd
}
