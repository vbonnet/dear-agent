package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/send"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/state"
)

func TestIsAutonomousRole(t *testing.T) {
	cases := []struct {
		tags []string
		want bool
	}{
		{[]string{"role:worker"}, true},
		{[]string{"role:orchestrator"}, true},
		{[]string{"role:overseer"}, true},
		{[]string{"role:meta-orchestrator"}, true},
		{[]string{"role:human"}, false},
		{[]string{}, false},
		{nil, false},
		{[]string{"other-tag", "role:worker"}, true},
		{[]string{"cap:web-search"}, false},
	}
	for _, c := range cases {
		got := isAutonomousRole(c.tags)
		if got != c.want {
			t.Errorf("isAutonomousRole(%v) = %v, want %v", c.tags, got, c.want)
		}
	}
}

func TestSendViaSharedOperationsFailsClosedWhenHarnessIsNotReady(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	storage := dolt.NewMockAdapter()
	if err := storage.CreateSession(&manifest.Manifest{
		SessionID: "codex-send-id",
		Name:      "codex-send",
		Harness:   "codex-cli",
		Tmux:      manifest.Tmux{SessionName: "codex-send-tmux"},
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	tmuxClient := session.NewMockTmux()
	tmuxClient.InputReadiness = session.InputReadiness{Ready: false, State: "WRONG_HARNESS", PaneID: "%4"}

	err := sendViaSharedOperations(t.Context(), "codex-send", "sender", "message-id", "message", "", false, false, storage, tmuxClient)
	if err == nil || !strings.Contains(err.Error(), "WRONG_HARNESS") {
		t.Fatalf("sendViaSharedOperations() error = %v, want WRONG_HARNESS", err)
	}
	if got, want := tmuxClient.AtomicInputChecks, []string{"codex-send-tmux:codex-cli"}; !slices.Equal(got, want) {
		t.Fatalf("atomic input checks = %v, want %v", got, want)
	}
	if len(tmuxClient.SentCommands) != 0 || len(tmuxClient.ExactPaneDeliveries) != 0 {
		t.Fatalf("not-ready delivery sent commands=%v panes=%v", tmuxClient.SentCommands, tmuxClient.ExactPaneDeliveries)
	}
	if _, statErr := os.Stat(filepath.Join(homeDir, ".agm", "pending", "codex-send")); !os.IsNotExist(statErr) {
		t.Fatalf("failed readiness created a pending delivery path: %v", statErr)
	}
}

func TestSendViaSharedOperationsPreservesBusyComposerRecoveryPolicy(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		force      bool
		autonomous bool
	}{
		{name: "force", force: true},
		{name: "autonomous", autonomous: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			storage := dolt.NewMockAdapter()
			if err := storage.CreateSession(&manifest.Manifest{
				SessionID: "codex-send-id",
				Name:      "codex-send",
				Harness:   "codex-cli",
				Tmux:      manifest.Tmux{SessionName: "codex-send-tmux"},
			}); err != nil {
				t.Fatalf("create session: %v", err)
			}
			tmuxClient := session.NewMockTmux()
			tmuxClient.InputReadiness = session.InputReadiness{State: "QUEUE", PaneID: "%9"}

			if err := sendViaSharedOperations(t.Context(), "codex-send", "sender", "message-id", "recovery message", "", testCase.force, testCase.autonomous, storage, tmuxClient); err != nil {
				t.Fatalf("sendViaSharedOperations(%s busy) error = %v", testCase.name, err)
			}
			if len(tmuxClient.AtomicInputOptions) != 1 || !tmuxClient.AtomicInputOptions[0].AllowBusyComposer {
				t.Fatalf("atomic delivery options = %#v, want %s busy-composer recovery", tmuxClient.AtomicInputOptions, testCase.name)
			}
			if got, want := tmuxClient.ExactPaneDeliveries, []string{"%9"}; !slices.Equal(got, want) {
				t.Fatalf("%s exact-pane deliveries = %v, want %v", testCase.name, got, want)
			}
		})
	}
}

func TestBusySingleSendReachesAtomicDeliveryForForceAndAutonomous(t *testing.T) {
	previousDelegate := msgDelegate
	msgDelegate = false
	t.Cleanup(func() { msgDelegate = previousDelegate })

	for _, testCase := range []struct {
		name   string
		policy cliInputDeliveryPolicy
	}{
		{name: "force", policy: cliInputDeliveryPolicy{Force: true}},
		{name: "autonomous", policy: cliInputDeliveryPolicy{Autonomous: true}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			directCalls := 0
			err := dispatchSendByCanReceiveWithDirect(
				t.Context(),
				"busy-session",
				"busy-session-tmux",
				"sender",
				"message-id",
				"formatted message",
				"message",
				"working",
				state.CanReceiveQueue,
				nil,
				testCase.policy,
				func() error {
					directCalls++
					return nil
				},
			)
			if err != nil {
				t.Fatalf("dispatch busy %s send: %v", testCase.name, err)
			}
			if directCalls != 1 {
				t.Fatalf("atomic direct calls = %d, want 1", directCalls)
			}
		})
	}
}

func TestMultiRecipientDeliveryUsesSharedAtomicReadiness(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	adapter, err := dolt.NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("open SQLite adapter: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	if err := adapter.CreateSession(&manifest.Manifest{
		SessionID: "multi-send-id",
		Name:      "multi-send",
		Harness:   "pi-cli",
		Tmux:      manifest.Tmux{SessionName: "multi-send-tmux"},
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	tmuxClient := session.NewMockTmux()
	tmuxClient.InputReadiness = session.InputReadiness{Ready: true, State: "YES", PaneID: "%11"}
	job := &send.DeliveryJob{
		Recipient:        "multi-send",
		Sender:           "sender",
		MessageID:        "multi-message-id",
		FormattedMessage: "multi message",
		ShouldInterrupt:  true,
	}

	if err := deliveryFuncWithDependencies(t.Context(), job, adapter, tmuxClient); err != nil {
		t.Fatalf("deliveryFuncWithDependencies() error = %v", err)
	}
	if got, want := tmuxClient.AtomicInputChecks, []string{"multi-send-tmux:pi-cli"}; !slices.Equal(got, want) {
		t.Fatalf("atomic input checks = %v, want %v", got, want)
	}
	if got, want := tmuxClient.ExactPaneDeliveries, []string{"%11"}; !slices.Equal(got, want) {
		t.Fatalf("exact-pane deliveries = %v, want %v", got, want)
	}
	if got, want := tmuxClient.SentCommands, []string{"multi message"}; !slices.Equal(got, want) {
		t.Fatalf("sent commands = %v, want %v", got, want)
	}
}

func TestMultiRecipientAgyDeliveryUsesSharedAtomicReadiness(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	adapter, err := dolt.NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("open SQLite adapter: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	if err := adapter.CreateSession(&manifest.Manifest{
		SessionID: "agy-multi-send-id",
		Name:      "agy-multi-send",
		Harness:   "agy",
		Tmux:      manifest.Tmux{SessionName: "agy-multi-send-tmux"},
		Agy:       &manifest.Agy{ConversationID: "agy-native-conversation"},
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	tmuxClient := session.NewMockTmux()
	tmuxClient.InputReadiness = session.InputReadiness{Ready: true, State: "YES", PaneID: "%12"}
	job := &send.DeliveryJob{
		Recipient:        "agy-multi-send",
		Sender:           "sender",
		MessageID:        "agy-multi-message-id",
		FormattedMessage: "header\nmessage body",
	}

	if err := deliveryFuncWithDependencies(t.Context(), job, adapter, tmuxClient); err != nil {
		t.Fatalf("deliveryFuncWithDependencies() error = %v", err)
	}
	if got, want := tmuxClient.AtomicInputChecks, []string{"agy-multi-send-tmux:agy"}; !slices.Equal(got, want) {
		t.Fatalf("atomic input checks = %v, want %v", got, want)
	}
	if got, want := tmuxClient.ExactPaneDeliveries, []string{"%12"}; !slices.Equal(got, want) {
		t.Fatalf("exact-pane deliveries = %v, want %v", got, want)
	}
	if got, want := tmuxClient.SentCommands, []string{"header\nmessage body"}; !slices.Equal(got, want) {
		t.Fatalf("sent commands = %v, want %v", got, want)
	}
}

func TestDirectCLIDeliveryRejectsUnregisteredTmuxSession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	adapter, err := dolt.NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("open SQLite adapter: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	tmuxClient := session.NewMockTmux()
	tmuxClient.Sessions["legacy-only"] = true

	err = sendDirectlyWithTmux(t.Context(), "legacy-only", "sender", "message-id", "message", "", adapter, tmuxClient)
	if err == nil || !strings.Contains(err.Error(), "verified delivery") {
		t.Fatalf("sendDirectlyWithTmux() error = %v, want verified-delivery rejection", err)
	}
	if len(tmuxClient.AtomicInputChecks) != 0 || len(tmuxClient.SentCommands) != 0 {
		t.Fatalf("unregistered delivery reached tmux: checks=%v commands=%v", tmuxClient.AtomicInputChecks, tmuxClient.SentCommands)
	}
}
