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

	// Initialized to empty (not nil) slices: encoding/json marshals a nil
	// slice as `null`, but flaglint-js's array operations never produce
	// null — a clean scan must report `"usages": []`, not `"usages": null`,
	// to match the cross-tool JSON contract (ADR 003) and not break a
	// consumer doing `result.usages.map(...)` or `jq '.usages[]'`.
	allUsages := []types.FlagUsage{}
	warnings := []types.ScanWarning{}

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
//
// Bindings are scoped per top-level function/method/closure (see
// collectScopedBindings) rather than tracked in one flat file-wide map: a
// same-named variable bound to an unrelated value in a different function
// must never be mistaken for this scope's client. dynamicIndex is still a
// single counter for the whole file, matching flaglint-js's per-file
// (not per-function) numbering.
//
// Known Phase 1 gap: a method value taken from a bound client
// (`f := client.BoolVariation; f(...)`) is not detected — detection
// requires the method to be called directly through a selector expression
// at the call site itself. This is the same class of indirection ADR 002
// already defers to the opt-in --strict-types pass, not a new exception.
func scanFile(fset *token.FileSet, file *ast.File, relPath string) []types.FlagUsage {
	imports := traceSDKImports(file)
	if !imports.present() {
		return nil
	}

	// base holds bindings visible to every scope in the file: package-level
	// `var` bindings and struct-field bindings (both are not function-scoped
	// in real Go semantics — see collectFieldBindings). Local variable
	// bindings are layered on top of this per function/closure scope.
	base := mergeBindings(
		collectPackageLevelBindings(file, imports),
		collectFieldBindings(file, imports),
	)

	d := &fileDetector{fset: fset, relPath: relPath}

	for _, decl := range file.Decls {
		switch n := decl.(type) {
		case *ast.FuncDecl:
			if n.Body == nil {
				continue // external/assembly function, no body to scan
			}
			d.detect(n.Body, collectScopedBindings(n.Body, base, imports), declaredParamTypes(n))
		case *ast.GenDecl:
			if n.Tok != token.VAR {
				continue
			}
			for _, spec := range n.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, val := range vs.Values {
					// Not just a direct `var x = func(){...}` — also catches
					// an immediately-invoked func literal (`var x =
					// func(){...}()`) or one passed as an argument, by
					// finding every FuncLit nested anywhere in the
					// initializer expression rather than requiring it to
					// be the initializer itself.
					ast.Inspect(val, func(inner ast.Node) bool {
						lit, ok := inner.(*ast.FuncLit)
						if !ok {
							return true
						}
						d.detect(lit.Body, collectScopedBindings(lit.Body, base, imports), paramTypesFromFuncLit(lit))
						return true
					})
				}
			}
		}
	}

	return d.usages
}

// fileDetector accumulates findings across every scope walked in one file,
// sharing a single dynamicIndex counter (per-file, not per-scope) to match
// flaglint-js's numbering.
type fileDetector struct {
	fset         *token.FileSet
	relPath      string
	dynamicIndex int
	usages       []types.FlagUsage
}

func (d *fileDetector) detect(scope ast.Node, bindings map[string]string, declared map[string]string) {
	ast.Inspect(scope, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := unparen(call.Fun).(*ast.SelectorExpr)
		if !ok {
			return true
		}
		receiver := resolveReceiver(sel.X, declared)
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

		pos := d.fset.Position(call.Pos())
		callType := types.CallType(sel.Sel.Name)
		sdk := sdkName(version)

		if spec.keyArgIndex == -1 {
			d.usages = append(d.usages, types.FlagUsage{
				FlagKey:          "*",
				IsDynamic:        false,
				File:             d.relPath,
				Line:             pos.Line,
				Column:           pos.Column,
				CallType:         callType,
				Fingerprint:      fingerprint.Generate("*", callType, d.relPath, nil),
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
			idx := d.dynamicIndex
			d.dynamicIndex++
			dynIdx = &idx
		}

		d.usages = append(d.usages, types.FlagUsage{
			FlagKey:          flagKey,
			IsDynamic:        isDynamic,
			File:             d.relPath,
			Line:             pos.Line,
			Column:           pos.Column,
			CallType:         callType,
			Fingerprint:      fingerprint.Generate(flagKey, callType, d.relPath, dynIdx),
			StalenessSignals: []types.StalenessSignal{},
			Language:         "go",
			SDK:              sdk,
			Risk:             riskFor(spec, isDynamic),
		})
		return true
	})
}

// uniqueFlags mirrors flaglint-js's uniqueFlags semantics: dynamic and
// bulk ("*") keys are excluded, and the result is sorted for deterministic
// output.
func uniqueFlags(usages []types.FlagUsage) []string {
	seen := map[string]bool{}
	keys := []string{} // never nil — see the allUsages/warnings comment in Scan above
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
