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

// unparen strips any number of enclosing parentheses, so `(ld.MakeClient)(...)`
// and `((client.BoolVariation))(...)` resolve the same as their unparenthesized
// forms.
func unparen(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}

// isSDKConstructorCall reports whether call is `<alias>.MakeClient(...)` or
// `<alias>.MakeCustomClient(...)` for a traced SDK alias, or a bare
// `MakeClient(...)`/`MakeCustomClient(...)` when the SDK was dot-imported.
// Returns the SDK version the call resolves to.
func isSDKConstructorCall(call *ast.CallExpr, imports sdkImports) (version string, ok bool) {
	switch fn := unparen(call.Fun).(type) {
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

// fileContext bundles the whole-scan and per-file state needed to resolve
// client identity for one file, so the binding-collection functions below
// don't need an ever-growing parameter list every time cross-file
// resolution gains a new capability (factory functions, multi-level field
// chains, ...).
type fileContext struct {
	imports sdkImports

	// ownPkgKey identifies this file's own package for same-package bare
	// factory calls (`FuncName()`) — see factory.go.
	ownPkgKey string
	// importAliases maps this file's local import identifiers to the
	// pkgKey of *our own scanned packages* only (see resolveImportAliases)
	// — used to resolve cross-package factory calls (`pkgAlias.FuncName()`).
	importAliases map[string]string
	// factoryFunctions is the whole-scan index of functions whose declared
	// return type resolves to the SDK client type (see factory.go).
	factoryFunctions map[factoryKey]string

	// structFieldTypes is the whole-scan index of "StructName.Field" ->
	// declared field type name (see structtypes.go), used to walk
	// multi-level field-selector chains one hop at a time.
	structFieldTypes map[string]string
}

// collectPackageLevelBindings finds client bindings established by
// top-level `var` declarations (file.Decls only — never descends into a
// function body), which are visible to every function in the file.
func collectPackageLevelBindings(file *ast.File, ctx fileContext) map[string]string {
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
			bindFromValueSpecOrFactoryCall(vs, ctx, bindings)
		}
	}
	return bindings
}

// collectFieldBindings finds every struct-field client binding (e.g.
// "s.Client, _ = ld.MakeClient(...)") in the file. Unlike local variables, a
// struct field is not function-scoped in real Go semantics — it represents
// object state that is legitimately set in one method and used from a
// completely different one (a constructor/setup method and the methods
// that use the client it configured). So field bindings are collected
// file-wide, in contrast to local identifier bindings, which are scoped
// per function by collectScopedBindings below.
//
// Bindings are keyed by the field's *declared type name* ("RealService.Client"),
// resolved syntactically from the enclosing function's receiver/parameter
// declarations — never by the receiver variable's name alone ("s.Client").
// Two unrelated struct types that happen to share both a receiver/parameter
// name and a field name (e.g. two different "Client" fields, each on a
// different struct, both accessed through a variable named "s" in
// different functions) would otherwise collide in a flat variable-keyed
// map — exactly the same false-positive class collectScopedBindings' per-
// function scoping fixes for local variables, reintroduced for fields
// because they're deliberately not function-scoped.
//
// When a receiver's type cannot be resolved this way (e.g. a local
// variable without an explicit declared type), the assignment is not
// bound at all. Phase 1 prefers a missed detection over a possible false
// positive here — this matches the project's safety-first philosophy
// (ADR 002): local variable struct construction with an inferred type is
// deferred to the opt-in --strict-types pass, which has real type
// information and does not need this restriction.
func collectFieldBindings(file *ast.File, ctx fileContext) map[string]string {
	bindings := map[string]string{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		declared := declaredParamTypes(fn)

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for i, rhs := range assign.Rhs {
				call, ok := rhs.(*ast.CallExpr)
				if !ok {
					continue
				}
				version, ok := isSDKConstructorCall(call, ctx.imports)
				if !ok || i >= len(assign.Lhs) {
					continue
				}
				sel, ok := assign.Lhs[i].(*ast.SelectorExpr)
				if !ok {
					continue
				}
				if key := qualifiedFieldKey(sel, declared, ctx.structFieldTypes); key != "" {
					bindings[key] = version
				}
			}
			return true
		})
	}
	return bindings
}

// declaredParamTypes maps each receiver/parameter identifier of fn to its
// declared type's simple name ("RealService", not "*pkg.RealService"),
// resolved purely from the AST — no build or type-checking required.
// Parameters whose type isn't a plain name or pointer-to-name (interfaces,
// generics, unnamed struct types, ...) are left unresolved.
func declaredParamTypes(fn *ast.FuncDecl) map[string]string {
	types := paramTypesFromFieldList(fn.Type.Params)
	for k, v := range paramTypesFromFieldList(fn.Recv) {
		types[k] = v
	}
	return types
}

// paramTypesFromFuncLit is declaredParamTypes' equivalent for a function
// literal, which has parameters but never a receiver.
func paramTypesFromFuncLit(lit *ast.FuncLit) map[string]string {
	return paramTypesFromFieldList(lit.Type.Params)
}

func paramTypesFromFieldList(fl *ast.FieldList) map[string]string {
	types := map[string]string{}
	if fl == nil {
		return types
	}
	for _, f := range fl.List {
		name := simpleTypeName(f.Type)
		if name == "" {
			continue
		}
		for _, n := range f.Names {
			types[n.Name] = name
		}
	}
	return types
}

// simpleTypeName returns the bare type name for an identifier, a pointer to
// one, a package-qualified name (the package qualifier is dropped since
// Phase 1 does not resolve cross-package types), or a generic
// instantiation/receiver (`FeatureFlag[T]`, `Map[K, V]` — the type
// arguments are dropped, only the base name matters for identity
// resolution) — "" for anything else.
//
// Found missing during field-testing against weaviate/weaviate: a method
// receiver on a generic struct (`func (f *FeatureFlag[T]) M()`) has type
// *ast.IndexExpr (single type param) or *ast.IndexListExpr (multiple), not
// the plain *ast.StarExpr/*ast.Ident this originally handled — so
// declaredParamTypes silently failed to resolve the receiver's type at
// all for any method on a generic struct, breaking every multi-level
// chain resolution rooted at one.
func simpleTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return simpleTypeName(t.X)
	case *ast.SelectorExpr:
		return t.Sel.Name
	case *ast.IndexExpr:
		return simpleTypeName(t.X)
	case *ast.IndexListExpr:
		return simpleTypeName(t.X)
	default:
		return ""
	}
}

// qualifiedFieldKey resolves a field selector — single-level ("s.Client")
// or multi-level ("f.ldInteg.ldClient") — to "TypeName.Field" for the
// *final* hop. Everything up to (but not including) the final field is
// resolved by resolveChainType, walking one hop at a time through
// structFieldTypes (a struct's own field-type declarations, collected
// whole-scan — see structtypes.go). Returns "" if any hop can't be
// resolved this way (see collectFieldBindings for why that means "don't
// bind" rather than guess).
func qualifiedFieldKey(sel *ast.SelectorExpr, declared, structFieldTypes map[string]string) string {
	baseType := resolveChainType(sel.X, declared, structFieldTypes)
	if baseType == "" {
		return ""
	}
	return baseType + "." + sel.Sel.Name
}

// resolveChainType resolves the declared *type* of expr — an identifier
// (via declared, the enclosing function's receiver/parameter types) or a
// field-selector chain (recursing through structFieldTypes one hop at a
// time). Used internally by qualifiedFieldKey to walk every hop of a
// multi-level chain except the last.
func resolveChainType(expr ast.Expr, declared, structFieldTypes map[string]string) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return declared[e.Name]
	case *ast.SelectorExpr:
		innerType := resolveChainType(e.X, declared, structFieldTypes)
		if innerType == "" {
			return ""
		}
		return structFieldTypes[innerType+"."+e.Sel.Name]
	default:
		return ""
	}
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
// Interface satisfaction (a value only known through an interface type,
// not a concrete client-typed variable) remains a known Phase 1 gap,
// deferred to the opt-in --strict-types pass (ADR 002). Indirection through
// a factory *function* is handled here via ctx.factoryFunctions — see
// factory.go for why this specific sub-case doesn't need go/types.
//
// Known Phase 1 limitation: this map is flat across the whole function, not
// block-scoped. A deliberate re-`:=` of the same name inside a nested block
// (e.g. a for-loop shadowing an outer real client variable with an
// unrelated value) is not modeled — the outer binding remains visible to
// the inner block. This is a narrower risk than the cross-function case
// this function was introduced to fix: it requires the same identifier to
// be deliberately reused for something unrelated within one function, a
// pattern most Go style guides (and `go vet -shadow`) already discourage.
// Full block scoping is deferred rather than risk destabilizing this fix
// under time pressure; tracked for a future pass if it proves necessary.
func collectScopedBindings(scope ast.Node, base map[string]string, ctx fileContext) map[string]string {
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
				version, ok := isSDKConstructorCall(call, ctx.imports)
				if !ok {
					version, ok = isFactoryCall(call, ctx.ownPkgKey, ctx.importAliases, ctx.factoryFunctions)
				}
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
			bindFromValueSpecOrFactoryCall(node, ctx, bindings)
		}
		return true
	})

	return bindings
}

// collectCompositeLiteralFieldBindings finds struct-field bindings
// established via composite literal — `&LDIntegration{ldClient: ldClient}`
// or `LDIntegration{ldClient: ldClient}` — where the field value is either
// a direct SDK constructor call or an identifier already known to be bound
// (via knownBindings, which the caller builds from package-level vars,
// direct field assignments, and this scope's own local variables). Found
// missing during field-testing against real-world code (weaviate,
// e2b-dev/infra both use this exact pattern to store a client into a
// wrapper struct) — collectFieldBindings only recognized `x.Field = value`
// assignment, not literal initialization.
//
// Keys are type-qualified ("LDIntegration.ldClient"), matching
// collectFieldBindings, for the same cross-type-collision reason.
func collectCompositeLiteralFieldBindings(scope ast.Node, knownBindings map[string]string, ctx fileContext) map[string]string {
	bindings := map[string]string{}
	ast.Inspect(scope, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		typeName := simpleTypeName(lit.Type)
		if typeName == "" {
			return true
		}
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			fieldIdent, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			var version string
			switch v := unparen(kv.Value).(type) {
			case *ast.CallExpr:
				ver, ok := isSDKConstructorCall(v, ctx.imports)
				if !ok {
					ver, ok = isFactoryCall(v, ctx.ownPkgKey, ctx.importAliases, ctx.factoryFunctions)
				}
				if !ok {
					continue
				}
				version = ver
			case *ast.Ident:
				ver, ok := knownBindings[v.Name]
				if !ok {
					continue
				}
				version = ver
			default:
				continue
			}
			bindings[typeName+"."+fieldIdent.Name] = version
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

// bindFromValueSpecOrFactoryCall handles both `var x = ld.MakeClient(...)`
// and `var x = pkg.SomeFactoryFunc(...)` forms for a single ValueSpec (used
// for both package-level `var` and local `var` inside a function body).
func bindFromValueSpecOrFactoryCall(vs *ast.ValueSpec, ctx fileContext, bindings map[string]string) {
	for i, rhs := range vs.Values {
		call, ok := rhs.(*ast.CallExpr)
		if !ok {
			continue
		}
		version, ok := isSDKConstructorCall(call, ctx.imports)
		if !ok {
			version, ok = isFactoryCall(call, ctx.ownPkgKey, ctx.importAliases, ctx.factoryFunctions)
		}
		if !ok || i >= len(vs.Names) {
			continue
		}
		if vs.Names[i].Name != "_" {
			bindings[vs.Names[i].Name] = version
		}
	}
}

// resolveReceiver returns the bindings-map key for a method call's receiver
// expression: a plain identifier resolves to its own name (local variable
// or package-level var lookup, unaffected by field type-qualification); a
// field selector — single-level ("s.Client") or multi-level
// ("f.ldInteg.ldClient") — resolves through qualifiedFieldKey to
// "TypeName.Client", matching how collectFieldBindings keys struct-field
// bindings. It deliberately never falls back to the raw selector text,
// which would reintroduce the cross-type collision collectFieldBindings's
// type-qualification exists to prevent. Anything else (index expressions,
// calls) returns "".
func resolveReceiver(recv ast.Expr, declared, structFieldTypes map[string]string) string {
	switch e := recv.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return qualifiedFieldKey(e, declared, structFieldTypes)
	default:
		return ""
	}
}
