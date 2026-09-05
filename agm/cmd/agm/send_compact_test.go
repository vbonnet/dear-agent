package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/cli"
	"github.com/vbonnet/dear-agent/agm/internal/compaction"
	"github.com/vbonnet/dear-agent/agm/internal/freshness"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
	governedoverride "github.com/vbonnet/dear-agent/internal/override"
	"github.com/vbonnet/dear-agent/internal/telemetry/usage"
	"github.com/vbonnet/dear-agent/pkg/workspace"
)

const commandTestHarnessStartTime = "Thu Aug 27 12:00:00 2026"

var commandTestVerificationTarget = compaction.VerificationTarget{
	SessionName:      "target",
	Harness:          "codex-cli",
	PaneID:           "%7",
	PanePID:          77,
	TargetPID:        701,
	HarnessStartTime: commandTestHarnessStartTime,
	TargetSessionID:  "$7",
	StableSessionID:  "stable-target",
}

func TestVerifyCompactionUsesCallerContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := verifyCompaction(ctx, commandTestVerificationTarget, time.Hour)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("verifyCompaction() error = %v, want context.Canceled", err)
	}
}

func TestVerifyCompactionTimeoutIsFailure(t *testing.T) {
	err := verifyCompaction(t.Context(), commandTestVerificationTarget, 0)
	if _, ok := errors.AsType[*compaction.UnverifiedError](err); !ok {
		t.Fatalf("verifyCompaction() error = %v, want *compaction.UnverifiedError", err)
	}
}

func TestRunOptionalCompactionVerificationSkipsDisabledVerifier(t *testing.T) {
	calls := 0
	err := runOptionalCompactionVerification(false, func() error {
		calls++
		return errors.New("must not run")
	})
	if err != nil {
		t.Fatalf("runOptionalCompactionVerification() error = %v", err)
	}
	if calls != 0 {
		t.Fatalf("runOptionalCompactionVerification() calls = %d, want 0", calls)
	}
}

func TestRunOptionalCompactionVerificationPropagatesVerifierFailure(t *testing.T) {
	want := &compaction.UnverifiedError{Reason: compaction.UnverifiedTimeout}
	err := runOptionalCompactionVerification(true, func() error { return want })
	if !errors.Is(err, compaction.ErrCompletionUnverified) {
		t.Fatalf("runOptionalCompactionVerification() error = %v, want ErrCompletionUnverified", err)
	}
}

func TestValidateRawCompactionInputRejectsControlsBeforeRuntimeWork(t *testing.T) {
	for _, test := range []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "multiline preservation is allowed", value: "preserve auth\nretain receipts"},
		{name: "escape rejected", value: "preserve\x1b[2J", wantErr: true},
		{name: "invalid UTF-8 rejected", value: string([]byte{0xff}), wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := captureCompactionStderr(t, func() error {
				return validateRawCompactionInput("focus", test.value)
			})
			if (err != nil) != test.wantErr {
				t.Fatalf("validateRawCompactionInput() error = %v, want error=%t", err, test.wantErr)
			}
			if test.wantErr && exitCodeFromError(err) != ExitBadInput {
				t.Fatalf("invalid raw control exit = %d, want %d", exitCodeFromError(err), ExitBadInput)
			}
		})
	}
}

func TestValidateCompactionTargetRejectsNonActiveAndPureAPI(t *testing.T) {
	for _, test := range []struct {
		name       string
		lifecycle  string
		harness    string
		wantCode   string
		wantReason string
	}{
		{name: "archived", lifecycle: manifest.LifecycleArchived, harness: "openai", wantCode: ops.ErrCodeSessionArchived},
		{name: "reaping", lifecycle: manifest.LifecycleReaping, harness: "openai", wantCode: ops.ErrCodeSessionNotReady, wantReason: "LIFECYCLE_reaping"},
		{name: "openai API", harness: "openai", wantCode: ops.ErrCodeSessionNotReady, wantReason: "PURE_API_SESSION"},
		{name: "gpt API", harness: "gpt", wantCode: ops.ErrCodeSessionNotReady, wantReason: "PURE_API_SESSION"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateCompactionTarget(ops.SessionDetail{
				ID:        "stable-retired",
				Name:      "retired",
				Lifecycle: test.lifecycle,
				Harness:   test.harness,
			})
			var problem *ops.OpError
			if !errors.As(err, &problem) || problem.Code != test.wantCode || problem.Status != 409 {
				t.Fatalf("preflight error = %T %v, want %s/409", err, err, test.wantCode)
			}
			if exitCodeFromError(err) != ExitStateConflict {
				t.Fatalf("preflight exit = %d, want %d", exitCodeFromError(err), ExitStateConflict)
			}
			if test.wantReason != "" && problem.Parameters["readiness"] != test.wantReason {
				t.Fatalf("preflight readiness = %q, want %q", problem.Parameters["readiness"], test.wantReason)
			}
		})
	}
}

func TestValidateInitialCompactionReadinessClassifiesFailuresAsNotReady(t *testing.T) {
	for _, test := range []struct {
		name    string
		observe initialCompactionObserver
	}{
		{
			name: "observer error",
			observe: func(context.Context, string, string) (*session.DetectionResult, error) {
				return nil, errors.New("tmux socket unavailable")
			},
		},
		{
			name: "observer returned nil",
			observe: func(context.Context, string, string) (*session.DetectionResult, error) {
				return nil, nil
			},
		},
		{
			name: "live but busy",
			observe: func(context.Context, string, string) (*session.DetectionResult, error) {
				return &session.DetectionResult{
					State:    manifest.StateWorking,
					Evidence: session.EvidenceLive,
				}, nil
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateInitialCompactionReadiness(
				t.Context(), "worker", "runtime", "codex-cli", test.observe,
			)
			var problem *ops.OpError
			if !errors.As(err, &problem) {
				t.Fatalf("preflight error = %T %v, want *ops.OpError", err, err)
			}
			if problem.Code != ops.ErrCodeSessionNotReady || problem.Status != 409 {
				t.Fatalf("preflight problem = %#v, want %s/409", problem, ops.ErrCodeSessionNotReady)
			}
			if exitCodeFromError(err) != ExitStateConflict {
				t.Fatalf("preflight exit = %d, want %d", exitCodeFromError(err), ExitStateConflict)
			}
		})
	}
}

func TestValidateSendCompactionOptionsRejectsForceDryRun(t *testing.T) {
	err := validateSendCompactionOptions(true, true)
	var problem *ops.OpError
	if !errors.As(err, &problem) || problem.Code != ops.ErrCodeInvalidInput || problem.Status != 400 {
		t.Fatalf("force dry-run error = %T %v, want AGM-005/400", err, err)
	}
	if validateSendCompactionOptions(true, false) != nil || validateSendCompactionOptions(false, true) != nil {
		t.Fatal("valid force or dry-run option was rejected")
	}
}

func TestSendCompactionGovernanceSkipsJudgeAndDeliveryWhenReadinessFails(t *testing.T) {
	original := requireCompactionOverride
	judgeCalls := 0
	requireCompactionOverride = func(context.Context, governedoverride.Guard, string) error {
		judgeCalls++
		return nil
	}
	t.Cleanup(func() { requireCompactionOverride = original })
	deliveryCalls := 0

	_, err := captureCompactionStderr(t, func() error {
		return withSendCompactionGovernance(
			t.Context(), "worker", "runtime", "codex-cli", true, "recovery reason",
			func(context.Context, string, string) (*session.DetectionResult, error) {
				return &session.DetectionResult{State: manifest.StateWorking, Evidence: session.EvidenceLive}, nil
			},
			func() error {
				deliveryCalls++
				return nil
			},
		)
	})
	if err == nil {
		t.Fatal("governance error = nil, want readiness failure")
	}
	if judgeCalls != 0 || deliveryCalls != 0 {
		t.Fatalf("judge/delivery calls = %d/%d, want 0/0", judgeCalls, deliveryCalls)
	}
}

func TestSendCompactionGovernanceStopsDeliveryWhenDurableAuditFails(t *testing.T) {
	original := requireCompactionOverride
	wantErr := errors.New("durable override audit failed")
	judgeCalls := 0
	requireCompactionOverride = func(context.Context, governedoverride.Guard, string) error {
		judgeCalls++
		return wantErr
	}
	t.Cleanup(func() { requireCompactionOverride = original })
	deliveryCalls := 0

	err := withSendCompactionGovernance(
		t.Context(), "worker", "runtime", "codex-cli", true, "recovery reason",
		func(context.Context, string, string) (*session.DetectionResult, error) {
			return &session.DetectionResult{State: manifest.StateReady, Evidence: session.EvidenceLive}, nil
		},
		func() error {
			deliveryCalls++
			return nil
		},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("governance error = %v, want durable audit failure", err)
	}
	if judgeCalls != 1 || deliveryCalls != 0 {
		t.Fatalf("judge/delivery calls = %d/%d, want 1/0", judgeCalls, deliveryCalls)
	}
}

func TestVerificationTargetPreservesAtomicDeliveryIdentity(t *testing.T) {
	got := verificationTarget(&ops.SessionCompactionDeliveryResult{
		SessionID:            "stable-session-id",
		TmuxName:             "runtime",
		Harness:              "agy",
		PaneID:               "%17",
		PanePID:              1717,
		TargetPID:            4242,
		HarnessStartTime:     commandTestHarnessStartTime,
		TmuxSessionID:        "$17",
		PostSubmitProcessing: true,
	})
	want := compaction.VerificationTarget{
		SessionName:               "runtime",
		Harness:                   "agy",
		PaneID:                    "%17",
		PanePID:                   1717,
		TargetPID:                 4242,
		HarnessStartTime:          commandTestHarnessStartTime,
		TargetSessionID:           "$17",
		StableSessionID:           "stable-session-id",
		InitialProcessingObserved: true,
	}
	if got != want {
		t.Fatalf("verificationTarget() = %#v, want %#v", got, want)
	}
}

func TestFinishCompactionDeliverySuppressesSuccessOnUncertainOrIncompleteAccounting(t *testing.T) {
	tests := []struct {
		name     string
		delivery *ops.SessionCompactionDeliveryResult
		err      error
	}{
		{
			name: "submission uncertain",
			delivery: &ops.SessionCompactionDeliveryResult{
				MayHaveStarted: true,
				PromptFile:     "/tmp/stable-id-compact-1.md",
			},
			err: ops.ErrDeliveryUncertain("worker", errors.New("acknowledgement lost")),
		},
		{
			name: "accounting incomplete",
			delivery: &ops.SessionCompactionDeliveryResult{
				Delivered:         true,
				AccountingPending: true,
				PromptFile:        "/tmp/stable-id-compact-1.md",
			},
			err: ops.ErrDeliveryAccounting("worker", errors.New("ledger write failed")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			successCalls := 0
			output, err := captureCompactionStdout(t, func() error {
				return finishCompactionDelivery(tt.delivery, tt.err, func(*ops.SessionCompactionDeliveryResult) error {
					successCalls++
					ui.PrintSuccess("Sent compaction; Compaction verified complete")
					return nil
				})
			})
			if err == nil {
				t.Fatal("finishCompactionDelivery() error = nil, want typed delivery failure")
			}
			if successCalls != 0 {
				t.Fatalf("success callback calls = %d, want 0", successCalls)
			}
			if strings.Contains(output, "Sent") || strings.Contains(output, "complete") {
				t.Fatalf("failed delivery emitted success/completion wording: %q", output)
			}
		})
	}
}

func TestFinishCompactionDeliverySurfacesExactRecoveryReceipt(t *testing.T) {
	originalFormat, originalMode := outputFormat, outputMode
	outputFormat, outputMode = "text", ModeHuman
	t.Cleanup(func() {
		outputFormat, outputMode = originalFormat, originalMode
	})
	delivery := &ops.SessionCompactionDeliveryResult{
		SessionID:        "stable-session-id",
		Name:             "worker",
		TmuxName:         "runtime-tmux",
		Harness:          "codex-cli",
		PaneID:           "%44",
		PanePID:          440,
		TargetPID:        4444,
		HarnessStartTime: commandTestHarnessStartTime,
		TmuxSessionID:    "$44",
		PromptFile:       "/audit/stable-session-id-compact-4.md",
		AttemptID:        "attempt-44",
		AttemptOutcome:   compaction.AttemptOutcomeUncertain,
		MayHaveStarted:   true,
	}

	stderr, err := captureCompactionStderr(t, func() error {
		return finishCompactionDelivery(
			delivery,
			ops.ErrDeliveryUncertain("worker", errors.New("acknowledgement lost")),
			func(*ops.SessionCompactionDeliveryResult) error {
				t.Fatal("uncertain delivery invoked success callback")
				return nil
			},
		)
	})
	if err == nil {
		t.Fatal("finishCompactionDelivery() error = nil, want delivery uncertainty")
	}
	for _, want := range []string{
		"Compaction recovery receipt (non-success)",
		"session_id: stable-session-id",
		"tmux_name: runtime-tmux",
		"harness: codex-cli",
		"pane_id: %44",
		"pane_pid: 440",
		"target_pid: 4444",
		"harness_start_time: " + commandTestHarnessStartTime,
		"tmux_session_id: $44",
		"attempt_id: attempt-44",
		"attempt_outcome: uncertain",
		"prompt_file: /audit/stable-session-id-compact-4.md",
		"may_have_started: true",
		"Error [AGM-018]",
	} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("recovery stderr missing %q: %q", want, stderr)
		}
	}
	if strings.Contains(stderr, "Sent compaction") || strings.Contains(stderr, "verified complete") {
		t.Fatalf("recovery receipt claimed success: %q", stderr)
	}
}

func TestFinishCompactionDeliveryJSONEmitsOneStructuredRecoveryError(t *testing.T) {
	useCompactionJSONOutput(t)

	delivery := &ops.SessionCompactionDeliveryResult{
		SessionID:         "stable-session-id",
		Name:              "worker",
		TmuxName:          "runtime-tmux",
		Harness:           "codex-cli",
		PaneID:            "%44",
		PanePID:           440,
		TargetPID:         4444,
		HarnessStartTime:  commandTestHarnessStartTime,
		TmuxSessionID:     "$44",
		PromptFile:        "/audit/stable-session-id-compact-4.md",
		AttemptID:         "attempt-44",
		AttemptOutcome:    compaction.AttemptOutcomePending,
		Delivered:         true,
		MayHaveStarted:    true,
		AccountingPending: true,
	}

	stderr, err := captureCompactionStderr(t, func() error {
		return finishCompactionDelivery(
			delivery,
			ops.ErrDeliveryAccounting("worker", errors.New("ledger write failed")),
			func(*ops.SessionCompactionDeliveryResult) error {
				t.Fatal("accounting-incomplete delivery invoked success callback")
				return nil
			},
		)
	})
	if err == nil {
		t.Fatal("finishCompactionDelivery() error = nil, want delivery accounting failure")
	}
	if strings.Contains(stderr, "Compaction recovery receipt") {
		t.Fatalf("JSON stderr contains a human recovery preamble: %q", stderr)
	}
	if got := strings.Count(strings.TrimSpace(stderr), "\n"); got != 0 {
		t.Fatalf("JSON stderr contains %d embedded line breaks, want one JSON object: %q", got, stderr)
	}

	var problem ops.OpError
	if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &problem); decodeErr != nil {
		t.Fatalf("decode JSON recovery error: %v; stderr=%q", decodeErr, stderr)
	}
	if problem.Code != ops.ErrCodeDeliveryAccounting {
		t.Fatalf("problem code = %q, want %q", problem.Code, ops.ErrCodeDeliveryAccounting)
	}
	wantParameters := map[string]string{
		"session":            "worker",
		"session_id":         "stable-session-id",
		"session_name":       "worker",
		"tmux_name":          "runtime-tmux",
		"harness":            "codex-cli",
		"pane_id":            "%44",
		"pane_pid":           "440",
		"target_pid":         "4444",
		"harness_start_time": commandTestHarnessStartTime,
		"tmux_session_id":    "$44",
		"attempt_id":         "attempt-44",
		"attempt_outcome":    "pending",
		"prompt_file":        "/audit/stable-session-id-compact-4.md",
		"may_have_started":   "true",
		"accounting_pending": "true",
	}
	for key, want := range wantParameters {
		if got := problem.Parameters[key]; got != want {
			t.Errorf("problem parameter %q = %q, want %q", key, got, want)
		}
	}
}

func TestCompactionDryRunJSONIsOneStructuredResult(t *testing.T) {
	useCompactionJSONOutput(t)

	stdout, err := captureCompactionStdout(t, func() error {
		return reportCompactionDryRun("/compact preserve context", "/audit/stable-id-compact-1.md")
	})
	if err != nil {
		t.Fatalf("reportCompactionDryRun() error = %v", err)
	}
	var result compactionCommandResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &result); err != nil {
		t.Fatalf("decode dry-run JSON: %v; stdout=%q", err, stdout)
	}
	if result.Status != compactionStatusDryRun || result.Command != "/compact preserve context" ||
		result.PromptFile != "/audit/stable-id-compact-1.md" || result.Delivery != nil || result.Verification != nil {
		t.Fatalf("dry-run result = %#v", result)
	}
	if strings.Contains(stdout, "Dry Run:") || strings.Count(strings.TrimSpace(stdout), "\n") != 0 {
		t.Fatalf("dry-run JSON contains prose or multiple records: %q", stdout)
	}
}

func TestFinishSendCompactionSuccessJSONSentAndVerified(t *testing.T) {
	useCompactionJSONOutput(t)
	delivery := &ops.SessionCompactionDeliveryResult{
		Operation:        "deliver_session_compaction",
		SessionID:        "stable-id",
		Name:             "worker",
		TmuxName:         "runtime",
		Harness:          "codex-cli",
		PaneID:           "%7",
		PanePID:          70,
		TargetPID:        707,
		HarnessStartTime: commandTestHarnessStartTime,
		TmuxSessionID:    "$7",
		PromptFile:       "/audit/stable-id-compact-1.md",
		AttemptID:        "attempt-7",
		AttemptOutcome:   compaction.AttemptOutcomeConfirmed,
		Delivered:        true,
		MayHaveStarted:   true,
	}

	for _, test := range []struct {
		name       string
		verify     bool
		wantStatus string
	}{
		{name: "sent", wantStatus: compactionStatusSent},
		{name: "verified", verify: true, wantStatus: compactionStatusVerified},
	} {
		t.Run(test.name, func(t *testing.T) {
			runnerCalls := 0
			stdout, err := captureCompactionStdout(t, func() error {
				return finishSendCompactionSuccess(t.Context(), delivery, "worker", test.verify,
					func(_ context.Context, target compaction.VerificationTarget, timeout, poll time.Duration) (compaction.Verification, error) {
						runnerCalls++
						if target != verificationTarget(delivery) || timeout != 5*time.Minute || poll != 10*time.Second {
							t.Fatalf("verification invocation = %#v/%s/%s", target, timeout, poll)
						}
						return compaction.Verification{Proof: compaction.ProofBusyThenStableReady, Elapsed: 1250 * time.Millisecond}, nil
					})
			})
			if err != nil {
				t.Fatalf("finishSendCompactionSuccess() error = %v", err)
			}
			if runnerCalls != boolInt(test.verify) {
				t.Fatalf("runner calls = %d, want %d", runnerCalls, boolInt(test.verify))
			}
			var result compactionCommandResult
			if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &result); err != nil {
				t.Fatalf("decode success JSON: %v; stdout=%q", err, stdout)
			}
			if result.Status != test.wantStatus || result.Delivery == nil || result.Delivery.SessionID != "stable-id" {
				t.Fatalf("success result = %#v", result)
			}
			if test.verify {
				if result.Verification == nil || result.Verification.Proof != compaction.ProofBusyThenStableReady || result.Verification.ElapsedMilliseconds != 1250 {
					t.Fatalf("verification receipt = %#v", result.Verification)
				}
			} else if result.Verification != nil {
				t.Fatalf("sent-only verification = %#v, want nil", result.Verification)
			}
			if strings.Contains(stdout, "✓") || strings.Contains(stdout, "Sent compaction") || strings.Count(strings.TrimSpace(stdout), "\n") != 0 {
				t.Fatalf("success JSON contains prose or multiple records: %q", stdout)
			}
		})
	}
}

func TestCompactionVerificationFailureJSONIsOneProblemWithoutSuccess(t *testing.T) {
	useCompactionJSONOutput(t)
	delivery := &ops.SessionCompactionDeliveryResult{
		SessionID:        "stable-id",
		Name:             "worker",
		TmuxName:         "runtime",
		Harness:          "codex-cli",
		PaneID:           "%7",
		PanePID:          70,
		TargetPID:        707,
		HarnessStartTime: commandTestHarnessStartTime,
		TmuxSessionID:    "$7",
		PromptFile:       "/audit/stable-id-compact-1.md",
		AttemptID:        "attempt-7",
		AttemptOutcome:   compaction.AttemptOutcomeConfirmed,
		Delivered:        true,
		MayHaveStarted:   true,
	}
	var stderr string
	stdout, err := captureCompactionStdout(t, func() error {
		var runErr error
		stderr, runErr = captureCompactionStderr(t, func() error {
			return finishSendCompactionSuccess(t.Context(), delivery, "worker", true,
				func(context.Context, compaction.VerificationTarget, time.Duration, time.Duration) (compaction.Verification, error) {
					return compaction.Verification{}, &compaction.UnverifiedError{Reason: compaction.UnverifiedCausalityLost}
				})
		})
		return runErr
	})
	if err == nil {
		t.Fatal("finishSendCompactionSuccess() error = nil, want unverified completion")
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("unverified JSON emitted success stdout: %q", stdout)
	}
	var problem ops.OpError
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &problem); err != nil {
		t.Fatalf("decode verification problem: %v; stderr=%q", err, stderr)
	}
	if problem.Code != ops.ErrCodeCompactionUnverified || problem.Parameters["reason"] != string(compaction.UnverifiedCausalityLost) ||
		problem.Parameters["pane_id"] != "%7" || problem.Parameters["target_pid"] != "707" {
		t.Fatalf("verification problem = %#v", problem)
	}
	if strings.Count(strings.TrimSpace(stderr), "\n") != 0 || strings.Contains(stderr, "Compaction recovery receipt") {
		t.Fatalf("verification JSON contains prose or multiple records: %q", stderr)
	}
}

func TestCompactionJSONOutputSilencesCobraErrorRendering(t *testing.T) {
	useCompactionJSONOutput(t)
	var cobraStderr bytes.Buffer
	root := &cobra.Command{Use: "agm"}
	leaf := &cobra.Command{
		Use: "compact",
		RunE: withCompactionJSONErrorBoundary(func(_ *cobra.Command, _ []string) error {
			return handleError(ops.ErrDeliveryUncertain("worker", errors.New("acknowledgement lost")))
		}),
	}
	root.AddCommand(leaf)
	root.SetArgs([]string{"compact"})
	root.SetErr(&cobraStderr)

	stderr, err := captureCompactionStderr(t, func() error {
		_, executeErr := root.ExecuteC()
		return executeErr
	})
	if err == nil {
		t.Fatal("ExecuteC() error = nil, want delivery uncertainty")
	}
	if cobraStderr.Len() != 0 {
		t.Fatalf("Cobra added prose to JSON error: %q", cobraStderr.String())
	}
	var problem ops.OpError
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &problem); err != nil {
		t.Fatalf("decode Cobra-path JSON: %v; stderr=%q", err, stderr)
	}
	if problem.Code != ops.ErrCodeDeliveryUncertain || strings.Count(strings.TrimSpace(stderr), "\n") != 0 {
		t.Fatalf("Cobra-path JSON = %#v, stderr=%q", problem, stderr)
	}
}

func TestSendCompactJSONBoundaryRendersEarlyRawFailureOnce(t *testing.T) {
	useCompactionJSONOutput(t)
	requireCompactionStorageOpenFailure(t)

	stderr, err := executeCompactionSurfaceForTest(t, "send", sendCompactCmd, "worker")
	problem := assertSingleCompactionJSONProblem(t, stderr, err, compactionCommandFailureCode, ExitGeneric)
	if problem.Type != "command/compaction_failed" || problem.Status != 500 || problem.Instance != "send/compact" {
		t.Fatalf("send compact raw problem = %#v", problem)
	}
	if !strings.Contains(problem.Detail, "failed to connect to Dolt storage") {
		t.Fatalf("send compact raw detail = %q, want early storage failure", problem.Detail)
	}
}

func TestCompactionJSONBoundaryRendersBadArityOnce(t *testing.T) {
	useCompactionJSONOutput(t)

	for _, test := range []struct {
		name      string
		group     string
		prototype *cobra.Command
	}{
		{name: "send compact", group: "send", prototype: sendCompactCmd},
		{name: "session compact", group: "session", prototype: sessionCompactCmd},
	} {
		t.Run(test.name, func(t *testing.T) {
			stderr, err := executeCompactionSurfaceForTest(t, test.group, test.prototype)
			problem := assertSingleCompactionJSONProblem(t, stderr, err, ops.ErrCodeInvalidInput, ExitBadInput)
			if problem.Type != "input/invalid" || problem.Status != 400 || problem.Instance != test.group+"/compact" {
				t.Fatalf("bad-arity problem = %#v", problem)
			}
			if problem.Parameters["field"] != "arguments" ||
				problem.Parameters["command"] != test.group+"/compact" ||
				!strings.Contains(problem.Detail, "accepts 1 arg") {
				t.Fatalf("bad-arity detail/parameters = %q/%#v", problem.Detail, problem.Parameters)
			}
		})
	}
}

func TestCompactionJSONFlagParseFailuresRenderInvalidInputOnce(t *testing.T) {
	for _, test := range []struct {
		name         string
		args         []string
		agentEnv     bool
		wantInstance string
		wantDetail   string
	}{
		{
			name:         "output before invalid duration",
			args:         []string{"--output", "json", "session", "compact", "worker", "--timeout", "nope"},
			wantInstance: "session/compact",
			wantDetail:   "invalid duration",
		},
		{
			name:         "output after invalid duration",
			args:         []string{"session", "compact", "worker", "--timeout", "nope", "--output", "json"},
			wantInstance: "session/compact",
			wantDetail:   "invalid duration",
		},
		{
			name:         "output before unknown flag",
			args:         []string{"--output", "json", "send", "compact", "worker", "--not-a-real-flag"},
			wantInstance: "send/compact",
			wantDetail:   "unknown flag",
		},
		{
			name:         "output after unknown flag",
			args:         []string{"send", "compact", "worker", "--not-a-real-flag", "--output", "json"},
			wantInstance: "send/compact",
			wantDetail:   "unknown flag",
		},
		{
			name:         "agent after invalid duration",
			args:         []string{"session", "compact", "worker", "--timeout", "nope", "--agent"},
			wantInstance: "session/compact",
			wantDetail:   "invalid duration",
		},
		{
			name:         "agent environment before persistent pre-run",
			args:         []string{"send", "compact", "worker", "--not-a-real-flag"},
			agentEnv:     true,
			wantInstance: "send/compact",
			wantDetail:   "unknown flag",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			preserveActualRootCompactionStateForTest(t)
			stdoutIsTTY = func() bool { return true }
			if test.agentEnv {
				t.Setenv("AGM_AGENT", "1")
			} else {
				t.Setenv("AGM_AGENT", "")
			}
			stderr, err := executeActualRootCompactionForTest(t, test.args...)
			problem := assertSingleCompactionJSONProblem(t, stderr, err, ops.ErrCodeInvalidInput, ExitBadInput)
			if problem.Status != 400 || problem.Type != "input/invalid" || problem.Instance != test.wantInstance ||
				problem.Parameters["field"] != "flags" || problem.Parameters["command"] != test.wantInstance ||
				!strings.Contains(problem.Detail, test.wantDetail) {
				t.Fatalf("flag parse problem = %#v, want invalid flags for %s", problem, test.wantInstance)
			}
		})
	}
}

func TestCompactionFlagPreparseRespectsDoubleDash(t *testing.T) {
	preserveActualRootCompactionStateForTest(t)
	stdoutIsTTY = func() bool { return true }
	t.Setenv("AGM_AGENT", "")
	prepareCompactionFlagErrorOutput([]string{
		"session", "compact", "worker", "--timeout", "nope", "--", "--output", "json", "--agent",
	})
	if outputFormat != "text" || outputMode != ModeHuman {
		t.Fatalf("output intent after -- changed flag-error mode to format=%q mode=%v", outputFormat, outputMode)
	}
}

func TestCompactionFlagPreparseDoesNotTreatFlagShapedValuesAsOutputIntent(t *testing.T) {
	for _, args := range [][]string{
		{"send", "compact", "worker", "--focus", "--output", "json", "--not-a-real-flag"},
		{"send", "compact", "worker", "--reason", "--agent", "--not-a-real-flag"},
		{"session", "compact", "worker", "--compact-args", "--output", "json", "--not-a-real-flag"},
		{"--config", "--output", "json", "send", "compact", "worker", "--not-a-real-flag"},
		{"-C", "--agent", "send", "compact", "worker", "--not-a-real-flag"},
	} {
		format, explicit, agent, noAgent := preparseOutputIntent(args)
		if format != "" || explicit || agent || noAgent {
			t.Fatalf("preparseOutputIntent(%q) = %q/%t/%t/%t, want no output intent", args, format, explicit, agent, noAgent)
		}
	}
}

func TestCompactionFlagShapedFocusValueKeepsTextFlagError(t *testing.T) {
	preserveActualRootCompactionStateForTest(t)
	stdoutIsTTY = func() bool { return true }
	t.Setenv("AGM_AGENT", "")

	stderr, err := executeActualRootCompactionForTest(t,
		"send", "compact", "worker", "--focus", "--output", "json", "--not-a-real-flag",
	)
	if err == nil || exitCodeFromError(err) != ExitGeneric {
		t.Fatalf("text flag parse error = %T %v (exit %d), want raw Cobra exit %d", err, err, exitCodeFromError(err), ExitGeneric)
	}
	if strings.Contains(stderr, `"code":"AGM-005"`) || strings.HasPrefix(strings.TrimSpace(stderr), "{") {
		t.Fatalf("flag-shaped focus value unexpectedly selected JSON output: %q", stderr)
	}
}

func TestCompactionTextFlagParseFailureKeepsCobraBehavior(t *testing.T) {
	preserveActualRootCompactionStateForTest(t)
	stdoutIsTTY = func() bool { return true }
	t.Setenv("AGM_AGENT", "")

	stderr, err := executeActualRootCompactionForTest(t,
		"session", "compact", "worker", "--timeout", "nope",
	)
	if err == nil || exitCodeFromError(err) != ExitGeneric {
		t.Fatalf("text flag parse error = %T %v (exit %d), want raw Cobra exit %d", err, err, exitCodeFromError(err), ExitGeneric)
	}
	if strings.Contains(stderr, `"code":"AGM-005"`) || strings.HasPrefix(strings.TrimSpace(stderr), "{") {
		t.Fatalf("text flag parse failure unexpectedly used JSON owner: %q", stderr)
	}
}

func TestNonCompactionJSONFlagParseFailureKeepsCobraBehavior(t *testing.T) {
	preserveActualRootCompactionStateForTest(t)
	stdoutIsTTY = func() bool { return true }
	t.Setenv("AGM_AGENT", "")

	stderr, err := executeActualRootCompactionForTest(t,
		"session", "list", "--not-a-real-flag", "--output", "json",
	)
	if err == nil || exitCodeFromError(err) != ExitGeneric {
		t.Fatalf("non-compaction flag parse error = %T %v (exit %d), want raw Cobra exit %d", err, err, exitCodeFromError(err), ExitGeneric)
	}
	if strings.Contains(stderr, `"code":"AGM-005"`) || strings.HasPrefix(strings.TrimSpace(stderr), "{") {
		t.Fatalf("non-compaction flag parse failure unexpectedly used compaction JSON owner: %q", stderr)
	}
}

func TestCompactionFlagParseHandlerLeavesTextErrorUnchanged(t *testing.T) {
	originalFormat := outputFormat
	outputFormat = "text"
	t.Cleanup(func() { outputFormat = originalFormat })
	want := errors.New("unknown flag: --broken")
	if got := handleCompactionFlagParseError(sendCompactCmd, want); !errors.Is(got, want) {
		t.Fatalf("text flag parse error = %v, want original %v", got, want)
	}
}

func TestRootCompactionJSONBoundaryRendersPersistentPreRunRawFailureOnce(t *testing.T) {
	preserveActualRootCompactionStateForTest(t)

	testRoot := t.TempDir()
	home := filepath.Join(testRoot, "home")
	workspace := filepath.Join(testRoot, "workspace")
	if err := os.MkdirAll(filepath.Join(home, ".agm"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(testRoot, "missing"), filepath.Join(home, ".agm", "dangling")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	t.Setenv("HOME", home)
	for _, key := range []string{"ENGRAM_TEST_MODE", "ENGRAM_TEST_WORKSPACE", "ENGRAM_WORKSPACE"} {
		t.Setenv(key, "")
	}

	configPath := filepath.Join(testRoot, "config.yaml")
	contents := "storage:\n  mode: centralized\n  workspace: " + workspace + "\n  relative_path: .agm-work\n"
	if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	providerFlag := newCmd.Flags().Lookup("sandbox-provider")
	providerFlag.Changed = false
	sandboxProvider = "auto"

	stderr, err := executeActualRootCompactionForTest(t,
		"--output", "json",
		"--config", configPath,
		"--sessions-dir", filepath.Join(home, "sessions"),
		"send", "compact", "worker",
	)
	problem := assertSingleCompactionJSONProblem(t, stderr, err, compactionCommandFailureCode, ExitGeneric)
	if problem.Type != "command/compaction_failed" || problem.Status != 500 || problem.Instance != "send/compact" {
		t.Fatalf("root pre-run raw problem = %#v", problem)
	}
	if !strings.Contains(problem.Detail, "setup centralized storage") {
		t.Fatalf("root pre-run raw detail = %q, want centralized bootstrap failure", problem.Detail)
	}
}

func TestRootCompactionJSONBoundaryRendersUnrenderedPreRunExitErrorOnce(t *testing.T) {
	preserveActualRootCompactionStateForTest(t)

	home := t.TempDir()
	t.Setenv("HOME", home)
	missingConfig := filepath.Join(home, "missing-config.yaml")
	stderr, err := executeActualRootCompactionForTest(t,
		"--output", "json",
		"--config", missingConfig,
		"--sessions-dir", filepath.Join(home, "sessions"),
		"session", "compact", "worker",
	)
	problem := assertSingleCompactionJSONProblem(t, stderr, err, compactionCommandFailureCode, ExitGeneric)
	if problem.Type != "command/compaction_failed" || problem.Instance != "session/compact" {
		t.Fatalf("root pre-run exit problem = %#v", problem)
	}
	if !strings.Contains(problem.Detail, "failed to read config file") {
		t.Fatalf("root pre-run exit detail = %q, want missing selected config", problem.Detail)
	}
}

func TestRootCompactionJSONNoAgentTTYFailureIsOneProblemWithoutHeader(t *testing.T) {
	preserveActualRootCompactionStateForTest(t)
	stdoutIsTTY = func() bool { return true }

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGM_SOURCE_DIR", filepath.Join(home, "missing-source"))
	requireCompactionStorageOpenFailure(t)

	stderr, err := executeActualRootCompactionForTest(t,
		"--output", "json",
		"--no-agent",
		"--sessions-dir", filepath.Join(home, "sessions"),
		"send", "compact", "worker",
	)
	problem := assertSingleCompactionJSONProblem(t, stderr, err, compactionCommandFailureCode, ExitGeneric)
	if problem.Type != "command/compaction_failed" || problem.Instance != "send/compact" {
		t.Fatalf("root TTY JSON problem = %#v", problem)
	}
	if strings.Contains(stderr, "agm "+Version+" (") {
		t.Fatalf("root TTY JSON stderr contains version header: %q", stderr)
	}
}

func TestRootCompactionJSONSuppressesWorkspaceDiagnosticsBeforeFailure(t *testing.T) {
	for _, test := range []struct {
		name          string
		workspaceFlag string
		writeConfig   func(*testing.T, string)
	}{
		{
			name: "invalid workspace config",
			writeConfig: func(t *testing.T, path string) {
				t.Helper()
				if err := os.WriteFile(path, []byte("invalid: yaml: content:\n  - broken"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:          "explicit missing workspace",
			workspaceFlag: "missing",
			writeConfig: func(t *testing.T, path string) {
				t.Helper()
				if err := workspace.SaveConfig(path, &workspace.Config{
					Version: 1,
					Workspaces: []workspace.Workspace{
						{Name: "present", Root: filepath.Dir(path), Enabled: true},
					},
				}); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			preserveActualRootCompactionStateForTest(t)
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("AGM_SOURCE_DIR", filepath.Join(home, "missing-source"))
			if err := os.MkdirAll(filepath.Join(home, ".agm"), 0o700); err != nil {
				t.Fatal(err)
			}
			test.writeConfig(t, filepath.Join(home, ".agm", "config.yaml"))
			requireCompactionStorageOpenFailure(t)

			args := []string{"--output", "json"}
			if test.workspaceFlag != "" {
				args = append(args, "--workspace", test.workspaceFlag)
			}
			args = append(args, "send", "compact", "worker")
			stderr, err := executeActualRootCompactionForTest(t, args...)
			problem := assertSingleCompactionJSONProblem(t, stderr, err, compactionCommandFailureCode, ExitGeneric)
			if problem.Type != "command/compaction_failed" || problem.Instance != "send/compact" {
				t.Fatalf("root workspace diagnostic problem = %#v", problem)
			}
		})
	}
}

func TestRootCompactionJSONSuppressesCentralizedMigrationNoticeBeforeFailure(t *testing.T) {
	preserveActualRootCompactionStateForTest(t)

	testRoot := t.TempDir()
	home := filepath.Join(testRoot, "home")
	workspaceRoot := filepath.Join(testRoot, "workspace")
	if err := os.MkdirAll(filepath.Join(home, ".agm"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(workspaceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".agm", "preserved"), []byte("state"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("AGM_SOURCE_DIR", filepath.Join(home, "missing-source"))
	configPath := filepath.Join(testRoot, "config.yaml")
	contents := "storage:\n  mode: centralized\n  workspace: " + workspaceRoot + "\n  relative_path: .agm-work\n"
	if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	requireCompactionStorageOpenFailure(t)

	stderr, err := executeActualRootCompactionForTest(t,
		"--output", "json",
		"--config", configPath,
		"--sessions-dir", filepath.Join(home, "sessions"),
		"send", "compact", "worker",
	)
	problem := assertSingleCompactionJSONProblem(t, stderr, err, compactionCommandFailureCode, ExitGeneric)
	if problem.Type != "command/compaction_failed" || problem.Instance != "send/compact" {
		t.Fatalf("root migration-notice problem = %#v", problem)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, ".agm-work", "preserved")); err != nil {
		t.Fatalf("quiet centralized bootstrap did not migrate state: %v", err)
	}
}

func TestRootCompactionJSONSuppressesStaleAndPostRunWarnings(t *testing.T) {
	preserveActualRootCompactionStateForTest(t)

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGM_SOURCE_DIR", filepath.Clean(filepath.Join("..", "..", "..")))
	GitCommit = "unknown"
	if result := freshness.Check(os.Getenv("AGM_SOURCE_DIR"), GitCommit); !result.Stale {
		t.Fatalf("freshness setup = %#v, want a stale result that would emit prose", result)
	}

	blockedParent := filepath.Join(t.TempDir(), "occupied")
	if err := os.WriteFile(blockedParent, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	tracker, err := usage.New(filepath.Join(blockedParent, "usage.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := tracker.TrackSync(usage.Event{Command: "preflight"}); err == nil {
		t.Fatal("usage tracker setup unexpectedly writable; post-run warning path would not execute")
	}
	usageTracker = tracker

	originalRunE := sendCompactCmd.RunE
	sendCompactCmd.RunE = func(*cobra.Command, []string) error { return nil }
	t.Cleanup(func() { sendCompactCmd.RunE = originalRunE })

	stderr, err := executeActualRootCompactionForTest(t,
		"--output", "json",
		"--sessions-dir", filepath.Join(home, "sessions"),
		"send", "compact", "worker",
	)
	if err != nil {
		t.Fatalf("root compaction success path error = %v; stderr=%q", err, stderr)
	}
	if stderr != "" {
		t.Fatalf("root compaction JSON appended best-effort prose: %q", stderr)
	}
}

func TestCompactionCommandFailureCodeIsUniqueAndPublished(t *testing.T) {
	if compactionCommandFailureCode != "AGM-022" {
		t.Fatalf("command-local compaction failure code = %q, want published AGM-022", compactionCommandFailureCode)
	}

	opsCatalog, err := os.ReadFile(filepath.Join("..", "..", "internal", "ops", "errors.go"))
	if err != nil {
		t.Fatalf("read shared operations error catalog: %v", err)
	}
	if strings.Contains(string(opsCatalog), `"`+compactionCommandFailureCode+`"`) {
		t.Fatalf("command-local code %s collides with the shared operations error catalog", compactionCommandFailureCode)
	}

	publicCatalog, err := os.ReadFile(filepath.Join("..", "..", "docs", "AGENTIC-API.md"))
	if err != nil {
		t.Fatalf("read public error catalog: %v", err)
	}
	wantRow := "| AGM-022 | 500 | `command/compaction_failed` |"
	if count := strings.Count(string(publicCatalog), wantRow); count != 1 {
		t.Fatalf("public command-failure catalog row count = %d, want 1 for %q", count, wantRow)
	}
}

func executeCompactionSurfaceForTest(t *testing.T, group string, prototype *cobra.Command, args ...string) (string, error) {
	t.Helper()
	root := &cobra.Command{Use: "agm"}
	parent := &cobra.Command{Use: group}
	leaf := &cobra.Command{
		Use:  prototype.Use,
		Args: prototype.Args,
		RunE: prototype.RunE,
	}
	parent.AddCommand(leaf)
	root.AddCommand(parent)
	root.SetArgs(append([]string{group, "compact"}, args...))

	return captureCompactionStderr(t, func() error {
		root.SetErr(os.Stderr)
		_, err := root.ExecuteC()
		return err
	})
}

func preserveActualRootCompactionStateForTest(t *testing.T) {
	t.Helper()
	restoreCommandTreeFlagsForTest(t, rootCmd)

	originalCfg := cfg
	originalAuditLogger := auditLogger
	originalUsageTracker := usageTracker
	originalOutputMode := outputMode
	originalCommandStartTime := commandStartTime
	originalGitCommit := GitCommit
	originalSandboxProvider := sandboxProvider
	originalStdoutIsTTY := stdoutIsTTY
	originalProjectDirectory := cli.GetProjectDirectory()
	originalTmuxTimeout := tmux.GetTimeout()
	originalUIConfig := ui.GetGlobalConfig()
	originalRootSilenceErrors, originalRootSilenceUsage := rootCmd.SilenceErrors, rootCmd.SilenceUsage
	originalSendSilenceErrors, originalSendSilenceUsage := sendCompactCmd.SilenceErrors, sendCompactCmd.SilenceUsage
	originalSessionSilenceErrors, originalSessionSilenceUsage := sessionCompactCmd.SilenceErrors, sessionCompactCmd.SilenceUsage
	t.Cleanup(func() {
		cfg = originalCfg
		auditLogger = originalAuditLogger
		usageTracker = originalUsageTracker
		outputMode = originalOutputMode
		commandStartTime = originalCommandStartTime
		GitCommit = originalGitCommit
		sandboxProvider = originalSandboxProvider
		stdoutIsTTY = originalStdoutIsTTY
		cli.SetProjectDirectory(originalProjectDirectory)
		tmux.SetTimeout(originalTmuxTimeout)
		ui.SetGlobalConfig(originalUIConfig)
		rootCmd.SilenceErrors, rootCmd.SilenceUsage = originalRootSilenceErrors, originalRootSilenceUsage
		sendCompactCmd.SilenceErrors, sendCompactCmd.SilenceUsage = originalSendSilenceErrors, originalSendSilenceUsage
		sessionCompactCmd.SilenceErrors, sessionCompactCmd.SilenceUsage = originalSessionSilenceErrors, originalSessionSilenceUsage
		rootCmd.SetArgs(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetOut(nil)
	})
}

func executeActualRootCompactionForTest(t *testing.T, args ...string) (string, error) {
	t.Helper()
	prepareCompactionFlagErrorOutput(args)
	rootCmd.SetArgs(args)
	return captureCompactionStderr(t, func() error {
		rootCmd.SetErr(os.Stderr)
		_, err := rootCmd.ExecuteC()
		return err
	})
}

func requireCompactionStorageOpenFailure(t *testing.T) {
	t.Helper()
	blockedParent := filepath.Join(t.TempDir(), "occupied")
	if err := os.WriteFile(blockedParent, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write blocked storage parent: %v", err)
	}
	t.Setenv("AGM_DB_PATH", filepath.Join(blockedParent, "agm.db"))
}

func assertSingleCompactionJSONProblem(t *testing.T, stderr string, executeErr error, wantCode string, wantExit int) ops.OpError {
	t.Helper()
	if executeErr == nil {
		t.Fatal("ExecuteC() error = nil, want command failure")
	}
	var rendered *exitError
	if !errors.As(executeErr, &rendered) || rendered.ExitCode() != wantExit {
		t.Fatalf("ExecuteC() error = %T %v, want exitError code %d", executeErr, executeErr, wantExit)
	}
	trimmed := strings.TrimSpace(stderr)
	if trimmed == "" {
		t.Fatal("JSON boundary emitted empty stderr")
	}
	if strings.Count(trimmed, "\n") != 0 || strings.Contains(trimmed, "Error:") || strings.Contains(trimmed, "Usage:") {
		t.Fatalf("JSON boundary emitted duplicate or prose stderr: %q", stderr)
	}
	var problem ops.OpError
	if err := json.Unmarshal([]byte(trimmed), &problem); err != nil {
		t.Fatalf("decode one RFC-7807 problem: %v; stderr=%q", err, stderr)
	}
	if problem.Code != wantCode || problem.Type == "" || problem.Title == "" || problem.Detail == "" {
		t.Fatalf("RFC-7807 problem = %#v, want code %q and complete classification", problem, wantCode)
	}
	return problem
}

func useCompactionJSONOutput(t *testing.T) {
	t.Helper()
	originalFormat, originalMode := outputFormat, outputMode
	outputFormat, outputMode = "json", ModeAgent
	t.Cleanup(func() {
		outputFormat, outputMode = originalFormat, originalMode
	})
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func TestFinishCompactionDeliveryUsesDurablePromptReceipt(t *testing.T) {
	wantPath := "/audit/stable-id-compact-4.md"
	output, err := captureCompactionStdout(t, func() error {
		return finishCompactionDelivery(&ops.SessionCompactionDeliveryResult{
			Delivered:        true,
			AttemptOutcome:   compaction.AttemptOutcomeConfirmed,
			PromptFile:       wantPath,
			PaneID:           "%11",
			PanePID:          110,
			TargetPID:        1101,
			HarnessStartTime: commandTestHarnessStartTime,
			TmuxSessionID:    "$11",
		}, nil, func(delivery *ops.SessionCompactionDeliveryResult) error {
			ui.PrintSuccess("Sent compaction (prompt saved: " + delivery.PromptFile + ")")
			return nil
		})
	})
	if err != nil {
		t.Fatalf("finishCompactionDelivery() error = %v", err)
	}
	if !strings.Contains(output, wantPath) {
		t.Fatalf("success output = %q, want durable prompt receipt %q", output, wantPath)
	}
}

func TestFinishCompactionDeliveryRejectsInconsistentSuccessReceipts(t *testing.T) {
	confirmed := &ops.SessionCompactionDeliveryResult{
		Delivered:        true,
		AttemptOutcome:   compaction.AttemptOutcomeConfirmed,
		PromptFile:       "/audit/stable-id-compact-1.md",
		PaneID:           "%11",
		PanePID:          110,
		TargetPID:        1101,
		HarnessStartTime: commandTestHarnessStartTime,
		TmuxSessionID:    "$11",
	}
	tests := []struct {
		name   string
		mutate func(*ops.SessionCompactionDeliveryResult)
	}{
		{name: "not delivered", mutate: func(result *ops.SessionCompactionDeliveryResult) { result.Delivered = false }},
		{name: "accounting pending", mutate: func(result *ops.SessionCompactionDeliveryResult) { result.AccountingPending = true }},
		{name: "attempt not confirmed", mutate: func(result *ops.SessionCompactionDeliveryResult) {
			result.AttemptOutcome = compaction.AttemptOutcomePending
		}},
		{name: "prompt receipt missing", mutate: func(result *ops.SessionCompactionDeliveryResult) { result.PromptFile = "" }},
		{name: "pane receipt missing", mutate: func(result *ops.SessionCompactionDeliveryResult) { result.PaneID = "" }},
		{name: "pane process receipt missing", mutate: func(result *ops.SessionCompactionDeliveryResult) { result.PanePID = 0 }},
		{name: "process receipt missing", mutate: func(result *ops.SessionCompactionDeliveryResult) { result.TargetPID = 0 }},
		{name: "process birth receipt missing", mutate: func(result *ops.SessionCompactionDeliveryResult) { result.HarnessStartTime = "" }},
		{name: "tmux incarnation receipt missing", mutate: func(result *ops.SessionCompactionDeliveryResult) { result.TmuxSessionID = "" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := *confirmed
			tt.mutate(&result)
			successCalls := 0
			output, err := captureCompactionStdout(t, func() error {
				return finishCompactionDelivery(&result, nil, func(*ops.SessionCompactionDeliveryResult) error {
					successCalls++
					ui.PrintSuccess("Sent compaction; Compaction verified complete")
					return nil
				})
			})
			if err == nil {
				t.Fatal("finishCompactionDelivery() error = nil, want inconsistent receipt rejection")
			}
			if successCalls != 0 {
				t.Fatalf("success callback calls = %d, want 0", successCalls)
			}
			if strings.Contains(output, "Sent") || strings.Contains(output, "complete") {
				t.Fatalf("inconsistent receipt emitted success/completion wording: %q", output)
			}
		})
	}
}

func TestAllocateDryRunCompactionPromptUsesStableSessionIDAndExclusiveFiles(t *testing.T) {
	baseDir := t.TempDir()
	const sessionID = "stable-session-id"
	const command = "/compact preserve the audit"

	first, err := allocateDryRunCompactionPrompt(baseDir, sessionID, command)
	if err != nil {
		t.Fatalf("allocateDryRunCompactionPrompt(first) error = %v", err)
	}
	second, err := allocateDryRunCompactionPrompt(baseDir, sessionID, command)
	if err != nil {
		t.Fatalf("allocateDryRunCompactionPrompt(second) error = %v", err)
	}
	if first == second {
		t.Fatalf("exclusive dry-run prompt paths are identical: %q", first)
	}
	if got := filepath.Base(first); !strings.HasPrefix(got, sessionID+"-compact-") {
		t.Fatalf("first prompt basename = %q, want stable ID prefix %q", got, sessionID)
	}
	contents, err := os.ReadFile(first)
	if err != nil {
		t.Fatalf("read first dry-run prompt: %v", err)
	}
	if string(contents) != command {
		t.Fatalf("first dry-run prompt = %q, want %q", contents, command)
	}
}

func TestAllocateDryRunCompactionPromptFailsClosedWhenAuditCannotBeSaved(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(baseDir, []byte("occupied"), 0o600); err != nil {
		t.Fatalf("write occupied base path: %v", err)
	}
	if _, err := allocateDryRunCompactionPrompt(baseDir, "stable-session-id", "/compact"); err == nil {
		t.Fatal("allocateDryRunCompactionPrompt() error = nil, want durable audit failure")
	}
}

func TestRunCompactionDryRunRejectsTerminalControlsBeforeAuditOrOutput(t *testing.T) {
	baseDir := t.TempDir()
	originalFormat, originalMode := outputFormat, outputMode
	outputFormat, outputMode = "text", ModeHuman
	t.Cleanup(func() {
		outputFormat, outputMode = originalFormat, originalMode
	})

	stdout, err := captureCompactionStdout(t, func() error {
		return runCompactionDryRun(baseDir, "stable-session-id", "/compact preserve\x1b]0;spoof\a")
	})
	if err == nil {
		t.Fatal("runCompactionDryRun() error = nil, want invalid terminal control rejection")
	}
	if stdout != "" || strings.Contains(stdout, "\x1b") {
		t.Fatalf("invalid dry-run command reached stdout: %q", stdout)
	}
	entries, readErr := os.ReadDir(baseDir)
	if readErr != nil {
		t.Fatalf("ReadDir(%s): %v", baseDir, readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("invalid dry-run command created audit files: %v", entries)
	}
}

func TestVerifyCompactionClaimsCompletionOnlyOnPositiveProof(t *testing.T) {
	t.Run("unverified", func(t *testing.T) {
		output, err := captureCompactionStdout(t, func() error {
			return verifyCompactionWithRunner(t.Context(), commandTestVerificationTarget, time.Minute,
				func(context.Context, compaction.VerificationTarget, time.Duration, time.Duration) (compaction.Verification, error) {
					return compaction.Verification{}, &compaction.UnverifiedError{Reason: compaction.UnverifiedTimeout}
				})
		})
		if !errors.Is(err, compaction.ErrCompletionUnverified) {
			t.Fatalf("verifyCompactionWithRunner() error = %v, want ErrCompletionUnverified", err)
		}
		if strings.Contains(output, "verified complete") {
			t.Fatalf("unverified output claimed completion: %q", output)
		}
	})

	t.Run("positive proof", func(t *testing.T) {
		output, err := captureCompactionStdout(t, func() error {
			return verifyCompactionWithRunner(t.Context(), commandTestVerificationTarget, time.Minute,
				func(context.Context, compaction.VerificationTarget, time.Duration, time.Duration) (compaction.Verification, error) {
					return compaction.Verification{Proof: compaction.ProofBusyThenStableReady, Elapsed: 4 * time.Second}, nil
				})
		})
		if err != nil {
			t.Fatalf("verifyCompactionWithRunner() error = %v", err)
		}
		if count := strings.Count(output, "Compaction verified complete"); count != 1 {
			t.Fatalf("positive output completion count = %d, want 1: %q", count, output)
		}
	})
}

func captureCompactionStdout(t *testing.T, run func() error) (string, error) {
	t.Helper()
	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe(): %v", err)
	}
	os.Stdout = writer
	runErr := run()
	if err := writer.Close(); err != nil {
		os.Stdout = original
		_ = reader.Close()
		t.Fatalf("close stdout capture: %v", err)
	}
	os.Stdout = original
	output, readErr := io.ReadAll(reader)
	if err := reader.Close(); err != nil && readErr == nil {
		readErr = err
	}
	if readErr != nil {
		t.Fatalf("read stdout capture: %v", readErr)
	}
	return string(output), runErr
}

func captureCompactionStderr(t *testing.T, run func() error) (string, error) {
	t.Helper()
	original := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe(): %v", err)
	}
	os.Stderr = writer
	runErr := run()
	if err := writer.Close(); err != nil {
		os.Stderr = original
		_ = reader.Close()
		t.Fatalf("close stderr capture: %v", err)
	}
	os.Stderr = original
	output, readErr := io.ReadAll(reader)
	if err := reader.Close(); err != nil && readErr == nil {
		readErr = err
	}
	if readErr != nil {
		t.Fatalf("read stderr capture: %v", readErr)
	}
	return string(output), runErr
}

func TestBuildCompactCommand(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		expected string
	}{
		{"no args", "", "/compact"},
		{"empty whitespace", "   ", "/compact"},
		{"with instructions", "preserve context about X", "/compact preserve context about X"},
		{"with leading whitespace", "  preserve auth context  ", "/compact preserve auth context"},
		{"single word", "everything", "/compact everything"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildCompactCommand(tt.args)
			if got != tt.expected {
				t.Errorf("buildCompactCommand(%q) = %q, want %q", tt.args, got, tt.expected)
			}
		})
	}
}

func TestSendCompactCommandMetadata(t *testing.T) {
	if sendCompactCmd.Use != "compact <identifier>" {
		t.Errorf("Use = %q, want %q", sendCompactCmd.Use, "compact <identifier>")
	}
	if sendCompactCmd.Short == "" {
		t.Error("Short description should not be empty")
	}
	if sendCompactCmd.RunE == nil {
		t.Error("RunE should be set")
	}
	if sendCompactCmd.Args == nil {
		t.Error("Args validator should be set")
	}
	if !strings.Contains(sendCompactCmd.Long, "AGM-registered") ||
		!strings.Contains(sendCompactCmd.Long, "durable identity") ||
		!strings.Contains(sendCompactCmd.Long, "stable-session-ID prompt") ||
		!strings.Contains(sendCompactCmd.Long, "audit accounting") {
		t.Fatalf("Long description does not disclose registered-session requirement: %q", sendCompactCmd.Long)
	}
}

func TestSendCompactRegistered(t *testing.T) {
	found := false
	for _, cmd := range sendGroupCmd.Commands() {
		if cmd.Name() == "compact" {
			found = true
			break
		}
	}
	if !found {
		t.Error("compact should be registered as a subcommand of send")
	}
}

func TestSendCompactFlagRegistration(t *testing.T) {
	flags := []struct {
		name     string
		defValue string
	}{
		{"focus", ""},
		{"verify", "false"},
		{"dry-run", "false"},
	}

	for _, f := range flags {
		t.Run(f.name, func(t *testing.T) {
			flag := sendCompactCmd.Flags().Lookup(f.name)
			if flag == nil {
				t.Fatalf("--%s flag should be registered", f.name)
				return
			}
			if flag.DefValue != f.defValue {
				t.Errorf("--%s default = %q, want %q", f.name, flag.DefValue, f.defValue)
			}
			if flag.Usage == "" {
				t.Errorf("--%s should have a usage description", f.name)
			}
		})
	}
}

func TestSendCompactOldArgsFlagRemoved(t *testing.T) {
	flag := sendCompactCmd.Flags().Lookup("args")
	if flag != nil {
		t.Error("--args flag should be removed (replaced by --focus)")
	}
}

func TestAgmBaseDir(t *testing.T) {
	t.Setenv("AGM_HOME", "/tmp/test-agm")
	dir := agmBaseDir()
	if dir != "/tmp/test-agm" {
		t.Errorf("agmBaseDir() = %q, want %q", dir, "/tmp/test-agm")
	}
}
