package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/stophook"
)

// writeStatus writes a minimal, canonical WAYFINDER-STATUS.md with the given
// project status and current waypoint.
func writeStatus(t *testing.T, dir, projectStatus, currentWaypoint string) {
	t.Helper()
	completionDate := ""
	if projectStatus == statusCompleted {
		completionDate = "completion_date: 2026-01-01T00:00:00Z\n"
	}
	history := strings.Builder{}
	for _, waypoint := range []string{"CHARTER", "PROBLEM", "RESEARCH", "DESIGN", "SPEC", "PLAN", "SETUP", "BUILD", "RETRO"} {
		if waypoint == currentWaypoint {
			if projectStatus == statusCompleted {
				fmt.Fprintf(&history, "  - name: %s\n    status: completed\n    started_at: 2026-01-01T00:00:00Z\n    completed_at: 2026-01-01T00:00:00Z\n", waypoint)
			}
			break
		}
		fmt.Fprintf(&history, "  - name: %s\n    status: completed\n    started_at: 2026-01-01T00:00:00Z\n    completed_at: 2026-01-01T00:00:00Z\n", waypoint)
	}
	historyYAML := "waypoint_history: []\n"
	if history.Len() > 0 {
		historyYAML = "waypoint_history:\n" + history.String()
	}
	raw := fmt.Sprintf("---\nschema_version: \"2.0\"\nproject_name: test\nproject_type: feature\nrisk_level: M\ncurrent_waypoint: %s\nstatus: %s\ncreated_at: 2026-01-01T00:00:00Z\nupdated_at: 2026-01-01T00:00:00Z\n%s%s---\n", currentWaypoint, projectStatus, completionDate, historyYAML)
	if err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), []byte(raw), 0o600); err != nil {
		t.Fatalf("write status: %v", err)
	}
}

func writeUnsupportedStatus(t *testing.T, dir string) {
	t.Helper()
	raw := "---\nschema_version: \"unsupported\"\ncurrent_waypoint: PROBLEM\nstatus: in-progress\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), []byte(raw), 0o600); err != nil {
		t.Fatalf("write unsupported status: %v", err)
	}
}

func requireBlockedInvalidStatus(t *testing.T, r *stophook.Result, check string) {
	t.Helper()
	for _, finding := range r.Findings {
		if finding.Check == check && finding.Severity == stophook.SeverityBlock {
			return
		}
	}
	t.Fatalf("expected %s to block invalid status, got %+v", check, r.Findings)
}

func newResult() *stophook.Result { return &stophook.Result{HookName: "test"} }

// --- checkPhase ---

func TestCheckPhase_InProgress(t *testing.T) {
	dir := t.TempDir()
	writeStatus(t, dir, "in-progress", "RESEARCH")

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
	writeStatus(t, dir, statusCompleted, "RETRO")

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
	writeStatus(t, dir, statusAbandoned, "RETRO")

	r := newResult()
	checkPhase(r, dir)

	for _, f := range r.Findings {
		if f.Check == "phase" && f.Severity == stophook.SeverityPass {
			return // want Pass for intentionally-ended project
		}
	}
	t.Errorf("expected Pass for abandoned project, got %+v", r.Findings)
}

func TestCheckPhase_IncompleteCompletedStatusBlocks(t *testing.T) {
	dir := t.TempDir()
	raw := "---\nschema_version: \"2.0\"\nstatus: completed\ncurrent_waypoint: RETRO\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	r := newResult()
	checkPhase(r, dir)

	requireBlockedInvalidStatus(t, r, "phase")
}

// B14 regression: "blocked" is not a valid status and must NOT be treated as Pass.
func TestCheckPhase_BlockedIsNotPass(t *testing.T) {
	dir := t.TempDir()
	// Manually write a status file with a non-standard "blocked" value.
	raw := "---\nschema_version: \"2.0\"\nproject_name: test\nstatus: blocked\ncurrent_waypoint: BUILD\n---\n"
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

func TestCheckPhase_UnsupportedStatusBlocks(t *testing.T) {
	dir := t.TempDir()
	writeUnsupportedStatus(t, dir)

	r := newResult()
	checkPhase(r, dir)

	requireBlockedInvalidStatus(t, r, "phase")
}

// --- checkRetrospective ---

func TestCheckRetrospective_CompletedNoRetro_IsBlock(t *testing.T) {
	dir := t.TempDir()
	writeStatus(t, dir, statusCompleted, "RETRO")

	r := newResult()
	checkRetrospective(r, dir)

	for _, f := range r.Findings {
		if f.Check == "retrospective" && f.Severity == stophook.SeverityBlock {
			return // want Block — completed project must have a retro
		}
	}
	t.Errorf("expected Block when project complete but no retrospective, got %+v", r.Findings)
}

func TestCheckRetrospective_AtRetroNoRetro_IsBlock(t *testing.T) {
	dir := t.TempDir()
	writeStatus(t, dir, "in-progress", "RETRO")

	r := newResult()
	checkRetrospective(r, dir)

	for _, f := range r.Findings {
		if f.Check == "retrospective" && f.Severity == stophook.SeverityBlock {
			return
		}
	}
	t.Errorf("expected Block when current phase is RETRO but no retrospective, got %+v", r.Findings)
}

func TestCheckRetrospective_InProgressMidPhase_IsPass(t *testing.T) {
	dir := t.TempDir()
	writeStatus(t, dir, "in-progress", "BUILD")

	r := newResult()
	checkRetrospective(r, dir)

	for _, f := range r.Findings {
		if f.Check == "retrospective" && f.Severity == stophook.SeverityPass {
			return // not at RETRO — no retro required
		}
	}
	t.Errorf("expected Pass when project is mid-phase, got %+v", r.Findings)
}

func TestCheckRetrospective_ExistsWithContent_IsPass(t *testing.T) {
	dir := t.TempDir()
	writeStatus(t, dir, statusCompleted, "RETRO")
	// Write a retro with substantial content (>100 bytes)
	content := make([]byte, 200)
	for i := range content {
		content[i] = 'x'
	}
	if err := os.WriteFile(filepath.Join(dir, "RETRO-retrospective.md"), content, 0o600); err != nil {
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
	writeStatus(t, dir, statusCompleted, "RETRO")
	if err := os.WriteFile(filepath.Join(dir, "RETRO-retrospective.md"), []byte("hi"), 0o600); err != nil {
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

func TestCheckRetrospective_UnsupportedStatusBlocks(t *testing.T) {
	dir := t.TempDir()
	writeUnsupportedStatus(t, dir)

	r := newResult()
	checkRetrospective(r, dir)

	requireBlockedInvalidStatus(t, r, "retrospective")
}
