package tofuimport

import "strings"

// benignImportFailures are the provider messages that mean "the remote object
// does not exist yet", so `tofu plan` will correctly propose creating it.
//
//	"not found" / "404" : repository or Dependabot configuration absent
//	"associated"        : no security configuration associated
//
// Anything else is a real failure. Treating an unrecognized message as benign
// would let the import loop continue past a genuine error and leave a
// partially imported state.
var benignImportFailures = []string{"not found", "404", "associated"}

// IsBenignImportFailure reports whether a failed `tofu import` means the remote
// object is merely absent.
func IsBenignImportFailure(providerOutput string) bool {
	lowered := strings.ToLower(providerOutput)
	for _, phrase := range benignImportFailures {
		if strings.Contains(lowered, phrase) {
			return true
		}
	}
	return false
}
