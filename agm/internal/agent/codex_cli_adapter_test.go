package agent

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCodexCreateSessionWaitsForComposer(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	origLookPath := lookPath
	origHasSession := codexHasSession
	origNewSession := codexNewSession
	origSendCommand := codexSendCommand
	origWaitForPrompt := codexWaitForPrompt
	t.Cleanup(func() {
		lookPath = origLookPath
		codexHasSession = origHasSession
		codexNewSession = origNewSession
		codexSendCommand = origSendCommand
		codexWaitForPrompt = origWaitForPrompt
	})

	lookPath = func(file string) (string, error) {
		if file == "codex" {
			return "/fake/codex", nil
		}
		return "", os.ErrNotExist
	}
	codexHasSession = func(string) (bool, error) { return false, nil }
	codexNewSession = func(string, string) error { return nil }

	var sent []string
	codexSendCommand = func(_ string, cmd string) error {
		sent = append(sent, cmd)
		return nil
	}

	waited := false
	codexWaitForPrompt = func(sessionName string, timeout time.Duration) error {
		waited = true
		if sessionName != "codex-wait-test" {
			t.Fatalf("codexWaitForPrompt session = %q, want codex-wait-test", sessionName)
		}
		if timeout != 30*time.Second {
			t.Fatalf("codexWaitForPrompt timeout = %v, want 30s", timeout)
		}
		return nil
	}

	adapter := &CodexCLIAdapter{sessionStore: &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}}
	_, err := adapter.CreateSession(SessionContext{
		Name:             "codex-wait-test",
		WorkingDirectory: "/work",
		Environment:      map[string]string{"AGM_MODEL": "5.4"},
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if !waited {
		t.Fatal("CreateSession did not wait for the Codex composer")
	}
	if len(sent) == 0 || !strings.Contains(sent[0], "__exec-codex") {
		t.Fatalf("CreateSession sent commands = %v, want Codex launch", sent)
	}
}

func TestCodexCreateSessionStoresMetadataEvenIfComposerWaitTimesOut(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	origLookPath := lookPath
	origHasSession := codexHasSession
	origNewSession := codexNewSession
	origSendCommand := codexSendCommand
	origWaitForPrompt := codexWaitForPrompt
	t.Cleanup(func() {
		lookPath = origLookPath
		codexHasSession = origHasSession
		codexNewSession = origNewSession
		codexSendCommand = origSendCommand
		codexWaitForPrompt = origWaitForPrompt
	})

	lookPath = func(file string) (string, error) {
		if file == "codex" {
			return "/fake/codex", nil
		}
		return "", os.ErrNotExist
	}
	codexHasSession = func(string) (bool, error) { return false, nil }
	codexNewSession = func(string, string) error { return nil }
	codexSendCommand = func(string, string) error { return nil }
	codexWaitForPrompt = func(string, time.Duration) error { return errors.New("timeout") }

	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	adapter := &CodexCLIAdapter{sessionStore: store}
	sessionID, err := adapter.CreateSession(SessionContext{Name: "codex-wait-timeout", WorkingDirectory: "/work"})
	if err != nil {
		t.Fatalf("CreateSession should keep prompt wait timeout non-fatal, got: %v", err)
	}
	if _, err := store.Get(sessionID); err != nil {
		t.Fatalf("CreateSession did not store metadata after non-fatal prompt wait timeout: %v", err)
	}
}

func TestCodexResumeSessionRestartsDeadProcess(t *testing.T) {
	t.Setenv("TMUX", "/tmp/fake-tmux")
	// Isolate the codex trust pre-write from the developer's real ~/.codex.
	t.Setenv("CODEX_HOME", t.TempDir())

	origHasSession := codexHasSession
	origNewSession := codexNewSession
	origSendCommand := codexSendCommand
	origWaitForPrompt := codexWaitForPrompt
	origIsProcessRunning := codexIsProcessRunning
	t.Cleanup(func() {
		codexHasSession = origHasSession
		codexNewSession = origNewSession
		codexSendCommand = origSendCommand
		codexWaitForPrompt = origWaitForPrompt
		codexIsProcessRunning = origIsProcessRunning
	})

	codexHasSession = func(string) (bool, error) { return true, nil }
	codexNewSession = func(string, string) error {
		t.Fatal("ResumeSession should not create an existing tmux session")
		return nil
	}
	codexIsProcessRunning = func(sessionName, processName string) (bool, error) {
		if sessionName != "codex-resume-dead" || processName != "codex" {
			t.Fatalf("codexIsProcessRunning(%q, %q), want codex-resume-dead/codex", sessionName, processName)
		}
		return false, nil
	}

	var sent string
	codexSendCommand = func(_ string, cmd string) error {
		sent = cmd
		return nil
	}
	waited := false
	codexWaitForPrompt = func(sessionName string, timeout time.Duration) error {
		waited = true
		if sessionName != "codex-resume-dead" {
			t.Fatalf("codexWaitForPrompt session = %q, want codex-resume-dead", sessionName)
		}
		if timeout != 5*time.Second {
			t.Fatalf("codexWaitForPrompt timeout = %v, want 5s", timeout)
		}
		return nil
	}

	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{
		"session-id": {
			TmuxName:   "codex-resume-dead",
			WorkingDir: "/work",
			UUID:       "native-codex-session",
		},
	}}
	adapter := &CodexCLIAdapter{sessionStore: store}
	if err := adapter.ResumeSession("session-id"); err != nil {
		t.Fatalf("ResumeSession returned error: %v", err)
	}
	if !strings.Contains(sent, "__exec-codex") || !strings.Contains(sent, "--resume-id 'native-codex-session'") {
		t.Fatalf("ResumeSession sent %q, want Codex resume command", sent)
	}
	if !waited {
		t.Fatal("ResumeSession did not wait for Codex prompt after restart")
	}
}

func TestCodexResumeSessionSkipsRunningProcess(t *testing.T) {
	t.Setenv("TMUX", "/tmp/fake-tmux")

	origHasSession := codexHasSession
	origSendCommand := codexSendCommand
	origIsProcessRunning := codexIsProcessRunning
	t.Cleanup(func() {
		codexHasSession = origHasSession
		codexSendCommand = origSendCommand
		codexIsProcessRunning = origIsProcessRunning
	})

	codexHasSession = func(string) (bool, error) { return true, nil }
	codexIsProcessRunning = func(string, string) (bool, error) { return true, nil }
	codexSendCommand = func(string, string) error {
		t.Fatal("ResumeSession should not send a resume command when Codex is already running")
		return nil
	}

	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{
		"session-id": {TmuxName: "codex-running", WorkingDir: "/work"},
	}}
	adapter := &CodexCLIAdapter{sessionStore: store}
	if err := adapter.ResumeSession("session-id"); err != nil {
		t.Fatalf("ResumeSession returned error: %v", err)
	}
}

func TestCodexExecuteCommandSetDirSendsEnter(t *testing.T) {
	origSendCommand := codexSendCommand
	t.Cleanup(func() { codexSendCommand = origSendCommand })

	var sent string
	codexSendCommand = func(_ string, cmd string) error {
		sent = cmd
		return nil
	}

	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{
		"session-id": {TmuxName: "codex-setdir"},
	}}
	adapter := &CodexCLIAdapter{sessionStore: store}
	err := adapter.ExecuteCommand(Command{
		Type: CommandSetDir,
		Params: map[string]any{
			"session_id": "session-id",
			"path":       "/tmp/work-dir",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteCommand returned error: %v", err)
	}
	want := "cd '/tmp/work-dir'\r"
	if sent != want {
		t.Fatalf("ExecuteCommand sent %q, want %q", sent, want)
	}
}
