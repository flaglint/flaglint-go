package cli

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

// Execute runs the CLI and returns the process exit code — it never calls
// os.Exit itself, so callers (cmd/flaglint-go/main.go, and tests) control
// process termination.
func Execute(version string) int {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		os.Exit(ExitInterrupted)
	}()

	root := newRootCommand(version)
	root.SilenceErrors = true
	root.SilenceUsage = true

	err := root.Execute()
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
