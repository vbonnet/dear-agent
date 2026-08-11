package ops

import (
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

func TestGetSessionOutput_LivePane(t *testing.T) {
	m := testManifest("worker-out", "", time.Time{})
	storage := &mockStorage{sessions: []*manifest.Manifest{m}}
	tmux := session.NewMockTmux()
	tmux.Sessions["worker-out"] = true
	tmux.PaneContents = map[string]string{"worker-out": "build ok\nall tests passed\n"}

	result, err := GetSessionOutput(&OpContext{Storage: storage, Tmux: tmux}, &GetSessionOutputRequest{Identifier: "worker-out"})
	if err != nil {
		t.Fatalf("GetSessionOutput: %v", err)
	}
	if result.Source != "live-pane" {
		t.Fatalf("source = %q, want live-pane", result.Source)
	}
	if !strings.Contains(result.Output, "all tests passed") {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestGetSessionOutput_FallsBackToFinalCapture(t *testing.T) {
	m := testManifest("worker-done", "", time.Time{})
	m.FinalOutput = "RESULT: shipped PR #7"
	m.FinalOutputAt = time.Now()
	storage := &mockStorage{sessions: []*manifest.Manifest{m}}
	tmux := session.NewMockTmux() // tmux session gone

	result, err := GetSessionOutput(&OpContext{Storage: storage, Tmux: tmux}, &GetSessionOutputRequest{Identifier: "worker-done"})
	if err != nil {
		t.Fatalf("GetSessionOutput: %v", err)
	}
	if result.Source != "final-capture" {
		t.Fatalf("source = %q, want final-capture", result.Source)
	}
	if result.Output != "RESULT: shipped PR #7" {
		t.Fatalf("output = %q", result.Output)
	}
}

func TestGetSessionOutput_NoOutputAnywhere(t *testing.T) {
	m := testManifest("worker-empty", "", time.Time{})
	storage := &mockStorage{sessions: []*manifest.Manifest{m}}
	tmux := session.NewMockTmux()

	if _, err := GetSessionOutput(&OpContext{Storage: storage, Tmux: tmux}, &GetSessionOutputRequest{Identifier: "worker-empty"}); err == nil {
		t.Fatal("expected error when no output exists anywhere")
	}
}

func TestListSessions_NoTmuxReportsUnknownNotStopped(t *testing.T) {
	m := testManifest("live-worker", "", time.Time{})
	storage := &mockStorage{sessions: []*manifest.Manifest{m}}

	result, err := ListSessions(&OpContext{Storage: storage}, &ListSessionsRequest{})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(result.Sessions) != 1 {
		t.Fatalf("got %d sessions", len(result.Sessions))
	}
	if got := result.Sessions[0].Status; got != "unknown" {
		t.Fatalf("status without tmux = %q, want unknown (reporting stopped made live sessions look dead, ce-0zng9)", got)
	}
}
