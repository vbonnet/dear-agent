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

func TestSendViaSharedOperationsPreservesQueuedAGMRecoveryPolicy(t *testing.T) {
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
			tmuxClient.InputReadiness = session.InputReadiness{State: "QUEUED_AGM", PaneID: "%9"}

			if err := sendViaSharedOperations(t.Context(), "codex-send", "sender", "message-id", "recovery message", "", testCase.force, testCase.autonomous, storage, tmuxClient); err != nil {
				t.Fatalf("sendViaSharedOperations(%s queued AGM) error = %v", testCase.name, err)
			}
			if len(tmuxClient.AtomicInputOptions) != 1 || !tmuxClient.AtomicInputOptions[0].AllowQueuedAGM {
				t.Fatalf("atomic delivery options = %#v, want %s queued-AGM recovery", tmuxClient.AtomicInputOptions, testCase.name)
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
				true,
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

func TestAPIForceAndAutonomousPreservePreliminaryDeliveryState(t *testing.T) {
	previousDelegate := msgDelegate
	msgDelegate = true
	t.Cleanup(func() { msgDelegate = previousDelegate })

	for _, policyCase := range []struct {
		name   string
		policy cliInputDeliveryPolicy
	}{
		{name: "force", policy: cliInputDeliveryPolicy{Force: true}},
		{name: "autonomous", policy: cliInputDeliveryPolicy{Autonomous: true}},
	} {
		for _, canReceive := range []state.CanReceive{state.CanReceiveQueue, state.CanReceiveNo, state.CanReceiveOverlay, state.CanReceiveNotFound} {
			t.Run(policyCase.name+"/"+string(canReceive), func(t *testing.T) {
				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)
				directCalls := 0
				err := dispatchSendByCanReceiveWithDirect(
					t.Context(), "api-session", "api-session-tmux", "sender", "message-id",
					"formatted message", "message", "working", canReceive, nil, policyCase.policy, false,
					func() error {
						directCalls++
						return nil
					},
				)
				if err == nil {
					t.Fatalf("unavailable API delivery state %s error = nil", canReceive)
				}
				if !strings.Contains(err.Error(), "deferred API delivery is unsupported") {
					t.Fatalf("API delivery state %s error = %v, want unsupported deferred-delivery error", canReceive, err)
				}
				if directCalls != 0 {
					t.Fatalf("API delivery state %s direct calls = %d, want 0", canReceive, directCalls)
				}
				if _, statErr := os.Stat(filepath.Join(homeDir, ".agm", "delegations")); !os.IsNotExist(statErr) {
					t.Fatalf("failed API delivery state %s created delegation state: %v", canReceive, statErr)
				}
			})
		}
	}
}

func TestSharedAtomicInputSupportIsCLIOnly(t *testing.T) {
	for _, harness := range []string{"claude-code", "codex-cli", "agy", "agy-cli", "antigravity", "gemini-cli", "opencode-cli", "pi-cli"} {
		if !supportsSharedAtomicInput(harness) {
			t.Errorf("supportsSharedAtomicInput(%q) = false", harness)
		}
	}
	for _, harness := range []string{"openai", "gpt", "", "unknown"} {
		if supportsSharedAtomicInput(harness) {
			t.Errorf("supportsSharedAtomicInput(%q) = true", harness)
		}
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

func TestSingleAndMultiRecipientAPIDeliveryRecheckCurrentReadiness(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	adapter, err := dolt.NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("open SQLite adapter: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	if err := adapter.CreateSession(&manifest.Manifest{
		SessionID: "multi-api-id",
		Name:      "multi-api",
		Harness:   "openai",
		Tmux:      manifest.Tmux{SessionName: "multi-api-tmux"},
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	job := &send.DeliveryJob{
		Recipient:        "multi-api",
		Sender:           "sender",
		MessageID:        "multi-api-message-id",
		FormattedMessage: "multi API message",
	}

	for _, surface := range []struct {
		name    string
		deliver func(t *testing.T, checkDelivery deliveryStateChecker) error
	}{
		{
			name: "single",
			deliver: func(t *testing.T, checkDelivery deliveryStateChecker) error {
				return sendDirectlyWithDependencies(t.Context(), job.Recipient, job.Sender, job.MessageID, job.FormattedMessage, job.PromptFile, adapter, session.NewMockTmux(), checkDelivery)
			},
		},
		{
			name: "fan-out",
			deliver: func(t *testing.T, checkDelivery deliveryStateChecker) error {
				return deliveryFuncWithStateChecker(t.Context(), job, adapter, session.NewMockTmux(), checkDelivery)
			},
		},
	} {
		for _, canReceive := range []state.CanReceive{state.CanReceiveQueue, state.CanReceiveNo, state.CanReceiveOverlay, state.CanReceiveNotFound} {
			t.Run(surface.name+"/"+string(canReceive), func(t *testing.T) {
				checkedTmuxName := ""
				err := surface.deliver(t, func(tmuxName string) state.CanReceive {
					checkedTmuxName = tmuxName
					return canReceive
				})
				if err == nil || !strings.Contains(err.Error(), "deferred API delivery is unsupported") {
					t.Fatalf("%s API delivery state %s error = %v, want unsupported deferred-delivery error", surface.name, canReceive, err)
				}
				if checkedTmuxName != "multi-api-tmux" {
					t.Fatalf("%s API delivery checked tmux name %q, want multi-api-tmux", surface.name, checkedTmuxName)
				}
				if _, statErr := os.Stat(filepath.Join(os.Getenv("HOME"), ".agm", "pending", "multi-api")); !os.IsNotExist(statErr) {
					t.Fatalf("failed %s API delivery state %s created pending delivery state: %v", surface.name, canReceive, statErr)
				}
			})
		}
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
