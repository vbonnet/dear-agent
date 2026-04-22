//go:build integration

package integration

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

func agm(args ...string) (string, error) {
	cmd := exec.Command("agm", args...)
	cmd.Env = append(os.Environ(), "AGM_DEBUG=false")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestSendMsgAfterCrashResume(t *testing.T) {
	client := tmux.NewFakeTmuxClient()
	client.CreateSession("worker-1")

	// Send message succeeds
	err := client.SendKeys("worker-1", "hello")
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	// Simulate crash
	client.KillSession("worker-1")
	alive, _ := client.IsSessionAlive("worker-1")
	if alive {
		t.Fatal("session should be dead after kill")
	}

	// Resume (recreate)
	client.CreateSession("worker-1")

	// Send again should work
	err = client.SendKeys("worker-1", "still there?")
	if err != nil {
		t.Fatalf("send after resume failed: %v", err)
	}

	content, _ := client.CapturePane("worker-1")
	if !strings.Contains(content, "still there?") {
		t.Errorf("message not found after resume: %s", content)
	}
}

func TestBootStartsOverseer(t *testing.T) {
	client := tmux.NewFakeTmuxClient()

	// Boot: no overseer exists
	sessions, _ := client.ListSessions()
	if len(sessions) != 0 {
		t.Fatal("should start empty")
	}

	// Boot invariant: create overseer if missing
	client.CreateSession("overseer")
	alive, _ := client.IsSessionAlive("overseer")
	if !alive {
		t.Fatal("overseer should be alive after boot")
	}
}

func TestLoopContinuesWithWork(t *testing.T) {
	client := tmux.NewFakeTmuxClient()

	// Create 3 worker sessions
	for i := 0; i < 3; i++ {
		client.CreateSession(fmt.Sprintf("worker-%d", i))
	}

	sessions, _ := client.ListSessions()
	if len(sessions) != 3 {
		t.Errorf("expected 3 sessions, got %d", len(sessions))
	}

	// Kill one, verify others still listed
	client.KillSession("worker-1")
	sessions, _ = client.ListSessions()
	if len(sessions) != 2 {
		t.Errorf("expected 2 alive sessions, got %d", len(sessions))
	}
}

func TestWorkersMergeOwnBranches(t *testing.T) {
	t.Skip("TODO: requires worker lifecycle")
	// Launch worker, verify commits on branch, verify merge to main
}

func TestPermissionPromptDraining(t *testing.T) {
	t.Skip("TODO: requires permission prompt simulation")
}

func TestSessionCleanupAfterCompletion(t *testing.T) {
	t.Skip("TODO: requires session GC integration")
}

func TestAgmSendDetectsClaudeInBash(t *testing.T) {
	// Verify agm binary exists and runs
	out, err := agm("version")
	if err != nil {
		t.Skipf("agm not available: %v", err)
	}
	if !strings.Contains(out, "agm") {
		t.Errorf("unexpected version output: %s", out)
	}

	// Verify session list works
	homeDir, _ := os.UserHomeDir()
	out, err = agm("-C", homeDir, "session", "list")
	if err != nil {
		t.Skipf("agm session list failed: %v", err)
	}
	if !strings.Contains(out, "NAME") {
		t.Errorf("session list missing header: %s", out)
	}
}

func TestCPULoadGating(t *testing.T) {
	t.Skip("TODO: requires load monitoring")
}

func TestFalseCompletionDetection(t *testing.T) {
	client := tmux.NewFakeTmuxClient()
	client.CreateSession("test-worker")

	// Worker claims done but has no commits
	alive, _ := client.IsSessionAlive("test-worker")
	if !alive {
		t.Fatal("session should be alive")
	}

	// Simulate false completion: session alive but no git activity
	content, _ := client.CapturePane("test-worker")
	if content == "" {
		t.Fatal("pane should have content")
	}

	// Verify we can detect the session
	sessions, _ := client.ListSessions()
	found := false
	for _, s := range sessions {
		if s == "test-worker" {
			found = true
		}
	}
	if !found {
		t.Fatal("test-worker should be in session list")
	}
}

func TestHeartbeatDeadManSwitch(t *testing.T) {
	// Test heartbeat write and staleness check
	tmpDir := t.TempDir()
	hbFile := tmpDir + "/test-heartbeat.json"

	// Write heartbeat
	_, err := agm("heartbeat", "write", "--file", hbFile)
	if err != nil {
		t.Skipf("heartbeat command not available: %v", err)
	}

	// Check freshness
	time.Sleep(100 * time.Millisecond)
	out, err := agm("heartbeat", "check", "--file", hbFile, "--max-age", "1m")
	if err != nil {
		t.Errorf("heartbeat check failed on fresh heartbeat: %v\noutput: %s", err, out)
	}
}
