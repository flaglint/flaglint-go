// Part of the opt-in --strict-types pass (ScanStrict, strict.go). See
// docs/adr/006-interprocedural-method-values.md for the original design
// this file implements (issue #26's "forwarding function" pattern), later
// extended past the ADR's own scope once real-world verification against
// the ADR's own motivating repo (e2b-dev/infra) showed the actual shape
// needed more: a package-level var constructed via a factory that stores
// a literal into a struct field, read back through a trivial accessor
// method, passed through two levels of function forwarding before
// reaching the SDK call. Still no go/ssa/go/callgraph (ADR 006's
// approach B) — every hop here is a small, individually-structural check,
// chained together via a whole-scan pass in the same fixed-point spirit
// as factory.go's cross-file factory-function resolution (ADR 004) and
// issue #18's Pass B loop.
package scanner

import (
	"go/ast"
	"go/token"
	gotypes "go/types"
	"path/filepath"
	"strconv"

	"golang.org/x/tools/go/packages"

	"github.com/flaglint/flaglint-go/internal/fingerprint"
	"github.com/flaglint/flaglint-go/internal/types"
)

// keySource records where one candidate call-argument position's value
// comes from: one of F's own parameters (paramIndex), read either as a
// bare reference (accessor == "") or through a zero-arg accessor method
// call on that parameter (accessor's name).
type keySource struct {
	paramIndex int
	accessor   string
}

// evalSummary records everything needed to recognize a call to one
// function F as a flag evaluation, resolved once per F (not per call
// site) during the whole-scan pass:
//
//   - SDK identity: either sdkFromParam >= 0 (F calls its own function-
//     typed parameter at that index directly — the SDK method varies per
//     call site, read from whatever concrete method value is passed
//     there) or sdkVersion/sdkMethod fixed (F is itself a "pass-through":
//     it calls a function with an *already known* evalSummary, supplying
//     an already-resolved, concrete SDK method value of its own rather
//     than forwarding one of its own parameters — e.g. a wrapper method
//     that always evaluates through the same client field).
//   - Flag key identity: keyCandidates maps a call-argument position
//     (within F's own invocation of its callback) to where that
//     position's value comes from. This must be a map, not a single best
//     guess: F's callback might be invoked with several of F's own
//     parameters at different positions (e.g. `getFromLaunchDarkly(ctx,
//     flag.Key(), ...)` — ctx is ALSO one of F's own parameters, at
//     position 0, but it isn't the flag key; flag.Key() at position 1
//     is). Which position is actually the key is only knowable once a
//     specific call site resolves the concrete SDK method and its real
//     methodSpecs.keyArgIndex — so every plausible candidate is kept, and
//     the right one is selected by that lookup, never by assuming
//     "whichever came first in the argument list".
type evalSummary struct {
	sdkFromParam int
	sdkVersion   string
	sdkMethod    string

	keyCandidates map[int]keySource
}

// accessorKey identifies a method by its receiver's package-qualified
// type name and the method name itself — not by *types.Func identity. A
// call through an interface-typed expression (`flag.Key()` where flag
// typedFlag[T]) resolves via go/types to the *interface's* abstract
// method object, a completely different *types.Func than the concrete
// type's own implementation (`func (f BoolFlag) Key() string {...}`) —
// even though at runtime it's the concrete implementation that actually
// runs. Since only the concrete implementation has a body to learn which
// field it reads from, and the concrete type is only known once a real
// value reaches a call site, matching by (type name, method name)
// instead of object identity is what lets a later concrete value's type
// be looked up directly. typeName is package-qualified (pkgPath + "." +
// name) specifically to rule out two unrelated same-named types in
// different packages of the scanned repo colliding on this key — found
// during independent review of the same-name-only version.
type accessorKey struct {
	typeName   string
	methodName string
}

// accessorFields finds every method shaped exactly like `func (recv S)
// MethodName() string { return recv.field }` — a trivial, single-
// statement field accessor, across every loaded package. Keyed by
// accessorKey{pkgPath.S, MethodName} so a later call through a value of
// concrete type S can be resolved straight back to which field it reads,
// even when the call site's own static type is an interface S merely
// satisfies.
func accessorFields(files []*ast.File, pkgPath string) map[accessorKey]string {
	result := map[accessorKey]string{}
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			if fn.Type.Params != nil && len(fn.Type.Params.List) > 0 {
				continue // accessor takes no arguments
			}
			if len(fn.Body.List) != 1 {
				continue // single-statement body only
			}
			ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
			if !ok || len(ret.Results) != 1 {
				continue
			}
			sel, ok := unparen(ret.Results[0]).(*ast.SelectorExpr)
			if !ok {
				continue
			}
			recvIdent, ok := sel.X.(*ast.Ident)
			if !ok || len(fn.Recv.List[0].Names) != 1 || fn.Recv.List[0].Names[0].Name != recvIdent.Name {
				continue // must read a field directly off the receiver itself
			}
			recvType := simpleTypeName(fn.Recv.List[0].Type)
			if recvType == "" {
				continue
			}
			result[accessorKey{typeName: pkgPath + "." + recvType, methodName: fn.Name.Name}] = sel.Sel.Name
		}
	}
	return result
}

// factoryFieldParams finds every function shaped like `func NewX(name
// string, ...) X { return X{name: name, ...} }` (the composite literal
// need not be directly in the return statement — assigned to a local
// first, then returned, is just as common) — a constructor that stores
// one of its own parameters directly into a field of its declared return
// type. Keyed by the constructor's *types.Func, mapping field name to
// the parameter index it came from.
func factoryFieldParams(files []*ast.File, typesInfo *gotypes.Info) map[*gotypes.Func]map[string]int {
	result := map[*gotypes.Func]map[string]int{}
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Type.Results == nil || len(fn.Type.Results.List) == 0 {
				continue
			}
			returnType := simpleTypeName(fn.Type.Results.List[0].Type)
			if returnType == "" {
				continue
			}
			paramIndex := buildParamIndex(fn.Type.Params)
			if len(paramIndex) == 0 {
				continue
			}

			fields := map[string]int{}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				lit, ok := n.(*ast.CompositeLit)
				if !ok || simpleTypeName(lit.Type) != returnType {
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
					valIdent, ok := unparen(kv.Value).(*ast.Ident)
					if !ok {
						continue
					}
					if pIdx, isParam := paramIndex[valIdent.Name]; isParam {
						fields[fieldIdent.Name] = pIdx
					}
				}
				return true
			})
			if len(fields) == 0 {
				continue
			}
			funcObj, ok := typesInfo.Defs[fn.Name].(*gotypes.Func)
			if !ok {
				continue
			}
			result[funcObj] = fields
		}
	}
	return result
}

// flagDescriptorLiterals finds every package-level `var X = NewSomething(
// "literal", ...)` whose initializer calls a known factoryFieldParams
// constructor with a string-literal argument at the position a tracked
// field comes from. Keyed by the var's *types.Var, mapping field name to
// its resolved literal value — so a later `x.Accessor()` call, once
// accessorFields (keyed by x's concrete type) says Accessor reads field
// "name", can look up flagDescriptorLiterals[xVarObj]["name"] to get the
// actual string.
func flagDescriptorLiterals(files []*ast.File, typesInfo *gotypes.Info, factoryFields map[*gotypes.Func]map[string]int) map[*gotypes.Var]map[string]string {
	result := map[*gotypes.Var]map[string]string{}
	if len(factoryFields) == 0 {
		return result
	}
	for _, file := range files {
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
				for i, name := range vs.Names {
					if name.Name == "_" || i >= len(vs.Values) {
						continue
					}
					call, ok := unparen(vs.Values[i]).(*ast.CallExpr)
					if !ok {
						continue
					}
					funcObj := calleeFuncObject(call.Fun, typesInfo)
					if funcObj == nil {
						continue
					}
					fields, known := factoryFields[funcObj]
					if !known {
						continue
					}
					literals := map[string]string{}
					for field, paramIdx := range fields {
						if paramIdx >= len(call.Args) {
							continue
						}
						lit, ok := unparen(call.Args[paramIdx]).(*ast.BasicLit)
						if !ok || lit.Kind != token.STRING {
							continue
						}
						unquoted, err := strconv.Unquote(lit.Value)
						if err != nil {
							continue
						}
						literals[field] = unquoted
					}
					if len(literals) == 0 {
						continue
					}
					varObj, ok := typesInfo.Defs[name].(*gotypes.Var)
					if !ok {
						continue
					}
					result[varObj] = literals
				}
			}
		}
	}
	return result
}

// buildParamIndex maps each named parameter to its flat positional index
// (an unnamed parameter still occupies a slot), matching call.Args'
// indexing for a call to the declaring function.
func buildParamIndex(params *ast.FieldList) map[string]int {
	index := map[string]int{}
	if params == nil {
		return index
	}
	idx := 0
	for _, field := range params.List {
		if len(field.Names) == 0 {
			idx++
			continue
		}
		for _, name := range field.Names {
			if name.Name != "_" {
				index[name.Name] = idx
			}
			idx++
		}
	}
	return index
}

// findEvalSummaries is issue #26's whole-scan pre-pass. First finds every
// "direct forwarding" function (F calls its own function-typed parameter
// directly — see findDirectForwarding); then, to a fixed point (bounded,
// same safety pattern as issue #18's Pass B loop), finds every "pass-
// through" function (G calls a function with an already-known evalSummary,
// forwarding one of G's own parameters into that known function's flag-
// descriptor position — see findPassThrough). A pass-through function
// discovered in one round can itself be forwarded-through by another
// function in a later round, which is what the real motivating case
// (e2b-dev/infra) needs: a two-level wrapper chain, not just one hop.
func findEvalSummaries(pkgs []*packages.Package) map[*gotypes.Func]evalSummary {
	summaries := map[*gotypes.Func]evalSummary{}
	type decl struct {
		fn      *ast.FuncDecl
		funcObj *gotypes.Func
		typInfo *gotypes.Info
	}
	var decls []decl
	for _, pkg := range pkgs {
		if pkg.TypesInfo == nil {
			continue
		}
		for _, file := range pkg.Syntax {
			for _, d := range file.Decls {
				fn, ok := d.(*ast.FuncDecl)
				if !ok || fn.Body == nil || fn.Type.Params == nil {
					continue
				}
				funcObj, ok := pkg.TypesInfo.Defs[fn.Name].(*gotypes.Func)
				if !ok {
					continue
				}
				decls = append(decls, decl{fn: fn, funcObj: funcObj, typInfo: pkg.TypesInfo})
			}
		}
	}

	for _, d := range decls {
		paramIndex := buildParamIndex(d.fn.Type.Params)
		if len(paramIndex) < 2 {
			continue
		}
		if s, ok := findDirectForwarding(d.fn.Body, paramIndex); ok {
			summaries[d.funcObj] = s
		}
	}

	// Fixed point, bounded by the number of declarations — a real
	// pass-through chain can never be deeper than the number of functions
	// in the scan, so this can never legitimately reach the cap.
	for round := 0; round <= len(decls); round++ {
		changed := false
		for _, d := range decls {
			if _, already := summaries[d.funcObj]; already {
				continue
			}
			paramIndex := buildParamIndex(d.fn.Type.Params)
			if len(paramIndex) == 0 {
				continue
			}
			if s, ok := findPassThrough(d.fn.Body, paramIndex, d.typInfo, summaries); ok {
				summaries[d.funcObj] = s
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	return summaries
}

// findDirectForwarding looks for the first call, anywhere in body, of a
// bare identifier matching one of paramIndex's names (F calling one of
// its own parameters directly) — the callback. Among that call's own
// arguments, the first one that's either a bare identifier matching a
// *different* one of F's parameters, or a zero-argument method call on
// one of F's parameters, is taken as the flag-key source (the method
// call case records just the method *name* — sel.Sel.Name — never a
// resolved *types.Func, since the receiver's static type here is
// whatever F declared it as, often an interface with no body of its own
// to learn a field from; resolution happens later, once a concrete value
// reaches a real call site). Only the first qualifying callback call is
// used — F calling its callback parameter more than once with different
// argument shapes is an unlikely edge case not worth the extra
// complexity for this heuristic.
func findDirectForwarding(body *ast.BlockStmt, paramIndex map[string]int) (evalSummary, bool) {
	var found *evalSummary
	ast.Inspect(body, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		ident, ok := call.Fun.(*ast.Ident)
		if !ok {
			return true
		}
		calleeIdx, isParam := paramIndex[ident.Name]
		if !isParam {
			return true
		}

		candidates := map[int]keySource{}
		for i, arg := range call.Args {
			switch e := unparen(arg).(type) {
			case *ast.Ident:
				if pIdx, isP := paramIndex[e.Name]; isP && pIdx != calleeIdx {
					candidates[i] = keySource{paramIndex: pIdx}
				}
			case *ast.CallExpr:
				if len(e.Args) != 0 {
					continue
				}
				sel, ok := e.Fun.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				recvIdent, ok := sel.X.(*ast.Ident)
				if !ok {
					continue
				}
				pIdx, isP := paramIndex[recvIdent.Name]
				if !isP || pIdx == calleeIdx {
					continue
				}
				candidates[i] = keySource{paramIndex: pIdx, accessor: sel.Sel.Name}
			}
		}
		if len(candidates) == 0 {
			return true
		}
		found = &evalSummary{sdkFromParam: calleeIdx, keyCandidates: candidates}
		return false
	})
	if found == nil {
		return evalSummary{}, false
	}
	return *found, true
}

// findPassThrough looks for the first call, anywhere in body, to a
// function H that already has a known evalSummary (from a prior round —
// possibly itself a pass-through, letting a chain deepen one hop per
// round). For that call to make G (body's own function) a pass-through
// too: H's SDK identity must be resolvable at THIS call site (either H
// already has a fixed one, or this call supplies a concrete, already-
// resolved method value — never one of G's own parameters, since G
// having its own separate callback parameter is a different, unhandled
// shape), and H's flag-descriptor argument at this call must be a bare
// reference to one of G's own parameters (G forwarding its own
// descriptor parameter straight through, unchanged).
func findPassThrough(body *ast.BlockStmt, paramIndex map[string]int, typesInfo *gotypes.Info, summaries map[*gotypes.Func]evalSummary) (evalSummary, bool) {
	var found *evalSummary
	ast.Inspect(body, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		calleeObj := calleeFuncObject(call.Fun, typesInfo)
		if calleeObj == nil {
			return true
		}
		hs, known := summaries[calleeObj]
		if !known {
			return true
		}

		var sdkVersion, sdkMethod string
		var spec methodSpec
		if hs.sdkFromParam >= 0 {
			if hs.sdkFromParam >= len(call.Args) {
				return true
			}
			sel, ok := unparen(call.Args[hs.sdkFromParam]).(*ast.SelectorExpr)
			if !ok {
				return true
			}
			v, ok := resolveByStaticType(sel.X, typesInfo)
			if !ok {
				return true
			}
			var known bool
			spec, known = methodSpecs[sel.Sel.Name]
			if !known {
				return true
			}
			sdkVersion, sdkMethod = v, sel.Sel.Name
		} else {
			sdkVersion, sdkMethod = hs.sdkVersion, hs.sdkMethod
			var known bool
			spec, known = methodSpecs[sdkMethod]
			if !known {
				return true
			}
		}

		src, hasKey := hs.keyCandidates[spec.keyArgIndex]
		if !hasKey || src.paramIndex >= len(call.Args) {
			return true
		}
		argIdent, ok := unparen(call.Args[src.paramIndex]).(*ast.Ident)
		if !ok {
			return true
		}
		gParamIdx, isParam := paramIndex[argIdent.Name]
		if !isParam {
			return true
		}

		found = &evalSummary{
			sdkFromParam: -1,
			sdkVersion:   sdkVersion,
			sdkMethod:    sdkMethod,
			keyCandidates: map[int]keySource{
				spec.keyArgIndex: {paramIndex: gParamIdx, accessor: src.accessor},
			},
		}
		return false
	})
	if found == nil {
		return evalSummary{}, false
	}
	return *found, true
}

// calleeFuncObject resolves a call expression's callee to the *types.Func
// it refers to, for both a bare identifier call (F(...)) and a qualified
// one (pkg.F(...)) — or nil for anything else (a method value, a
// function literal, an unresolvable expression).
func calleeFuncObject(fun ast.Expr, info *gotypes.Info) *gotypes.Func {
	switch fn := unparen(fun).(type) {
	case *ast.Ident:
		obj, _ := info.Uses[fn].(*gotypes.Func)
		return obj
	case *ast.SelectorExpr:
		obj, _ := info.Uses[fn.Sel].(*gotypes.Func)
		return obj
	default:
		return nil
	}
}

// namedTypeName returns t's package-qualified name (pkgPath + "." +
// name) if it (or the type it points to) is a defined (named) type —
// "" otherwise. Used to resolve a concrete value's type name for an
// accessorKey lookup — package-qualified to match accessorFields' own
// keying, so two unrelated same-named types in different packages of the
// scanned repo can never collide.
func namedTypeName(t gotypes.Type) string {
	if ptr, ok := t.(*gotypes.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*gotypes.Named)
	if !ok {
		return ""
	}
	pkg := named.Obj().Pkg()
	if pkg == nil {
		return ""
	}
	return pkg.Path() + "." + named.Obj().Name()
}

// resolveFlagDescriptorKey resolves the static flag key for arg (the
// value passed at a call site for a function's flag-descriptor
// parameter), given the accessor method name whose return value is the
// key — "" means arg's own value IS the key already (a plain string
// literal). Supports arg being a bare reference to a known
// flagDescriptorLiterals var, or a direct inline call to a known
// factoryFieldParams constructor with a literal argument.
func resolveFlagDescriptorKey(arg ast.Expr, accessorMethod string, typesInfo *gotypes.Info, accessors map[accessorKey]string, literalVars map[*gotypes.Var]map[string]string, factoryFields map[*gotypes.Func]map[string]int) (string, bool) {
	arg = unparen(arg)
	if accessorMethod == "" {
		lit, ok := arg.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return "", false
		}
		unquoted, err := strconv.Unquote(lit.Value)
		if err != nil {
			return "", false
		}
		return unquoted, true
	}

	switch e := arg.(type) {
	case *ast.Ident:
		varObj, ok := typesInfo.Uses[e].(*gotypes.Var)
		if !ok {
			return "", false
		}
		typeName := namedTypeName(varObj.Type())
		if typeName == "" {
			return "", false
		}
		field, known := accessors[accessorKey{typeName: typeName, methodName: accessorMethod}]
		if !known {
			return "", false
		}
		literals, known := literalVars[varObj]
		if !known {
			return "", false
		}
		val, known := literals[field]
		return val, known
	case *ast.CallExpr:
		funcObj := calleeFuncObject(e.Fun, typesInfo)
		if funcObj == nil {
			return "", false
		}
		sig, ok := funcObj.Type().(*gotypes.Signature)
		if !ok || sig.Results().Len() == 0 {
			return "", false
		}
		typeName := namedTypeName(sig.Results().At(0).Type())
		if typeName == "" {
			return "", false
		}
		field, known := accessors[accessorKey{typeName: typeName, methodName: accessorMethod}]
		if !known {
			return "", false
		}
		fields, known := factoryFields[funcObj]
		if !known {
			return "", false
		}
		paramIdx, known := fields[field]
		if !known || paramIdx >= len(e.Args) {
			return "", false
		}
		lit, ok := unparen(e.Args[paramIdx]).(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return "", false
		}
		unquoted, err := strconv.Unquote(lit.Value)
		if err != nil {
			return "", false
		}
		return unquoted, true
	default:
		return "", false
	}
}

// detectForwardingCallUsages walks every loaded package's syntax for
// calls to a known evalSummary function (findEvalSummaries) whose
// resolved SDK identity and flag key are both provable — issue #26. A
// dynamic (non-statically-resolvable) key is a deliberate miss, not a
// bug: this trades recall for confidence given the heuristic's
// structural, non-general nature (see ADR 006's "what this does not
// catch").
func detectForwardingCallUsages(fset *token.FileSet, pkgs []*packages.Package, absRoot string, summaries map[*gotypes.Func]evalSummary, accessors map[accessorKey]string, literalVars map[*gotypes.Var]map[string]string, factoryFields map[*gotypes.Func]map[string]int) []types.FlagUsage {
	var usages []types.FlagUsage
	if len(summaries) == 0 {
		return usages
	}

	for _, pkg := range pkgs {
		if pkg.TypesInfo == nil {
			continue
		}
		for _, file := range pkg.Syntax {
			filePos := fset.Position(file.Pos())
			if filePos.Filename == "" {
				continue
			}
			relPath, err := filepath.Rel(absRoot, filePos.Filename)
			if err != nil {
				continue
			}

			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				funcObj := calleeFuncObject(call.Fun, pkg.TypesInfo)
				if funcObj == nil {
					return true
				}
				s, known := summaries[funcObj]
				if !known {
					return true
				}

				var version, method string
				if s.sdkFromParam >= 0 {
					if s.sdkFromParam >= len(call.Args) {
						return true
					}
					sel, ok := unparen(call.Args[s.sdkFromParam]).(*ast.SelectorExpr)
					if !ok {
						return true
					}
					v, ok := resolveByStaticType(sel.X, pkg.TypesInfo)
					if !ok {
						return true
					}
					if _, known := methodSpecs[sel.Sel.Name]; !known {
						return true
					}
					version, method = v, sel.Sel.Name
				} else {
					version, method = s.sdkVersion, s.sdkMethod
				}

				spec, known := methodSpecs[method]
				if !known {
					return true
				}
				src, hasKey := s.keyCandidates[spec.keyArgIndex]
				if !hasKey || src.paramIndex >= len(call.Args) {
					return true
				}
				flagKey, ok := resolveFlagDescriptorKey(call.Args[src.paramIndex], src.accessor, pkg.TypesInfo, accessors, literalVars, factoryFields)
				if !ok {
					return true
				}

				pos := fset.Position(call.Pos())
				callType := types.CallType(method)
				usages = append(usages, types.FlagUsage{
					FlagKey:          flagKey,
					IsDynamic:        false,
					File:             relPath,
					Line:             pos.Line,
					Column:           pos.Column,
					CallType:         callType,
					Fingerprint:      fingerprint.Generate(flagKey, callType, relPath, nil),
					StalenessSignals: []types.StalenessSignal{},
					Language:         "go",
					SDK:              sdkName(version),
					Risk:             riskFor(spec, false),
					DetectedBy:       "strict-types",
				})
				return true
			})
		}
	}
	return usages
}
