package main

// Tests for reportPostCleanup, the archive command's post-cleanup reporting.
// Kept out of archive_test.go, which is already at the structural-health size
// ratchet.

import (
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

// TestReportPostCleanup_SandboxRemovalFailedIsVisible covers the fix for
// ce-93lw.27 gap #3: a sandbox that existed but could not be removed used to
// only under-report a count (SandboxRemoved stayed false with no other
// signal). It must now print a visible, impossible-to-miss error instead of
// silently doing nothing.
func TestReportPostCleanup_SandboxRemovalFailedIsVisible(t *testing.T) {
	out := captureStdout(t, func() {
		reportPostCleanup(&ops.CleanupResult{SandboxRemovalFailed: true})
	})
	if !strings.Contains(out, "Sandbox cleanup failed") {
		t.Errorf("expected a visible sandbox cleanup failure message, got: %q", out)
	}
	if !strings.Contains(out, "agm sandbox gc") {
		t.Errorf("expected the failure message to suggest a read-only scan, got: %q", out)
	}
	if strings.Contains(out, "agm sandbox gc --reap") || !strings.Contains(out, "unavailable") {
		t.Errorf("expected the failure message to refuse the unavailable reap path, got: %q", out)
	}
}

// TestReportPostCleanup_SandboxRemovedIsSilentAboutFailure proves the happy
// path (sandbox actually removed) does not also print the failure warning.
func TestReportPostCleanup_SandboxRemovedIsSilentAboutFailure(t *testing.T) {
	out := captureStdout(t, func() {
		reportPostCleanup(&ops.CleanupResult{SandboxRemoved: true})
	})
	if strings.Contains(out, "Sandbox cleanup failed") {
		t.Errorf("did not expect a failure message when sandbox removal succeeded, got: %q", out)
	}
	if !strings.Contains(out, "Removed sandbox directory") {
		t.Errorf("expected the success message, got: %q", out)
	}
}

// TestReportPostCleanup_BranchKeptOpenPRIsReported covers the gated
// branch-deletion fix (ce-93lw.27 gap #4): when cleanup preserved the local
// branch because it has an open PR, that must be visible in the report, not
// silently indistinguishable from "nothing happened".
func TestReportPostCleanup_BranchKeptOpenPRIsReported(t *testing.T) {
	out := captureStdout(t, func() {
		reportPostCleanup(&ops.CleanupResult{BranchKeptOpenPR: true})
	})
	if !strings.Contains(out, "open PR") {
		t.Errorf("expected the report to mention the branch was kept for an open PR, got: %q", out)
	}
}
