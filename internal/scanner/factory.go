// Package scanner (this file): cross-package factory-function resolution.
//
// Found missing during field-testing against real-world code: the
// official launchdarkly-labs/ld-sample-app-go and CMS-Enterprise/mint-app
// both wire the client through a package-level singleton getter
// (`func GetLdClient() *ld.LDClient`) or a constructor function
// (`func NewLaunchDarklyClient(...) (*ld.LDClient, error)`) called from a
// *different* package — indirection ADR 002 originally deferred entirely
// to a hypothetical go/types-based Phase 2. This turned out to be common
// enough in practice (the *official sample app* uses this exact pattern)
// to implement without go/types: a function's declared return type is
// available directly from its signature in the AST — no build or type-
// checking required, same "syntax-only" spirit as the rest of Phase 1.
//
// Identity is still never inferred from a function's *name* — only from
// its declared return type resolving, through this file's own traced SDK
// import, to the client type.
package scanner

import (
	"go/ast"
	"strconv"
)

// factoryKey identifies one function by the Go import path of its
// declaring package and its name. pkgKey is either a real import path
// (when a go.mod was found — see findModule) or a "dir:<absolute path>"
// fallback that can only ever match same-package (same-directory) calls,
// never a cross-package qualified call — see pkgKeyFor.
type factoryKey struct {
	pkgKey   string
	funcName string
}

// pkgKeyFor returns the identifier used to key factoryFunctions and to
// resolve same-package bare calls for a package declared in dir.
func pkgKeyFor(dir, modulePath, moduleRoot string, hasModule bool) string {
	if !hasModule {
		return "dir:" + dir
	}
	ip, err := packageImportPath(modulePath, moduleRoot, dir)
	if err != nil {
		return "dir:" + dir
	}
	return ip
}

// returnsSDKClient reports whether fn's first declared result type is
// `*<alias>.LDClient` for one of imports' traced SDK aliases (or bare
// `*LDClient` when dot-imported). Only the first result is considered —
// matches the real-world pattern `func X(...) (*ld.LDClient, error)`; a
// client returned as a later result is unusual enough not to special-case.
func returnsSDKClient(fn *ast.FuncDecl, imports sdkImports) (version string, ok bool) {
	if fn.Type.Results == nil || len(fn.Type.Results.List) == 0 {
		return "", false
	}
	return starLDClientType(fn.Type.Results.List[0].Type, imports)
}

// starLDClientType reports whether typeExpr is exactly `*<alias>.LDClient`
// for one of imports' traced SDK aliases (or bare `*LDClient` when dot-
// imported). Shared by returnsSDKClient (a function's declared return
// type) and paramClientBindings (a parameter's declared type) — both are
// "trust the declared type, no build required" checks, just applied to
// different parts of a signature.
func starLDClientType(typeExpr ast.Expr, imports sdkImports) (version string, ok bool) {
	star, ok := typeExpr.(*ast.StarExpr)
	if !ok {
		return "", false
	}
	switch t := star.X.(type) {
	case *ast.SelectorExpr:
		pkgIdent, ok := t.X.(*ast.Ident)
		if !ok || t.Sel.Name != "LDClient" {
			return "", false
		}
		v, traced := imports.aliases[pkgIdent.Name]
		return v, traced
	case *ast.Ident:
		if t.Name != "LDClient" {
			return "", false
		}
		for v := range imports.dotImported {
			return v, true
		}
		return "", false
	default:
		return "", false
	}
}

// paramClientBindings returns bindings for any parameter (or receiver) in
// fl whose *declared type* is directly `*<alias>.LDClient` — the same
// "trust the signature" principle as returnsSDKClient, applied to
// parameters instead of a return value. Found missing during field-
// testing against CMS-Enterprise/mint-app, which passes an already-
// constructed client into a struct's constructor as a plain parameter —
// there is no assignment to trace at all; the parameter's type annotation
// is the only place identity is ever established.
func paramClientBindings(fl *ast.FieldList, imports sdkImports) map[string]string {
	bindings := map[string]string{}
	if fl == nil {
		return bindings
	}
	for _, f := range fl.List {
		version, ok := starLDClientType(f.Type, imports)
		if !ok {
			continue
		}
		for _, name := range f.Names {
			if name.Name != "_" {
				bindings[name.Name] = version
			}
		}
	}
	return bindings
}

// collectFactoryFunctions registers every free function (no receiver —
// methods are out of scope here) in file whose declared return type
// resolves to the SDK client type, keyed by pkgKey so cross-package call
// sites can look it up.
func collectFactoryFunctions(file *ast.File, pkgKey string, imports sdkImports, index map[factoryKey]string) {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil {
			continue
		}
		version, ok := returnsSDKClient(fn, imports)
		if !ok {
			continue
		}
		index[factoryKey{pkgKey, fn.Name.Name}] = version
	}
}

// resolveImportAliases maps every local identifier file uses to refer to
// an import, to that import's pkgKey — but only for imports that resolve
// to one of *our own scanned packages* (importPathToPkgKey, built
// whole-scan). An import of an external dependency (stdlib, a third-party
// module) is deliberately left unmapped: it can never appear in
// factoryFunctions (which only contains functions declared within the
// scanned tree), so there's nothing to gain from resolving it, and
// guessing its local identifier without seeing its source would be a
// name-based heuristic — exactly what ADR 002 forbids.
func resolveImportAliases(file *ast.File, importPathToPkgKey, importPathToPkgName map[string]string) map[string]string {
	aliases := map[string]string{}
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		pkgKey, known := importPathToPkgKey[path]
		if !known {
			continue
		}
		switch {
		case imp.Name == nil:
			// No local alias given: the identifier used at call sites is
			// whatever the imported package's own `package X` clause
			// declares. Since this import resolves to one of our scanned
			// packages, its declared name is already known precisely —
			// no guessing from the import path's text.
			if name, ok := importPathToPkgName[path]; ok {
				aliases[name] = pkgKey
			}
		case imp.Name.Name == "_" || imp.Name.Name == ".":
			// Blank/dot imports of a factory-function package are rare
			// enough (and semantically odd for this pattern) to skip.
		default:
			aliases[imp.Name.Name] = pkgKey
		}
	}
	return aliases
}

// isFactoryCall reports whether call invokes a registered factory
// function — either cross-package (`pkgAlias.FuncName()`, resolved via
// importAliases) or same-package (bare `FuncName()`, resolved via ownPkgKey).
func isFactoryCall(call *ast.CallExpr, ownPkgKey string, importAliases map[string]string, factoryFunctions map[factoryKey]string) (string, bool) {
	switch fn := unparen(call.Fun).(type) {
	case *ast.SelectorExpr:
		pkgIdent, ok := fn.X.(*ast.Ident)
		if !ok {
			return "", false
		}
		pkgKey, ok := importAliases[pkgIdent.Name]
		if !ok {
			return "", false
		}
		version, ok := factoryFunctions[factoryKey{pkgKey, fn.Sel.Name}]
		return version, ok
	case *ast.Ident:
		version, ok := factoryFunctions[factoryKey{ownPkgKey, fn.Name}]
		return version, ok
	default:
		return "", false
	}
}
