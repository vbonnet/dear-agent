//go:build integration

package helpers

import (
	"strings"
	"testing"
)

// The isolated environment must declare the spawn circuit breaker's
// host-resource gates, not inherit them.
//
// Their defaults are derived from the machine (MaxLoad5 is NumCPU*2), so on a
// shared CI runner building and testing in parallel the five-minute load
// average routinely exceeds the threshold and the breaker refuses the spawn.
// The lifecycle tests then fail with "circuit breaker: spawn refused (load
// level: GREEN)" for a reason that has nothing to do with the path under test.
// Observed on ubuntu-latest at load 11.7 against a threshold of 8.
//
// This asserts the declaration rather than the outcome, because the outcome
// depends on the load of whatever machine runs the test, which is the very
// dependency being removed.
func TestIsolatedEnvironmentDeclaresSpawnResourceGates(t *testing.T) {
	env := NewIsolatedEnvironment(t)

	declared := map[string]string{}
	for _, entry := range env.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if ok {
			declared[name] = value
		}
	}

	for _, name := range []string{
		"AGM_MAX_LOAD5",
		"AGM_MIN_FREE_MEM_PCT",
		"AGM_MIN_FREE_DISK_GB",
		"AGM_MAX_AGENT_PROCS",
	} {
		if declared[name] == "" {
			t.Errorf("%s is not declared; the spawn breaker would inherit this host's "+
				"resource state and refuse under unrelated load", name)
		}
	}
}
