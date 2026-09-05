package compaction

import (
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

func TestValidateReadyRejectsCompatibilityDone(t *testing.T) {
	err := ValidateReady(session.DetectionResult{
		State:    manifest.StateDone,
		Evidence: session.EvidenceLive,
	})
	if err == nil {
		t.Fatal("ValidateReady() error = nil for DONE display projection")
	}
}
