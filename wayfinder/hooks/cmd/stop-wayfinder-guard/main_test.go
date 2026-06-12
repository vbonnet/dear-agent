package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/stophook"
)

func TestCheckRetrospective_NoStatusFile(t *testing.T) {
	t.Parallel()
	r := &stophook.Result{HookName: "test"}
	checkRetrospective(r, t.TempDir())
	if r.ExitCode() != 0 {
		t.Errorf("checkRetrospective(no status file) exit = %d, want 0", r.ExitCode())
	}
}

func TestCheckPhase_NoStatusFile(t *testing.T) {
	t.Parallel()
	r := &stophook.Result{HookName: "test"}
	checkPhase(r, t.TempDir())
	if r.ExitCode() != 0 {
		t.Errorf("checkPhase(no status file) exit = %d, want 0", r.ExitCode())
	}
}

func TestCheckPhase_AbandonedProject(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), []byte("status: abandoned\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	r := &stophook.Result{HookName: "test"}
	checkPhase(r, dir)
	if r.ExitCode() != 0 {
		t.Errorf("checkPhase(abandoned) exit = %d, want 0", r.ExitCode())
	}
}

func TestCheckArtifacts_EmptyDir(t *testing.T) {
	t.Parallel()
	r := &stophook.Result{HookName: "test"}
	checkArtifacts(r, t.TempDir())
	if r.ExitCode() != 0 {
		t.Errorf("checkArtifacts(empty dir) exit = %d, want 0", r.ExitCode())
	}
}

func TestCheckArtifacts_MisplacedArtifact(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "S5-plan.md"), []byte("plan\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	r := &stophook.Result{HookName: "test"}
	checkArtifacts(r, dir)
	// Misplaced artifact → warns but does not block.
	if r.ExitCode() != 0 {
		t.Errorf("checkArtifacts(misplaced) exit = %d, want 0", r.ExitCode())
	}
}

func TestCheckBeads_BdNotAvailable(t *testing.T) {
	t.Parallel()
	// Without bd installed/reachable, checkBeads passes gracefully.
	r := &stophook.Result{HookName: "test"}
	checkBeads(r, t.TempDir())
	if r.ExitCode() != 0 {
		t.Errorf("checkBeads(bd unavailable) exit = %d, want 0", r.ExitCode())
	}
}
