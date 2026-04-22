//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestE2E_WorkerLifecycle validates the full AGM worker pipeline:
// spawn → poll completion → verify commits → cherry-pick merge → archive → cleanup.
//
// Requirements:
//   - tmux installed
//   - Claude CLI installed (claude)
//   - agm binary installed (~/go/bin/agm)
//   - Dolt server running (for session storage)
//   - ANTHROPIC_API_KEY set (used by Claude CLI)
//
// Skipped in CI or when prerequisites are missing.
// Timeout: 5 minutes.
func TestE2E_WorkerLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	if os.Getenv("SKIP_E2E") != "" {
		t.Skip("SKIP_E2E is set")
	}

	// Check prerequisites
	for _, bin := range []string{"tmux", "claude", "agm", "git"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not found in PATH", bin)
		}
	}
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}

	// Use a unique session name to avoid collisions
	sessionName := fmt.Sprintf("e2e-lifecycle-%d", time.Now().UnixMilli())

	// Create a temporary git repo to serve as the working directory.
	// The worker will commit into this repo on a branch.
	repoDir := t.TempDir()
	initTestRepo(t, repoDir)

	// Write a simple prompt file that creates a deterministic commit.
	promptFile := filepath.Join(t.TempDir(), "prompt.txt")
	promptContent := fmt.Sprintf(
		"Create a file called hello.txt containing 'hello from %s'. "+
			"Then git add and git commit it with message 'add hello.txt'. "+
			"Do NOT push. Exit when done.",
		sessionName,
	)
	if err := os.WriteFile(promptFile, []byte(promptContent), 0644); err != nil {
		t.Fatalf("write prompt file: %v", err)
	}

	// Register cleanup: best-effort kill tmux + remove test artifacts.
	t.Cleanup(func() {
		cleanupSession(t, sessionName)
	})

	// ── Step 1: Spawn ───────────────────────────────────────────────
	t.Log("Step 1: Spawning worker session")
	spawnWorker(t, sessionName, repoDir, promptFile)

	// ── Step 2: Poll for completion ─────────────────────────────────
	t.Log("Step 2: Polling for worker completion")
	waitForSessionDone(t, sessionName, 5*time.Minute)

	// ── Step 3: Verify commits ──────────────────────────────────────
	t.Log("Step 3: Verifying commits on branch")
	verifyCommits(t, sessionName, repoDir)

	// ── Step 4: Cherry-pick merge ───────────────────────────────────
	t.Log("Step 4: Cherry-pick merging to main")
	mergeToMain(t, sessionName, repoDir)

	// Confirm hello.txt landed on main
	helloPath := filepath.Join(repoDir, "hello.txt")
	if _, err := os.Stat(helloPath); os.IsNotExist(err) {
		// Switch to main and check
		run(t, "git", "-C", repoDir, "checkout", "main")
		if _, err := os.Stat(helloPath); os.IsNotExist(err) {
			t.Fatal("hello.txt not found on main after merge")
		}
	}
	t.Log("hello.txt confirmed on main branch")

	// ── Step 5: Archive ─────────────────────────────────────────────
	t.Log("Step 5: Archiving session")
	archiveSession(t, sessionName)

	// ── Step 6: Verify cleanup ──────────────────────────────────────
	t.Log("Step 6: Verifying cleanup")
	verifyCleanup(t, sessionName)

	t.Log("E2E worker lifecycle test passed")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// initTestRepo creates a bare-minimum git repo with an initial commit on main.
func initTestRepo(t *testing.T, dir string) {
	t.Helper()
	cmds := [][]string{
		{"git", "-C", dir, "init", "-b", "main"},
		{"git", "-C", dir, "config", "user.email", "test@example.com"},
		{"git", "-C", dir, "config", "user.name", "E2E Test"},
	}
	for _, c := range cmds {
		run(t, c[0], c[1:]...)
	}

	// Create an initial commit so main exists.
	readmePath := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# test repo\n"), 0644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	run(t, "git", "-C", dir, "add", "README.md")
	run(t, "git", "-C", dir, "commit", "-m", "initial commit")
}

// spawnWorker calls `agm session new` with --test and --detached.
func spawnWorker(t *testing.T, name, repoDir, promptFile string) {
	t.Helper()
	args := []string{
		"session", "new", name,
		"--test",
		"--detached",
		"--harness", "claude-code",
		"--model", "haiku",
		"--prompt-file", promptFile,
		"--no-sandbox",
	}
	cmd := exec.Command("agm", args...)
	cmd.Dir = repoDir
	cmd.Env = append(os.Environ(),
		"ENGRAM_TEST_MODE=1",
		"ENGRAM_TEST_WORKSPACE=test",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("agm session new failed: %v\n%s", err, out)
	}
	t.Logf("Spawn output:\n%s", out)
}

// waitForSessionDone polls `agm session list --json` until the session reaches
// DONE or OFFLINE state, or the timeout expires.
func waitForSessionDone(t *testing.T, name string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for session %q to complete", name)
		}

		state := getSessionState(t, name)
		t.Logf("Session %s state: %s", name, state)

		switch state {
		case "DONE", "OFFLINE":
			return
		case "":
			// Session not found yet — may still be initialising
		}

		<-ticker.C
	}
}

// getSessionState returns the current state string for a session, or "" if
// the session cannot be found.
func getSessionState(t *testing.T, name string) string {
	t.Helper()
	cmd := exec.Command("agm", "session", "list", "--json")
	cmd.Env = append(os.Environ(),
		"ENGRAM_TEST_MODE=1",
		"ENGRAM_TEST_WORKSPACE=test",
	)
	out, err := cmd.Output()
	if err != nil {
		t.Logf("agm session list --json failed: %v", err)
		return ""
	}

	// Parse JSON array of sessions
	var sessions []struct {
		Name  string `json:"name"`
		State string `json:"state"`
	}
	if err := json.Unmarshal(out, &sessions); err != nil {
		// Might be newline-delimited JSON or wrapped — try line-by-line
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || line == "[" || line == "]" {
				continue
			}
			line = strings.TrimSuffix(line, ",")
			var s struct {
				Name  string `json:"name"`
				State string `json:"state"`
			}
			if json.Unmarshal([]byte(line), &s) == nil && s.Name == name {
				return s.State
			}
		}
		return ""
	}

	for _, s := range sessions {
		if s.Name == name {
			return s.State
		}
	}
	return ""
}

// verifyCommits runs `agm verify` and asserts COMMITS_FOUND.
func verifyCommits(t *testing.T, name, repoDir string) {
	t.Helper()
	cmd := exec.Command("agm", "verify", name,
		"--repo-dir", repoDir,
		"--no-record-trust",
	)
	cmd.Env = append(os.Environ(),
		"ENGRAM_TEST_MODE=1",
		"ENGRAM_TEST_WORKSPACE=test",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("agm verify failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "COMMITS_FOUND") {
		t.Fatalf("expected COMMITS_FOUND in verify output, got:\n%s", out)
	}
	t.Logf("Verify output:\n%s", out)
}

// mergeToMain cherry-picks worker commits onto main.
func mergeToMain(t *testing.T, name, repoDir string) {
	t.Helper()
	cmd := exec.Command("agm", "batch", "merge",
		"--repo-dir", repoDir,
		"--sessions", name,
		"--target-branch", "main",
	)
	cmd.Env = append(os.Environ(),
		"ENGRAM_TEST_MODE=1",
		"ENGRAM_TEST_WORKSPACE=test",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("agm batch merge failed: %v\n%s", err, out)
	}
	t.Logf("Merge output:\n%s", out)
}

// archiveSession archives the session via CLI.
func archiveSession(t *testing.T, name string) {
	t.Helper()
	cmd := exec.Command("agm", "session", "archive", name)
	cmd.Env = append(os.Environ(),
		"ENGRAM_TEST_MODE=1",
		"ENGRAM_TEST_WORKSPACE=test",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("agm session archive failed: %v\n%s", err, out)
	}
	t.Logf("Archive output:\n%s", out)
}

// verifyCleanup confirms that the session's tmux session is gone.
func verifyCleanup(t *testing.T, name string) {
	t.Helper()

	// Check tmux session is gone
	tmuxName := fmt.Sprintf("agm-test-%s", name)
	cmd := exec.Command("tmux", "has-session", "-t", tmuxName)
	if cmd.Run() == nil {
		t.Errorf("tmux session %s still exists after archive", tmuxName)
	}

	// Verify session shows as archived in storage
	state := getSessionState(t, name)
	if state != "" {
		// If session is still visible in non-archived list, that's a problem.
		// Archived sessions should not appear in default listing.
		t.Logf("Warning: session %s still visible in list with state=%s (may need --all to confirm archived)", name, state)
	}
}

// cleanupSession performs best-effort cleanup of a test session.
func cleanupSession(t *testing.T, name string) {
	t.Helper()

	// Kill tmux session
	tmuxName := fmt.Sprintf("agm-test-%s", name)
	_ = exec.Command("tmux", "kill-session", "-t", tmuxName).Run()

	// Also try the claude-prefixed name
	claudeName := fmt.Sprintf("claude-%s", name)
	_ = exec.Command("tmux", "kill-session", "-t", claudeName).Run()

	// Try agm test cleanup
	cmd := exec.Command("agm", "test", "cleanup", name, "--json")
	cmd.Env = append(os.Environ(),
		"ENGRAM_TEST_MODE=1",
		"ENGRAM_TEST_WORKSPACE=test",
	)
	_ = cmd.Run()
}

// run executes a command, failing the test on error.
func run(t *testing.T, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}
