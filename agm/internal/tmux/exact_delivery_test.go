package tmux

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAtomicPasteRefusesReusedTmuxIDsAfterServerRestart(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping isolated tmux identity-reuse integration in short mode")
	}
	skipIfNoTmux(t)
	socketPath, cleanup := setupTestSocket(t)
	defer cleanup()

	old := createExactTargetTestSession(t, socketPath, "exact-reuse", "stable-session")
	if output, err := exec.Command("tmux", "-S", socketPath, "kill-server").CombinedOutput(); err != nil {
		t.Fatalf("kill first tmux server: %v: %s", err, output)
	}
	replacement := createExactTargetTestSession(t, socketPath, "exact-reuse", "stable-session")
	if replacement.PaneID != old.PaneID || replacement.SessionID != old.SessionID {
		t.Skipf("tmux did not reuse pane/session IDs (%s/%s -> %s/%s)", old.PaneID, old.SessionID, replacement.PaneID, replacement.SessionID)
	}
	if replacement.PanePID == old.PanePID {
		t.Skipf("operating system unexpectedly reused pane PID %d", old.PanePID)
	}

	bufferName := "agm-cmd-exact-reuse"
	loadTmuxBufferForTest(t, socketPath, bufferName, "NEVER_PASTE_INTO_REPLACEMENT")
	err := pasteBufferToExactTarget(t.Context(), socketPath, bufferName, old, "codex-cli", false)
	if err == nil || PromptSubmissionMayHaveOccurred(err) {
		t.Fatalf("identity-reused paste error = %v, want definite refusal", err)
	}
	capture := captureExactDeliveryPane(t, socketPath, replacement.PaneID)
	if strings.Contains(capture, "NEVER_PASTE_INTO_REPLACEMENT") {
		t.Fatalf("replacement pane was mutated despite exact-target refusal: %q", capture)
	}
}

func TestAtomicEnterRefusesReplacementWithoutSubmittingItsDraft(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping isolated tmux exact-Enter integration in short mode")
	}
	skipIfNoTmux(t)
	socketPath, cleanup := setupTestSocket(t)
	defer cleanup()

	old := createExactTargetTestSession(t, socketPath, "exact-enter", "stable-session")
	if output, err := exec.Command("tmux", "-S", socketPath, "kill-server").CombinedOutput(); err != nil {
		t.Fatalf("kill first tmux server: %v: %s", err, output)
	}
	replacement := createExactTargetTestSession(t, socketPath, "exact-enter", "stable-session")
	if replacement.PaneID != old.PaneID || replacement.SessionID != old.SessionID || replacement.PanePID == old.PanePID {
		t.Skip("tmux/OS did not expose the intended reused-ID/different-PID fixture")
	}

	marker := filepath.Join(t.TempDir(), "replacement-draft-submitted")
	draft := "touch " + strconv.Quote(marker)
	if output, err := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", replacement.PaneID, "-l", draft).CombinedOutput(); err != nil {
		t.Fatalf("seed replacement draft: %v: %s", err, output)
	}
	err := sendEnterToExactTarget(t.Context(), socketPath, old)
	if err == nil || PromptSubmissionMayHaveOccurred(err) {
		t.Fatalf("identity-reused Enter error = %v, want definite refusal from conditional key send", err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("replacement draft was submitted; marker stat error = %v", statErr)
	}
	if capture := captureExactDeliveryPane(t, socketPath, replacement.PaneID); !strings.Contains(capture, "replacement-draft-submitted") {
		t.Fatalf("replacement draft disappeared despite Enter refusal: %q", capture)
	}
}

func TestStrictCompactionMutationRefusesAttachedHumanDraft(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping isolated tmux attached-client integration in short mode")
	}
	skipIfNoTmux(t)
	socketPath, cleanup := setupTestSocket(t)
	defer cleanup()

	target := createExactTargetTestSession(t, socketPath, "exact-attached", "stable-session")
	target.RequireNoAttachedClients = true
	client := exec.Command("tmux", "-S", socketPath, "-C", "attach-session", "-t", target.SessionID)
	client.Stdout = io.Discard
	client.Stderr = io.Discard
	clientInput, err := client.StdinPipe()
	if err != nil {
		t.Fatalf("create control-client input: %v", err)
	}
	if err := client.Start(); err != nil {
		t.Fatalf("attach isolated tmux control client: %v", err)
	}
	t.Cleanup(func() {
		_ = clientInput.Close()
		if client.Process != nil {
			_ = client.Process.Kill()
		}
		_ = client.Wait()
	})
	waitForAttachedTmuxClient(t, socketPath, target.PaneID)

	const draft = "HUMAN_DRAFT_MUST_NOT_BE_COMBINED"
	if output, err := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", target.PaneID, "-l", draft).CombinedOutput(); err != nil {
		t.Fatalf("seed attached-client draft: %v: %s", err, output)
	}
	bufferName := "agm-cmd-attached"
	loadTmuxBufferForTest(t, socketPath, bufferName, "/compact")
	if err := pasteBufferToExactTarget(t.Context(), socketPath, bufferName, target, "codex-cli", false); err == nil || PromptSubmissionMayHaveOccurred(err) {
		t.Fatalf("attached-client paste error = %v, want definite refusal", err)
	}
	capture := captureExactDeliveryPane(t, socketPath, target.PaneID)
	if !strings.Contains(capture, draft) || strings.Contains(capture, "/compact") {
		t.Fatalf("attached draft changed despite atomic paste refusal: %q", capture)
	}

	marker := filepath.Join(t.TempDir(), "attached-draft-submitted")
	if output, err := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", target.PaneID, "C-u").CombinedOutput(); err != nil {
		t.Fatalf("clear attached-client fixture draft: %v: %s", err, output)
	}
	if output, err := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", target.PaneID, "-l", "touch "+strconv.Quote(marker)).CombinedOutput(); err != nil {
		t.Fatalf("seed attached-client submit fixture: %v: %s", err, output)
	}
	if err := sendEnterToExactTarget(t.Context(), socketPath, target); err == nil || PromptSubmissionMayHaveOccurred(err) {
		t.Fatalf("attached-client Enter error = %v, want definite refusal", err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("attached-client draft was submitted; marker stat error = %v", statErr)
	}
}

func waitForAttachedTmuxClient(t *testing.T, socketPath, paneID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		output, err := exec.Command("tmux", "-S", socketPath, "display-message", "-p", "-t", paneID, "#{session_attached}").CombinedOutput()
		if err == nil && strings.TrimSpace(string(output)) != "0" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("isolated tmux control client did not attach before deadline")
}

func createExactTargetTestSession(t *testing.T, socketPath, name, stableID string) exactPasteTarget {
	t.Helper()
	format := "#{pane_id}|#{pane_pid}|#{session_id}"
	output, err := exec.Command("tmux", "-S", socketPath,
		"new-session", "-d", "-P", "-F", format, "-s", name).CombinedOutput()
	if err != nil {
		t.Fatalf("create exact-target session: %v: %s", err, output)
	}
	if setOutput, setErr := exec.Command("tmux", "-S", socketPath,
		"set-option", "-t", name, stableSessionIdentityOption, stableID).CombinedOutput(); setErr != nil {
		t.Fatalf("bind exact-target stable ID: %v: %s", setErr, setOutput)
	}
	fields := strings.Split(strings.TrimSpace(string(output)), "|")
	if len(fields) != 3 {
		t.Fatalf("exact-target identity = %q", output)
	}
	panePID, err := strconv.Atoi(fields[1])
	if err != nil || panePID <= 0 {
		t.Fatalf("exact-target pane PID = %q", fields[1])
	}
	return exactPasteTarget{
		PaneID: fields[0], PanePID: panePID, SessionID: fields[2], StableSessionID: stableID,
	}
}

func loadTmuxBufferForTest(t *testing.T, socketPath, bufferName, content string) {
	t.Helper()
	cmd := exec.Command("tmux", "-S", socketPath, "load-buffer", "-b", bufferName, "-")
	cmd.Stdin = strings.NewReader(content)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("load exact-target buffer: %v: %s", err, output)
	}
}

func captureExactDeliveryPane(t *testing.T, socketPath, paneID string) string {
	t.Helper()
	output, err := exec.Command("tmux", "-S", socketPath, "capture-pane", "-p", "-t", paneID).CombinedOutput()
	if err != nil {
		t.Fatalf("capture exact-target pane: %v: %s", err, output)
	}
	return string(output)
}
