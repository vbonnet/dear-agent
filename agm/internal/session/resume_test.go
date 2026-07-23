package session

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/harnessexec"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

func TestClaudeResumeStagesHandoffBeforeCreatingTmux(t *testing.T) {
	origPrepare := resumePrepareClaudeCommand
	origNewSession := resumeNewSession
	t.Cleanup(func() {
		resumePrepareClaudeCommand = origPrepare
		resumeNewSession = origNewSession
	})

	prepareErr := errors.New("state directory is unwritable")
	resumePrepareClaudeCommand = func(harnessexec.ClaudeLaunch, []string) (harnessexec.PreparedCommand, error) {
		return harnessexec.PreparedCommand{}, prepareErr
	}
	resumeNewSession = func(string, string) error {
		t.Fatal("tmux session was created before private handoff preparation")
		return nil
	}

	err := ensureClaudeResumeProcess(testClaudeResumeManifest(), false)
	if !errors.Is(err, prepareErr) {
		t.Fatalf("ensureClaudeResumeProcess error = %v, want preparation error", err)
	}
}

func TestClaudeResumeRollsBackCreatedTmuxOnDeliveryFailure(t *testing.T) {
	origPrepare := resumePrepareClaudeCommand
	origNewSession := resumeNewSession
	origSendCommand := resumeSendCommand
	origKillSession := resumeKillSession
	t.Cleanup(func() {
		resumePrepareClaudeCommand = origPrepare
		resumeNewSession = origNewSession
		resumeSendCommand = origSendCommand
		resumeKillSession = origKillSession
	})

	resumePrepareClaudeCommand = func(harnessexec.ClaudeLaunch, []string) (harnessexec.PreparedCommand, error) {
		return harnessexec.PreparedCommand{Command: "private-resume"}, nil
	}
	created := ""
	resumeNewSession = func(name, _ string) error {
		created = name
		return nil
	}
	deliveryErr := errors.New("delivery failed")
	resumeSendCommand = func(name, command string) error {
		if name != "claude-resume-test" || command != "private-resume" {
			t.Fatalf("resumeSendCommand(%q, %q)", name, command)
		}
		return deliveryErr
	}
	cleanupErr := errors.New("cleanup failed")
	killed := ""
	resumeKillSession = func(name string) error {
		killed = name
		return cleanupErr
	}

	err := ensureClaudeResumeProcess(testClaudeResumeManifest(), false)
	if created != "claude-resume-test" || killed != created {
		t.Fatalf("created/killed = %q/%q, want same attempt-owned session", created, killed)
	}
	if !errors.Is(err, deliveryErr) || !errors.Is(err, cleanupErr) {
		t.Fatalf("ensureClaudeResumeProcess error = %v, want delivery and cleanup errors", err)
	}
	if !strings.Contains(err.Error(), "clean up created tmux session") {
		t.Fatalf("ensureClaudeResumeProcess error = %v, want cleanup context", err)
	}
}

func TestClaudeResumePreservesExistingTmuxOnDeliveryFailure(t *testing.T) {
	origIsClaudeRunning := resumeIsClaudeRunning
	origPrepare := resumePrepareClaudeCommand
	origSendCommand := resumeSendCommand
	origKillSession := resumeKillSession
	t.Cleanup(func() {
		resumeIsClaudeRunning = origIsClaudeRunning
		resumePrepareClaudeCommand = origPrepare
		resumeSendCommand = origSendCommand
		resumeKillSession = origKillSession
	})

	resumeIsClaudeRunning = func(string) (bool, error) { return false, nil }
	resumePrepareClaudeCommand = func(harnessexec.ClaudeLaunch, []string) (harnessexec.PreparedCommand, error) {
		return harnessexec.PreparedCommand{Command: "private-resume"}, nil
	}
	deliveryErr := errors.New("delivery failed")
	resumeSendCommand = func(string, string) error { return deliveryErr }
	resumeKillSession = func(string) error {
		t.Fatal("delivery failure killed a pre-existing tmux session")
		return nil
	}

	err := ensureClaudeResumeProcess(testClaudeResumeManifest(), true)
	if !errors.Is(err, deliveryErr) {
		t.Fatalf("ensureClaudeResumeProcess error = %v, want delivery error", err)
	}
}

func TestClaudeResumePreservesHandoffAndCreatedTmuxAfterUncertainSubmission(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("AGM_STATE_DIR", stateDir)
	origPrepare := resumePrepareClaudeCommand
	origNewSession := resumeNewSession
	origSendCommand := resumeSendCommand
	origWaitForReady := resumeWaitForClaudeReady
	origKillSession := resumeKillSession
	t.Cleanup(func() {
		resumePrepareClaudeCommand = origPrepare
		resumeNewSession = origNewSession
		resumeSendCommand = origSendCommand
		resumeWaitForClaudeReady = origWaitForReady
		resumeKillSession = origKillSession
	})

	resumePrepareClaudeCommand = harnessexec.PrepareClaudeCommand
	resumeNewSession = func(string, string) error { return nil }
	resumeSendCommand = func(string, string) error {
		return tmux.MarkPromptSubmissionUncertain(errors.New("lost acknowledgement"))
	}
	waited := false
	resumeWaitForClaudeReady = func(string, time.Duration) error {
		waited = true
		return nil
	}
	resumeKillSession = func(string) error {
		t.Fatal("uncertain submission killed the attempt-created tmux session")
		return nil
	}

	if err := ensureClaudeResumeProcess(testClaudeResumeManifest(), false); err != nil {
		t.Fatalf("ensureClaudeResumeProcess returned uncertain submission as failure: %v", err)
	}
	if !waited {
		t.Fatal("uncertain submission did not continue to readiness")
	}
	entries, err := os.ReadDir(filepath.Join(stateDir, "private-launch"))
	if err != nil {
		t.Fatalf("read private handoff directory: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("private handoffs = %v, want exactly one preserved handoff", entries)
	}
}

func testClaudeResumeManifest() *manifest.Manifest {
	return &manifest.Manifest{
		SessionID: "agm-session-id",
		Context:   manifest.Context{Project: "/work"},
		Claude:    manifest.Claude{UUID: "claude-session-id"},
		Tmux:      manifest.Tmux{SessionName: "claude-resume-test"},
	}
}
