// Package scanner detects github.com/launchdarkly/go-server-sdk (v6/v7)
// call sites in Go source. Client identity is proven through import-alias
// tracing, constructor-call binding, and (as of the whole-scan pre-pass
// below) cross-file struct-field and factory-function resolution — never
// through name matching alone. See docs/adr/002-client-identity-model.md.
package scanner

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/flaglint/flaglint-go/internal/config"
	"github.com/flaglint/flaglint-go/internal/fingerprint"
	"github.com/flaglint/flaglint-go/internal/types"
)

// parsedFile is one successfully parsed source file, kept in memory for
// the whole scan (rather than discarded after each file, as Phase 1
// originally did) because the whole-scan pre-pass below needs to see
// every file before any single file can be fully resolved: a struct's
// field types, a factory function's return type, and a package-level var
// binding are all frequently declared in a *different* file than where
// they're used — sometimes in a different package entirely.
type parsedFile struct {
	relPath string
	dir     string
	file    *ast.File
	imports sdkImports
}

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
	warnings := []types.ScanWarning{}

	fset := token.NewFileSet()
	var parsed []parsedFile

	for _, rel := range relFiles {
		full := filepath.Join(absRoot, rel)
		src, err := os.ReadFile(full)
		if err != nil {
			warnings = append(warnings, types.ScanWarning{
				Kind: "read-failure", File: rel, FsCode: fsCode(err),
			})
			continue
		}
		file, err := parser.ParseFile(fset, full, src, parser.SkipObjectResolution)
		if err != nil {
			warnings = append(warnings, types.ScanWarning{Kind: "parse-failure", File: rel})
			continue
		}
		parsed = append(parsed, parsedFile{
			relPath: rel,
			dir:     filepath.Dir(full),
			file:    file,
			imports: traceSDKImports(file),
		})
	}

	allUsages := runWholeScanAnalysis(fset, parsed)

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

// runWholeScanAnalysis is Scan's core: a pre-pass that builds cross-file
// identity-resolution state, followed by two binding-discovery passes and
// a final detection pass. See docs/adr/002-client-identity-model.md for
// why this exists — found necessary by field-testing against real-world
// code (the official launchdarkly-labs/ld-sample-app-go, weaviate,
// e2b-dev/infra, CMS-Enterprise/mint-app), every one of which wraps the
// client behind a struct field, a factory/getter function, or a
// multi-level field chain declared in a different file (sometimes a
// different package) than where it's used.
func runWholeScanAnalysis(fset *token.FileSet, parsed []parsedFile) []types.FlagUsage {
	// Pre-pass 1: package identity. ownPkgKey identifies each file's own
	// package (for same-package bare factory calls); importPathToPkgKey and
	// importPathToPkgName let OTHER files resolve a qualified
	// `pkgAlias.FuncName()` call back to one of our own scanned packages
	// (see factory.go) — never a name-based guess, only real import-path
	// matching when a go.mod is present.
	//
	// findModule is resolved per-file (from pf.dir, not absRoot), not once
	// for the whole scan — this is what correctly handles a monorepo with
	// independent nested go.mod submodules (issue #17): a file under a
	// submodule resolves against that submodule's own go.mod, not
	// whichever one happens to be nearest the scan root. moduleCache
	// memoizes the search so files sharing the same nearest go.mod (the
	// common case) don't each re-walk the filesystem.
	moduleCache := map[string]moduleInfo{}
	ownPkgKey := make(map[*ast.File]string, len(parsed))
	importPathToPkgKey := map[string]string{}
	importPathToPkgName := map[string]string{}
	for _, pf := range parsed {
		modulePath, moduleRoot, hasModule := findModule(pf.dir, moduleCache)
		key := pkgKeyFor(pf.dir, modulePath, moduleRoot, hasModule)
		ownPkgKey[pf.file] = key
		if !hasModule {
			continue
		}
		importPath, err := packageImportPath(modulePath, moduleRoot, pf.dir)
		if err != nil {
			continue
		}
		importPathToPkgKey[importPath] = key
		name := pf.file.Name.Name
		if existing, ok := importPathToPkgName[importPath]; !ok || (strings.HasSuffix(existing, "_test") && !strings.HasSuffix(name, "_test")) {
			importPathToPkgName[importPath] = name
		}
	}

	// Pre-pass 2: struct field types (every file, regardless of whether it
	// imports the SDK itself — the file declaring a wrapper struct often
	// doesn't) and factory functions (naturally a no-op for a file with no
	// SDK import, since returnsSDKClient checks against that file's own
	// traced aliases).
	//
	// structFieldTypes is partitioned per-package (pkgKey -> "Type.Field" ->
	// declared field type), NOT one flat map across the whole scan: Go lets
	// two unrelated packages independently declare same-named structs and
	// fields (two different "Service.Client", say), and an unqualified flat
	// map would let one package's struct shape leak into another's chain
	// resolution — the same class of false positive ADR 002 exists to
	// prevent, just at struct-field granularity instead of variable-name
	// granularity. Bare identifiers are never visible outside their own
	// package in real Go anyway, so this partitioning also just matches
	// actual Go semantics.
	structFieldTypesByPkg := map[string]map[string]string{}
	factoryFunctions := map[factoryKey]string{}
	for _, pf := range parsed {
		pkg := pkgBindings(structFieldTypesByPkg, ownPkgKey[pf.file])
		for k, v := range collectStructFieldTypes(pf.file) {
			pkg[k] = v
		}
		collectFactoryFunctions(pf.file, ownPkgKey[pf.file], pf.imports, factoryFunctions)
	}

	// Pre-pass 3: per-file contexts, now that every whole-scan index above
	// is complete. Each file's structFieldTypes is scoped to its own
	// package's partition only — see the comment above.
	ctxs := make(map[*ast.File]fileContext, len(parsed))
	for _, pf := range parsed {
		ctxs[pf.file] = fileContext{
			imports:          pf.imports,
			ownPkgKey:        ownPkgKey[pf.file],
			importAliases:    resolveImportAliases(pf.file, importPathToPkgKey, importPathToPkgName),
			factoryFunctions: factoryFunctions,
			structFieldTypes: pkgBindings(structFieldTypesByPkg, ownPkgKey[pf.file]),
		}
	}

	// Pass A: package-level vars, direct field assignments, and every
	// function scope's local bindings — merged into `base`, partitioned
	// per-package for the same reason as structFieldTypes above: an
	// unqualified package-level `var client = ...` or struct field
	// `Service.Client` in one package must never resolve a same-named
	// binding in a completely unrelated package. Struct fields and package
	// vars are not *function*- or *file*-scoped in real Go semantics
	// (ADR 002), so — unlike local variables — they're resolved across
	// every file in their own package, not just the one file that declares
	// them; but they remain scoped to that one package, matching how an
	// unqualified identifier is only ever visible within its own package.
	base := map[string]map[string]string{}
	fileScopes := make(map[*ast.File][]funcScope, len(parsed))
	for _, pf := range parsed {
		ctx := ctxs[pf.file]
		pkg := pkgBindings(base, ctx.ownPkgKey)
		for k, v := range collectPackageLevelBindings(pf.file, ctx) {
			pkg[k] = v
		}
		for k, v := range collectFieldBindings(pf.file, ctx) {
			pkg[k] = v
		}
		fileScopes[pf.file] = collectFuncScopes(pf.file, ctx.imports)
	}

	// Pass B: composite-literal field bindings (`&LDIntegration{ldClient:
	// ldClient}`) — each scope's own local bindings are needed as context
	// (the value stored might be a local variable bound earlier in the
	// same function, not just a direct constructor call), so this can't
	// run until every scope's local bindings are computable, which needs
	// `base` from Pass A first.
	//
	// Looped to a fixed point (issue #18) rather than a single forward
	// sweep: a composite literal in file A can itself depend on a
	// composite-literal binding established in file B — a "wrapper-of-
	// wrapper" split across two files — which only resolves if B happens
	// to be processed first in a single pass. (Two-level struct field
	// *type* chains resolved via qualifiedFieldKey/resolveChainType and
	// structFieldTypes are NOT affected by this — only composite-literal
	// *value* bindings that depend on another composite literal's
	// binding.) The loop stops once a full sweep discovers nothing new;
	// bounded by len(parsed)+1 sweeps as a safety cap that a real fixed
	// point can never reach — a genuine wrapper-of-wrapper chain can't
	// legitimately be deeper than the number of files in the scan, so
	// hitting the cap without converging would only be possible from a
	// bug, not real code.
	for sweep := 0; sweep <= len(parsed); sweep++ {
		changed := false
		for _, pf := range parsed {
			ctx := ctxs[pf.file]
			pkg := pkgBindings(base, ctx.ownPkgKey)
			for _, s := range fileScopes[pf.file] {
				walkScoped(s.body, mergeBindings(pkg, s.paramBindings), ctx, func(n ast.Node, current map[string]string, _ map[string]methodValueBinding) {
					lit, ok := n.(*ast.CompositeLit)
					if !ok {
						return
					}
					for k, v := range compositeLiteralFieldBindings(lit, current, ctx) {
						if existing, ok := pkg[k]; !ok || existing != v {
							pkg[k] = v
							changed = true
						}
					}
				})
			}
		}
		if !changed {
			break
		}
	}

	// Pass C: detection, with the now-complete binding set — a composite
	// literal in one file/function can make a field binding visible to a
	// different file processed earlier in this loop.
	var allUsages []types.FlagUsage
	for _, pf := range parsed {
		ctx := ctxs[pf.file]
		pkg := pkgBindings(base, ctx.ownPkgKey)
		d := &fileDetector{fset: fset, relPath: pf.relPath}
		for _, s := range fileScopes[pf.file] {
			d.detect(s.body, mergeBindings(pkg, s.paramBindings), s.declared, ctx.structFieldTypes, ctx)
		}
		allUsages = append(allUsages, d.usages...)
	}

	if allUsages == nil {
		allUsages = []types.FlagUsage{}
	}
	return allUsages
}

// pkgBindings returns byPkg's inner map for pkgKey, creating it on first
// use. Every whole-scan binding index that isn't already qualified by a
// real Go identity (factoryFunctions is keyed by pkgKey+funcName directly)
// is partitioned this way — see the comments at structFieldTypesByPkg and
// base in runWholeScanAnalysis for why.
func pkgBindings(byPkg map[string]map[string]string, pkgKey string) map[string]string {
	m, ok := byPkg[pkgKey]
	if !ok {
		m = map[string]string{}
		byPkg[pkgKey] = m
	}
	return m
}

// funcScope is one function/method/closure body worth detecting usages in,
// paired with its receiver/parameter declared types (needed to resolve
// field-selector receivers — see qualifiedFieldKey) and any parameter/
// receiver bindings established purely by a declared `*ld.LDClient`
// parameter type (see paramClientBindings — no assignment to trace at
// all, the type annotation alone proves identity).
type funcScope struct {
	body          *ast.BlockStmt
	declared      map[string]string
	paramBindings map[string]string
}

// collectFuncScopes enumerates every top-level function/method body in
// file, plus any function literal nested inside a top-level `var`
// initializer (an immediately-invoked closure, or one passed as an
// argument) — not just a directly-assigned `var x = func(){...}`.
func collectFuncScopes(file *ast.File, imports sdkImports) []funcScope {
	var scopes []funcScope
	for _, decl := range file.Decls {
		switch n := decl.(type) {
		case *ast.FuncDecl:
			if n.Body == nil {
				continue // external/assembly function, no body to scan
			}
			scopes = append(scopes, funcScope{
				body:          n.Body,
				declared:      declaredParamTypes(n),
				paramBindings: paramClientBindings(n.Type.Params, imports),
			})
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
					ast.Inspect(val, func(inner ast.Node) bool {
						lit, ok := inner.(*ast.FuncLit)
						if !ok {
							return true
						}
						scopes = append(scopes, funcScope{
							body:          lit.Body,
							declared:      paramTypesFromFuncLit(lit),
							paramBindings: paramClientBindings(lit.Type.Params, imports),
						})
						return true
					})
				}
			}
		}
	}
	return scopes
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

// bindings is the scope's initial bindings (whole-scan package bindings
// merged with this scope's own parameter bindings) — walkScoped resolves
// the block-scoped view in effect at each call site from there, so a
// shadowed local variable is never mistaken for this scope's client. See
// walkScoped's doc comment (identity.go) for why that requires more than
// just scoping the map by block.
//
// Known Phase 1 gap: a method value passed *across a function boundary*
// (captured in one function, called from inside a different one it was
// passed into — the shape a real e2b-dev/infra field-testing repro
// actually needed) is not detected; only a method value used within the
// same scope it was captured in is (see methodValueBinding, issue #6).
// Interprocedural propagation is a meaningfully larger undertaking,
// deferred to the opt-in --strict-types pass, not a new exception to the
// identity model.
func (d *fileDetector) detect(scope ast.Node, bindings, declared, structFieldTypes map[string]string, ctx fileContext) {
	walkScoped(scope, bindings, ctx, func(n ast.Node, current map[string]string, methodValues map[string]methodValueBinding) {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return
		}

		var version, callTypeName string
		switch fn := unparen(call.Fun).(type) {
		case *ast.SelectorExpr:
			// A chained call — `pkg.GetLdClient().Method(...)`, no
			// intermediate variable (issue #20) — has the receiver's
			// version known directly from the inner call itself;
			// resolveAssignedBinding already does exactly this "is this a
			// recognized constructor/factory call" check for assignment
			// RHS values, and applies here unchanged. Only fall back to
			// the bindings-map lookup when the receiver isn't itself such
			// a call.
			v, bound := resolveAssignedBinding(unparen(fn.X), ctx)
			if !bound {
				receiver := resolveReceiver(fn.X, declared, structFieldTypes)
				if receiver == "" {
					return
				}
				v, bound = current[receiver]
				if !bound {
					return
				}
			}
			version, callTypeName = v, fn.Sel.Name
		case *ast.Ident:
			// A bare call on a method value captured earlier in this same
			// scope (`f := client.BoolVariation; f(...)`, issue #6) — the
			// version and method name are already known from the capture
			// itself, resolved by walkScoped's bindLocalValue.
			mv, bound := methodValues[fn.Name]
			if !bound {
				return
			}
			version, callTypeName = mv.version, mv.method
		default:
			return
		}

		spec, known := methodSpecs[callTypeName]
		if !known {
			return
		}

		pos := d.fset.Position(call.Pos())
		callType := types.CallType(callTypeName)
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
			return
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
