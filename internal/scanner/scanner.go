// Package scanner detects github.com/launchdarkly/go-server-sdk (v6/v7)
// call sites in Go source. Client identity is proven through import-alias
// tracing and constructor-call binding — never through name matching alone.
// See docs/adr/002-client-identity-model.md.
package scanner

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/flaglint/flaglint-go/internal/config"
	"github.com/flaglint/flaglint-go/internal/fingerprint"
	"github.com/flaglint/flaglint-go/internal/types"
)

// Scan walks root for files matching cfg's include/exclude patterns, parses
// each as Go source, and returns every detected LaunchDarkly Go SDK call
// site. File read/parse failures are recorded as warnings rather than
// aborting the scan.
func Scan(root string, cfg config.Config) (types.ScanResult, error) {
	start := time.Now()

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return types.ScanResult{}, err
	}

	relFiles, err := discoverFiles(absRoot, cfg.Include, cfg.Exclude)
	if err != nil {
		return types.ScanResult{}, err
	}
	sort.Strings(relFiles) // deterministic output order, independent of filesystem walk order

	var (
		allUsages []types.FlagUsage
		warnings  []types.ScanWarning
	)

	for _, rel := range relFiles {
		full := filepath.Join(absRoot, rel)
		src, err := os.ReadFile(full)
		if err != nil {
			warnings = append(warnings, types.ScanWarning{
				Kind: "read-failure", File: rel, FsCode: fsCode(err),
			})
			continue
		}

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, full, src, parser.SkipObjectResolution)
		if err != nil {
			warnings = append(warnings, types.ScanWarning{Kind: "parse-failure", File: rel})
			continue
		}

		allUsages = append(allUsages, scanFile(fset, file, rel)...)
	}

	return types.ScanResult{
		ScannedAt:      time.Now().UTC().Format(time.RFC3339),
		ScanRoot:       absRoot,
		ScannedFiles:   len(relFiles),
		TotalUsages:    len(allUsages),
		UniqueFlags:    uniqueFlags(allUsages),
		Usages:         allUsages,
		ScanDurationMs: time.Since(start).Milliseconds(),
		Warnings:       warnings,
	}, nil
}

// scanFile detects LaunchDarkly SDK call sites in a single parsed file.
// relPath is the file's path relative to the scan root (never to a go.mod
// boundary) — see docs/adr/003-cross-tool-contract.md for why: module-
// relative paths would collide across sibling modules with identically
// named files, corrupting fingerprints.
func scanFile(fset *token.FileSet, file *ast.File, relPath string) []types.FlagUsage {
	imports := traceSDKImports(file)
	if !imports.present() {
		return nil
	}

	bindings := collectClientBindings(file, imports)
	if len(bindings) == 0 {
		return nil
	}

	var usages []types.FlagUsage
	dynamicIndex := 0

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		receiver := exprString(sel.X)
		if receiver == "" {
			return true
		}
		version, bound := bindings[receiver]
		if !bound {
			return true
		}
		spec, known := methodSpecs[sel.Sel.Name]
		if !known {
			return true
		}

		pos := fset.Position(call.Pos())
		callType := types.CallType(sel.Sel.Name)
		sdk := sdkName(version)

		if spec.keyArgIndex == -1 {
			usages = append(usages, types.FlagUsage{
				FlagKey:          "*",
				IsDynamic:        false,
				File:             relPath,
				Line:             pos.Line,
				Column:           pos.Column,
				CallType:         callType,
				Fingerprint:      fingerprint.Generate("*", callType, relPath, nil),
				StalenessSignals: []types.StalenessSignal{},
				Language:         "go",
				SDK:              sdk,
				Risk:             spec.risk,
			})
			return true
		}

		var keyArg ast.Expr
		if spec.keyArgIndex < len(call.Args) {
			keyArg = call.Args[spec.keyArgIndex]
		}
		flagKey, isDynamic := extractFlagKey(keyArg)

		var dynIdx *int
		if isDynamic {
			idx := dynamicIndex
			dynamicIndex++
			dynIdx = &idx
		}

		usages = append(usages, types.FlagUsage{
			FlagKey:          flagKey,
			IsDynamic:        isDynamic,
			File:             relPath,
			Line:             pos.Line,
			Column:           pos.Column,
			CallType:         callType,
			Fingerprint:      fingerprint.Generate(flagKey, callType, relPath, dynIdx),
			StalenessSignals: []types.StalenessSignal{},
			Language:         "go",
			SDK:              sdk,
			Risk:             spec.risk,
		})
		return true
	})

	return usages
}

// uniqueFlags mirrors flaglint-js's uniqueFlags semantics: dynamic and
// bulk ("*") keys are excluded, and the result is sorted for deterministic
// output.
func uniqueFlags(usages []types.FlagUsage) []string {
	seen := map[string]bool{}
	var keys []string
	for _, u := range usages {
		if u.IsDynamic || u.FlagKey == "*" {
			continue
		}
		if !seen[u.FlagKey] {
			seen[u.FlagKey] = true
			keys = append(keys, u.FlagKey)
		}
	}
	sort.Strings(keys)
	return keys
}

func fsCode(err error) string {
	switch {
	case os.IsNotExist(err):
		return "ENOENT"
	case os.IsPermission(err):
		return "EACCES"
	default:
		return "EUNKNOWN"
	}
}
