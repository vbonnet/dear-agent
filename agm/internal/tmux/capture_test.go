package tmux

import (
	"context"
	"reflect"
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
