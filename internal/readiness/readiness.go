// Package readiness computes a simple, honest risk-readiness signal from
// scan results. Unlike flaglint-js's readiness score (which measures
// migration automatability — flaglint-go does not migrate Go code yet, see
// ADR 002's Phase 1 scope), this measures what fraction of detected call
// sites are low risk (a static flag key on a simple Variation method) —
// the ones that would be trivially safe to migrate once migration ships.
// No magic weights: score is a plain percentage.
package readiness

import "github.com/flaglint/flaglint-go/internal/types"

type Grade string

const (
	GradeReady         Grade = "ready"
	GradeModerate      Grade = "moderate"
	GradeComplex       Grade = "complex"
	GradeNotApplicable Grade = "not-applicable"
)

type Report struct {
	Score           *int  `json:"score"` // nil when there are no usages to score
	Grade           Grade `json:"grade"`
	TotalCalls      int   `json:"totalCalls"`
	LowRiskCalls    int   `json:"lowRiskCalls"`
	MediumRiskCalls int   `json:"mediumRiskCalls"`
	HighRiskCalls   int   `json:"highRiskCalls"`
}

// Compute derives a Report from usages. Score is (lowRiskCalls / totalCalls)
// * 100, nil/not-applicable when there are no usages at all.
func Compute(usages []types.FlagUsage) Report {
	r := Report{}
	for _, u := range usages {
		r.TotalCalls++
		switch u.Risk {
		case types.RiskLow:
			r.LowRiskCalls++
		case types.RiskMedium:
			r.MediumRiskCalls++
		case types.RiskHigh:
			r.HighRiskCalls++
		}
	}

	if r.TotalCalls == 0 {
		r.Grade = GradeNotApplicable
		return r
	}

	score := r.LowRiskCalls * 100 / r.TotalCalls
	r.Score = &score
	r.Grade = gradeFromScore(score)
	return r
}

func gradeFromScore(score int) Grade {
	switch {
	case score >= 80:
		return GradeReady
	case score >= 50:
		return GradeModerate
	default:
		return GradeComplex
	}
}
