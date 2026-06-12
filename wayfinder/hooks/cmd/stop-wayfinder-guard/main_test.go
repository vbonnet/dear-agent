package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/stophook"
)

func result() *stophook.Result { return &stophook.Result{HookName: "test"} }

func TestCheckRetrospective_NoStatusFile_Passes(t *testing.T) {
	r := result()
	checkRetrospective(r, t.TempDir())
	if r.ExitCode() == 1 {
		t.Error("checkRetrospective without WAYFINDER-STATUS.md should not block")
	}
}

func TestCheckRetrospective_WithS11Retro_Passes(t *testing.T) {
	dir := t.TempDir()
	content := make([]byte, 200) // >100 bytes of content
	for i := range content {
		content[i] = 'x'
	}
	if err := os.WriteFile(filepath.Join(dir, "S11-retrospective.md"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	r := result()
	checkRetrospective(r, dir)
	if r.ExitCode() == 1 {
		t.Error("checkRetrospective with S11-retrospective.md content should not block")
	}
}

func TestCheckPhase_NoStatusFile_Passes(t *testing.T) {
	r := result()
	checkPhase(r, t.TempDir())
	if r.ExitCode() == 1 {
		t.Error("checkPhase without WAYFINDER-STATUS.md should not block")
	}
}

func TestCheckPhase_AbandonedStatus_Passes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), []byte("status: abandoned"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := result()
	checkPhase(r, dir)
	if r.ExitCode() == 1 {
		t.Error("checkPhase with abandoned status should not block")
	}
}

func TestCheckArtifacts_NoArtifacts_Passes(t *testing.T) {
	r := result()
	checkArtifacts(r, t.TempDir())
	if r.ExitCode() == 1 {
		t.Error("checkArtifacts on empty dir should not block")
	}
}

func TestCheckArtifacts_MisplacedArtifact_Warns(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "D1-charter.md"), []byte("# Charter"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := result()
	checkArtifacts(r, dir)
	// Misplaced artifact should warn (but not block with exit 1).
	if r.ExitCode() == 1 {
		t.Error("checkArtifacts with misplaced artifact should warn, not block")
	}
}

func TestCheckBeads_BdNotAvailable_Passes(t *testing.T) {
	// bd is unlikely to be on PATH during tests; should pass gracefully.
	r := result()
	checkBeads(r, t.TempDir())
	// Either bd is found (pass/warn) or not (pass gracefully) — should not block.
	if r.ExitCode() == 1 {
		t.Error("checkBeads when bd unavailable should not block")
	}
}
