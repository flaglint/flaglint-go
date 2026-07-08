package scanner

import (
	"go/ast"
	"go/token"
)

// collectStructFieldTypes walks top-level `type X struct { ... }`
// declarations in file and returns "StructName.FieldName" -> the field's
// declared type name (via simpleTypeName). This is collected whole-scan
// (merged across every file — see Scan) because a struct's field types
// are frequently declared in a different file than any chain that walks
// through them: found missing during field-testing against weaviate,
// where the LDIntegration struct (and its ldClient field) is declared in
// one file, while `f.ldInteg.ldClient.StringVariation(...)` — a two-level
// chain — is used from a different file in the same package.
func collectStructFieldTypes(file *ast.File) map[string]string {
	types := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok || st.Fields == nil {
				continue
			}
			for _, f := range st.Fields.List {
				fieldType := simpleTypeName(f.Type)
				if fieldType == "" {
					continue
				}
				if len(f.Names) == 0 {
					// Embedded field: Go refers to it by its type name
					// (e.g. `x.LDIntegration.ldClient` when LDIntegration
					// is embedded without an explicit field name).
					types[ts.Name.Name+"."+fieldType] = fieldType
					continue
				}
				for _, name := range f.Names {
					types[ts.Name.Name+"."+name.Name] = fieldType
				}
			}
		}
	}
	return types
}

// collectPackageVarTypes returns "VarName" -> declared/inferred type name
// for every package-level `var` declaration in file — either from an
// explicit type annotation (`var svc Svc`) or inferred from a composite-
// literal initializer's own type (`var svc = &Svc{...}` or `var svc =
// Svc{...}` — ast.Inspect finds the literal either way, regardless of
// address-of wrapping).
//
// Found missing during corpus testing (flaglint/corpus:
// composite-literal-binding): resolveChainType (identity.go) only ever
// consulted the enclosing function's own declared parameter/receiver
// types for a bare identifier — a chain rooted at a *package-level*
// variable (`svc.Client.BoolVariation(...)` called from an ordinary
// function, not a method on Svc) always came up empty, since nothing
// recorded what type "svc" itself was.
func collectPackageVarTypes(file *ast.File) map[string]string {
	types := map[string]string{}
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
			if vs.Type != nil {
				typeName := simpleTypeName(vs.Type)
				if typeName == "" {
					continue
				}
				for _, name := range vs.Names {
					if name.Name != "_" {
						types[name.Name] = typeName
					}
				}
				continue
			}
			for i, name := range vs.Names {
				if name.Name == "_" || i >= len(vs.Values) {
					continue
				}
				var lit *ast.CompositeLit
				ast.Inspect(vs.Values[i], func(n ast.Node) bool {
					if lit != nil {
						return false
					}
					if l, ok := n.(*ast.CompositeLit); ok {
						lit = l
						return false
					}
					return true
				})
				if lit == nil {
					continue
				}
				typeName := simpleTypeName(lit.Type)
				if typeName == "" {
					continue
				}
				types[name.Name] = typeName
			}
		}
	}
	return types
}
