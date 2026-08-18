package tmux

import (
	"context"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCapturePaneArgsUseCanonicalSocketAndNormalizedTarget(t *testing.T) {
	t.Setenv("AGM_TMUX_SOCKET", "/tmp/agm-capture-test.sock")

	want := []string{
		"-S", "/tmp/agm-capture-test.sock",
		"capture-pane", "-t", "session-with-separators",
		"-p", "-S", "-50",
	}
	got := CapturePaneCommandArgs("session.with:separators", 50)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CapturePaneCommandArgs() = %#v, want %#v", got, want)
	}
}

func TestCapturePaneANSIArgsPreserveStyles(t *testing.T) {
	t.Setenv("AGM_TMUX_SOCKET", "/tmp/agm-capture-test.sock")

	want := []string{
		"-S", "/tmp/agm-capture-test.sock",
		"capture-pane", "-t", "session-with-separators",
		"-p", "-e", "-S", "-12",
	}
	got := CapturePaneANSICommandArgs("session.with:separators", 12)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CapturePaneANSICommandArgs() = %#v, want %#v", got, want)
	}
}

func TestCapturePaneLogicalANSIArgsJoinFullExactPaneHistory(t *testing.T) {
	t.Setenv("AGM_TMUX_SOCKET", "/tmp/agm-capture-test.sock")

	want := []string{
		"-S", "/tmp/agm-capture-test.sock",
		"capture-pane", "-t", "%7",
		"-p", "-e", "-J", "-S", "-",
	}
	got := capturePaneTargetCommandArgsWithOptions("%7", 0, true, true)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("logical exact-pane capture args = %#v, want %#v", got, want)
	}
}

func TestRealTmuxLogicalANSICaptureJoinsNarrowPaneWraps(t *testing.T) {
	skipIfNoTmux(t)
	socketPath, cleanup := setupTestSocket(t)
	defer cleanup()

	const sessionName = "logical-wrap-capture"
	if output, err := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-x", "24", "-y", "8", "-s", sessionName).CombinedOutput(); err != nil {
		t.Fatalf("create narrow tmux pane: %v (%s)", err, output)
	}
	const logicalLine = "AGM_LOGICAL_CAPTURE_abcdefghijklmnopqrstuvwxyz_0123456789"
	command := "printf '%s\\n' " + logicalLine
	if output, err := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "-l", command).CombinedOutput(); err != nil {
		t.Fatalf("type wrapped fixture: %v (%s)", err, output)
	}
	if output, err := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "Enter").CombinedOutput(); err != nil {
		t.Fatalf("submit wrapped fixture: %v (%s)", err, output)
	}
	time.Sleep(100 * time.Millisecond)

	paneOutput, err := exec.Command("tmux", "-S", socketPath, "display-message", "-p", "-t", sessionName, "#{pane_id}").CombinedOutput()
	if err != nil {
		t.Fatalf("resolve exact pane: %v (%s)", err, paneOutput)
	}
	paneID := strings.TrimSpace(string(paneOutput))
	got, err := CapturePaneLogicalANSIOutputTargetContext(context.Background(), paneID)
	if err != nil {
		t.Fatalf("CapturePaneLogicalANSIOutputTargetContext() error = %v", err)
	}
	if !strings.Contains(stripANSI(got), logicalLine) {
		t.Fatalf("logical capture did not rejoin wrapped line %q:\n%s", logicalLine, stripANSI(got))
	}
}

func TestCapturePaneArgsUseFullHistoryForNonPositiveLines(t *testing.T) {
	t.Setenv("AGM_TMUX_SOCKET", "/tmp/agm-capture-test.sock")

	got := CapturePaneCommandArgs("session", 0)
	if got[len(got)-1] != "-" {
		t.Fatalf("CapturePaneCommandArgs() start = %q, want full history", got[len(got)-1])
	}
}

func TestCapturePaneRejectsEmptySession(t *testing.T) {
	if _, err := CapturePaneOutput("", 50); err == nil {
		t.Fatal("CapturePaneOutput() error = nil, want empty-session error")
	}
	if _, err := CapturePaneHistoryOutput(""); err == nil {
		t.Fatal("CapturePaneHistoryOutput() error = nil, want empty-session error")
	}
	if _, err := CapturePaneLogicalANSIOutput("", 50); err == nil {
		t.Fatal("CapturePaneLogicalANSIOutput() error = nil, want empty-session error")
	}
}

func TestCapturePaneCommandIsBoundedAndIsolated(t *testing.T) {
	t.Setenv("AGM_TMUX_SOCKET", "/tmp/agm-capture-test.sock")

	cmd := newCapturePaneCommand(context.Background(), "session", 50, CapturePanePolicy())
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatal("capture command must run in an isolated process group")
	}
	if cmd.Cancel == nil {
		t.Fatal("capture command must cancel its process group")
	}
	if cmd.WaitDelay != time.Second {
		t.Fatalf("capture command WaitDelay = %v, want %v", cmd.WaitDelay, time.Second)
	}
}
