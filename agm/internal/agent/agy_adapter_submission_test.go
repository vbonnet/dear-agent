package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

func TestAgyCreateSessionPreservesHandoffAfterUncertainSubmission(t *testing.T) {
	preserveAgyAdapterSeams(t)
	stateDir := t.TempDir()
	t.Setenv("AGM_STATE_DIR", stateDir)

	agyHasSession = func(string) (bool, error) { return false, nil }
	agyNewSession = func(string, string) error { return nil }
	sent := 0
	agySendCommand = func(string, string) error {
		sent++
		return tmux.MarkPromptSubmissionUncertain(errors.New("lost acknowledgement"))
	}
	waited := false
	agyWaitForPrompt = func(context.Context, string, time.Duration) error {
		waited = true
		return nil
	}
	killed := false
	agyKillSession = func(string) error {
		killed = true
		return nil
	}

	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	_, err := (&AgyAdapter{sessionStore: store}).CreateSession(SessionContext{
		Name:             "agy-uncertain",
		WorkingDirectory: "/work",
		Model:            "3.5-flash-low",
		Environment:      map[string]string{"AGY_CONVERSATION_ID": "native-conversation-id"},
	})
	if err != nil {
		t.Fatalf("CreateSession returned uncertain submission as failure: %v", err)
	}
	if killed {
		t.Fatal("uncertain submission rolled back the tmux session")
	}
	if !waited {
		t.Fatal("uncertain submission did not continue to readiness")
	}
	if sent != 1 {
		t.Fatalf("uncertain submission sent %d launch commands, want exactly one", sent)
	}
	assertOnePrivateHandoff(t, stateDir)
}

func TestAgyResumeSessionPreservesHandoffAfterUncertainSubmission(t *testing.T) {
	preserveAgyAdapterSeams(t)
	stateDir := t.TempDir()
	t.Setenv("AGM_STATE_DIR", stateDir)
	t.Setenv("TMUX", "fixture")

	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	sessionID := SessionID("adapter-session")
	if err := store.Set(sessionID, &SessionMetadata{
		TmuxName: "agy-resume-uncertain", WorkingDir: "/work", UUID: "native-conversation-id",
		Model: "3.5-flash-low",
	}); err != nil {
		t.Fatal(err)
	}
	agyHasSession = func(string) (bool, error) { return false, nil }
	agyNewSession = func(string, string) error { return nil }
	sent := 0
	agySendCommand = func(string, string) error {
		sent++
		return tmux.MarkPromptSubmissionUncertain(errors.New("lost acknowledgement"))
	}
	waited := false
	agyWaitForResumePrompt = func(context.Context, string, time.Duration) error {
		waited = true
		return nil
	}
	killed := false
	agyKillSession = func(string) error {
		killed = true
		return nil
	}

	if err := (&AgyAdapter{sessionStore: store}).ResumeSession(sessionID); err != nil {
		t.Fatalf("ResumeSession returned uncertain submission as failure: %v", err)
	}
	if killed {
		t.Fatal("uncertain submission rolled back the tmux session")
	}
	if !waited {
		t.Fatal("uncertain submission did not continue to readiness")
	}
	if sent != 1 {
		t.Fatalf("uncertain submission sent %d launch commands, want exactly one", sent)
	}
	assertOnePrivateHandoff(t, stateDir)
}
