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
	if len(sent) == 0 || !strings.Contains(sent[0], "codex -m") {
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
