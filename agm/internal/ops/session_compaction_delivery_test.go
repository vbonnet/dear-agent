package ops

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/compaction"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

type compactionDeliverySender struct {
	session.TmuxInterface

	readiness session.InputReadiness
	err       error
	calls     int
	delivered int
	ctx       context.Context
	tmuxName  string
	harness   string
	command   string
	options   session.InputDeliveryOptions
	// omitRuntimeIdentity lets receipt-validation regressions model a broken
	// AtomicInputSender. Normal fake success models the production runtime by
	// returning both tmux incarnation and stable binding.
	omitRuntimeIdentity bool
}

func (s *compactionDeliverySender) SendKeysIfInputReady(
	ctx context.Context,
	tmuxName, harness, command string,
	options session.InputDeliveryOptions,
) (session.InputReadiness, error) {
	s.calls++
	s.ctx = ctx
	s.tmuxName = tmuxName
	s.harness = harness
	s.command = command
	s.options = options
	if s.readiness.Ready && !s.omitRuntimeIdentity {
		if s.readiness.PanePID == 0 {
			s.readiness.PanePID = 9001
		}
		if s.readiness.TargetSessionID == "" {
			s.readiness.TargetSessionID = "$test"
		}
		if s.readiness.StableSessionID == "" {
			s.readiness.StableSessionID = options.ExpectedStableSessionID
		}
		if s.readiness.HarnessStartTime == "" {
			s.readiness.HarnessStartTime = "Thu Aug 27 07:00:00 2026"
		}
	}
	if s.err == nil && s.readiness.Ready {
		s.delivered++
	}
	return s.readiness, s.err
}

func TestDeliverSessionCompactionReturnsExactRuntimeIdentity(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	storage := dolt.NewMockAdapter()
	if err := storage.CreateSession(&manifest.Manifest{
		SessionID: "stable-session-id",
		Name:      "friendly-name",
		Harness:   "agy-cli",
		Tmux:      manifest.Tmux{SessionName: "runtime-tmux"},
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	requestCtx := context.WithValue(t.Context(), compactionDeliveryContextKey{}, "request-value")
	baseDir := t.TempDir()
	sender := &compactionDeliverySender{readiness: session.InputReadiness{
		Ready:            true,
		State:            "YES",
		PaneID:           "%17",
		PanePID:          1717,
		TargetPID:        4242,
		HarnessStartTime: "Thu Aug 27 07:00:00 2026",
		TargetSessionID:  "$17",
		StableSessionID:  "stable-session-id",
	}}

	result, err := DeliverSessionCompaction(&OpContext{
		Context:           requestCtx,
		Storage:           storage,
		Tmux:              sender,
		CompactionBaseDir: baseDir,
	}, &SessionCompactionDeliveryRequest{
		Identifier: "stable-session-id",
		Command:    "/compact preserve the delivery receipt\n- keep exact runtime identity",
	})
	if err != nil {
		t.Fatalf("DeliverSessionCompaction() error = %v", err)
	}
	if result == nil {
		t.Fatal("DeliverSessionCompaction() result is nil")
	}
	if result.Operation != "deliver_session_compaction" ||
		result.SessionID != "stable-session-id" ||
		result.Name != "friendly-name" ||
		result.TmuxName != "runtime-tmux" ||
		result.Harness != "agy" ||
		result.PaneID != "%17" ||
		result.PanePID != 1717 ||
		result.TargetPID != 4242 ||
		result.HarnessStartTime != "Thu Aug 27 07:00:00 2026" ||
		result.TmuxSessionID != "$17" ||
		!result.Delivered ||
		!result.MayHaveStarted ||
		result.PostSubmitProcessing ||
		result.AccountingPending ||
		result.AttemptID == "" ||
		result.AttemptOutcome != compaction.AttemptOutcomeConfirmed {
		t.Fatalf("DeliverSessionCompaction() result = %#v", result)
	}
	prompt, readErr := os.ReadFile(result.PromptFile)
	if readErr != nil {
		t.Fatalf("read prompt audit: %v", readErr)
	}
	if string(prompt) != "/compact preserve the delivery receipt\n- keep exact runtime identity" {
		t.Fatalf("prompt audit = %q", prompt)
	}
	if filepath.Base(result.PromptFile) != "stable-session-id-compact-1.md" {
		t.Fatalf("prompt audit path = %q", result.PromptFile)
	}
	if sender.calls != 1 || sender.delivered != 1 {
		t.Fatalf("sender calls/deliveries = %d/%d, want 1/1", sender.calls, sender.delivered)
	}
	if sender.ctx != requestCtx || sender.ctx.Value(compactionDeliveryContextKey{}) != "request-value" {
		t.Fatal("atomic sender did not receive the request context")
	}
	if sender.tmuxName != "runtime-tmux" || sender.harness != "agy" || sender.command != "/compact preserve the delivery receipt\n- keep exact runtime identity" {
		t.Fatalf("atomic sender target = %q/%q command %q", sender.tmuxName, sender.harness, sender.command)
	}
	if sender.options.AllowQueuedAGM {
		t.Fatal("compaction delivery unexpectedly allowed queued AGM input")
	}
	if !sender.options.RequireSubmissionConfirmation {
		t.Fatal("compaction delivery did not require post-Enter submission confirmation")
	}
	if !sender.options.RawBracketedPaste {
		t.Fatal("multiline compaction delivery did not require raw bracketed paste")
	}
	if sender.options.ExpectedStableSessionID != "stable-session-id" {
		t.Fatalf("stable delivery binding = %q, want stable-session-id", sender.options.ExpectedStableSessionID)
	}
}

func TestDeliverSessionCompactionUsesManifestFallbacks(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	storage := dolt.NewMockAdapter()
	if err := storage.CreateSession(&manifest.Manifest{SessionID: "legacy-id", Name: "legacy-name"}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	sender := &compactionDeliverySender{readiness: session.InputReadiness{Ready: true, State: "YES", PaneID: "%2", TargetPID: 2002}}

	result, err := DeliverSessionCompaction(&OpContext{Storage: storage, Tmux: sender, CompactionBaseDir: t.TempDir()}, &SessionCompactionDeliveryRequest{
		Identifier: "legacy-id",
		Command:    "/compact",
	})
	if err != nil {
		t.Fatalf("DeliverSessionCompaction() error = %v", err)
	}
	if result.TmuxName != "legacy-name" || result.Harness != manifest.DefaultHarness {
		t.Fatalf("fallback identity = %q/%q, want legacy-name/%s", result.TmuxName, result.Harness, manifest.DefaultHarness)
	}
}

func TestDeliverSessionCompactionNotReadyRecordsDefiniteNonDelivery(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	storage := compactionDeliveryStorage(t, &manifest.Manifest{SessionID: "id-not-ready", Name: "not-ready"})
	sender := &compactionDeliverySender{readiness: session.InputReadiness{Ready: false, State: "NO", PaneID: "%3"}}

	result, err := DeliverSessionCompaction(&OpContext{
		Storage: storage, Tmux: sender, CompactionBaseDir: t.TempDir(),
	}, &SessionCompactionDeliveryRequest{Identifier: "id-not-ready", Command: "/compact"})
	if result == nil || result.SessionID != "id-not-ready" || result.AttemptID == "" {
		t.Fatalf("non-delivery result = %#v, want resolved stable identity and attempt", result)
	}
	var opErr *OpError
	if !errors.As(err, &opErr) || opErr.Code != ErrCodeSessionNotReady {
		t.Fatalf("DeliverSessionCompaction() error = %v, want %s", err, ErrCodeSessionNotReady)
	}
	if result.Delivered || result.MayHaveStarted || result.AccountingPending || result.AttemptOutcome != compaction.AttemptOutcomeDefiniteNotSent {
		t.Fatalf("non-delivery accounting = %#v", result)
	}
	if sender.delivered != 0 || sender.options.AllowQueuedAGM {
		t.Fatalf("not-ready sender deliveries/options = %d/%#v", sender.delivered, sender.options)
	}
}

func TestDeliverSessionCompactionRejectsNonCompactionCommandBeforeResolution(t *testing.T) {
	result, err := DeliverSessionCompaction(&OpContext{}, &SessionCompactionDeliveryRequest{
		Identifier: "target",
		Command:    "/help",
	})
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}
	var opErr *OpError
	if !errors.As(err, &opErr) || opErr.Code != ErrCodeInvalidInput {
		t.Fatalf("DeliverSessionCompaction() error = %v, want %s", err, ErrCodeInvalidInput)
	}
}

func TestDeliverSessionCompactionRejectsTerminalControlsBeforeResolution(t *testing.T) {
	for _, command := range []string{
		"/compact preserve\r/help",
		"/compact preserve\r\n/help",
		"/compact preserve\x1b[201~/help",
		"/compact preserve\x00/help",
		"/compact preserve\t/help",
		"/compact preserve\x7f/help",
		"/compact preserve\u009b/help",
		"/compact preserve\xff/help",
	} {
		result, err := DeliverSessionCompaction(&OpContext{}, &SessionCompactionDeliveryRequest{
			Identifier: "target",
			Command:    command,
		})
		if result != nil {
			t.Fatalf("result for %q = %#v, want nil", command, result)
		}
		var opErr *OpError
		if !errors.As(err, &opErr) || opErr.Code != ErrCodeInvalidInput {
			t.Fatalf("DeliverSessionCompaction(%q) error = %v, want %s", command, err, ErrCodeInvalidInput)
		}
	}
}

func TestDeliverSessionCompactionHonorsCanceledRequest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	storage := compactionDeliveryStorage(t, &manifest.Manifest{SessionID: "cancel-id", Name: "cancel-name"})
	sender := &compactionDeliverySender{readiness: session.InputReadiness{Ready: true, State: "YES", PaneID: "%4", TargetPID: 4004}}
	requestCtx, cancel := context.WithCancel(t.Context())
	cancel()

	result, err := DeliverSessionCompaction(&OpContext{
		Context:           requestCtx,
		Storage:           storage,
		Tmux:              sender,
		CompactionBaseDir: t.TempDir(),
	}, &SessionCompactionDeliveryRequest{Identifier: "cancel-id", Command: "/compact"})
	if result != nil {
		t.Fatalf("canceled delivery result = %#v, want nil before lock", result)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("DeliverSessionCompaction() error = %v, want context.Canceled", err)
	}
	if sender.calls != 0 || sender.delivered != 0 {
		t.Fatalf("canceled request reached sender: calls/deliveries = %d/%d", sender.calls, sender.delivered)
	}
}

func TestDeliverSessionCompactionReloadsLifecycleUnderStableIDLock(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	active := &manifest.Manifest{SessionID: "lifecycle-id", Name: "lifecycle-name", Harness: "claude-code"}
	archived := *active
	archived.Name = "renamed-before-archive"
	archived.Lifecycle = manifest.LifecycleArchived
	storage := &compactionDeliveryReloadStorage{initial: active, reloaded: &archived}
	sender := &compactionDeliverySender{readiness: session.InputReadiness{Ready: true, State: "YES", PaneID: "%5", TargetPID: 5005}}

	result, err := DeliverSessionCompaction(&OpContext{Storage: storage, Tmux: sender, CompactionBaseDir: t.TempDir()}, &SessionCompactionDeliveryRequest{
		Identifier: "lifecycle-id",
		Command:    "/compact",
	})
	if result == nil || result.SessionID != "lifecycle-id" || result.Name != "renamed-before-archive" {
		t.Fatalf("reload result = %#v, want current stable identity", result)
	}
	var opErr *OpError
	if !errors.As(err, &opErr) || opErr.Code != ErrCodeSessionArchived {
		t.Fatalf("DeliverSessionCompaction() error = %v, want %s", err, ErrCodeSessionArchived)
	}
	if storage.getCalls != 2 {
		t.Fatalf("GetSession() calls = %d, want initial resolve plus locked reload", storage.getCalls)
	}
	if sender.calls != 0 || sender.delivered != 0 {
		t.Fatal("lifecycle transition reached atomic sender")
	}
}

func TestDeliverSessionCompactionRejectsReloadedNonActiveLifecycle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	active := &manifest.Manifest{SessionID: "reaping-id", Name: "reaping-name", Harness: "claude-code"}
	reaping := *active
	reaping.Lifecycle = manifest.LifecycleReaping
	storage := &compactionDeliveryReloadStorage{initial: active, reloaded: &reaping}
	sender := &compactionDeliverySender{readiness: session.InputReadiness{Ready: true, State: "YES", PaneID: "%8", TargetPID: 8008}}

	result, err := DeliverSessionCompaction(&OpContext{Storage: storage, Tmux: sender, CompactionBaseDir: t.TempDir()}, &SessionCompactionDeliveryRequest{
		Identifier: "reaping-id",
		Command:    "/compact",
	})
	if result == nil || result.SessionID != "reaping-id" {
		t.Fatalf("reload result = %#v, want current stable identity", result)
	}
	var opErr *OpError
	if !errors.As(err, &opErr) || opErr.Code != ErrCodeSessionNotReady {
		t.Fatalf("DeliverSessionCompaction() error = %v, want %s", err, ErrCodeSessionNotReady)
	}
	if sender.calls != 0 || sender.delivered != 0 {
		t.Fatal("non-active lifecycle reached atomic sender")
	}
}

func TestDeliverSessionCompactionRejectsPureAPISession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	storage := compactionDeliveryStorage(t, &manifest.Manifest{
		SessionID: "api-id",
		Name:      "api-name",
		Harness:   "openai",
		OpenAI:    &manifest.OpenAI{},
	})
	sender := &compactionDeliverySender{readiness: session.InputReadiness{Ready: true, State: "YES", PaneID: "%6", TargetPID: 6006}}

	result, err := DeliverSessionCompaction(&OpContext{Storage: storage, Tmux: sender, CompactionBaseDir: t.TempDir()}, &SessionCompactionDeliveryRequest{
		Identifier: "api-id",
		Command:    "/compact",
	})
	if result == nil || result.SessionID != "api-id" {
		t.Fatalf("API rejection result = %#v, want stable identity", result)
	}
	var opErr *OpError
	if !errors.As(err, &opErr) || opErr.Code != ErrCodeSessionNotReady {
		t.Fatalf("DeliverSessionCompaction() error = %v, want %s", err, ErrCodeSessionNotReady)
	}
	if sender.calls != 0 || sender.delivered != 0 {
		t.Fatal("pure API session reached tmux sender")
	}
}

func TestDeliverSessionCompactionPersistsUncertainOutcomeBeforeReturning(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	baseDir := t.TempDir()
	storage := compactionDeliveryStorage(t, &manifest.Manifest{SessionID: "uncertain-id", Name: "uncertain-name"})
	cause := errors.New("final Enter acknowledgement lost")
	sender := &compactionDeliverySender{
		readiness: session.InputReadiness{
			Ready:          false,
			State:          "UNKNOWN",
			PaneID:         "%31",
			TargetPID:      3131,
			MayHaveStarted: true,
		},
		err: cause,
	}

	result, err := DeliverSessionCompaction(&OpContext{
		Storage: storage, Tmux: sender, CompactionBaseDir: baseDir,
	}, &SessionCompactionDeliveryRequest{Identifier: "uncertain-id", Command: "/compact preserve identity"})
	var opErr *OpError
	if !errors.As(err, &opErr) || opErr.Code != ErrCodeDeliveryUncertain || !errors.Is(err, cause) {
		t.Fatalf("DeliverSessionCompaction() error = %v, want %s preserving cause", err, ErrCodeDeliveryUncertain)
	}
	if result == nil || result.Delivered || !result.MayHaveStarted || result.AccountingPending ||
		result.PaneID != "%31" || result.TargetPID != 3131 || result.AttemptID == "" ||
		result.AttemptOutcome != compaction.AttemptOutcomeUncertain {
		t.Fatalf("uncertain result = %#v", result)
	}
	state := readCompactionDeliveryLedger(t, baseDir, "uncertain-id")
	if len(state.History) != 1 || state.History[0].AttemptID != result.AttemptID ||
		state.History[0].Outcome != compaction.AttemptOutcomeUncertain || state.CompactionCount != 1 {
		t.Fatalf("uncertain ledger = %#v", state)
	}
}

func TestDeliverSessionCompactionConfirmedWithoutExactReceiptIsUncertain(t *testing.T) {
	for _, test := range []struct {
		name                string
		readiness           session.InputReadiness
		omitRuntimeIdentity bool
	}{
		{name: "missing pane", readiness: session.InputReadiness{Ready: true, State: "YES", TargetPID: 3232}},
		{name: "missing process", readiness: session.InputReadiness{Ready: true, State: "YES", PaneID: "%32"}},
		{
			name: "missing pane process",
			readiness: session.InputReadiness{
				Ready: true, State: "YES", PaneID: "%32", TargetPID: 3232,
				HarnessStartTime: "Thu Aug 27 07:00:00 2026",
				TargetSessionID:  "$32", StableSessionID: "receipt-id",
			},
			omitRuntimeIdentity: true,
		},
		{
			name: "missing tmux incarnation",
			readiness: session.InputReadiness{
				Ready: true, State: "YES", PaneID: "%32", PanePID: 323, TargetPID: 3232,
				HarnessStartTime: "Thu Aug 27 07:00:00 2026",
				StableSessionID:  "receipt-id",
			},
			omitRuntimeIdentity: true,
		},
		{
			name: "missing harness birth identity",
			readiness: session.InputReadiness{
				Ready: true, State: "YES", PaneID: "%32", PanePID: 323, TargetPID: 3232,
				TargetSessionID: "$32", StableSessionID: "receipt-id",
			},
			omitRuntimeIdentity: true,
		},
		{
			name: "mismatched stable binding",
			readiness: session.InputReadiness{
				Ready: true, State: "YES", PaneID: "%32", PanePID: 323, TargetPID: 3232,
				TargetSessionID: "$replacement", StableSessionID: "replacement-id",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			storage := compactionDeliveryStorage(t, &manifest.Manifest{SessionID: "receipt-id", Name: "receipt-name"})
			sender := &compactionDeliverySender{
				readiness: test.readiness, omitRuntimeIdentity: test.omitRuntimeIdentity,
			}

			result, err := DeliverSessionCompaction(&OpContext{
				Storage: storage, Tmux: sender, CompactionBaseDir: t.TempDir(),
			}, &SessionCompactionDeliveryRequest{Identifier: "receipt-id", Command: "/compact"})
			var opErr *OpError
			if !errors.As(err, &opErr) || opErr.Code != ErrCodeDeliveryUncertain {
				t.Fatalf("DeliverSessionCompaction() error = %v, want %s", err, ErrCodeDeliveryUncertain)
			}
			if result == nil || result.Delivered || !result.MayHaveStarted ||
				result.AttemptOutcome != compaction.AttemptOutcomeUncertain {
				t.Fatalf("receipt-less result = %#v", result)
			}
			if sender.delivered != 1 {
				t.Fatalf("atomic sender confirmed %d deliveries, want 1", sender.delivered)
			}
		})
	}
}

func TestDeliverSessionCompactionConfirmedAccountingFailureForbidsRetry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	storage := compactionDeliveryStorage(t, &manifest.Manifest{SessionID: "accounting-id", Name: "accounting-name"})
	sender := &compactionDeliverySender{readiness: session.InputReadiness{Ready: true, State: "YES", PaneID: "%33", TargetPID: 3333}}
	markErr := errors.New("ledger replacement failed")
	attempt := &compactionDeliveryAttemptStub{id: "attempt-accounting", markErr: markErr}
	accounting := &compactionDeliveryAccountingStub{attempt: attempt}

	result, err := DeliverSessionCompaction(&OpContext{
		Storage: storage, Tmux: sender, CompactionBaseDir: t.TempDir(), compactionAccounting: accounting,
	}, &SessionCompactionDeliveryRequest{Identifier: "accounting-id", Command: "/compact"})
	var opErr *OpError
	if !errors.As(err, &opErr) || opErr.Code != ErrCodeDeliveryAccounting || !errors.Is(err, markErr) {
		t.Fatalf("DeliverSessionCompaction() error = %v, want %s preserving cause", err, ErrCodeDeliveryAccounting)
	}
	if result == nil || !result.Delivered || !result.MayHaveStarted || !result.AccountingPending ||
		result.AttemptOutcome != compaction.AttemptOutcomePending {
		t.Fatalf("accounting-failure result = %#v", result)
	}
	if len(attempt.marks) != 1 || attempt.marks[0] != compaction.AttemptOutcomeConfirmed {
		t.Fatalf("attempt marks = %#v", attempt.marks)
	}
}

func TestDeliverSessionCompactionPolicyRejectsBeforeSecondDeliveryUnlessForced(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	baseDir := t.TempDir()
	storage := compactionDeliveryStorage(t, &manifest.Manifest{SessionID: "policy-id", Name: "policy-name"})
	sender := &compactionDeliverySender{readiness: session.InputReadiness{Ready: true, State: "YES", PaneID: "%34", TargetPID: 3434}}
	opCtx := &OpContext{Storage: storage, Tmux: sender, CompactionBaseDir: baseDir}
	req := &SessionCompactionDeliveryRequest{Identifier: "policy-id", Command: "/compact"}

	first, err := DeliverSessionCompaction(opCtx, req)
	if err != nil || first == nil || !first.Delivered {
		t.Fatalf("first delivery = %#v, %v", first, err)
	}
	second, err := DeliverSessionCompaction(opCtx, req)
	var opErr *OpError
	if !errors.As(err, &opErr) || opErr.Code != ErrCodeCompactionPolicy {
		t.Fatalf("second delivery error = %v, want %s", err, ErrCodeCompactionPolicy)
	}
	if second == nil || second.Delivered || second.MayHaveStarted || second.AttemptID != "" || second.PromptFile != "" {
		t.Fatalf("policy rejection result = %#v", second)
	}
	if sender.calls != 1 {
		t.Fatalf("sender calls after policy rejection = %d, want 1", sender.calls)
	}
	prompts, readErr := os.ReadDir(filepath.Join(baseDir, "compaction-prompts"))
	if readErr != nil || len(prompts) != 1 {
		t.Fatalf("prompt audits after rejected attempt = %v, %v; want one bound prompt", prompts, readErr)
	}

	forced, err := DeliverSessionCompaction(opCtx, &SessionCompactionDeliveryRequest{
		Identifier: "policy-id", Command: "/compact", Forced: true,
	})
	if err != nil || forced == nil || !forced.Delivered || forced.AttemptOutcome != compaction.AttemptOutcomeConfirmed {
		t.Fatalf("forced delivery = %#v, %v", forced, err)
	}
	if sender.calls != 2 {
		t.Fatalf("sender calls after forced delivery = %d, want 2", sender.calls)
	}
	if filepath.Base(forced.PromptFile) != "policy-id-compact-2.md" {
		t.Fatalf("forced prompt path = %q, want rejected allocation reclaimed", forced.PromptFile)
	}
}

func TestDeliverSessionCompactionDefiniteNonDeliveryReleasesPolicyBudget(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	storage := compactionDeliveryStorage(t, &manifest.Manifest{SessionID: "release-id", Name: "release-name"})
	sender := &compactionDeliverySender{readiness: session.InputReadiness{Ready: false, State: "NO"}}
	opCtx := &OpContext{Storage: storage, Tmux: sender, CompactionBaseDir: t.TempDir()}
	req := &SessionCompactionDeliveryRequest{Identifier: "release-id", Command: "/compact"}

	first, err := DeliverSessionCompaction(opCtx, req)
	var opErr *OpError
	if !errors.As(err, &opErr) || opErr.Code != ErrCodeSessionNotReady ||
		first == nil || first.AttemptOutcome != compaction.AttemptOutcomeDefiniteNotSent {
		t.Fatalf("definite non-delivery = %#v, %v", first, err)
	}
	sender.readiness = session.InputReadiness{Ready: true, State: "YES", PaneID: "%35", TargetPID: 3535}
	second, err := DeliverSessionCompaction(opCtx, req)
	if err != nil || second == nil || !second.Delivered {
		t.Fatalf("delivery after released attempt = %#v, %v", second, err)
	}
}

func TestDeliverSessionCompactionRejectsActiveRenameBeforeAccounting(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	initial := &manifest.Manifest{SessionID: "rename-id", Name: "old-name", Harness: "claude-code"}
	reloaded := *initial
	reloaded.Name = "new-name"
	storage := &compactionDeliveryReloadStorage{initial: initial, reloaded: &reloaded}
	sender := &compactionDeliverySender{readiness: session.InputReadiness{Ready: true, State: "YES", PaneID: "%36", TargetPID: 3636}}
	accounting := &compactionDeliveryAccountingStub{}

	result, err := DeliverSessionCompaction(&OpContext{
		Storage: storage, Tmux: sender, CompactionBaseDir: t.TempDir(), compactionAccounting: accounting,
	}, &SessionCompactionDeliveryRequest{Identifier: "rename-id", Command: "/compact preserve old-name"})
	var opErr *OpError
	if !errors.As(err, &opErr) || opErr.Code != ErrCodeSessionNotReady {
		t.Fatalf("DeliverSessionCompaction() error = %v, want %s", err, ErrCodeSessionNotReady)
	}
	if result == nil || result.Name != "new-name" || result.AttemptID != "" || result.PromptFile != "" {
		t.Fatalf("rename rejection result = %#v", result)
	}
	if accounting.allocateCalls != 0 || accounting.beginCalls != 0 || sender.calls != 0 {
		t.Fatalf("rename reached accounting/sender: %d/%d/%d", accounting.allocateCalls, accounting.beginCalls, sender.calls)
	}
}

func TestDeliverSessionCompactionRejectsPreservationDriftBeforeAccounting(t *testing.T) {
	tests := []struct {
		name          string
		mutate        func(*manifest.Manifest)
		initialState  string
		reloadedState string
	}{
		{
			name: "project changed",
			mutate: func(m *manifest.Manifest) {
				m.Context.Project = "new-project"
			},
		},
		{
			name: "purpose changed",
			mutate: func(m *manifest.Manifest) {
				m.Context.Purpose = "new purpose"
			},
		},
		{
			name: "tags changed",
			mutate: func(m *manifest.Manifest) {
				m.Context.Tags = []string{"new-tag"}
			},
		},
		{
			name: "state file changed",
			initialState: `{
				"session_id":"preservation-id",
				"managed_sessions":{"old-worker":{"status":"READY","notes":""}},
				"policy":{"guard":"preserve old policy"}
			}`,
			reloadedState: `{
				"session_id":"preservation-id",
				"managed_sessions":{"new-worker":{"status":"READY","notes":""}},
				"policy":{"guard":"preserve new policy"}
			}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			baseDir := t.TempDir()
			initial := &manifest.Manifest{
				SessionID: "preservation-id",
				Name:      "worker",
				Harness:   "claude-code",
				Context: manifest.Context{
					Project: "old-project",
					Purpose: "old purpose",
					Tags:    []string{"old-tag"},
				},
			}
			statePath := filepath.Join(baseDir, initial.Name+"-state.json")
			if test.initialState != "" {
				if err := os.WriteFile(statePath, []byte(test.initialState), 0o600); err != nil {
					t.Fatalf("write initial state: %v", err)
				}
			}
			focus := "preserve receipts"
			command, err := ComposeSessionCompactionPreservation(
				baseDir,
				toSessionDetail(initial, ""),
				focus,
			)
			if err != nil {
				t.Fatalf("compose initial preservation command: %v", err)
			}

			reloaded := *initial
			reloaded.Context.Tags = append([]string(nil), initial.Context.Tags...)
			if test.mutate != nil {
				test.mutate(&reloaded)
			}
			storage := &compactionDeliveryReloadStorage{initial: initial, reloaded: &reloaded}
			if test.reloadedState != "" {
				storage.beforeReload = func() {
					if err := os.WriteFile(statePath, []byte(test.reloadedState), 0o600); err != nil {
						t.Errorf("write reloaded state: %v", err)
					}
				}
			}
			sender := &compactionDeliverySender{readiness: session.InputReadiness{
				Ready: true, State: "YES", PaneID: "%38", TargetPID: 3838,
			}}
			accounting := &compactionDeliveryAccountingStub{}

			result, err := DeliverSessionCompaction(&OpContext{
				Storage: storage, Tmux: sender, CompactionBaseDir: baseDir, compactionAccounting: accounting,
			}, &SessionCompactionDeliveryRequest{
				Identifier: "preservation-id",
				Command:    command,
				ExpectedPreservation: &SessionCompactionPreservationExpectation{
					Focus: focus,
				},
			})
			var opErr *OpError
			if !errors.As(err, &opErr) || opErr.Code != ErrCodeSessionNotReady ||
				opErr.Parameters["readiness"] != "PRESERVATION_CONTEXT_CHANGED" {
				t.Fatalf("DeliverSessionCompaction() error = %v, want preservation-drift %s", err, ErrCodeSessionNotReady)
			}
			if result == nil || result.Name != "worker" || result.AttemptID != "" || result.PromptFile != "" {
				t.Fatalf("preservation rejection result = %#v", result)
			}
			if accounting.allocateCalls != 0 || accounting.beginCalls != 0 || sender.calls != 0 {
				t.Fatalf(
					"preservation drift reached accounting/sender: %d/%d/%d",
					accounting.allocateCalls,
					accounting.beginCalls,
					sender.calls,
				)
			}
		})
	}
}

func TestDeliverSessionCompactionExactBindingRejectsSameNamedReplacement(t *testing.T) {
	exactErr := errors.New("stable session disappeared")
	storage := &compactionDeliveryExactBindingStorage{
		exactErr: exactErr,
		replacement: &manifest.Manifest{
			SessionID: "replacement-id",
			Name:      "expected-stable-id",
			Harness:   "claude-code",
		},
	}
	sender := &compactionDeliverySender{readiness: session.InputReadiness{Ready: true, State: "YES", PaneID: "%37", TargetPID: 3737}}
	accounting := &compactionDeliveryAccountingStub{}

	result, err := DeliverSessionCompaction(&OpContext{
		Storage: storage, Tmux: sender, CompactionBaseDir: t.TempDir(), compactionAccounting: accounting,
	}, &SessionCompactionDeliveryRequest{
		Identifier: "expected-stable-id",
		Command:    "/compact",
	})
	var opErr *OpError
	if result != nil || !errors.As(err, &opErr) || opErr.Code != ErrCodeStorageError || !errors.Is(err, exactErr) {
		t.Fatalf("exact-binding result/error = %#v, %v", result, err)
	}
	if storage.getCalls != 1 || storage.listCalls != 0 {
		t.Fatalf("exact binding storage calls = get:%d list:%d, want 1/0", storage.getCalls, storage.listCalls)
	}
	if sender.calls != 0 || accounting.allocateCalls != 0 || accounting.beginCalls != 0 {
		t.Fatalf("same-named replacement reached sender/accounting: %d/%d/%d", sender.calls, accounting.allocateCalls, accounting.beginCalls)
	}
}

func TestDeliverSessionCompactionExactBindingReportsMissingStableID(t *testing.T) {
	storage := &compactionDeliveryExactBindingStorage{}
	result, err := DeliverSessionCompaction(&OpContext{
		Storage: storage, Tmux: &compactionDeliverySender{}, CompactionBaseDir: t.TempDir(),
	}, &SessionCompactionDeliveryRequest{
		Identifier: "missing-stable-id",
		Command:    "/compact",
	})
	var opErr *OpError
	if result != nil || !errors.As(err, &opErr) || opErr.Code != ErrCodeSessionNotFound {
		t.Fatalf("missing exact stable ID result/error = %#v, %v; want %s", result, err, ErrCodeSessionNotFound)
	}
	if storage.getCalls != 1 || storage.listCalls != 0 {
		t.Fatalf("exact missing storage calls = get:%d list:%d, want 1/0", storage.getCalls, storage.listCalls)
	}
}

func TestDeliverSessionCompactionRequiresTrustedAccountingRoot(t *testing.T) {
	storage := compactionDeliveryStorage(t, &manifest.Manifest{SessionID: "root-id", Name: "root-name"})
	result, err := DeliverSessionCompaction(&OpContext{Storage: storage}, &SessionCompactionDeliveryRequest{
		Identifier: "root-id", Command: "/compact",
	})
	var opErr *OpError
	if result != nil || !errors.As(err, &opErr) || opErr.Code != ErrCodeStorageError {
		t.Fatalf("missing-root result/error = %#v, %v", result, err)
	}
}

type compactionDeliveryAttemptStub struct {
	id      string
	markErr error
	marks   []compaction.AttemptOutcome
}

func (a *compactionDeliveryAttemptStub) ID() string { return a.id }

func (a *compactionDeliveryAttemptStub) Mark(outcome compaction.AttemptOutcome) error {
	a.marks = append(a.marks, outcome)
	return a.markErr
}

type compactionDeliveryAccountingStub struct {
	allocation    compaction.PromptAllocation
	allocateErr   error
	attempt       compactionDeliveryAttempt
	beginErr      error
	allocateCalls int
	beginCalls    int
}

func (a *compactionDeliveryAccountingStub) AllocatePrompt(baseDir, sessionID, content string) (compaction.PromptAllocation, error) {
	a.allocateCalls++
	if a.allocateErr != nil {
		return compaction.PromptAllocation{}, a.allocateErr
	}
	if a.allocation.Path == "" {
		a.allocation = compaction.PromptAllocation{Number: 1, Path: filepath.Join(baseDir, sessionID+"-compact-1.md")}
	}
	return a.allocation, nil
}

func (a *compactionDeliveryAccountingStub) BeginAttempt(_, _, _, _ string, _ bool) (compactionDeliveryAttempt, error) {
	a.beginCalls++
	if a.beginErr != nil {
		return nil, a.beginErr
	}
	if a.attempt == nil {
		a.attempt = &compactionDeliveryAttemptStub{id: "attempt-stub"}
	}
	return a.attempt, nil
}

func readCompactionDeliveryLedger(t *testing.T, baseDir, sessionID string) compaction.CompactionState {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(baseDir, "compaction-state", sessionID+".json"))
	if err != nil {
		t.Fatalf("read compaction ledger: %v", err)
	}
	var state compaction.CompactionState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("parse compaction ledger: %v", err)
	}
	return state
}

type compactionDeliveryContextKey struct{}

func compactionDeliveryStorage(t *testing.T, m *manifest.Manifest) *dolt.MockAdapter {
	t.Helper()
	storage := dolt.NewMockAdapter()
	if err := storage.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	return storage
}

type compactionDeliveryReloadStorage struct {
	dolt.Storage

	mu           sync.Mutex
	initial      *manifest.Manifest
	reloaded     *manifest.Manifest
	beforeReload func()
	getCalls     int
}

type compactionDeliveryExactBindingStorage struct {
	dolt.Storage

	exactErr    error
	replacement *manifest.Manifest
	getCalls    int
	listCalls   int
}

func (s *compactionDeliveryExactBindingStorage) GetSession(string) (*manifest.Manifest, error) {
	s.getCalls++
	return nil, s.exactErr
}

func (s *compactionDeliveryExactBindingStorage) ListSessions(*dolt.SessionFilter) ([]*manifest.Manifest, error) {
	s.listCalls++
	return []*manifest.Manifest{s.replacement}, nil
}

func (s *compactionDeliveryReloadStorage) GetSession(string) (*manifest.Manifest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getCalls++
	if s.getCalls == 1 {
		copy := *s.initial
		return &copy, nil
	}
	if s.beforeReload != nil {
		s.beforeReload()
		s.beforeReload = nil
	}
	copy := *s.reloaded
	return &copy, nil
}

func TestRemoveUnboundCompactionPromptSyncsParentDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unbound-prompt.md")
	if err := os.WriteFile(path, []byte("sensitive preservation context"), 0o600); err != nil {
		t.Fatal(err)
	}
	synced := ""
	if err := removeUnboundCompactionPromptWithDirSync(path, func(got string) error {
		synced = got
		return nil
	}); err != nil {
		t.Fatalf("removeUnboundCompactionPromptWithDirSync() error = %v", err)
	}
	if synced != dir {
		t.Fatalf("synced directory = %q, want %q", synced, dir)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed prompt stat error = %v, want not exist", err)
	}
}

func TestRemoveUnboundCompactionPromptFailsClosedWhenDirectorySyncFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unbound-prompt.md")
	if err := os.WriteFile(path, []byte("sensitive preservation context"), 0o600); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("directory sync failed")
	err := removeUnboundCompactionPromptWithDirSync(path, func(string) error { return wantErr })
	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), path) {
		t.Fatalf("cleanup error = %v, want path and sync failure", err)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("removed prompt stat error = %v, want not exist", statErr)
	}
}
