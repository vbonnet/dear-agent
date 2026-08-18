package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

func TestStartRejectsWhitespaceProjectNameBeforeWritingState(t *testing.T) {
	dir := t.TempDir()
	SetProjectDirectory(dir)
	t.Cleanup(func() { projectDirectory = "" })

	err := runStart(newStartCmdWithFlags(), []string{"  \t "})
	if err == nil || !strings.Contains(err.Error(), "project name is required") {
		t.Fatalf("runStart() error = %v, want project name validation", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, status.StatusFilename)); !os.IsNotExist(statErr) {
		t.Fatalf("status file was created before project name validation: %v", statErr)
	}
}

func TestStartRejectsNonGitProjectDirectoryBeforeWritingState(t *testing.T) {
	dir := t.TempDir()
	SetProjectDirectory(dir)
	t.Cleanup(func() { projectDirectory = "" })

	err := runStart(newStartCmdWithFlags(), []string{"orphan-session"})
	if err == nil || !strings.Contains(err.Error(), "must be inside a Git work tree") {
		t.Fatalf("runStart() error = %v, want Git worktree validation", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, status.StatusFilename)); !os.IsNotExist(statErr) {
		t.Fatalf("status file was created outside a Git worktree: %v", statErr)
	}
}

func TestValidateStartMetadataRejectsInvalidEnums(t *testing.T) {
	for _, test := range []struct {
		projectType string
		riskLevel   string
	}{
		{projectType: "typo", riskLevel: "M"},
		{projectType: "feature", riskLevel: "typo"},
	} {
		if err := validateStartMetadata(test.projectType, test.riskLevel); err == nil {
			t.Fatalf("validateStartMetadata(%q, %q) accepted invalid input", test.projectType, test.riskLevel)
		}
	}
	if err := validateStartMetadata("feature", "M"); err != nil {
		t.Fatalf("valid start metadata rejected: %v", err)
	}
}

func TestPhaseOutcomeValidation(t *testing.T) {
	if !validPhaseOutcome("success") || !validPhaseOutcome("partial") || !validPhaseOutcome("skipped") {
		t.Fatal("documented outcomes must be valid")
	}
	if validPhaseOutcome("typo") {
		t.Fatal("invalid outcome accepted")
	}
}

func TestLifecycleMetadataRequiresActionableDetails(t *testing.T) {
	for _, test := range []struct {
		state, blockedOn, errorMessage, inputNeeded string
		wantErr                                     bool
	}{
		{state: "input-required", wantErr: true},
		{state: "input-required", inputNeeded: "choose API"},
		{state: "dependency-blocked", wantErr: true},
		{state: "dependency-blocked", blockedOn: "worker-1"},
		{state: "failed", wantErr: true},
		{state: "failed", errorMessage: "build failed"},
	} {
		err := validateLifecycleMetadata(test.state, test.blockedOn, test.errorMessage, test.inputNeeded)
		if (err != nil) != test.wantErr {
			t.Fatalf("validateLifecycleMetadata(%q) error = %v, wantErr=%v", test.state, err, test.wantErr)
		}
	}
}

func TestValidateLifecycleCompletionRequiresCompletedWaypoints(t *testing.T) {
	st := &status.StatusV2{}
	if err := validateLifecycleCompletion(st, status.LifecycleWorking); err != nil {
		t.Fatalf("validateLifecycleCompletion() rejected nonterminal update: %v", err)
	}
	if err := validateLifecycleCompletion(st, status.LifecycleCompleted); err == nil || !strings.Contains(err.Error(), "required Wayfinder phases are incomplete") {
		t.Fatalf("validateLifecycleCompletion() error = %v, want completion guard", err)
	}
}

func TestApplyLifecycleStateKeepsCanonicalStatusValid(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	for _, test := range []struct {
		state, blockedOn, errorMessage, inputNeeded string
		wantStatus, wantBlockedReason               string
		wantCompletion                              bool
	}{
		{state: status.LifecycleInputRequired, inputNeeded: "choose API", wantStatus: status.StatusV2Blocked, wantBlockedReason: "choose API"},
		{state: status.LifecycleDependencyBlocked, blockedOn: "worker-1", wantStatus: status.StatusV2Blocked, wantBlockedReason: "blocked on worker-1"},
		{state: status.LifecycleFailed, errorMessage: "build failed", wantStatus: status.StatusV2Blocked, wantBlockedReason: "build failed"},
		{state: status.LifecycleCompleted, wantStatus: status.StatusV2Completed, wantCompletion: true},
	} {
		t.Run(test.state, func(t *testing.T) {
			st := status.NewStatusV2("test", status.ProjectTypeFeature, status.RiskLevelS)
			if test.state == status.LifecycleCompleted {
				st.CurrentWaypoint = status.WaypointV2Retro
				for _, waypointName := range status.AllWaypointsV2Schema() {
					st.WaypointHistory = append(st.WaypointHistory, status.WaypointHistory{
						Name:        waypointName,
						Status:      status.WaypointStatusV2Completed,
						StartedAt:   now,
						CompletedAt: &now,
					})
				}
			}
			applyLifecycleState(st, test.state, test.blockedOn, test.errorMessage, test.inputNeeded, now)
			if err := status.ValidateV2(st); err != nil {
				t.Fatalf("ValidateV2 after %s: %v", test.state, err)
			}
			if st.Status != test.wantStatus || st.BlockedReason != test.wantBlockedReason {
				t.Fatalf("status=%q blocked_reason=%q, want %q and %q", st.Status, st.BlockedReason, test.wantStatus, test.wantBlockedReason)
			}
			if (st.CompletionDate != nil) != test.wantCompletion {
				t.Fatalf("completion_date present=%v, want %v", st.CompletionDate != nil, test.wantCompletion)
			}
		})
	}
}
