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

// opensScope reports whether n introduces a new Go lexical scope for
// local-variable purposes: a block body, or a switch/select case's own
// statement list — the latter is lexically its own scope too, even though
// (unlike a plain `{ }` block) it isn't wrapped in its own *ast.BlockStmt
// in the AST.
//
// Not handled: the Init clause of an if/for/switch statement is lexically
// its own (wider) scope enclosing the whole statement, one level outside
// its Body block — treating the Body's *ast.BlockStmt as the only scope
// boundary means an Init-declared variable (`if x := f(); ... `) is
// attributed to the enclosing scope instead of its own slightly narrower
// one. This narrower inaccuracy is unrelated to the shadowing bug
// opensScope/walkScoped were introduced to fix (see walkScoped) and is
// accepted rather than adding further scope-tracking complexity for it.
func opensScope(n ast.Node) bool {
	switch n.(type) {
	case *ast.BlockStmt, *ast.CaseClause, *ast.CommClause:
		return true
	default:
		return false
	}
}

// resolveAssignedBinding reports what a single assignment-target position
// should resolve to: the SDK version, if rhs is a recognized constructor
// or factory call, or ok=false for anything else — including a value we
// simply don't recognize. Callers that track scope (see walkScoped) must
// treat ok=false as "actively clear any inherited binding for this name",
// not just "don't add one" — see walkScoped's doc comment for why.
func resolveAssignedBinding(rhs ast.Expr, ctx fileContext) (version string, ok bool) {
	call, ok := rhs.(*ast.CallExpr)
	if !ok {
		return "", false
	}
	version, ok = isSDKConstructorCall(call, ctx.imports)
	if ok {
		return version, true
	}
	return isFactoryCall(call, ctx.ownPkgKey, ctx.importAliases, ctx.factoryFunctions)
}

// walkScoped walks scope (a function/method/closure body) depth-first in
// source order, maintaining a stack of block-scoped local-variable
// bindings that models real Go lexical scoping — a binding established
// via `:=` or `var` inside a nested block (or switch/select case) is
// visible to that block and anything nested deeper within it, but is
// never visible again once the block ends. visit is invoked at every
// node with the bindings map currently in effect at that exact point; the
// map instance is swapped out whenever a new scope opens (a copy-on-write
// clone of the parent's bindings), so visit must not cache a reference to
// it across calls. Struct-field bindings are intentionally not tracked
// here; see collectFieldBindings. Interface satisfaction (a value only
// known through an interface type, not a concrete client-typed variable)
// remains a known Phase 1 gap, deferred to the opt-in --strict-types pass
// (ADR 002).
//
// This function (and its caller, invoked once per top-level function or
// closure via collectFuncScopes) is what keeps a same-named local variable
// bound to an unrelated value in a *different* function from being
// mistaken for this scope's client — the flat, whole-file binding table
// this design replaced had exactly that false-positive risk.
//
// Fixes flaglint-go issue #5: a deliberate re-`:=` of the same name inside
// a nested block, shadowing an outer real client variable with an
// unrelated value, was previously invisible to this scanner — the outer
// binding remained visible to the inner block because bindings were once
// tracked in one flat map for the whole function. Getting this right
// requires more than scoping the *map* by block: a `:=`/`var` whose value
// isn't a recognized client must actively clear any same-named binding
// inherited from an enclosing scope (resolveAssignedBinding's ok=false
// case), not just fail to add a new one — otherwise the shadow would
// still resolve to the outer (wrong) binding via inheritance.
func walkScoped(scope ast.Node, base map[string]string, ctx fileContext, visit func(n ast.Node, bindings map[string]string)) {
	stack := []map[string]string{base}
	var pushed []bool // parallel stack: did entering this node open a new scope

	ast.Inspect(scope, func(n ast.Node) bool {
		if n == nil {
			// Post-order signal (see ast.Inspect's doc: f(nil) is called
			// once a node's entire subtree has been visited) — pop the
			// frame most recently pushed by the matching pre-order visit.
			last := pushed[len(pushed)-1]
			pushed = pushed[:len(pushed)-1]
			if last {
				stack = stack[:len(stack)-1]
			}
			return true
		}

		opened := opensScope(n)
		if opened {
			top := stack[len(stack)-1]
			child := make(map[string]string, len(top))
			for k, v := range top {
				child[k] = v
			}
			stack = append(stack, child)
		}
		pushed = append(pushed, opened)

		current := stack[len(stack)-1]
		switch node := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range node.Lhs {
				ident, ok := lhs.(*ast.Ident)
				if !ok || ident.Name == "_" {
					continue
				}
				if i >= len(node.Rhs) {
					// Multi-value RHS assigned across more LHS targets
					// than there are RHS expressions (`x, err := f()`) —
					// not resolvable syntactically; leave any existing
					// binding alone rather than guess. Pre-existing
					// limitation, not introduced by this scope tracking.
					continue
				}
				if version, ok := resolveAssignedBinding(node.Rhs[i], ctx); ok {
					current[ident.Name] = version
				} else {
					delete(current, ident.Name)
				}
			}
		case *ast.ValueSpec:
			for i, name := range node.Names {
				if name.Name == "_" {
					continue
				}
				if i >= len(node.Values) {
					// `var x, y T` with no initializer at all — still a
					// fresh local declaration that shadows any outer
					// same-named variable.
					delete(current, name.Name)
					continue
				}
				if version, ok := resolveAssignedBinding(node.Values[i], ctx); ok {
					current[name.Name] = version
				} else {
					delete(current, name.Name)
				}
			}
		}

		visit(n, current)
		return true
	})
}

// compositeLiteralFieldBindings finds struct-field bindings established by
// a single composite literal — `&LDIntegration{ldClient: ldClient}` or
// `LDIntegration{ldClient: ldClient}` — where the field value is either a
// direct SDK constructor call or an identifier already known to be bound
// (via knownBindings, the scope-correct bindings in effect at lit's
// position — see walkScoped). Found missing during field-testing against
// real-world code (weaviate, e2b-dev/infra both use this exact pattern to
// store a client into a wrapper struct) — collectFieldBindings only
// recognized `x.Field = value` assignment, not literal initialization.
//
// Keys are type-qualified ("LDIntegration.ldClient"), matching
// collectFieldBindings, for the same cross-type-collision reason.
func compositeLiteralFieldBindings(lit *ast.CompositeLit, knownBindings map[string]string, ctx fileContext) map[string]string {
	bindings := map[string]string{}
	typeName := simpleTypeName(lit.Type)
	if typeName == "" {
		return bindings
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
			ver, ok := resolveAssignedBinding(v, ctx)
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
// and `var x = pkg.SomeFactoryFunc(...)` forms for a single package-level
// ValueSpec. Unlike walkScoped's scope-aware ValueSpec handling, this never
// clears an existing entry for a name it doesn't recognize — package-level
// bindings aren't tracked per-scope in the first place, so there's nothing
// to unshadow.
func bindFromValueSpecOrFactoryCall(vs *ast.ValueSpec, ctx fileContext, bindings map[string]string) {
	for i, rhs := range vs.Values {
		if i >= len(vs.Names) || vs.Names[i].Name == "_" {
			continue
		}
		if version, ok := resolveAssignedBinding(rhs, ctx); ok {
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
