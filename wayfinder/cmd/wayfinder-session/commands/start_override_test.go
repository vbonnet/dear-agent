package commands

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/internal/override"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// newStartCmdWithFlags builds a minimal cobra.Command that has all the flags
// registered by StartCmd's init(), so runStart can read them without panicking.
func newStartCmdWithFlags() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().String("reason", "", "")
	cmd.Flags().String("project-type", "feature", "")
	cmd.Flags().String("risk-level", "M", "")
	cmd.Flags().Bool("skip-roadmap", false, "")
	return cmd
}

// TestStartForce_NoReason verifies --force without --reason is denied before any I/O.
func TestStartForce_NoReason(t *testing.T) {
	cmd := newStartCmdWithFlags()
	if err := cmd.Flags().Set("force", "true"); err != nil {
		t.Fatal(err)
	}
	// reason left as empty string

	err := runStart(cmd, []string{"my-project"})
	var denied *override.DeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("expected *override.DeniedError, got %T: %v", err, err)
	}
}

// TestStartForce_GoodReason verifies the guard configuration permits a valid reason.
func TestStartForce_GoodReason(t *testing.T) {
	err := override.Require(context.Background(), override.Guard{
		Tool: "wayfinder-session start",
		Flag: "--force",
		Gate: "overwrite-existing wayfinder session",
		Risk: override.RiskP2,
	}, "session file is corrupted and needs to be re-created from scratch")
	if err != nil {
		t.Fatalf("guard should allow a valid reason, got: %v", err)
	}
}

func TestShouldEnsureSessionBead(t *testing.T) {
	tests := []struct {
		name  string
		st    *status.StatusV2
		phase string
		want  bool
	}{
		{name: "setup", st: &status.StatusV2{}, phase: status.WaypointV2Setup, want: true},
		{name: "build after skipped setup", st: &status.StatusV2{SkipRoadmap: true}, phase: status.WaypointV2Build, want: true},
		{name: "ordinary build", st: &status.StatusV2{}, phase: status.WaypointV2Build, want: false},
		{name: "earlier phase", st: &status.StatusV2{SkipRoadmap: true}, phase: status.WaypointV2Plan, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldEnsureSessionBead(tt.st, tt.phase); got != tt.want {
				t.Fatalf("shouldEnsureSessionBead(%q) = %v, want %v", tt.phase, got, tt.want)
			}
		})
	}
}
