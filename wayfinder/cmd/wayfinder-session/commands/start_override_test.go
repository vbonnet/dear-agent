package commands

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/internal/override"
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
