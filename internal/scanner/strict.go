package scanner

import (
	"go/token"
	gotypes "go/types"
	"os"
	"path/filepath"
	"sort"
	"strconv"

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

	strictUsages, strictMigrationItems := strictTypesUsages(pkgs, absRoot)
	mergeStrictTypesUsages(&result, strictUsages, strictMigrationItems)
	// forwardingCallUsages' call sites don't directly show the LD method's
	// own (key, context, fallback) arguments (see MigrationInventory's doc
	// comment, types.go) — no migration items to merge for these.
	mergeStrictTypesUsages(&result, forwardingCallUsages(pkgs, absRoot), nil)
	return result, nil
}

// forwardingCallUsages is issue #26's detection entry point (ADR 006,
// extended — see forwarding.go's package doc comment for why): it builds
// the whole-scan indices (accessor methods, factory-constructor field
// sources, package-level flag-descriptor var literals, and the fixed-
// point "which functions evaluate a flag through a forwarded/pass-through
// callback" index) across every loaded package, then walks every loaded
// package's syntax for call sites matching that shape. Kept separate from
// strictTypesUsages/runWholeScanAnalysis, since none of this needs
// scope-tracked bindings at all (resolveByStaticType queries go/types
// directly for whatever expression it's given), unlike every other
// Phase 1/2a detection mechanism.
func forwardingCallUsages(pkgs []*packages.Package, absRoot string) []types.FlagUsage {
	var fset *token.FileSet
	accessors := map[accessorKey]string{}
	factoryFields := map[*gotypes.Func]map[string]int{}
	for _, pkg := range pkgs {
		if pkg.Fset == nil || pkg.TypesInfo == nil {
			continue
		}
		if fset == nil {
			fset = pkg.Fset
		}
		for k, v := range accessorFields(pkg.Syntax, pkg.PkgPath) {
			accessors[k] = v
		}
		for k, v := range factoryFieldParams(pkg.Syntax, pkg.TypesInfo) {
			factoryFields[k] = v
		}
	}
	if fset == nil {
		return nil
	}

	literalVars := map[*gotypes.Var]map[string]string{}
	for _, pkg := range pkgs {
		if pkg.TypesInfo == nil {
			continue
		}
		for k, v := range flagDescriptorLiterals(pkg.Syntax, pkg.TypesInfo, factoryFields) {
			literalVars[k] = v
		}
	}

	summaries := findEvalSummaries(pkgs)
	return detectForwardingCallUsages(fset, pkgs, absRoot, summaries, accessors, literalVars, factoryFields)
}

// strictTypesUsages re-runs the whole-scan analysis over go/packages-loaded
// ASTs, which carry real go/types information (Scan's own ASTs, parsed
// independently via go/parser, never do) — this is what lets
// resolveAssignedBinding's static-type fallback (identity.go) fire.
// Re-running the *full* whole-scan analysis, rather than a bespoke walk
// narrowly targeting interface satisfaction, means every existing Phase 1
// detection mechanism gains type-backed coverage uniformly for free — not
// just the one pattern this pass was originally written for.
func strictTypesUsages(pkgs []*packages.Package, absRoot string) ([]types.FlagUsage, []types.MigrationInventoryItem) {
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
			// Read separately from go/packages' own loading (a second read
			// of a file already-parsed) — migrationInventory needs each
			// call/argument's exact source text, which go/packages' parsed
			// ASTs don't retain any more than Scan's own parser.ParseFile
			// does. A read failure here isn't reported as a scan warning:
			// it only degrades this one file's migrationInventory items to
			// missing expression text (exprText's nil-src bounds check
			// handles that safely), never the FlagUsage detection itself.
			src, _ := os.ReadFile(pos.Filename)
			parsed = append(parsed, parsedFile{
				relPath:   rel,
				dir:       filepath.Dir(pos.Filename),
				file:      file,
				src:       src,
				imports:   traceSDKImports(file),
				typesInfo: pkg.TypesInfo,
			})
		}
	}
	if len(parsed) == 0 {
		return nil, nil
	}
	sort.Slice(parsed, func(i, j int) bool { return parsed[i].relPath < parsed[j].relPath })
	return runWholeScanAnalysis(fset, parsed)
}

// mergeStrictTypesUsages folds extra into result in place, keeping every
// existing entry byte-for-byte unchanged and adding only call sites
// result doesn't already have — the additive guarantee ADR 005 promises,
// enforced here rather than assumed from strictTypesUsages happening to be
// a superset of Scan's own result for the files it covers.
//
// Deduping by (File, Line, Column) — a real call site's precise position,
// identical between Phase 1's and the strict pass's independent parses of
// the same source text — not by Fingerprint: fingerprint.Generate
// (internal/fingerprint) deliberately omits line/column (it's a
// cross-tool-contract baseline identity, meant to survive line-number
// churn from unrelated edits elsewhere in the file), so two genuinely
// different call sites in the same file sharing a callType and a static
// flag key produce the *same* fingerprint. Deduping on fingerprint alone
// would silently drop a real strict-types-only finding whenever it
// happened to collide with an unrelated Phase 1 finding's fingerprint —
// found via independent review, reproduced directly (two same-flag-key
// call sites in one file, one Phase-1-visible, one interface-satisfaction-
// only: the second vanished with no warning).
// extraMigrationItems, when non-nil, is index-aligned with extra (both
// built by the same runWholeScanAnalysis/detect() walk, in the same
// order) — the migration item for extra[i] is extraMigrationItems[i] if
// present. forwardingCallUsages has no migration-item equivalent at all
// (its call sites don't directly show the LD method's own (key, context,
// fallback) arguments — see MigrationInventory's doc comment, types.go),
// so its caller passes nil here; the length check below just means no
// item is merged for any of its usages, never a panic.
func mergeStrictTypesUsages(result *types.ScanResult, extra []types.FlagUsage, extraMigrationItems []types.MigrationInventoryItem) {
	if len(extra) == 0 {
		return
	}
	callSiteKey := func(u types.FlagUsage) string {
		return u.File + ":" + strconv.Itoa(u.Line) + ":" + strconv.Itoa(u.Column)
	}
	seen := make(map[string]bool, len(result.Usages))
	for _, u := range result.Usages {
		seen[callSiteKey(u)] = true
	}
	added := false
	migrationAdded := false
	for i, u := range extra {
		key := callSiteKey(u)
		if seen[key] {
			continue
		}
		seen[key] = true
		// Only stamped on entries that actually survive the dedup above —
		// an entry strictTypesUsages also happened to (re-)find that Phase
		// 1 already reported keeps its original DetectedBy ("", meaning
		// Phase 1) unchanged, since it's Phase 1's own copy of that finding
		// that's kept, not this re-scanned one.
		u.DetectedBy = "strict-types"
		result.Usages = append(result.Usages, u)
		added = true
		if i < len(extraMigrationItems) {
			result.MigrationInventory = append(result.MigrationInventory, extraMigrationItems[i])
			migrationAdded = true
		}
	}
	if migrationAdded {
		sort.Slice(result.MigrationInventory, func(i, j int) bool {
			a, b := result.MigrationInventory[i], result.MigrationInventory[j]
			if a.File != b.File {
				return a.File < b.File
			}
			if a.Line != b.Line {
				return a.Line < b.Line
			}
			return a.Column < b.Column
		})
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
