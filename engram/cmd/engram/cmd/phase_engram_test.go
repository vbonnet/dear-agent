package cmd

import (
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/phaseengram"
)

func TestPhaseEngramHelpUsesCanonicalRegistry(t *testing.T) {
	for _, phase := range phaseengram.KnownPhases() {
		if !strings.Contains(phaseEngramCmd.Long, phase) {
			t.Errorf("phase-engram help omits canonical phase %q", phase)
		}
	}
	if strings.Contains(phaseEngramCmd.Long, "DECISION") {
		t.Fatal("phase-engram help advertises retired DECISION phase")
	}
}
