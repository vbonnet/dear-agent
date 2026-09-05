package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/compaction"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

func TestMonitorCompactionUsesCallerContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := monitorCompaction(ctx, commandTestVerificationTarget, "missing-session", time.Hour)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("monitorCompaction() error = %v, want context.Canceled", err)
	}
}

func TestMonitorCompactionTimeoutIsFailure(t *testing.T) {
	err := monitorCompaction(t.Context(), commandTestVerificationTarget, "missing-session", 0)
	if _, ok := errors.AsType[*compaction.UnverifiedError](err); !ok {
		t.Fatalf("monitorCompaction() error = %v, want *compaction.UnverifiedError", err)
	}
}

func TestSessionCompactRejectsNonpositiveMonitorTimeoutBeforeStorage(t *testing.T) {
	useCompactionJSONOutput(t)
	requireCompactionStorageOpenFailure(t)
	originalArgs, originalMonitor, originalTimeout := sessionCompactArgs, sessionCompactMonitor, sessionCompactTimeout
	sessionCompactArgs, sessionCompactMonitor, sessionCompactTimeout = "", true, 0
	t.Cleanup(func() {
		sessionCompactArgs, sessionCompactMonitor, sessionCompactTimeout = originalArgs, originalMonitor, originalTimeout
	})
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())

	stderr, err := captureCompactionStderr(t, func() error {
		return runSessionCompact(cmd, []string{"worker"})
	})
	problem := assertSingleCompactionJSONProblem(t, stderr, err, ops.ErrCodeInvalidInput, ExitBadInput)
	if problem.Status != 400 || problem.Parameters["field"] != "timeout" {
		t.Fatalf("timeout problem = %#v, want AGM-005 timeout validation", problem)
	}
}

func TestSessionCompactMonitorDisabledIgnoresTimeout(t *testing.T) {
	originalMonitor, originalTimeout := sessionCompactMonitor, sessionCompactTimeout
	sessionCompactMonitor, sessionCompactTimeout = false, 0
	t.Cleanup(func() {
		sessionCompactMonitor, sessionCompactTimeout = originalMonitor, originalTimeout
	})
	if err := validateSessionCompactionOptions(); err != nil {
		t.Fatalf("monitor=false timeout validation error = %v, want nil", err)
	}
}

func TestMonitorCompactionClaimsCompletionOnlyOnPositiveProof(t *testing.T) {
	t.Run("unverified", func(t *testing.T) {
		output, err := captureCompactionStdout(t, func() error {
			return monitorCompactionWithRunner(t.Context(), commandTestVerificationTarget, "target", time.Minute,
				func(context.Context, compaction.VerificationTarget, time.Duration, time.Duration) (compaction.Verification, error) {
					return compaction.Verification{}, &compaction.UnverifiedError{Reason: compaction.UnverifiedEvidenceLost}
				})
		})
		if !errors.Is(err, compaction.ErrCompletionUnverified) {
			t.Fatalf("monitorCompactionWithRunner() error = %v, want ErrCompletionUnverified", err)
		}
		if strings.Contains(output, "Compaction completed") {
			t.Fatalf("unverified output claimed completion: %q", output)
		}
	})

	t.Run("positive proof", func(t *testing.T) {
		output, err := captureCompactionStdout(t, func() error {
			return monitorCompactionWithRunner(t.Context(), commandTestVerificationTarget, "target", time.Minute,
				func(context.Context, compaction.VerificationTarget, time.Duration, time.Duration) (compaction.Verification, error) {
					return compaction.Verification{Proof: compaction.ProofBusyThenStableReady, Elapsed: 4 * time.Second}, nil
				})
		})
		if err != nil {
			t.Fatalf("monitorCompactionWithRunner() error = %v", err)
		}
		if count := strings.Count(output, "Compaction completed"); count != 1 {
			t.Fatalf("positive output completion count = %d, want 1: %q", count, output)
		}
	})
}

func TestSessionCompactMonitorFalseStopsAfterConfirmedDelivery(t *testing.T) {
	verifyCalls := 0
	output, err := captureCompactionStdout(t, func() error {
		return finishCompactionDelivery(&ops.SessionCompactionDeliveryResult{
			Delivered:        true,
			AttemptOutcome:   compaction.AttemptOutcomeConfirmed,
			PromptFile:       "/audit/stable-session-id-compact-1.md",
			PaneID:           "%12",
			PanePID:          120,
			TargetPID:        1201,
			HarnessStartTime: commandTestHarnessStartTime,
			TmuxSessionID:    "$12",
		}, nil, func(delivery *ops.SessionCompactionDeliveryResult) error {
			ui.PrintSuccess("Sent /compact (prompt saved: " + delivery.PromptFile + ")")
			return runOptionalCompactionVerification(false, func() error {
				verifyCalls++
				return errors.New("monitor must not run")
			})
		})
	})
	if err != nil {
		t.Fatalf("monitor=false delivery path error = %v", err)
	}
	if verifyCalls != 0 {
		t.Fatalf("monitor calls = %d, want 0", verifyCalls)
	}
	if !strings.Contains(output, "Sent /compact") || strings.Contains(output, "Compaction completed") {
		t.Fatalf("monitor=false output = %q, want delivery receipt without completion claim", output)
	}
}

func TestFinishSessionCompactionSuccessJSONSentAndVerified(t *testing.T) {
	useCompactionJSONOutput(t)
	delivery := &ops.SessionCompactionDeliveryResult{
		Operation:        "deliver_session_compaction",
		SessionID:        "stable-id",
		Name:             "worker",
		TmuxName:         "runtime",
		Harness:          "claude-code",
		PaneID:           "%8",
		PanePID:          80,
		TargetPID:        808,
		HarnessStartTime: commandTestHarnessStartTime,
		TmuxSessionID:    "$8",
		PromptFile:       "/audit/stable-id-compact-2.md",
		AttemptID:        "attempt-8",
		AttemptOutcome:   compaction.AttemptOutcomeConfirmed,
		Delivered:        true,
		MayHaveStarted:   true,
	}

	for _, test := range []struct {
		name       string
		monitor    bool
		wantStatus string
	}{
		{name: "sent", wantStatus: compactionStatusSent},
		{name: "verified", monitor: true, wantStatus: compactionStatusVerified},
	} {
		t.Run(test.name, func(t *testing.T) {
			runnerCalls := 0
			stdout, err := captureCompactionStdout(t, func() error {
				return finishSessionCompactionSuccess(
					t.Context(), delivery, "/compact", "worker", test.monitor, 45*time.Second,
					func(_ context.Context, target compaction.VerificationTarget, timeout, poll time.Duration) (compaction.Verification, error) {
						runnerCalls++
						if target != verificationTarget(delivery) || timeout != 45*time.Second || poll != 2*time.Second {
							t.Fatalf("monitor invocation = %#v/%s/%s", target, timeout, poll)
						}
						return compaction.Verification{Proof: compaction.ProofBusyThenStableReady, Elapsed: 2200 * time.Millisecond}, nil
					},
				)
			})
			if err != nil {
				t.Fatalf("finishSessionCompactionSuccess() error = %v", err)
			}
			if runnerCalls != boolInt(test.monitor) {
				t.Fatalf("runner calls = %d, want %d", runnerCalls, boolInt(test.monitor))
			}
			var result compactionCommandResult
			if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &result); err != nil {
				t.Fatalf("decode session compact JSON: %v; stdout=%q", err, stdout)
			}
			if result.Status != test.wantStatus || result.Delivery == nil || result.Delivery.SessionID != "stable-id" {
				t.Fatalf("session compact result = %#v", result)
			}
			if test.monitor {
				if result.Verification == nil || result.Verification.ElapsedMilliseconds != 2200 {
					t.Fatalf("session compact verification = %#v", result.Verification)
				}
			} else if result.Verification != nil {
				t.Fatalf("session compact sent verification = %#v, want nil", result.Verification)
			}
			if strings.Contains(stdout, "Sent /compact") || strings.Contains(stdout, "Compaction completed") ||
				strings.Count(strings.TrimSpace(stdout), "\n") != 0 {
				t.Fatalf("session compact JSON contains prose or multiple records: %q", stdout)
			}
		})
	}
}

func TestSessionCompactJSONBoundaryRendersEarlyRawFailureOnce(t *testing.T) {
	useCompactionJSONOutput(t)
	requireCompactionStorageOpenFailure(t)

	stderr, err := executeCompactionSurfaceForTest(t, "session", sessionCompactCmd, "worker")
	problem := assertSingleCompactionJSONProblem(t, stderr, err, compactionCommandFailureCode, ExitGeneric)
	if problem.Type != "command/compaction_failed" || problem.Status != 500 || problem.Instance != "session/compact" {
		t.Fatalf("session compact raw problem = %#v", problem)
	}
	if !strings.Contains(problem.Detail, "failed to connect to Dolt storage") {
		t.Fatalf("session compact raw detail = %q, want early storage failure", problem.Detail)
	}
}

func TestSessionCompactCommandMetadata(t *testing.T) {
	if sessionCompactCmd.Use != "compact <identifier>" {
		t.Errorf("Use = %q, want %q", sessionCompactCmd.Use, "compact <identifier>")
	}
	if sessionCompactCmd.Short == "" {
		t.Error("Short description should not be empty")
	}
	if sessionCompactCmd.RunE == nil {
		t.Error("RunE should be set")
	}
	if sessionCompactCmd.Args == nil {
		t.Error("Args validator should be set")
	}
	if !strings.Contains(sessionCompactCmd.Long, "stable-ID prompt") ||
		!strings.Contains(sessionCompactCmd.Long, "anti-loop") ||
		!strings.Contains(sessionCompactCmd.Long, "audit accounting") {
		t.Fatalf("Long description does not disclose shared stable accounting path: %q", sessionCompactCmd.Long)
	}
}

func TestSessionCompactFlags(t *testing.T) {
	tests := []struct {
		name     string
		flag     string
		defValue string
	}{
		{"compact-args", "compact-args", ""},
		{"monitor", "monitor", "true"},
		{"timeout", "timeout", "5m0s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := sessionCompactCmd.Flags().Lookup(tt.flag)
			if flag == nil {
				t.Fatalf("--%s flag should be registered", tt.flag)
				return
			}
			if flag.DefValue != tt.defValue {
				t.Errorf("--%s default = %q, want %q", tt.flag, flag.DefValue, tt.defValue)
			}
			if flag.Usage == "" {
				t.Errorf("--%s should have a usage description", tt.flag)
			}
		})
	}
}

func TestSessionCompactRegistered(t *testing.T) {
	found := false
	for _, cmd := range sessionCmd.Commands() {
		if cmd.Name() == "compact" {
			found = true
			break
		}
	}
	if !found {
		t.Error("compact should be registered as a subcommand of session")
	}
}

func TestSessionCompactTimeoutDefault(t *testing.T) {
	flag := sessionCompactCmd.Flags().Lookup("timeout")
	if flag == nil {
		t.Fatal("--timeout flag should be registered")
		return
	}

	// Parse the default value to verify it's a valid duration
	d, err := time.ParseDuration(flag.DefValue)
	if err != nil {
		t.Fatalf("--timeout default should be a valid duration: %v", err)
	}
	if d != 5*time.Minute {
		t.Errorf("--timeout default = %v, want 5m", d)
	}
}
