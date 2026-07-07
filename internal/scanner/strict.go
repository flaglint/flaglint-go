package scanner

import (
	"go/token"
	"path/filepath"
	"sort"

	"golang.org/x/tools/go/packages"

	"github.com/flaglint/flaglint-go/internal/config"
	"github.com/flaglint/flaglint-go/internal/typecheck"
	"github.com/flaglint/flaglint-go/internal/types"
)

// ScanStrict runs Scan (Phase 1, entirely unchanged) and then augments its
// result with additional findings only provable with real go/types
// information — see docs/adr/005-strict-types-pass.md. This is strictly
// additive: every Phase 1 finding is preserved exactly as Scan produced
// it; this pass can only add findings Phase 1's syntactic tracing
// structurally could not prove (today: interface satisfaction, issue #15).
//
// Loading root as a Go module can fail, or fail per-package (a mid-
// refactor branch, a partial checkout, broken dependencies) — this never
// fails the scan outright. A total load failure or a per-package failure
// is recorded as a "typecheck-failure" warning, and Phase 1's result is
// returned with whatever additional findings the packages that *did*
// type-check made possible (zero, if none did).
func ScanStrict(root string, cfg config.Config) (types.ScanResult, error) {
	result, err := Scan(root, cfg)
	if err != nil {
		return types.ScanResult{}, err
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return types.ScanResult{}, err
	}

	pkgs, failures, err := typecheck.Load(absRoot)
	if err != nil {
		result.Warnings = append(result.Warnings, types.ScanWarning{
			Kind:   "typecheck-failure",
			File:   root,
			Reason: err.Error(),
		})
		return result, nil
	}
	for _, f := range failures {
		result.Warnings = append(result.Warnings, types.ScanWarning{
			Kind:   "typecheck-failure",
			File:   f.PkgPath,
			Reason: f.Reason,
		})
	}

	mergeStrictTypesUsages(&result, strictTypesUsages(pkgs, absRoot))
	return result, nil
}

// strictTypesUsages re-runs the whole-scan analysis over go/packages-loaded
// ASTs, which carry real go/types information (Scan's own ASTs, parsed
// independently via go/parser, never do) — this is what lets
// resolveAssignedBinding's static-type fallback (identity.go) fire.
// Re-running the *full* whole-scan analysis, rather than a bespoke walk
// narrowly targeting interface satisfaction, means every existing Phase 1
// detection mechanism gains type-backed coverage uniformly for free — not
// just the one pattern this pass was originally written for.
func strictTypesUsages(pkgs []*packages.Package, absRoot string) []types.FlagUsage {
	var fset *token.FileSet
	var parsed []parsedFile
	for _, pkg := range pkgs {
		if pkg.Fset == nil || pkg.TypesInfo == nil {
			continue
		}
		if fset == nil {
			fset = pkg.Fset
		}
		for _, file := range pkg.Syntax {
			pos := pkg.Fset.Position(file.Pos())
			if pos.Filename == "" {
				continue
			}
			rel, err := filepath.Rel(absRoot, pos.Filename)
			if err != nil {
				continue
			}
			parsed = append(parsed, parsedFile{
				relPath:   rel,
				dir:       filepath.Dir(pos.Filename),
				file:      file,
				imports:   traceSDKImports(file),
				typesInfo: pkg.TypesInfo,
			})
		}
	}
	if len(parsed) == 0 {
		return nil
	}
	sort.Slice(parsed, func(i, j int) bool { return parsed[i].relPath < parsed[j].relPath })
	return runWholeScanAnalysis(fset, parsed)
}

// mergeStrictTypesUsages folds extra into result in place, keeping every
// existing entry byte-for-byte unchanged and adding only fingerprints
// result doesn't already have — the additive guarantee ADR 005 promises,
// enforced here rather than assumed from strictTypesUsages happening to be
// a superset of Scan's own result for the files it covers.
func mergeStrictTypesUsages(result *types.ScanResult, extra []types.FlagUsage) {
	if len(extra) == 0 {
		return
	}
	seen := make(map[string]bool, len(result.Usages))
	for _, u := range result.Usages {
		seen[u.Fingerprint] = true
	}
	added := false
	for _, u := range extra {
		if seen[u.Fingerprint] {
			continue
		}
		seen[u.Fingerprint] = true
		// Only stamped on entries that actually survive the dedup above —
		// an entry strictTypesUsages also happened to (re-)find that Phase
		// 1 already reported keeps its original DetectedBy ("", meaning
		// Phase 1) unchanged, since it's Phase 1's own copy of that finding
		// that's kept, not this re-scanned one.
		u.DetectedBy = "strict-types"
		result.Usages = append(result.Usages, u)
		added = true
	}
	if !added {
		return
	}
	sort.Slice(result.Usages, func(i, j int) bool {
		a, b := result.Usages[i], result.Usages[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Column < b.Column
	})
	result.TotalUsages = len(result.Usages)
	result.UniqueFlags = uniqueFlags(result.Usages)
}
