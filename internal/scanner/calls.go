package scanner

import "github.com/flaglint/flaglint-go/internal/types"

// methodSpec describes one detected LaunchDarkly Go SDK method: its risk
// classification (ADR 002) and which call argument holds the flag key.
// keyArgIndex is -1 for bulk calls that take no flag key (AllFlagsState).
type methodSpec struct {
	risk        types.Risk
	keyArgIndex int
}

// methodSpecs covers github.com/launchdarkly/go-server-sdk v6 and v7 —
// verified directly against both modules' real API surface (go doc), not
// assumed from the SDK reference docs. Both major versions expose the same
// method set for Phase 1's purposes; v7 additionally has the *Ctx variants
// (a leading context.Context makes the flag key the second argument, not
// the first) and MigrationVariation, which is out of scope until a
// migration-specific ADR (see ADR 002).
var methodSpecs = map[string]methodSpec{
	"BoolVariation":    {types.RiskLow, 0},
	"StringVariation":  {types.RiskLow, 0},
	"IntVariation":     {types.RiskLow, 0},
	"Float64Variation": {types.RiskLow, 0},
	"JSONVariation":    {types.RiskMedium, 0},

	"BoolVariationCtx":    {types.RiskLow, 1},
	"StringVariationCtx":  {types.RiskLow, 1},
	"IntVariationCtx":     {types.RiskLow, 1},
	"Float64VariationCtx": {types.RiskLow, 1},
	"JSONVariationCtx":    {types.RiskMedium, 1},

	"BoolVariationDetail":       {types.RiskHigh, 0},
	"StringVariationDetail":     {types.RiskHigh, 0},
	"IntVariationDetail":        {types.RiskHigh, 0},
	"Float64VariationDetail":    {types.RiskHigh, 0},
	"JSONVariationDetail":       {types.RiskHigh, 0},
	"BoolVariationDetailCtx":    {types.RiskHigh, 1},
	"StringVariationDetailCtx":  {types.RiskHigh, 1},
	"IntVariationDetailCtx":     {types.RiskHigh, 1},
	"Float64VariationDetailCtx": {types.RiskHigh, 1},
	"JSONVariationDetailCtx":    {types.RiskHigh, 1},

	"AllFlagsState": {types.RiskHigh, -1},
}

// riskFor combines a method's static risk classification with the dynamic-
// key rule from ADR 002's risk table: a dynamic flag key is high risk
// regardless of which method it's on, since it "cannot resolve statically"
// no matter how simple the underlying method otherwise is. Without this,
// e.g. a dynamic-key BoolVariation call would be scored low risk purely
// from its method table entry, silently inflating an audit's readiness
// score with exactly the calls that most need manual review.
func riskFor(spec methodSpec, isDynamic bool) types.Risk {
	if isDynamic {
		return types.RiskHigh
	}
	return spec.risk
}

func sdkName(version string) string {
	return "go-server-sdk-" + version
}
