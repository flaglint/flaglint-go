package fingerprint

import (
	"testing"

	"github.com/flaglint/flaglint-go/internal/types"
)

func TestGenerate(t *testing.T) {
	tests := []struct {
		name         string
		flagKey      string
		callType     types.CallType
		filePath     string
		dynamicIndex *int
		want         string
	}{
		{
			name:     "static key",
			flagKey:  "checkout-v2",
			callType: types.CallTypeBoolVariation,
			filePath: "services/checkout/flags.go",
			want:     "launchdarkly:BoolVariation:checkout-v2:services/checkout/flags.go",
		},
		{
			name:     "windows path separators normalized",
			flagKey:  "checkout-v2",
			callType: types.CallTypeBoolVariation,
			filePath: `services\checkout\flags.go`,
			want:     "launchdarkly:BoolVariation:checkout-v2:services/checkout/flags.go",
		},
		{
			name:     "leading dot-slash stripped",
			flagKey:  "checkout-v2",
			callType: types.CallTypeBoolVariation,
			filePath: "./flags.go",
			want:     "launchdarkly:BoolVariation:checkout-v2:flags.go",
		},
		{
			name:     "wildcard flag key for bulk call",
			flagKey:  "*",
			callType: types.CallTypeAllFlagsState,
			filePath: "flags.go",
			want:     "launchdarkly:AllFlagsState:*:flags.go",
		},
		{
			name:     "empty flag key collapses to wildcard",
			flagKey:  "",
			callType: types.CallTypeAllFlagsState,
			filePath: "flags.go",
			want:     "launchdarkly:AllFlagsState:*:flags.go",
		},
		{
			name:         "dynamic index appended",
			flagKey:      "checkout-v2",
			callType:     types.CallTypeStringVariation,
			filePath:     "flags.go",
			dynamicIndex: intPtr(0),
			want:         "launchdarkly:StringVariation:checkout-v2:flags.go:0",
		},
		{
			name:         "second dynamic index in same file",
			flagKey:      "checkout-v2",
			callType:     types.CallTypeStringVariation,
			filePath:     "flags.go",
			dynamicIndex: intPtr(1),
			want:         "launchdarkly:StringVariation:checkout-v2:flags.go:1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Generate(tt.flagKey, tt.callType, tt.filePath, tt.dynamicIndex)
			if got != tt.want {
				t.Errorf("Generate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func intPtr(i int) *int { return &i }
