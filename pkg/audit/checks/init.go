package checks

import "github.com/vbonnet/dear-agent/pkg/audit"

// Refiners are registered separately from checks because they have
// no init-order dependency on the registry being ready (there is
// none — the registry is package-level mutable state). We register
// at package init for symmetry with checks; tests that want a clean
// universe build their own *audit.Registry.
func init() {
	if err := audit.Default.RegisterRefiner(LintGapRefiner{}); err != nil {
		panic(err)
	}
}
