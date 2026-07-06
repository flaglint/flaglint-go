// Package cli wires flaglint-go's cobra commands together. Exit codes match
// flaglint-js exactly — see docs/adr/003-cross-tool-contract.md.
package cli

import "fmt"

// Exit code contract, shared across every command:
//
//	0   success, no violations
//	1   policy/staleness failure (validate only — audit/scan never use this)
//	2   invalid user input (bad --format, missing/malformed config, bad flags)
//	3   internal/unexpected error
//	130 SIGINT
const (
	ExitSuccess       = 0
	ExitPolicyFailure = 1
	ExitInvalidInput  = 2
	ExitInternalError = 3
	ExitInterrupted   = 130
)

// ExitError carries the exit code a command should terminate with. Command
// RunE functions return this (via cobra's SilenceErrors/SilenceUsage, see
// root.go) instead of calling os.Exit directly, so exit-code selection stays
// in one place (Execute in root.go).
type ExitError struct {
	Code    int
	Message string
}

func (e *ExitError) Error() string { return e.Message }

func invalidInput(format string, args ...any) *ExitError {
	return &ExitError{Code: ExitInvalidInput, Message: fmt.Sprintf(format, args...)}
}

func internalError(format string, args ...any) *ExitError {
	return &ExitError{Code: ExitInternalError, Message: fmt.Sprintf(format, args...)}
}
