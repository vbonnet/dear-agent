package csm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewPrompter(t *testing.T) {
	p := NewPrompter()
	if p == nil {
		t.Fatal("NewPrompter should not return nil")
	}

	if p.tmpDir != "/tmp" {
		t.Errorf("default tmpDir: got %s, want /tmp", p.tmpDir)
	}
}

func TestNewPrompterWithTmpDir(t *testing.T) {
	customTmpDir := "/custom/tmp"
	p := NewPrompterWithTmpDir(customTmpDir)

	if p.tmpDir != customTmpDir {
		t.Errorf("tmpDir: got %s, want %s", p.tmpDir, customTmpDir)
	}
}

func TestInjectPrompt_EmptySessionName(t *testing.T) {
	p := NewPrompter()
	err := p.InjectPrompt("", "bead-1", "test prompt")

	if err == nil {
		t.Fatal("InjectPrompt should fail for empty session name")
	}

	if err.Error() != "session name cannot be empty" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInjectPrompt_EmptyBeadID(t *testing.T) {
	p := NewPrompter()
	err := p.InjectPrompt("test-session", "", "test prompt")

	if err == nil {
		t.Fatal("InjectPrompt should fail for empty bead ID")
	}

	if err.Error() != "bead ID cannot be empty" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInjectPrompt_EmptyPrompt(t *testing.T) {
	p := NewPrompter()
	err := p.InjectPrompt("test-session", "bead-1", "")

	if err == nil {
		t.Fatal("InjectPrompt should fail for empty prompt")
	}

	if err.Error() != "prompt cannot be empty" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInjectPrompt_TempFileCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	p := NewPrompterWithTmpDir(tmpDir)

	beadID := "test-bead-cleanup"
	tmpPath := filepath.Join(tmpDir, "bead-"+beadID+".md")

	// Inject prompt (will fail because no tmux session, but that's okay)
	_ = p.InjectPrompt("nonexistent-session", beadID, "test prompt")

	// Verify temp file was cleaned up
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("temp file should be cleaned up after InjectPrompt")
	}
}

func TestInjectPromptDirect_EmptySessionName(t *testing.T) {
	p := NewPrompter()
	err := p.InjectPromptDirect("", "test prompt")

	if err == nil {
		t.Fatal("InjectPromptDirect should fail for empty session name")
	}

	if err.Error() != "session name cannot be empty" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInjectPromptDirect_EmptyPrompt(t *testing.T) {
	p := NewPrompter()
	err := p.InjectPromptDirect("test-session", "")

	if err == nil {
		t.Fatal("InjectPromptDirect should fail for empty prompt")
	}

	if err.Error() != "prompt cannot be empty" {
		t.Errorf("unexpected error: %v", err)
	}
}

// Integration tests - require tmux and AGM

func TestInjectPrompt_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create test session
	orch := NewOrchestrator()
	sessionName := "test-autonomous-swarm-inject"

	// Cleanup
	_ = orch.Archive(sessionName)

	// Create session
	if err := orch.Create(sessionName); err != nil {
		t.Skipf("AGM not available: %v", err)
	}

	defer func() {
		_ = orch.Archive(sessionName)
	}()

	// Wait for session to be ready
	if err := orch.WaitForSession(sessionName, 5*1000000000); err != nil {
		t.Fatalf("Session not ready: %v", err)
	}

	// Inject prompt
	p := NewPrompter()
	prompt := "# Test Prompt\n\nThis is a test bead prompt."
	if err := p.InjectPrompt(sessionName, "test-bead", prompt); err != nil {
		t.Fatalf("InjectPrompt failed: %v", err)
	}

	// Note: We can't easily verify the prompt was injected without reading tmux pane
	// This test verifies the command succeeds without error
}

func TestInjectPromptDirect_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create test session
	orch := NewOrchestrator()
	sessionName := "test-autonomous-swarm-inject-direct"

	// Cleanup
	_ = orch.Archive(sessionName)

	// Create session
	if err := orch.Create(sessionName); err != nil {
		t.Skipf("AGM not available: %v", err)
	}

	defer func() {
		_ = orch.Archive(sessionName)
	}()

	// Wait for session
	if err := orch.WaitForSession(sessionName, 5*1000000000); err != nil {
		t.Fatalf("Session not ready: %v", err)
	}

	// Inject short prompt directly
	p := NewPrompter()
	prompt := "echo 'Test prompt'"
	if err := p.InjectPromptDirect(sessionName, prompt); err != nil {
		t.Fatalf("InjectPromptDirect failed: %v", err)
	}
}

func TestInjectPrompt_LargePrompt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create test session
	orch := NewOrchestrator()
	sessionName := "test-autonomous-swarm-large-prompt"

	// Cleanup
	_ = orch.Archive(sessionName)

	// Create session
	if err := orch.Create(sessionName); err != nil {
		t.Skipf("AGM not available: %v", err)
	}

	defer func() {
		_ = orch.Archive(sessionName)
	}()

	// Wait for session
	if err := orch.WaitForSession(sessionName, 5*1000000000); err != nil {
		t.Fatalf("Session not ready: %v", err)
	}

	// Inject large prompt (>1KB)
	largePrompt := "# Large Bead Prompt\n\n"
	for i := 0; i < 100; i++ {
		largePrompt += "This is line " + string(rune(i)) + " of a large prompt for testing.\n"
	}

	p := NewPrompter()
	if err := p.InjectPrompt(sessionName, "large-bead", largePrompt); err != nil {
		t.Fatalf("InjectPrompt failed for large prompt: %v", err)
	}
}
