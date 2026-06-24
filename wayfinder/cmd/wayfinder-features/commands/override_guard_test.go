package commands

import (
	"context"
	"errors"
	"testing"

	"github.com/vbonnet/dear-agent/internal/override"
)

func helperSetBool(t *testing.T, p *bool, v bool) {
	t.Helper()
	old := *p
	*p = v
	t.Cleanup(func() { *p = old })
}

func helperSetString(t *testing.T, p *string, v string) {
	t.Helper()
	old := *p
	*p = v
	t.Cleanup(func() { *p = old })
}

func isDeniedError(err error) bool {
	var d *override.DeniedError
	return errors.As(err, &d)
}

// --- wayfinder-features init --force ---

// TestInitForce_NoReason verifies --force without --reason is denied before any I/O.
func TestInitForce_NoReason(t *testing.T) {
	helperSetBool(t, &initForce, true)
	helperSetString(t, &initReason, "")

	err := runInit(InitCmd, []string{})
	if !isDeniedError(err) {
		t.Fatalf("expected *override.DeniedError, got %T: %v", err, err)
	}
}

// TestInitForce_GoodReason verifies the guard configuration permits a valid reason.
func TestInitForce_GoodReason(t *testing.T) {
	helperSetBool(t, &initForce, true)
	helperSetString(t, &initReason, "")

	err := override.Require(context.Background(), override.Guard{
		Tool: "wayfinder-features init",
		Flag: "--force",
		Gate: "overwrite-existing features progress.json",
		Risk: override.RiskP2,
	}, "re-running init after plan was revised and old progress.json is stale")
	if err != nil {
		t.Fatalf("guard should allow a valid reason, got: %v", err)
	}
}

// --- wayfinder-features start --force ---

// TestStartForce_NoReason verifies --force without --reason is denied before any I/O.
func TestStartForce_NoReason(t *testing.T) {
	helperSetBool(t, &startForce, true)
	helperSetString(t, &startReason, "")

	err := runStart(StartCmd, []string{"any-feature"})
	if !isDeniedError(err) {
		t.Fatalf("expected *override.DeniedError, got %T: %v", err, err)
	}
}

// TestStartForce_GoodReason verifies the guard configuration permits a valid reason.
func TestStartForce_GoodReason(t *testing.T) {
	err := override.Require(context.Background(), override.Guard{
		Tool: "wayfinder-features start",
		Flag: "--force",
		Gate: "overwrite-existing feature state (restart a passing feature)",
		Risk: override.RiskP2,
	}, "feature regressed after a dependency update; restarting to re-verify")
	if err != nil {
		t.Fatalf("guard should allow a valid reason, got: %v", err)
	}
}
