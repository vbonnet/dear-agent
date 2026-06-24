package main

import (
	"context"
	"errors"
	"testing"

	"github.com/vbonnet/dear-agent/internal/override"
)

// helperSetBool sets a bool flag and restores it after the test.
func helperSetBool(t *testing.T, p *bool, v bool) {
	t.Helper()
	old := *p
	*p = v
	t.Cleanup(func() { *p = old })
}

// helperSetString sets a string flag and restores it after the test.
func helperSetString(t *testing.T, p *string, v string) {
	t.Helper()
	old := *p
	*p = v
	t.Cleanup(func() { *p = old })
}

// isDeniedError reports whether err is an *override.DeniedError.
func isDeniedError(err error) bool {
	var d *override.DeniedError
	return errors.As(err, &d)
}

// requireGuard calls override.Require with the same guard parameters used by
// each Category-B command. This lets us test the guard wiring offline without
// needing cobra command context or storage.
func requireGuard(tool, flag, gate string, reason string) error {
	return override.Require(context.Background(), override.Guard{
		Tool: tool,
		Flag: flag,
		Gate: gate,
		Risk: override.RiskP2,
	}, reason)
}

// --- session kill --force ---

// TestKillForce_NoReason verifies that --force without --reason returns a
// DeniedError before any storage I/O (fail-fast path through runKillCommand).
func TestKillForce_NoReason(t *testing.T) {
	helperSetBool(t, &forceKill, true)
	helperSetString(t, &killReason, "")

	err := runKillCommand(killCmd, []string{"my-session"})
	if !isDeniedError(err) {
		t.Fatalf("expected *override.DeniedError, got %T: %v", err, err)
	}
}

// TestKillForce_GoodReason verifies the guard configuration permits a valid reason.
func TestKillForce_GoodReason(t *testing.T) {
	err := requireGuard("agm session kill", "--force", "interactive kill confirmation",
		"automated reaper has no TTY; killing stale stopped session")
	if err != nil {
		t.Fatalf("guard should allow a valid reason, got: %v", err)
	}
}

// --- session archive --force ---

// TestArchiveForce_NoReason verifies --force without --reason is denied early.
func TestArchiveForce_NoReason(t *testing.T) {
	helperSetBool(t, &forceArchive, true)
	helperSetString(t, &archiveReason, "")

	err := archiveSession(archiveCmd, []string{"my-session"})
	if !isDeniedError(err) {
		t.Fatalf("expected *override.DeniedError, got %T: %v", err, err)
	}
}

// TestArchiveForce_GoodReason verifies the guard configuration permits a valid reason.
func TestArchiveForce_GoodReason(t *testing.T) {
	err := requireGuard("agm session archive", "--force",
		"pre-archive verification (uncommitted changes, unmerged branch)",
		"unmerged branch is intentional; work was squash-merged externally")
	if err != nil {
		t.Fatalf("guard should allow a valid reason, got: %v", err)
	}
}

// --- session unarchive --force ---

// TestUnarchiveForce_NoReason verifies --force without --reason is denied early.
func TestUnarchiveForce_NoReason(t *testing.T) {
	helperSetBool(t, &unarchiveForce, true)
	helperSetString(t, &unarchiveReason, "")

	err := runUnarchive(unarchiveCmd, []string{"my-session"})
	if !isDeniedError(err) {
		t.Fatalf("expected *override.DeniedError, got %T: %v", err, err)
	}
}

// TestUnarchiveForce_GoodReason verifies the guard configuration permits a valid reason.
func TestUnarchiveForce_GoodReason(t *testing.T) {
	err := requireGuard("agm session unarchive", "--force",
		"interactive restore confirmation",
		"scripted bulk restore after storage migration; no TTY available")
	if err != nil {
		t.Fatalf("guard should allow a valid reason, got: %v", err)
	}
}

// --- session gc --force ---

// TestGCForce_NoReason verifies --force without --reason is denied early.
func TestGCForce_NoReason(t *testing.T) {
	helperSetBool(t, &gcForce, true)
	helperSetString(t, &gcReason, "")

	err := runGC(gcCmd, []string{})
	if !isDeniedError(err) {
		t.Fatalf("expected *override.DeniedError, got %T: %v", err, err)
	}
}

// TestGCForce_GoodReason verifies the guard configuration permits a valid reason.
func TestGCForce_GoodReason(t *testing.T) {
	err := requireGuard("agm session gc", "--force",
		"pre-archive verification (uncommitted changes, unmerged branch)",
		"post-migration sweep; sessions already verified clean externally")
	if err != nil {
		t.Fatalf("guard should allow a valid reason, got: %v", err)
	}
}
