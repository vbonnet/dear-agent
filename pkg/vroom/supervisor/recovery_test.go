package supervisor

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestAGMRecovery_SendsWakeLoop(t *testing.T) {
	var gotName string
	var gotArgs []string
	r := &AGMRecovery{
		RunCommand: func(_ context.Context, name string, args ...string) ([]byte, error) {
			gotName = name
			gotArgs = args
			return []byte("ok"), nil
		},
	}

	if err := r.Recover(context.Background(), RoleOverseer, "heartbeat stale"); err != nil {
		t.Fatalf("Recover returned error: %v", err)
	}

	if gotName != "agm" {
		t.Errorf("binary = %q, want agm", gotName)
	}
	want := []string{"send", "wake-loop", "overseer", "--prompt", DefaultWakePrompt}
	if strings.Join(gotArgs, " ") != strings.Join(want, " ") {
		t.Errorf("args = %v, want %v", gotArgs, want)
	}
}

func TestAGMRecovery_CustomBinaryPromptAndSession(t *testing.T) {
	var gotName string
	var gotArgs []string
	r := &AGMRecovery{
		Binary:     "/usr/local/bin/agm",
		Prompt:     "/loop 10m /orchestrate",
		SessionFor: func(role Role) string { return string(role) + "-v2" },
		RunCommand: func(_ context.Context, name string, args ...string) ([]byte, error) {
			gotName = name
			gotArgs = args
			return nil, nil
		},
	}

	if err := r.Recover(context.Background(), RoleMetaOrchestrator, "stale"); err != nil {
		t.Fatalf("Recover returned error: %v", err)
	}
	if gotName != "/usr/local/bin/agm" {
		t.Errorf("binary = %q, want /usr/local/bin/agm", gotName)
	}
	want := []string{"send", "wake-loop", "meta-orchestrator-v2", "--prompt", "/loop 10m /orchestrate"}
	if strings.Join(gotArgs, " ") != strings.Join(want, " ") {
		t.Errorf("args = %v, want %v", gotArgs, want)
	}
}

func TestAGMRecovery_PropagatesCommandError(t *testing.T) {
	r := &AGMRecovery{
		RunCommand: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return []byte("session not found"), errors.New("exit status 1")
		},
	}
	err := r.Recover(context.Background(), RoleOrchestrator, "stale")
	if err == nil {
		t.Fatal("expected error from failed wake command, got nil")
	}
	if !strings.Contains(err.Error(), "orchestrator") {
		t.Errorf("error %q should name the peer session", err)
	}
	if !strings.Contains(err.Error(), "session not found") {
		t.Errorf("error %q should include command output", err)
	}
}

func TestAGMRecovery_EmptySessionIsError(t *testing.T) {
	r := &AGMRecovery{
		SessionFor: func(Role) string { return "" },
		RunCommand: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			t.Fatal("RunCommand should not be called when session is empty")
			return nil, nil
		},
	}
	if err := r.Recover(context.Background(), RoleOverseer, "stale"); err == nil {
		t.Fatal("expected error for empty session name, got nil")
	}
}
