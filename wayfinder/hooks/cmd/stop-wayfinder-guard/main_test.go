package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/stophook"
	"github.com/vbonnet/dear-agent/wayfinder/internal/status"
)

// writeStatus writes a minimal WAYFINDER-STATUS.md with the given status value.
func writeStatus(t *testing.T, dir, projectStatus, currentPhase string) {
	t.Helper()
	s := &status.Status{
		SchemaVersion: status.SchemaVersion,
		SessionID:     "test-session",
		ProjectPath:   dir,
		StartedAt:     time.Now(),
		Status:        projectStatus,
		CurrentPhase:  currentPhase,
	}
	if err := s.WriteTo(dir); err != nil {
		t.Fatalf("writeTo: %v", err)
	}
}

func newResult() *stophook.Result { return &stophook.Result{HookName: "test"} }

// --- checkPhase ---

func TestCheckPhase_InProgress(t *testing.T) {
	dir := t.TempDir()
	writeStatus(t, dir, status.StatusInProgress, "S5")

	r := newResult()
	checkPhase(r, dir)

	for _, f := range r.Findings {
		if f.Check == "phase" && f.Severity == stophook.SeverityWarn {
			return // want Warn for in-progress
		}
	}
	t.Errorf("expected Warn for in-progress project, got %+v", r.Findings)
}

func TestCheckPhase_Completed(t *testing.T) {
	dir := t.TempDir()
	writeStatus(t, dir, status.StatusCompleted, "S11")

	r := newResult()
	checkPhase(r, dir)

	for _, f := range r.Findings {
		if f.Check == "phase" && f.Severity == stophook.SeverityPass {
			return // want Pass for completed
		}
	}
	t.Errorf("expected Pass for completed project, got %+v", r.Findings)
}

func TestCheckPhase_Abandoned(t *testing.T) {
	dir := t.TempDir()
	writeStatus(t, dir, status.StatusAbandoned, "")

	r := newResult()
	checkPhase(r, dir)

	for _, f := range r.Findings {
		if f.Check == "phase" && f.Severity == stophook.SeverityPass {
			return // want Pass for intentionally-ended project
		}
	}
	t.Errorf("expected Pass for abandoned project, got %+v", r.Findings)
}

// B14 regression: "blocked" is not a valid status and must NOT be treated as Pass.
func TestCheckPhase_BlockedIsNotPass(t *testing.T) {
	dir := t.TempDir()
	// Manually write a status file with a non-standard "blocked" value.
	raw := fmt.Sprintf("---\nschema_version: \"2.0\"\nsession_id: test\nproject_path: %s\nstarted_at: 2026-01-01T00:00:00Z\nstatus: blocked\ncurrent_phase: S3\n---\n\n# Wayfinder Session\n", dir)
	if err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	r := newResult()
	checkPhase(r, dir)

	// Must NOT be Pass — "blocked" is not a recognised terminal state.
	for _, f := range r.Findings {
		if f.Check == "phase" && f.Severity == stophook.SeverityPass {
			t.Errorf("B14 regression: 'blocked' status must NOT yield Pass, got %+v", f)
		}
	}
}

func TestCheckPhase_NoStatusFile(t *testing.T) {
	dir := t.TempDir()
	r := newResult()
	checkPhase(r, dir)

	for _, f := range r.Findings {
		if f.Check == "phase" && f.Severity == stophook.SeverityPass {
			return // graceful skip → Pass
		}
	}
	t.Errorf("expected Pass (graceful skip) when no status file, got %+v", r.Findings)
}

// --- checkRetrospective ---

func TestCheckRetrospective_CompletedNoRetro_IsBlock(t *testing.T) {
	dir := t.TempDir()
	writeStatus(t, dir, status.StatusCompleted, "S11")

	r := newResult()
	checkRetrospective(r, dir)

	for _, f := range r.Findings {
		if f.Check == "retrospective" && f.Severity == stophook.SeverityBlock {
			return // want Block — completed project must have a retro
		}
	}
	t.Errorf("expected Block when project complete but no retrospective, got %+v", r.Findings)
}

func TestCheckRetrospective_AtS11NoRetro_IsBlock(t *testing.T) {
	dir := t.TempDir()
	writeStatus(t, dir, status.StatusInProgress, "S11")

	r := newResult()
	checkRetrospective(r, dir)

	for _, f := range r.Findings {
		if f.Check == "retrospective" && f.Severity == stophook.SeverityBlock {
			return
		}
	}
	t.Errorf("expected Block when current phase is S11 but no retrospective, got %+v", r.Findings)
}

func TestCheckRetrospective_InProgressMidPhase_IsPass(t *testing.T) {
	dir := t.TempDir()
	writeStatus(t, dir, status.StatusInProgress, "S5")

	r := newResult()
	checkRetrospective(r, dir)

	for _, f := range r.Findings {
		if f.Check == "retrospective" && f.Severity == stophook.SeverityPass {
			return // not at S11 — no retro required
		}
	}
	t.Errorf("expected Pass when project is mid-phase, got %+v", r.Findings)
}

func TestCheckRetrospective_ExistsWithContent_IsPass(t *testing.T) {
	dir := t.TempDir()
	writeStatus(t, dir, status.StatusCompleted, "S11")
	// Write a retro with substantial content (>100 bytes)
	content := make([]byte, 200)
	for i := range content {
		content[i] = 'x'
	}
	if err := os.WriteFile(filepath.Join(dir, "S11-retrospective.md"), content, 0o600); err != nil {
		t.Fatal(err)
	}

	r := newResult()
	checkRetrospective(r, dir)

	for _, f := range r.Findings {
		if f.Check == "retrospective" && f.Severity == stophook.SeverityPass {
			return
		}
	}
	t.Errorf("expected Pass when retro exists with content, got %+v", r.Findings)
}

func TestCheckRetrospective_ExistsMinimalContent_IsBlock(t *testing.T) {
	dir := t.TempDir()
	writeStatus(t, dir, status.StatusCompleted, "S11")
	if err := os.WriteFile(filepath.Join(dir, "S11-retrospective.md"), []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}

	r := newResult()
	checkRetrospective(r, dir)

	for _, f := range r.Findings {
		if f.Check == "retrospective" && f.Severity == stophook.SeverityBlock {
			return
		}
	}
	t.Errorf("expected Block when retro is nearly empty, got %+v", r.Findings)
}
