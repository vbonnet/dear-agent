package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/config"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
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
				false,
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

func TestAPIDeliveryUsesAdapterReadinessInsteadOfTmuxState(t *testing.T) {
	previousDelegate := msgDelegate
	msgDelegate = false
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
				directCalls := 0
				err := dispatchSendByCanReceiveWithDirect(
					t.Context(), "api-session", "api-session-tmux", "sender", "message-id",
					"formatted message", "message", "working", canReceive, nil, policyCase.policy, false, true,
					func() error {
						directCalls++
						return nil
					},
				)
				if err != nil {
					t.Fatalf("API delivery with tmux state %s error = %v", canReceive, err)
				}
				if directCalls != 1 {
					t.Fatalf("API delivery with tmux state %s direct calls = %d, want 1 adapter-status check", canReceive, directCalls)
				}
			})
		}
	}
}

func TestEnsureRecipientReadyDoesNotRequireTmuxForPureAPISession(t *testing.T) {
	previousConfig := cfg
	cfg = &config.Config{SessionsDir: t.TempDir()}
	t.Cleanup(func() { cfg = previousConfig })

	for _, harnessType := range []string{"openai", "gpt"} {
		t.Run(harnessType, func(t *testing.T) {
			storage, err := dolt.NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
			if err != nil {
				t.Fatalf("open session storage: %v", err)
			}
			t.Cleanup(func() { _ = storage.Close() })
			if err := storage.CreateSession(&manifest.Manifest{
				SessionID: "api-id-" + harnessType,
				Name:      "api-no-tmux-" + harnessType,
				Harness:   harnessType,
			}); err != nil {
				t.Fatalf("create API session: %v", err)
			}

			if err := ensureRecipientReady("api-no-tmux-"+harnessType, storage); err != nil {
				t.Fatalf("ensure API recipient ready without tmux: %v", err)
			}
		})
	}
}

func TestFailedAPIAdapterReadinessCreatesNoDelegation(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	previousDelegate := msgDelegate
	msgDelegate = true
	t.Cleanup(func() { msgDelegate = previousDelegate })

	err := dispatchSendByCanReceiveWithDirect(
		t.Context(), "api-session", "api-session", "sender", "message-id",
		"formatted message", "message", "working", state.CanReceiveNotFound, nil,
		cliInputDeliveryPolicy{Force: true}, false, true,
		func() error { return errors.New("adapter session is terminated") },
	)
	if err == nil || !strings.Contains(err.Error(), "adapter session is terminated") {
		t.Fatalf("failed API adapter readiness error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(homeDir, ".agm", "delegations")); !os.IsNotExist(statErr) {
		t.Fatalf("failed API adapter readiness created delegation state: %v", statErr)
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

func TestSingleAndMultiRecipientAPIDeliveryUsesAdapterReadiness(t *testing.T) {
	adapter, err := dolt.NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("open SQLite adapter: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	if err := adapter.CreateSession(&manifest.Manifest{
		SessionID: "multi-api-id",
		Name:      "multi-api",
		Harness:   "openai",
		OpenAI: &manifest.OpenAI{
			SessionsDir: "/api/sessions",
			BaseURL:     "https://api.example.test",
		},
		Tmux: manifest.Tmux{SessionName: "multi-api-tmux"},
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
		deliver func(t *testing.T, newAPIAgent apiAgentFactory) error
	}{
		{
			name: "single",
			deliver: func(t *testing.T, newAPIAgent apiAgentFactory) error {
				return sendDirectlyWithDependencies(t.Context(), job.Recipient, job.Sender, job.MessageID, job.FormattedMessage, job.PromptFile, adapter, session.NewMockTmux(), newAPIAgent)
			},
		},
		{
			name: "fan-out",
			deliver: func(t *testing.T, newAPIAgent apiAgentFactory) error {
				return deliveryFuncWithAgentFactory(t.Context(), job, adapter, session.NewMockTmux(), newAPIAgent)
			},
		},
	} {
		for _, testCase := range []struct {
			name        string
			status      agent.Status
			statusError error
			sendError   error
			wantSuccess bool
		}{
			{name: "active", status: agent.StatusActive, wantSuccess: true},
			{name: "idle", status: agent.StatusIdle, wantSuccess: true},
			{name: "suspended", status: agent.StatusSuspended},
			{name: "terminated", status: agent.StatusTerminated},
			{name: "status-error", statusError: errors.New("status unavailable")},
			{name: "send-error", status: agent.StatusActive, sendError: errors.New("send unavailable")},
		} {
			t.Run(surface.name+"/"+testCase.name, func(t *testing.T) {
				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)
				mockAgent := &mockAgentAdapter{
					sessionStatus: testCase.status,
					statusError:   testCase.statusError,
					sendError:     testCase.sendError,
				}
				err := surface.deliver(t, func(_ context.Context, apiManifest *manifest.Manifest) (agent.Agent, error) {
					if apiManifest.Harness != "openai" {
						t.Fatalf("API factory harness = %q, want openai", apiManifest.Harness)
					}
					if apiManifest.OpenAI == nil || apiManifest.OpenAI.SessionsDir != "/api/sessions" || apiManifest.OpenAI.BaseURL != "https://api.example.test" {
						t.Fatalf("API factory lost persisted runtime locator: %#v", apiManifest.OpenAI)
					}
					return mockAgent, nil
				})
				if testCase.wantSuccess && err != nil {
					t.Fatalf("%s API delivery status %s error = %v", surface.name, testCase.status, err)
				}
				if !testCase.wantSuccess && err == nil {
					t.Fatalf("%s API delivery %s error = nil", surface.name, testCase.name)
				}
				wantMessages := 0
				if testCase.wantSuccess {
					wantMessages = 1
				}
				if len(mockAgent.sentMessages) != wantMessages {
					t.Fatalf("%s API delivery %s sent messages = %d, want %d", surface.name, testCase.name, len(mockAgent.sentMessages), wantMessages)
				}
				_, statErr := os.Stat(filepath.Join(homeDir, ".agm", "pending", "multi-api"))
				if testCase.wantSuccess && statErr != nil {
					t.Fatalf("successful %s API delivery %s pending artifact: %v", surface.name, testCase.name, statErr)
				}
				if !testCase.wantSuccess && !os.IsNotExist(statErr) {
					t.Fatalf("failed %s API delivery %s created pending artifact: %v", surface.name, testCase.name, statErr)
				}
			})
		}
	}
}

func TestAPIDeliverySerializesReadinessCompletionAndPersistenceByStableSessionID(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := &manifest.Manifest{
		SessionID: "stable-api-session-id",
		Name:      "renamable-api-session",
		Harness:   "openai",
	}
	storage := dolt.NewMockAdapter()
	if err := storage.CreateSession(m); err != nil {
		t.Fatalf("create API session: %v", err)
	}

	var calls atomic.Int32
	factoryEntered := make(chan int32, 2)
	firstSendEntered := make(chan struct{}, 1)
	releaseFirst := make(chan struct{})
	factory := func(_ context.Context, _ *manifest.Manifest) (agent.Agent, error) {
		call := calls.Add(1)
		factoryEntered <- call
		return &mockAgentAdapter{sendFunc: func(_ agent.SessionID, _ agent.Message) error {
			if call == 1 {
				firstSendEntered <- struct{}{}
				<-releaseFirst
			}
			return nil
		}}, nil
	}

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- sendToAPIAgentIfReady(t.Context(), m, m.Name, "sender", "first-id", "first", "", storage, factory)
	}()
	select {
	case call := <-factoryEntered:
		if call != 1 {
			t.Fatalf("first factory call = %d, want 1", call)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first adapter construction did not start")
	}
	select {
	case <-firstSendEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("first provider completion did not start")
	}

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- sendToAPIAgentIfReady(t.Context(), m, m.Name, "sender", "second-id", "second", "", storage, factory)
	}()
	select {
	case call := <-factoryEntered:
		close(releaseFirst)
		t.Fatalf("adapter construction %d crossed the first session transaction", call)
	case <-time.After(150 * time.Millisecond):
	}
	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	select {
	case call := <-factoryEntered:
		if call != 2 {
			t.Fatalf("second factory call = %d, want 2", call)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second adapter construction did not start after first transaction")
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second delivery: %v", err)
	}
}

func TestDirectAPIDeliveryRejectsArchivedSessionBeforeAdapterConstruction(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	adapter, err := dolt.NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("open SQLite adapter: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	if err := adapter.CreateSession(&manifest.Manifest{
		SessionID: "archived-api-id",
		Name:      "archived-api",
		Harness:   "openai",
		Lifecycle: manifest.LifecycleArchived,
	}); err != nil {
		t.Fatalf("create archived API session: %v", err)
	}

	factoryCalled := false
	err = sendDirectlyWithDependencies(t.Context(), "archived-api-id", "sender", "message-id", "message", "", adapter, session.NewMockTmux(), func(context.Context, *manifest.Manifest) (agent.Agent, error) {
		factoryCalled = true
		return &mockAgentAdapter{}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "archived") {
		t.Fatalf("archived API send error = %v, want archived rejection", err)
	}
	if factoryCalled {
		t.Fatal("archived API send constructed an adapter")
	}
	if _, statErr := os.Stat(filepath.Join(homeDir, ".agm", "pending", "archived-api-id")); !os.IsNotExist(statErr) {
		t.Fatalf("archived API send created a pending artifact: %v", statErr)
	}
}

func TestAPIDeliveryReloadsLifecycleInsideStableSessionLock(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	storage := dolt.NewMockAdapter()
	staleActive := &manifest.Manifest{
		SessionID: "reload-api-lifecycle-id",
		Name:      "reload-api-lifecycle",
		Harness:   "openai",
	}
	if err := storage.CreateSession(staleActive); err != nil {
		t.Fatalf("create API session: %v", err)
	}
	archived, err := storage.GetSession(staleActive.SessionID)
	if err != nil {
		t.Fatalf("get API session: %v", err)
	}
	archived.Lifecycle = manifest.LifecycleArchived
	if err := storage.UpdateSession(archived); err != nil {
		t.Fatalf("archive API session: %v", err)
	}

	factoryCalled := false
	err = sendToAPIAgentIfReady(t.Context(), staleActive, staleActive.Name, "sender", "message-id", "message", "", storage, func(context.Context, *manifest.Manifest) (agent.Agent, error) {
		factoryCalled = true
		return &mockAgentAdapter{}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "archived") {
		t.Fatalf("stale active API delivery error = %v, want archived rejection", err)
	}
	if factoryCalled {
		t.Fatal("stale active API delivery constructed adapter after locked lifecycle reload")
	}
}

func TestAPIDeliveryRejectsReapingLifecycleInsideStableSessionLock(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	storage := dolt.NewMockAdapter()
	staleActive := &manifest.Manifest{
		SessionID: "reaping-api-lifecycle-id",
		Name:      "reaping-api-lifecycle",
		Harness:   "openai",
	}
	if err := storage.CreateSession(staleActive); err != nil {
		t.Fatalf("create API session: %v", err)
	}
	reaping, err := storage.GetSession(staleActive.SessionID)
	if err != nil {
		t.Fatalf("get API session: %v", err)
	}
	reaping.Lifecycle = manifest.LifecycleReaping
	if err := storage.UpdateSession(reaping); err != nil {
		t.Fatalf("mark API session reaping: %v", err)
	}

	factoryCalled := false
	err = sendToAPIAgentIfReady(t.Context(), staleActive, staleActive.Name, "sender", "message-id", "message", "", storage, func(context.Context, *manifest.Manifest) (agent.Agent, error) {
		factoryCalled = true
		return &mockAgentAdapter{}, nil
	})
	var opErr *ops.OpError
	if !errors.As(err, &opErr) || opErr.Code != ops.ErrCodeSessionNotReady || opErr.Parameters["readiness"] != "LIFECYCLE_reaping" {
		t.Fatalf("reaping API delivery error = %v, want typed lifecycle not-ready rejection", err)
	}
	if factoryCalled {
		t.Fatal("reaping API delivery constructed adapter after locked lifecycle reload")
	}
}

func TestAPIDeliveryRejectsAdapterWithoutContextDelivery(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	storage := dolt.NewMockAdapter()
	m := &manifest.Manifest{
		SessionID: "unbounded-api-adapter-id",
		Name:      "unbounded-api-adapter",
		Harness:   "openai",
	}
	if err := storage.CreateSession(m); err != nil {
		t.Fatalf("create API session: %v", err)
	}
	legacy := &mockAgentAdapter{sessionStatus: agent.StatusActive}
	legacyAgentOnly := struct{ agent.Agent }{Agent: legacy}

	err := sendToAPIAgentIfReady(t.Context(), m, m.Name, "sender", "message-id", "message", "", storage, func(context.Context, *manifest.Manifest) (agent.Agent, error) {
		return legacyAgentOnly, nil
	})
	if err == nil || !strings.Contains(err.Error(), "does not support context-aware delivery") {
		t.Fatalf("unbounded API adapter error = %v, want fail-closed context rejection", err)
	}
	if len(legacy.sentMessages) != 0 {
		t.Fatalf("unbounded API adapter received %d messages, want none", len(legacy.sentMessages))
	}
}

func TestNewAPIHarnessAdapterReportsPureAPISessionReadyWithoutTmux(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "test-api-key")

	for _, harnessType := range []string{"openai", "gpt"} {
		t.Run(harnessType, func(t *testing.T) {
			sessionsDir := t.TempDir()
			creator, err := agent.NewOpenAIAdapter(t.Context(), &agent.OpenAIConfig{
				APIKey:          "test-api-key",
				Model:           "gpt-4o",
				Temperature:     1.1,
				MaxTokens:       321,
				SessionsDir:     sessionsDir,
				BaseURL:         "https://azure.example.test",
				IsAzure:         true,
				AzureAPIVersion: "2025-01-01-preview",
			})
			if err != nil {
				t.Fatalf("create configured OpenAI adapter: %v", err)
			}
			sessionID, err := creator.CreateSession(agent.SessionContext{Name: "pure-api-" + harnessType})
			if err != nil {
				t.Fatalf("create pure API session for %q: %v", harnessType, err)
			}
			adapter, err := newAPIHarnessAdapter(t.Context(), &manifest.Manifest{
				SessionID: string(sessionID),
				Name:      "pure-api-" + harnessType,
				Harness:   harnessType,
				Model:     "gpt-3.5-turbo",
				OpenAI: &manifest.OpenAI{
					SessionsDir: sessionsDir,
					BaseURL:     "https://wrong.example.test",
				},
			})
			if err != nil {
				t.Fatalf("newAPIHarnessAdapter(%q): %v", harnessType, err)
			}
			if got := adapter.Version(); got != "gpt-4o" {
				t.Fatalf("restored API model = %q, want persisted gpt-4o", got)
			}
			status, err := adapter.GetSessionStatus(sessionID)
			if err != nil {
				t.Fatalf("get pure API session status for %q: %v", harnessType, err)
			}
			if status != agent.StatusActive {
				t.Fatalf("pure API session status for %q = %s, want %s", harnessType, status, agent.StatusActive)
			}
		})
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
