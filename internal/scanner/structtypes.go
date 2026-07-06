package scanner

import "go/ast"

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
