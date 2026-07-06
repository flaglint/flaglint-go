package readiness

import (
	"testing"

	"github.com/flaglint/flaglint-go/internal/types"
)

func usage(risk types.Risk) types.FlagUsage {
	return types.FlagUsage{Risk: risk}
}

func TestCompute_noUsages(t *testing.T) {
	r := Compute(nil)
	if r.Grade != GradeNotApplicable {
		t.Errorf("Grade = %q, want not-applicable", r.Grade)
	}
	if r.Score != nil {
		t.Errorf("Score = %v, want nil", r.Score)
	}
}

func TestCompute_allLowRisk(t *testing.T) {
	r := Compute([]types.FlagUsage{usage(types.RiskLow), usage(types.RiskLow)})
	if r.Score == nil || *r.Score != 100 {
		t.Fatalf("Score = %v, want 100", r.Score)
	}
	if r.Grade != GradeReady {
		t.Errorf("Grade = %q, want ready", r.Grade)
	}
}

func TestCompute_gradeThresholds(t *testing.T) {
	tests := []struct {
		name       string
		low, total int
		wantGrade  Grade
	}{
		{"exactly 80 is ready", 8, 10, GradeReady},
		{"just under 80 is moderate", 79, 100, GradeModerate},
		{"exactly 50 is moderate", 5, 10, GradeModerate},
		{"just under 50 is complex", 49, 100, GradeComplex},
		{"zero low risk is complex", 0, 10, GradeComplex},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usages := make([]types.FlagUsage, tt.total)
			for i := 0; i < tt.low; i++ {
				usages[i] = usage(types.RiskLow)
			}
			for i := tt.low; i < tt.total; i++ {
				usages[i] = usage(types.RiskHigh)
			}
			r := Compute(usages)
			if r.Grade != tt.wantGrade {
				t.Errorf("Grade = %q, want %q (score=%v)", r.Grade, tt.wantGrade, r.Score)
			}
		})
	}
}

func TestCompute_riskBreakdown(t *testing.T) {
	r := Compute([]types.FlagUsage{
		usage(types.RiskLow), usage(types.RiskLow),
		usage(types.RiskMedium),
		usage(types.RiskHigh), usage(types.RiskHigh), usage(types.RiskHigh),
	})
	if r.TotalCalls != 6 || r.LowRiskCalls != 2 || r.MediumRiskCalls != 1 || r.HighRiskCalls != 3 {
		t.Errorf("got %+v, want total=6 low=2 medium=1 high=3", r)
	}
}
