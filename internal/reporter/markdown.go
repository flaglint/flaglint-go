package reporter

import (
	"fmt"
	"sort"
	"strings"

	"github.com/flaglint/flaglint-go/internal/types"
)

func formatMarkdown(result types.ScanResult, opts Options) string {
	title := opts.Title
	if title == "" {
		title = "FlagLint Go Scan Report"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "Scanned %d file(s) in %dms — %d unique flag(s) across %d call site(s).\n\n",
		result.ScannedFiles, result.ScanDurationMs, len(result.UniqueFlags), result.TotalUsages)

	staticUsages := make([]types.FlagUsage, 0, len(result.Usages))
	dynamicUsages := make([]types.FlagUsage, 0)
	bulkUsages := make([]types.FlagUsage, 0)
	for _, u := range result.Usages {
		switch {
		case u.IsDynamic:
			dynamicUsages = append(dynamicUsages, u)
		case u.FlagKey == "*":
			bulkUsages = append(bulkUsages, u)
		default:
			staticUsages = append(staticUsages, u)
		}
	}

	if len(staticUsages) > 0 {
		b.WriteString("## Flags\n\n")
		b.WriteString("| Flag | Call Type | Risk | File |\n")
		b.WriteString("|---|---|---|---|\n")
		sorted := append([]types.FlagUsage(nil), staticUsages...)
		sort.SliceStable(sorted, func(i, j int) bool { return riskRank(sorted[i].Risk) > riskRank(sorted[j].Risk) })
		for _, u := range sorted {
			fmt.Fprintf(&b, "| `%s` | %s | %s | %s:%d |\n", esc(u.FlagKey), u.CallType, u.Risk, esc(u.File), u.Line)
		}
		b.WriteString("\n")
	}

	if len(bulkUsages) > 0 {
		b.WriteString("## Bulk Evaluation Calls (Manual Review Required)\n\n")
		b.WriteString("Calls with no single flag key — no direct OpenFeature equivalent:\n\n")
		for _, u := range bulkUsages {
			fmt.Fprintf(&b, "- `%s` at %s:%d\n", u.CallType, esc(u.File), u.Line)
		}
		b.WriteString("\n")
	}

	if len(dynamicUsages) > 0 {
		b.WriteString("## Dynamic Flag Keys (Manual Review Required)\n\n")
		b.WriteString("Calls whose flag key could not be resolved statically:\n\n")
		for _, u := range dynamicUsages {
			fmt.Fprintf(&b, "- `%s` at %s:%d — key determined at runtime\n", u.CallType, esc(u.File), u.Line)
		}
		b.WriteString("\n")
	}

	if len(result.Warnings) > 0 {
		b.WriteString("## Warnings\n\n")
		for _, w := range result.Warnings {
			if w.FsCode != "" {
				fmt.Fprintf(&b, "- %s: %s (%s)\n", w.Kind, esc(w.File), w.FsCode)
			} else {
				fmt.Fprintf(&b, "- %s: %s\n", w.Kind, esc(w.File))
			}
		}
		b.WriteString("\n")
	}

	if result.TotalUsages == 0 {
		b.WriteString("No LaunchDarkly Go SDK usage detected.\n")
	}

	return strings.TrimRight(b.String(), "\n") + "\n"
}

func riskRank(r types.Risk) int {
	switch r {
	case types.RiskHigh:
		return 2
	case types.RiskMedium:
		return 1
	default:
		return 0
	}
}

func esc(s string) string {
	replacer := strings.NewReplacer("|", "\\|", "`", "\\`")
	return replacer.Replace(s)
}
