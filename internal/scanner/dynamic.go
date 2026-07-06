package scanner

import (
	"go/ast"
	"go/token"
	"strconv"
)

// extractFlagKey mirrors flaglint-js's extractFlagKey (src/scanner/index.ts):
// a string-literal argument yields its literal value; anything else
// (identifier, fmt.Sprintf call, string concatenation, missing argument)
// yields the fixed placeholder "dynamic" with isDynamic=true. The literal
// string "dynamic" (not the source expression text) is intentional — it is
// the cross-tool contract's placeholder value, matching flaglint-js exactly.
func extractFlagKey(arg ast.Expr) (flagKey string, isDynamic bool) {
	if arg == nil {
		return "dynamic", true
	}
	lit, ok := arg.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "dynamic", true
	}
	unquoted, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "dynamic", true
	}
	return unquoted, false
}
