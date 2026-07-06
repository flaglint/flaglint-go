// Package reporter formats a scan result as JSON or Markdown. Field names
// and structure match flaglint-js's reporter contract — see
// docs/adr/003-cross-tool-contract.md.
package reporter

import (
	"fmt"

	"github.com/flaglint/flaglint-go/internal/types"
)

type Format string

const (
	FormatJSON     Format = "json"
	FormatMarkdown Format = "markdown"
)

type Options struct {
	Format Format
	Title  string
}

// Format renders result according to opts.Format. Returns an error for an
// unrecognized format — callers should treat this as an exit-2 condition
// (invalid user input), matching flaglint-js's --format validation.
func Render(result types.ScanResult, opts Options) (string, error) {
	switch opts.Format {
	case FormatJSON:
		return formatJSON(result), nil
	case FormatMarkdown:
		return formatMarkdown(result, opts), nil
	default:
		return "", fmt.Errorf("unsupported format %q: must be one of json, markdown", opts.Format)
	}
}
