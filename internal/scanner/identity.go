package scanner

import (
	"go/ast"
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

// collectClientBindings finds every variable, package-level var, or struct
// field that is directly assigned the result of a verified SDK constructor
// call, and records which SDK version it was constructed from. Binding is
// tracked by identifier/selector text within the file — Phase 1 does not
// require full type information (see ADR 002); indirection through a
// factory function or interface satisfaction is a known Phase 1 gap,
// deferred to the opt-in --strict-types pass.
func collectClientBindings(file *ast.File, imports sdkImports) map[string]string {
	bindings := map[string]string{}

	record := func(lhs ast.Expr, version string) {
		switch e := lhs.(type) {
		case *ast.Ident:
			if e.Name != "_" {
				bindings[e.Name] = version
			}
		case *ast.SelectorExpr:
			bindings[exprString(e)] = version
		}
	}

	ast.Inspect(file, func(n ast.Node) bool {
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
				record(node.Lhs[i], version)
			}
		case *ast.ValueSpec:
			for i, rhs := range node.Values {
				call, ok := rhs.(*ast.CallExpr)
				if !ok {
					continue
				}
				version, ok := isSDKConstructorCall(call, imports)
				if !ok || i >= len(node.Names) {
					continue
				}
				bindings[node.Names[i].Name] = version
			}
		}
		return true
	})

	return bindings
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
