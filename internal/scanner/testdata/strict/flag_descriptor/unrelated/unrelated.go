// Package unrelated deliberately declares a type named exactly "BoolFlag"
// with a method also named "Key" — same names as the real flag
// descriptor in the parent package, but reading a *different* field
// ("identifier", not "name"). Proves accessorKey's package-qualification
// (found via independent review of an earlier, unqualified version of
// this mechanism) actually keeps the two apart, rather than one
// package's entry silently overwriting the other's in the shared
// cross-package accessorFields index — which would otherwise make
// TestScanStrict_positiveFlagDescriptorChain's result depend on Go's
// randomized map iteration order.
package unrelated

type BoolFlag struct {
	identifier string
	value      bool
}

func (f BoolFlag) Key() string    { return f.identifier }
func (f BoolFlag) Fallback() bool { return f.value }

func NewBoolFlag(identifier string, value bool) BoolFlag {
	return BoolFlag{identifier: identifier, value: value}
}

var UnrelatedConfigFlag = NewBoolFlag("unrelated-config-flag", false)
