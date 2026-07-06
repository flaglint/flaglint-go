package fixtures

// Different file, same package as positive_multilevel_types.go — the
// struct/field declarations live in one file, the real two-level chain
// usage in another. Proves struct field types are resolved whole-scan
// (across files in the same package), not just within one file.
func newMultiLevelFeatureFlag(integ *MultiLevelIntegration) *MultiLevelFeatureFlag {
	return &MultiLevelFeatureFlag{integ: integ}
}

func (f *MultiLevelFeatureFlag) Evaluate() bool {
	v, _ := f.integ.ldClient.BoolVariation("multilevel-flag", nil, false)
	return v
}
