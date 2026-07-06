package cli

import (
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
)

// Execute runs the CLI and returns the process exit code — it never calls
// os.Exit itself, so the sole os.Exit call in the whole program is
// cmd/flaglint-go/main.go's `os.Exit(cli.Execute(version))`. That means a
// signal arriving right as a command finishes can never race against a
// second, independent os.Exit call: whichever of the select's two cases
// fires first is the only exit code ever returned.
//
// Only SIGINT maps to 130 — that's the only signal ADR 003's exit-code
// contract defines. SIGTERM is deliberately left to Go's default
// disposition (process termination) rather than folded into the same code,
// since a graceful-shutdown SIGTERM and an interactive Ctrl-C are not the
// same event and callers may reasonably want to distinguish them.
func Execute(version string) int {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	root := newRootCommand(version)
	root.SilenceErrors = true
	root.SilenceUsage = true

	done := make(chan error, 1)
	go func() { done <- root.Execute() }()

	select {
	case <-sigCh:
		return ExitInterrupted
	case err := <-done:
		return classifyResult(err)
	}
}

func classifyResult(err error) int {
	if err == nil {
		return ExitSuccess
	}
	if exitErr, ok := err.(*ExitError); ok {
		fmt.Fprintf(os.Stderr, "Error: %s\n", exitErr.Message)
		return exitErr.Code
	}
	// Any error not already classified as an *ExitError (a cobra flag-
	// parsing error, for instance) is treated as invalid input, not an
	// internal error — it means the user's invocation was malformed.
	fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
	return ExitInvalidInput
}

func newRootCommand(version string) *cobra.Command {
	root := &cobra.Command{
		Use:     "flaglint-go",
		Short:   "Audit LaunchDarkly Go SDK usage — a native Go counterpart to flaglint-js",
		Version: version,
	}
	root.AddCommand(newAuditCommand())
	root.AddCommand(newScanCommand())
	return root
}
