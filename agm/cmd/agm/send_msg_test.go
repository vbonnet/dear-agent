package main

import (
	"context"
	"errors"
	"fmt"
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
	"github.com/vbonnet/dear-agent/agm/internal/messages"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/send"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

func sendAPIViaSharedOperation(ctx context.Context, recipient, senderName, messageID, formattedMessage, promptFile string, storage dolt.Storage, newAPIDelivery apiDeliveryFactory) error {
	_, err := sendViaSharedOperationsWithFactory(ctx, recipient, senderName, messageID, formattedMessage, promptFile, false, false, storage, nil, newAPIDelivery)
	return err
}

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

func TestDispatchSendByOperationOutcome(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		directErr    error
		wantQueue    string
		wantOverlay  bool
		wantOriginal bool
	}{
		{name: "delivered"},
		{name: "queue", directErr: ops.ErrSessionNotReady("recipient", "QUEUE"), wantQueue: "QUEUE"},
		{name: "queued AGM", directErr: ops.ErrSessionNotReady("recipient", "QUEUED_AGM"), wantQueue: "QUEUED_AGM"},
		{name: "permission", directErr: ops.ErrSessionNotReady("recipient", "PERMISSION"), wantQueue: "PERMISSION"},
		{name: "onboarding", directErr: ops.ErrSessionNotReady("recipient", "ONBOARDING"), wantQueue: "ONBOARDING"},
		{name: "legacy no", directErr: ops.ErrSessionNotReady("recipient", "NO"), wantQueue: "NO"},
		{name: "legacy unknown", directErr: ops.ErrSessionNotReady("recipient", "UNKNOWN"), wantQueue: "UNKNOWN"},
		{name: "overlay", directErr: ops.ErrSessionNotReady("recipient", "OVERLAY"), wantOverlay: true},
		{name: "not found", directErr: ops.ErrSessionNotReady("recipient", "NOT_FOUND"), wantOriginal: true},
		{name: "wrong harness", directErr: ops.ErrSessionNotReady("recipient", "WRONG_HARNESS"), wantOriginal: true},
		{name: "review required", directErr: ops.ErrSessionNotReady("recipient", "REVIEW_REQUIRED"), wantOriginal: true},
		{name: "unverified pane", directErr: ops.ErrSessionNotReady("recipient", "UNVERIFIED_PANE"), wantOriginal: true},
		{name: "non operation error", directErr: errors.New("provider unavailable"), wantOriginal: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			directCalls := 0
			queued := ""
			overlayCalls := 0
			err := dispatchSendByOperationOutcome(
				func() error {
					directCalls++
					return test.directErr
				},
				func(readiness string) error {
					queued = readiness
					return nil
				},
				func() error {
					overlayCalls++
					return nil
				},
			)
			if directCalls != 1 {
				t.Fatalf("direct calls = %d, want 1", directCalls)
			}
			if queued != test.wantQueue {
				t.Fatalf("queued readiness = %q, want %q", queued, test.wantQueue)
			}
			if got := overlayCalls == 1; got != test.wantOverlay {
				t.Fatalf("overlay called = %t, want %t", got, test.wantOverlay)
			}
			if test.wantOriginal && !errors.Is(err, test.directErr) {
				t.Fatalf("dispatch error = %v, want original %v", err, test.directErr)
			}
			if !test.wantOriginal && err != nil {
				t.Fatalf("dispatch error = %v, want nil", err)
			}
		})
	}
}

func TestHandleQueueConstructionError(t *testing.T) {
	t.Run("unsafe storage fails closed", func(t *testing.T) {
		constructionErr := fmt.Errorf("open message queue: %w", messages.ErrUnsafeQueueStorage)
		fallbackCalls := 0

		err := handleQueueConstructionError(constructionErr, func() error {
			fallbackCalls++
			return nil
		})

		if err != constructionErr {
			t.Fatalf("handleQueueConstructionError() error = %v, want original construction error %v", err, constructionErr)
		}
		if !errors.Is(err, messages.ErrUnsafeQueueStorage) {
			t.Fatalf("handleQueueConstructionError() error = %v, want ErrUnsafeQueueStorage identity", err)
		}
		if fallbackCalls != 0 {
			t.Fatalf("fallback calls = %d, want 0", fallbackCalls)
		}
	})

	t.Run("ordinary construction failure uses fallback", func(t *testing.T) {
		constructionErr := errors.New("queue unavailable")
		fallbackErr := errors.New("direct delivery failed")
		fallbackCalls := 0

		err := handleQueueConstructionError(constructionErr, func() error {
			fallbackCalls++
			return fallbackErr
		})

		if err != fallbackErr {
			t.Fatalf("handleQueueConstructionError() error = %v, want fallback result %v", err, fallbackErr)
		}
		if fallbackCalls != 1 {
			t.Fatalf("fallback calls = %d, want 1", fallbackCalls)
		}
	})
}

func TestOverlayRecoveryRetriesSharedOperation(t *testing.T) {
	t.Parallel()

	t.Run("left dismissal delivers", func(t *testing.T) {
		leftCalls := 0
		escapeCalls := 0
		pauseCalls := 0
		directCalls := 0
		err := retryOverlayDelivery(
			"recipient",
			func() error { leftCalls++; return nil },
			func() error { escapeCalls++; return nil },
			func() { pauseCalls++ },
			func() error { directCalls++; return nil },
			func(string) error { t.Fatal("successful retry queued"); return nil },
		)
		if err != nil {
			t.Fatalf("retryOverlayDelivery() error: %v", err)
		}
		if leftCalls != 1 || escapeCalls != 0 || pauseCalls != 1 || directCalls != 1 {
			t.Fatalf("calls left/escape/pause/direct = %d/%d/%d/%d, want 1/0/1/1", leftCalls, escapeCalls, pauseCalls, directCalls)
		}
	})

	t.Run("escape retry translates final readiness", func(t *testing.T) {
		leftCalls := 0
		escapeCalls := 0
		pauseCalls := 0
		directCalls := 0
		queued := ""
		err := retryOverlayDelivery(
			"recipient",
			func() error { leftCalls++; return nil },
			func() error { escapeCalls++; return nil },
			func() { pauseCalls++ },
			func() error {
				directCalls++
				if directCalls == 1 {
					return ops.ErrSessionNotReady("recipient", "OVERLAY")
				}
				return ops.ErrSessionNotReady("recipient", "PERMISSION")
			},
			func(readiness string) error { queued = readiness; return nil },
		)
		if err != nil {
			t.Fatalf("retryOverlayDelivery() error: %v", err)
		}
		if leftCalls != 1 || escapeCalls != 1 || pauseCalls != 2 || directCalls != 2 || queued != "PERMISSION" {
			t.Fatalf("calls left/escape/pause/direct queue = %d/%d/%d/%d %q, want 1/1/2/2 PERMISSION", leftCalls, escapeCalls, pauseCalls, directCalls, queued)
		}
	})
}

func TestPrepareRecipientDeliveryDoesNotRequireTmuxForPureAPISession(t *testing.T) {
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

			prepareRecipientDelivery("api-no-tmux-"+harnessType, storage)
		})
	}
}

func TestAPIRecipientStateSkipsTmuxPersistence(t *testing.T) {
	previousConfig := cfg
	cfg = &config.Config{SessionsDir: t.TempDir()}
	t.Cleanup(func() { cfg = previousConfig })

	storage, err := dolt.NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("open session storage: %v", err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	if err := storage.CreateSession(&manifest.Manifest{
		SessionID: "api-state-id",
		Name:      "api-state",
		Harness:   "openai",
	}); err != nil {
		t.Fatalf("create API session: %v", err)
	}

	stateResolved := false
	statePersisted := false
	currentState, tmuxName, harnessType := resolveRecipientStateWithDependencies(
		"api-state",
		storage,
		func(string, string, string, time.Time) string {
			stateResolved = true
			return manifest.StateOffline
		},
		func(string, string, string, string, *dolt.Adapter) error {
			statePersisted = true
			return nil
		},
	)
	if currentState == manifest.StateOffline || harnessType != "openai" {
		t.Fatalf("API recipient state = (%q, %q), want non-tmux state and openai", currentState, harnessType)
	}
	if stateResolved || statePersisted {
		t.Fatalf("API recipient used tmux state resolver=%t persistence=%t", stateResolved, statePersisted)
	}
	if tmuxName == "" {
		t.Fatal("API recipient lost its display-state identity")
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

func TestMultiRecipientDeliveryRenewsDeadlinePerRecipient(t *testing.T) {
	jobs := []*send.DeliveryJob{
		{Recipient: "first", MessageID: "first-message"},
		{Recipient: "second", MessageID: "second-message"},
	}
	deadlines := make([]time.Time, 0, len(jobs))
	results := deliverMultiRecipientJobs(t.Context(), jobs, time.Second, func(ctx context.Context, _ *send.DeliveryJob) error {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("multi-recipient delivery context has no deadline")
		}
		deadlines = append(deadlines, deadline)
		if len(deadlines) == 1 {
			time.Sleep(time.Millisecond)
		}
		return nil
	})
	if len(results) != len(jobs) || !results[0].Success || !results[1].Success {
		t.Fatalf("multi-recipient results = %#v, want two successes", results)
	}
	if len(deadlines) != len(jobs) || !deadlines[1].After(deadlines[0]) {
		t.Fatalf("delivery deadlines = %v, want a fresh later deadline for the second recipient", deadlines)
	}
}

func TestMultiRecipientDeliveryAllowsFullProviderDeadline(t *testing.T) {
	jobs := []*send.DeliveryJob{{Recipient: "api", MessageID: "api-message"}}
	results := deliverMultiRecipientJobs(t.Context(), jobs, agent.OpenAIDeliveryTimeout, func(ctx context.Context, _ *send.DeliveryJob) error {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("multi-recipient API delivery context has no deadline")
		}
		if remaining := time.Until(deadline); remaining <= agent.OpenAICompletionTimeout {
			t.Fatalf("multi-recipient API delivery budget = %s, must exceed provider ceiling %s", remaining, agent.OpenAICompletionTimeout)
		}
		return nil
	})
	if len(results) != 1 || !results[0].Success {
		t.Fatalf("multi-recipient API result = %#v, want success", results)
	}
}

func TestAPIDeliveryReservesFullCompletionBudgetAfterPreflight(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	storage := dolt.NewMockAdapter()
	m := &manifest.Manifest{SessionID: "budget-api-id", Name: "budget-api", Harness: "openai"}
	if err := storage.CreateSession(m); err != nil {
		t.Fatalf("create API session: %v", err)
	}
	mockAgent := &mockAgentAdapter{sessionStatus: agent.StatusActive}
	jobs := []*send.DeliveryJob{{Recipient: m.Name, MessageID: "budget-message"}}
	var factoryCtx context.Context
	results := deliverMultiRecipientJobs(t.Context(), jobs, agent.OpenAIDeliveryTimeout, func(deliveryCtx context.Context, _ *send.DeliveryJob) error {
		return sendAPIViaSharedOperation(deliveryCtx, m.Name, "sender", "budget-message", "message", "", storage, func(ctx context.Context, _ *manifest.Manifest) (ops.APISessionDeliveryAdapter, error) {
			factoryCtx = ctx
			timer := time.NewTimer(50 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-timer.C:
				return mockAgent, nil
			}
		})
	})
	if len(results) != 1 || !results[0].Success {
		t.Fatalf("API delivery result = %#v, want success after preflight", results)
	}
	preflightDeadline, ok := factoryCtx.Deadline()
	if !ok {
		t.Fatal("API reconstruction context has no preflight deadline")
	}
	if remaining := time.Until(preflightDeadline); remaining > agent.OpenAIPreflightTimeout {
		t.Fatalf("API preflight budget = %s, want at most %s", remaining, agent.OpenAIPreflightTimeout)
	}
	completionDeadline, ok := mockAgent.sendContext.Deadline()
	if !ok {
		t.Fatal("API completion parent context has no deadline")
	}
	if remaining := time.Until(completionDeadline); remaining <= agent.OpenAICompletionTimeout {
		t.Fatalf("API completion parent budget after preflight = %s, must exceed full provider ceiling %s", remaining, agent.OpenAICompletionTimeout)
	}
	if agent.OpenAIDeliveryTimeout-agent.OpenAIPreflightTimeout <= agent.OpenAICompletionTimeout {
		t.Fatalf("delivery budget %s minus preflight %s must exceed completion ceiling %s", agent.OpenAIDeliveryTimeout, agent.OpenAIPreflightTimeout, agent.OpenAICompletionTimeout)
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
		deliver func(t *testing.T, newAPIDelivery apiDeliveryFactory) error
	}{
		{
			name: "single",
			deliver: func(t *testing.T, newAPIDelivery apiDeliveryFactory) error {
				return sendDirectlyWithDependencies(t.Context(), job.Recipient, job.Sender, job.MessageID, job.FormattedMessage, job.PromptFile, adapter, session.NewMockTmux(), newAPIDelivery)
			},
		},
		{
			name: "fan-out",
			deliver: func(t *testing.T, newAPIDelivery apiDeliveryFactory) error {
				return deliveryFuncWithDeliveryFactory(t.Context(), job, adapter, session.NewMockTmux(), newAPIDelivery)
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
				err := surface.deliver(t, func(_ context.Context, apiManifest *manifest.Manifest) (ops.APISessionDeliveryAdapter, error) {
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
	factory := func(_ context.Context, _ *manifest.Manifest) (ops.APISessionDeliveryAdapter, error) {
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
		firstDone <- sendAPIViaSharedOperation(t.Context(), m.SessionID, "sender", "first-id", "first", "", storage, factory)
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
		secondDone <- sendAPIViaSharedOperation(t.Context(), m.SessionID, "sender", "second-id", "second", "", storage, factory)
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

func TestAPIDeliveryPassesCallerContextToReadiness(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	type contextKey struct{}
	wantCtx := context.WithValue(t.Context(), contextKey{}, "readiness-request")
	storage := dolt.NewMockAdapter()
	m := &manifest.Manifest{
		SessionID: "context-api-session-id",
		Name:      "context-api-session",
		Harness:   "openai",
	}
	if err := storage.CreateSession(m); err != nil {
		t.Fatalf("create API session: %v", err)
	}
	mockAgent := &mockAgentAdapter{sessionStatus: agent.StatusActive}
	err := sendAPIViaSharedOperation(wantCtx, m.Name, "sender", "message-id", "message", "", storage, func(context.Context, *manifest.Manifest) (ops.APISessionDeliveryAdapter, error) {
		return mockAgent, nil
	})
	if err != nil {
		t.Fatalf("context-aware API delivery: %v", err)
	}
	if mockAgent.statusContext == nil || mockAgent.statusContext.Value(contextKey{}) != "readiness-request" {
		t.Fatalf("API readiness context = %v, want caller context", mockAgent.statusContext)
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
	err = sendDirectlyWithDependencies(t.Context(), "archived-api-id", "sender", "message-id", "message", "", adapter, session.NewMockTmux(), func(context.Context, *manifest.Manifest) (ops.APISessionDeliveryAdapter, error) {
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
	err = sendAPIViaSharedOperation(t.Context(), staleActive.SessionID, "sender", "message-id", "message", "", storage, func(context.Context, *manifest.Manifest) (ops.APISessionDeliveryAdapter, error) {
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

func TestAPIDeliveryUsesLockedManifestForAuditArtifact(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	storage := dolt.NewMockAdapter()
	stale := &manifest.Manifest{
		SessionID: "renamed-api-session-id",
		Name:      "old-api-name",
		Harness:   "openai",
	}
	if err := storage.CreateSession(stale); err != nil {
		t.Fatalf("create API session: %v", err)
	}
	current, err := storage.GetSession(stale.SessionID)
	if err != nil {
		t.Fatalf("get API session: %v", err)
	}
	current.Name = "current-api-name"
	if err := storage.UpdateSession(current); err != nil {
		t.Fatalf("rename API session: %v", err)
	}

	err = sendAPIViaSharedOperation(t.Context(), stale.SessionID, "sender", "message-id", "message", "", storage, func(context.Context, *manifest.Manifest) (ops.APISessionDeliveryAdapter, error) {
		return &mockAgentAdapter{sessionStatus: agent.StatusActive}, nil
	})
	if err != nil {
		t.Fatalf("deliver to renamed API session: %v", err)
	}
	if _, err := os.Stat(filepath.Join(homeDir, ".agm", "pending", "current-api-name")); err != nil {
		t.Fatalf("current-name pending artifact: %v", err)
	}
	if _, err := os.Stat(filepath.Join(homeDir, ".agm", "pending", "old-api-name")); !os.IsNotExist(err) {
		t.Fatalf("stale-name pending artifact exists: %v", err)
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
	err = sendAPIViaSharedOperation(t.Context(), staleActive.SessionID, "sender", "message-id", "message", "", storage, func(context.Context, *manifest.Manifest) (ops.APISessionDeliveryAdapter, error) {
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

func TestNewAPISessionDeliveryAdapterReportsPureAPISessionReadyWithoutTmux(t *testing.T) {
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
			adapter, err := newAPIDeliveryAdapter(t.Context(), &manifest.Manifest{
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
				t.Fatalf("newAPIDeliveryAdapter(%q): %v", harnessType, err)
			}
			openAIAdapter, ok := adapter.(*agent.OpenAIAdapter)
			if !ok {
				t.Fatalf("newAPIDeliveryAdapter(%q) = %T, want *agent.OpenAIAdapter", harnessType, adapter)
			}
			if got := openAIAdapter.Version(); got != "gpt-4o" {
				t.Fatalf("restored API model = %q, want persisted gpt-4o", got)
			}
			status, err := openAIAdapter.GetSessionStatus(sessionID)
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
	var opErr *ops.OpError
	if !errors.As(err, &opErr) || opErr.Code != ops.ErrCodeSessionNotFound {
		t.Fatalf("sendDirectlyWithTmux() error = %v, want %s", err, ops.ErrCodeSessionNotFound)
	}
	if len(tmuxClient.AtomicInputChecks) != 0 || len(tmuxClient.SentCommands) != 0 {
		t.Fatalf("unregistered delivery reached tmux: checks=%v commands=%v", tmuxClient.AtomicInputChecks, tmuxClient.SentCommands)
	}
}
