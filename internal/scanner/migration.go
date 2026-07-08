// Package scanner (this file): builds MigrationInventoryItem records —
// migrationInventory is a richer, migration-focused view of a call site
// than FlagUsage, additive to flaglint-go's JSON output for cross-tool
// parity with flaglint-js (see docs/adr/003-cross-tool-contract.md and
// flaglint-js's src/scanner/index.ts, which this deliberately mirrors
// field-for-field). flaglint-go has no `migrate` command yet (ADR 002's
// Phase 1 scope), so nothing in this package *consumes* these records —
// they exist so a future migrate command (or another tool reading this
// JSON) has the same rewrite-relevant detail flaglint-js already provides.
package scanner

import (
	"go/ast"
	"go/token"

	"github.com/flaglint/flaglint-go/internal/types"
)

// migrationValueTypes maps each known LaunchDarkly Go SDK method name to
// the OpenFeature evaluation-method category its return value maps to.
// Unlike flaglint-js (whose generic variation()/isFeatureEnabled() calls
// require inferring this from the fallback argument's runtime type), every
// Go SDK method name already fully determines this — no fallback
// inspection needed, and "unknown-fallback" (a real flaglint-js reason)
// can never actually be produced here as a result.
var migrationValueTypes = map[string]types.MigrationValueType{
	"BoolVariation": types.MigrationValueBoolean, "BoolVariationCtx": types.MigrationValueBoolean,
	"BoolVariationDetail": types.MigrationValueBoolean, "BoolVariationDetailCtx": types.MigrationValueBoolean,
	"StringVariation": types.MigrationValueString, "StringVariationCtx": types.MigrationValueString,
	"StringVariationDetail": types.MigrationValueString, "StringVariationDetailCtx": types.MigrationValueString,
	"IntVariation": types.MigrationValueNumber, "IntVariationCtx": types.MigrationValueNumber,
	"IntVariationDetail": types.MigrationValueNumber, "IntVariationDetailCtx": types.MigrationValueNumber,
	"Float64Variation": types.MigrationValueNumber, "Float64VariationCtx": types.MigrationValueNumber,
	"Float64VariationDetail": types.MigrationValueNumber, "Float64VariationDetailCtx": types.MigrationValueNumber,
	"JSONVariation": types.MigrationValueObject, "JSONVariationCtx": types.MigrationValueObject,
	"JSONVariationDetail": types.MigrationValueObject, "JSONVariationDetailCtx": types.MigrationValueObject,
}

// migrationDetailMethods is every "*VariationDetail(Ctx)" method name —
// always manual-review (returns ldreason.EvaluationDetail, no direct
// OpenFeature equivalent), matching flaglint-js's detail-method reason.
var migrationDetailMethods = map[string]bool{
	"BoolVariationDetail": true, "BoolVariationDetailCtx": true,
	"StringVariationDetail": true, "StringVariationDetailCtx": true,
	"IntVariationDetail": true, "IntVariationDetailCtx": true,
	"Float64VariationDetail": true, "Float64VariationDetailCtx": true,
	"JSONVariationDetail": true, "JSONVariationDetailCtx": true,
}

// exprText extracts node's exact source text and byte-offset range from
// src, using fset to convert AST positions to offsets. Returns ok == false
// for a nil node or a position outside src's bounds (never expected in
// practice — pos/end always come from nodes parsed out of src itself — but
// checked rather than assumed, since a malformed range would otherwise
// panic on the slice below).
func exprText(fset *token.FileSet, src []byte, node ast.Expr) (text string, start, end int, ok bool) {
	if node == nil {
		return "", 0, 0, false
	}
	startPos := fset.Position(node.Pos())
	endPos := fset.Position(node.End())
	if startPos.Offset < 0 || endPos.Offset > len(src) || startPos.Offset > endPos.Offset {
		return "", 0, 0, false
	}
	return string(src[startPos.Offset:endPos.Offset]), startPos.Offset, endPos.Offset, true
}

// buildMigrationInventoryItem builds one MigrationInventoryItem for call,
// a confirmed LaunchDarkly Go SDK call site (spec/callTypeName already
// resolved by the caller — see fileDetector.detect, scanner.go). src is
// this file's raw source bytes (parsedFile.src), needed to extract each
// expression's exact source text alongside its already-parsed AST shape.
func buildMigrationInventoryItem(fset *token.FileSet, src []byte, relPath string, call *ast.CallExpr, spec methodSpec, callTypeName string, pos token.Position, flagKey string, isDynamic bool) types.MigrationInventoryItem {
	item := types.MigrationInventoryItem{
		File:               relPath,
		Line:               pos.Line,
		Column:             pos.Column,
		LaunchDarklyMethod: types.CallType(callTypeName),
		IsDynamic:          isDynamic,
	}
	if text, start, end, ok := exprText(fset, src, call); ok {
		item.CallExpression = text
		item.RangeStart = start
		item.RangeEnd = end
	}

	// Bulk call (AllFlagsState) — no flag key; args[0] is the evaluation
	// context, matching flaglint-js's identical handling of its bulk-call
	// equivalent.
	if spec.keyArgIndex == -1 {
		item.ValueType = types.MigrationValueUnknown
		if len(call.Args) > 0 {
			if text, _, _, ok := exprText(fset, src, call.Args[0]); ok {
				item.EvaluationContextExpression = text
			}
		}
		item.ManualReviewReason = types.MigrationReasonBulkInventoryCall
		return item
	}

	if spec.keyArgIndex < len(call.Args) {
		if text, _, _, ok := exprText(fset, src, call.Args[spec.keyArgIndex]); ok {
			item.FlagKeyExpression = text
		}
	}
	if !isDynamic {
		item.StaticFlagKey = flagKey
	}
	if ctxIdx := spec.keyArgIndex + 1; ctxIdx < len(call.Args) {
		if text, _, _, ok := exprText(fset, src, call.Args[ctxIdx]); ok {
			item.EvaluationContextExpression = text
		}
	}
	if fallbackIdx := spec.keyArgIndex + 2; fallbackIdx < len(call.Args) {
		if text, _, _, ok := exprText(fset, src, call.Args[fallbackIdx]); ok {
			item.FallbackExpression = text
		}
	}

	item.ValueType = migrationValueTypes[callTypeName]

	switch {
	case isDynamic:
		item.ManualReviewReason = types.MigrationReasonDynamicKey
	case migrationDetailMethods[callTypeName]:
		item.ManualReviewReason = types.MigrationReasonDetailMethod
	default:
		item.SafelyAutomatable = true
	}

	return item
}
