// Package specgovernance owns repository-side SPEC-governance integration data.
package specgovernance

import "slices"

var canonicalNames = [...]string{"audit-specs", "write-spec"}

// Names returns the complete ordered canonical skill-name set.
func Names() []string {
	return slices.Clone(canonicalNames[:])
}

// NativeExports returns the complete ordered Claude plugin skill export set.
func NativeExports() []string {
	exports := make([]string, len(canonicalNames))
	for index, name := range canonicalNames {
		exports[index] = "./skills/" + name
	}
	return exports
}
