package scanner

import (
	"go/ast"
	"go/token"
	"strconv"
)

const (
	sdkImportV6     = "github.com/launchdarkly/go-server-sdk/v6"
	sdkImportV7     = "github.com/launchdarkly/go-server-sdk/v7"
	sdkDefaultAlias = "ldclient" // the SDK package's declared name (package ldclient), used when no explicit import alias is given
)

// sdkImports records, for one file, which local identifiers refer to the
// LaunchDarkly Go SDK package and which SDK major version they resolve to.
type sdkImports struct {
	aliases     map[string]string // local identifier -> "v6" | "v7"
	dotImported map[string]bool   // "v6"/"v7" -> true, if dot-imported in this file
}

func (s sdkImports) present() bool {
	return len(s.aliases) > 0 || len(s.dotImported) > 0
}

// traceSDKImports walks file's import declarations for the LaunchDarkly Go
// SDK (v6 or v7) and records whatever local alias is in play — this is the
// only source of truth for client identity; see ADR 002.
func traceSDKImports(file *ast.File) sdkImports {
	res := sdkImports{aliases: map[string]string{}, dotImported: map[string]bool{}}
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		var version string
		switch path {
		case sdkImportV6:
			version = "v6"
		case sdkImportV7:
			version = "v7"
		default:
			continue
		}
		switch {
		case imp.Name == nil:
			res.aliases[sdkDefaultAlias] = version
		case imp.Name.Name == "_":
			// blank import: SDK symbols not reachable from this file
		case imp.Name.Name == ".":
			res.dotImported[version] = true
		default:
			res.aliases[imp.Name.Name] = version
		}
	}
	return res
}

// isSDKConstructorCall reports whether call is `<alias>.MakeClient(...)` or
// `<alias>.MakeCustomClient(...)` for a traced SDK alias, or a bare
// `MakeClient(...)`/`MakeCustomClient(...)` when the SDK was dot-imported.
// Returns the SDK version the call resolves to.
func isSDKConstructorCall(call *ast.CallExpr, imports sdkImports) (version string, ok bool) {
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		pkgIdent, ok := fn.X.(*ast.Ident)
		if !ok {
			return "", false
		}
		v, traced := imports.aliases[pkgIdent.Name]
		if !traced || !isConstructorName(fn.Sel.Name) {
			return "", false
		}
		return v, true
	case *ast.Ident:
		if !isConstructorName(fn.Name) {
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

func isConstructorName(name string) bool {
	return name == "MakeClient" || name == "MakeCustomClient"
}

// collectPackageLevelBindings finds client bindings established by
// top-level `var` declarations (file.Decls only — never descends into a
// function body), which are visible to every function in the file.
func collectPackageLevelBindings(file *ast.File, imports sdkImports) map[string]string {
	bindings := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			bindFromValueSpec(vs, imports, bindings)
		}
	}
	return bindings
}

// collectFieldBindings finds every struct-field client binding (e.g.
// "s.Client, _ = ld.MakeClient(...)") anywhere in the file. Unlike local
// variables, a struct field is not function-scoped in real Go semantics —
// it represents object state that is legitimately set in one method and
// used from a completely different one (a constructor/setup method and the
// methods that use the client it configured). So field bindings are
// collected file-wide, in contrast to local identifier bindings, which are
// scoped per function by collectScopedBindings below.
func collectFieldBindings(file *ast.File, imports sdkImports) map[string]string {
	bindings := map[string]string{}
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, rhs := range assign.Rhs {
			call, ok := rhs.(*ast.CallExpr)
			if !ok {
				continue
			}
			version, ok := isSDKConstructorCall(call, imports)
			if !ok || i >= len(assign.Lhs) {
				continue
			}
			if sel, ok := assign.Lhs[i].(*ast.SelectorExpr); ok {
				bindings[exprString(sel)] = version
			}
		}
		return true
	})
	return bindings
}

// collectScopedBindings returns a copy of base extended with every *local
// variable* client binding established anywhere within scope (typically
// one top-level function or method body) — struct-field bindings are
// intentionally not tracked here; see collectFieldBindings. A fresh copy is
// returned per top-level function/closure so that a same-named local
// variable bound to an unrelated value in a *different* function is never
// mistaken for this scope's client — the flat, whole-file binding table
// this replaced had exactly that false-positive risk. Nested closures
// (ast.Inspect descends into FuncLit automatically) correctly continue to
// see their enclosing function's bindings, because they are walked as part
// of the same scope.
//
// Indirection through a factory function or interface satisfaction is a
// known Phase 1 gap, deferred to the opt-in --strict-types pass (ADR 002).
func collectScopedBindings(scope ast.Node, base map[string]string, imports sdkImports) map[string]string {
	bindings := make(map[string]string, len(base))
	for k, v := range base {
		bindings[k] = v
	}

	ast.Inspect(scope, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			for i, rhs := range node.Rhs {
				call, ok := rhs.(*ast.CallExpr)
				if !ok {
					continue
				}
				version, ok := isSDKConstructorCall(call, imports)
				if !ok || i >= len(node.Lhs) {
					continue
				}
				// Only plain identifiers here — struct-field assignments
				// are handled file-wide by collectFieldBindings, not
				// scoped to this function.
				if ident, ok := node.Lhs[i].(*ast.Ident); ok && ident.Name != "_" {
					bindings[ident.Name] = version
				}
			}
		case *ast.ValueSpec:
			bindFromValueSpec(node, imports, bindings)
		}
		return true
	})

	return bindings
}

// mergeBindings returns a new map containing every entry from both inputs;
// b's entries win on key collision (there should never be one in practice,
// since package-level and struct-field bindings key on disjoint identifier
// shapes).
func mergeBindings(a, b map[string]string) map[string]string {
	merged := make(map[string]string, len(a)+len(b))
	for k, v := range a {
		merged[k] = v
	}
	for k, v := range b {
		merged[k] = v
	}
	return merged
}

func bindFromValueSpec(vs *ast.ValueSpec, imports sdkImports, bindings map[string]string) {
	for i, rhs := range vs.Values {
		call, ok := rhs.(*ast.CallExpr)
		if !ok {
			continue
		}
		version, ok := isSDKConstructorCall(call, imports)
		if !ok || i >= len(vs.Names) {
			continue
		}
		if vs.Names[i].Name != "_" {
			bindings[vs.Names[i].Name] = version
		}
	}
}

// exprString renders a simple identifier or selector chain (e.g. "s.Client")
// as a string key. Returns "" for expressions it doesn't recognize (index
// expressions, calls, etc.) — those are never treated as client bindings.
func exprString(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		base := exprString(v.X)
		if base == "" {
			return ""
		}
		return base + "." + v.Sel.Name
	default:
		return ""
	}
}
