package compaction

import (
	"errors"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

func TestDetectionFromHarnessReadinessPreservesProofClass(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		readiness tmux.HarnessInputReadiness
		wantState string
		wantProof session.ObservationEvidence
	}{
		{
			name:      "live ready",
			readiness: tmux.HarnessInputReadiness{Ready: true, State: tmux.HarnessInputReady},
			wantState: manifest.StateReady,
			wantProof: session.EvidenceLive,
		},
		{
			name:      "positively classified processing",
			readiness: tmux.HarnessInputReadiness{State: tmux.HarnessInputProcessing},
			wantState: manifest.StateWorking,
			wantProof: session.EvidenceLive,
		},
		{
			name:      "generic busy is ambiguous",
			readiness: tmux.HarnessInputReadiness{State: tmux.HarnessInputBusy},
			wantState: manifest.StateDone,
			wantProof: session.EvidenceUnknown,
		},
		{
			name:      "queued AGM is unsent input",
			readiness: tmux.HarnessInputReadiness{State: tmux.HarnessInputQueuedAGM},
			wantState: manifest.StateUserPrompt,
			wantProof: session.EvidenceLive,
		},
		{
			name:      "blocked permission",
			readiness: tmux.HarnessInputReadiness{State: tmux.HarnessInputPermission},
			wantState: manifest.StatePermissionPrompt,
			wantProof: session.EvidenceLive,
		},
		{
			name:      "target lost",
			readiness: tmux.HarnessInputReadiness{State: tmux.HarnessInputNotFound},
			wantState: manifest.StateOffline,
			wantProof: session.EvidenceAbsent,
		},
		{
			name:      "expected harness stopped",
			readiness: tmux.HarnessInputReadiness{State: tmux.HarnessInputWrongHarness},
			wantState: manifest.StateDone,
			wantProof: session.EvidenceTerminal,
		},
		{
			name:      "ready flag missing",
			readiness: tmux.HarnessInputReadiness{State: tmux.HarnessInputReady},
			wantState: manifest.StateDone,
			wantProof: session.EvidenceUnknown,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := detectionFromHarnessReadiness(test.readiness)
			if got.State != test.wantState || got.Evidence != test.wantProof {
				t.Fatalf("detectionFromHarnessReadiness() = %#v, want state %q evidence %q", got, test.wantState, test.wantProof)
			}
		})
	}
}

func TestValidateVerificationReadinessIdentityRejectsTmuxIncarnationDrift(t *testing.T) {
	t.Parallel()

	target := testVerificationTarget
	tests := []struct {
		name      string
		readiness tmux.HarnessInputReadiness
		wantErr   bool
	}{
		{
			name: "exact stable-bound incarnation",
			readiness: tmux.HarnessInputReadiness{
				State:            tmux.HarnessInputProcessing,
				TargetPanePID:    target.PanePID,
				HarnessStartTime: target.HarnessStartTime,
				TargetSessionID:  target.TargetSessionID,
				StableSessionID:  target.StableSessionID,
			},
		},
		{
			name:      "deleted pane remains typed absence",
			readiness: tmux.HarnessInputReadiness{State: tmux.HarnessInputNotFound},
		},
		{
			name: "tmux server incarnation changed",
			readiness: tmux.HarnessInputReadiness{
				State:            tmux.HarnessInputProcessing,
				TargetPanePID:    target.PanePID,
				HarnessStartTime: target.HarnessStartTime,
				TargetSessionID:  "$replacement",
				StableSessionID:  target.StableSessionID,
			},
			wantErr: true,
		},
		{
			name: "stable binding changed",
			readiness: tmux.HarnessInputReadiness{
				State:            tmux.HarnessInputProcessing,
				TargetPanePID:    target.PanePID,
				HarnessStartTime: target.HarnessStartTime,
				TargetSessionID:  target.TargetSessionID,
				StableSessionID:  "replacement-stable-id",
			},
			wantErr: true,
		},
		{
			name: "pane root process changed",
			readiness: tmux.HarnessInputReadiness{
				State:            tmux.HarnessInputProcessing,
				TargetPanePID:    target.PanePID + 1,
				HarnessStartTime: target.HarnessStartTime,
				TargetSessionID:  target.TargetSessionID,
				StableSessionID:  target.StableSessionID,
			},
			wantErr: true,
		},
		{
			name: "harness PID recycled with a new birth identity",
			readiness: tmux.HarnessInputReadiness{
				State:            tmux.HarnessInputProcessing,
				TargetPanePID:    target.PanePID,
				HarnessStartTime: "Thu Aug 27 07:00:01 2026",
				TargetSessionID:  target.TargetSessionID,
				StableSessionID:  target.StableSessionID,
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := validateVerificationReadinessIdentity(target, test.readiness)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateVerificationReadinessIdentity() error = %v, want error=%t", err, test.wantErr)
			}
		})
	}
}

func TestOccupiedComposerCannotArmCompactionVerification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		readiness  tmux.HarnessInputReadiness
		wantReason UnverifiedReason
	}{
		{
			name:       "human or generic busy composer",
			readiness:  tmux.HarnessInputReadiness{State: tmux.HarnessInputBusy},
			wantReason: UnverifiedEvidenceLost,
		},
		{
			name:       "queued AGM composer",
			readiness:  tmux.HarnessInputReadiness{State: tmux.HarnessInputQueuedAGM},
			wantReason: UnverifiedBlocked,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			first := detectionFromHarnessReadiness(test.readiness)
			observer := &scriptedStateObserver{steps: []observationStep{
				{result: first},
				observed(manifest.StateReady, session.EvidenceLive),
				observed(manifest.StateReady, session.EvidenceLive),
			}}
			verifier := newVerifierWithClock(
				observer.observe,
				&fakeVerifierClock{now: time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)},
				time.Second,
			)

			_, err := verifier.Verify(t.Context(), testVerificationTarget, time.Minute)
			var unverified *UnverifiedError
			if !errors.As(err, &unverified) || unverified.Reason != test.wantReason {
				t.Fatalf("Verify() error = %v, want reason %q", err, test.wantReason)
			}
			if observer.calls != 1 {
				t.Fatalf("Verify() observations = %d, want fail closed before ready frames", observer.calls)
			}
		})
	}
}
