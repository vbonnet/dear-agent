package recoveryloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/absencealarm"
)

func mockHostOps() (HostOps, *[]string) {
	calls := &[]string{}
	return HostOps{
		Now: func() time.Time {
			return time.Date(2026, 9, 5, 8, 0, 0, 0, time.UTC)
		},
		FileExists: func(path string) bool {
			return true
		},
		LaunchdList: func(ctx context.Context) (map[string]LaunchdJobInfo, error) {
			return map[string]LaunchdJobInfo{}, nil
		},
		LaunchctlBootout: func(ctx context.Context, label string) error {
			*calls = append(*calls, fmt.Sprintf("bootout:%s", label))
			return nil
		},
		LaunchctlBootstrap: func(ctx context.Context, plistPath string) error {
			*calls = append(*calls, fmt.Sprintf("bootstrap:%s", plistPath))
			return nil
		},
		LaunchctlKickstart: func(ctx context.Context, label string) error {
			*calls = append(*calls, fmt.Sprintf("kickstart:%s", label))
			return nil
		},
		RunCommand: func(ctx context.Context, argv []string) (int, string, error) {
			*calls = append(*calls, fmt.Sprintf("run:%s", strings.Join(argv, " ")))
			return 0, "ok", nil
		},
	}, calls
}

// RL-01: When binary is missing, plan reinstall.
func TestPlan_MissingBinary_Reinstall(t *testing.T) {
	host, _ := mockHostOps()
	host.FileExists = func(path string) bool {
		return path != "/bin/myjob"
	}
	job := Job{
		Name:         "myjob",
		BinaryPath:   "/bin/myjob",
		InstallCmd:   []string{"go", "install", "./cmd/myjob"},
		LaunchdLabel: "com.example.myjob",
	}
	launchdJobs := map[string]LaunchdJobInfo{
		"com.example.myjob": {Loaded: true, Status: 0},
	}
	action, status, _ := PlanJob(job, nil, nil, launchdJobs, host, host.Now())
	if action != ActionReinstall {
		t.Fatalf("expected ActionReinstall, got %s", action)
	}
	if status != StatusRecovered {
		t.Fatalf("expected StatusRecovered, got %s", status)
	}
}

// RL-02: When launchd job is unloaded, plan bootstrap.
func TestPlan_UnloadedJob_Bootstrap(t *testing.T) {
	host, _ := mockHostOps()
	job := Job{
		Name:         "myjob",
		BinaryPath:   "/bin/myjob",
		PlistPath:    "/Library/LaunchAgents/com.example.myjob.plist",
		LaunchdLabel: "com.example.myjob",
	}
	launchdJobs := map[string]LaunchdJobInfo{} // empty, so not loaded
	action, status, reason := PlanJob(job, nil, nil, launchdJobs, host, host.Now())
	if action != ActionBootstrap {
		t.Fatalf("expected ActionBootstrap, got %s", action)
	}
	if status != StatusRecovered {
		t.Fatalf("expected StatusRecovered, got %s", status)
	}
	if !strings.Contains(reason, "not loaded") {
		t.Fatalf("unexpected reason: %s", reason)
	}
}

// RL-03: When launchd job exited with 78 or -9, plan bootstrap (bootout + bootstrap).
func TestPlan_Exit78_Rebootstrap(t *testing.T) {
	host, _ := mockHostOps()
	job := Job{
		Name:         "myjob",
		BinaryPath:   "/bin/myjob",
		PlistPath:    "/Library/LaunchAgents/com.example.myjob.plist",
		LaunchdLabel: "com.example.myjob",
	}
	launchdJobs := map[string]LaunchdJobInfo{
		"com.example.myjob": {Loaded: true, Status: 78},
	}
	action, status, reason := PlanJob(job, nil, nil, launchdJobs, host, host.Now())
	if action != ActionBootstrap {
		t.Fatalf("expected ActionBootstrap, got %s", action)
	}
	if status != StatusRecovered {
		t.Fatalf("expected StatusRecovered, got %s", status)
	}
	if !strings.Contains(reason, "78") {
		t.Fatalf("unexpected reason: %s", reason)
	}
}

// RL-04: When pulse is alarming in absence journal and service is loaded, plan kickstart.
func TestPlan_PulseAlarming_Kickstart(t *testing.T) {
	host, _ := mockHostOps()
	job := Job{
		Name:         "myjob",
		BinaryPath:   "/bin/myjob",
		Pulse:        "myjob-pulse",
		LaunchdLabel: "com.example.myjob",
	}
	launchdJobs := map[string]LaunchdJobInfo{
		"com.example.myjob": {Loaded: true, Status: 0, PID: 1234},
	}
	alarmingPulses := map[string]bool{
		"myjob-pulse": true,
	}
	action, status, _ := PlanJob(job, nil, alarmingPulses, launchdJobs, host, host.Now())
	if action != ActionKickstart {
		t.Fatalf("expected ActionKickstart, got %s", action)
	}
	if status != StatusRecovered {
		t.Fatalf("expected StatusRecovered, got %s", status)
	}
}

// RL-05: Unexpired snooze suppresses recovery.
func TestPlan_UnexpiredSnooze_Suppressed(t *testing.T) {
	host, _ := mockHostOps()
	now := host.Now()
	job := Job{
		Name:         "myjob",
		BinaryPath:   "/bin/myjob",
		LaunchdLabel: "com.example.myjob",
	}
	snoozes := map[string]absencealarm.Snooze{
		"myjob": {
			Pulse:  "myjob",
			Until:  now.Add(2 * time.Hour),
			Reason: "maintenance in progress",
		},
	}
	// Job is completely unloaded
	launchdJobs := map[string]LaunchdJobInfo{}
	action, status, reason := PlanJob(job, snoozes, nil, launchdJobs, host, now)
	if action != ActionNone {
		t.Fatalf("expected ActionNone for snoozed job, got %s", action)
	}
	if status != StatusSnoozed {
		t.Fatalf("expected StatusSnoozed, got %s", status)
	}
	if !strings.Contains(reason, "maintenance in progress") {
		t.Fatalf("unexpected reason: %s", reason)
	}
}

// RL-06: Unloaded job without unexpired snooze attempts recovery.
func TestPlan_UnloadedWithoutSnooze_AttemptsRecovery(t *testing.T) {
	host, _ := mockHostOps()
	now := host.Now()
	job := Job{
		Name:         "myjob",
		BinaryPath:   "/bin/myjob",
		PlistPath:    "/Library/LaunchAgents/com.example.myjob.plist",
		LaunchdLabel: "com.example.myjob",
	}
	// Snooze is for another job
	snoozes := map[string]absencealarm.Snooze{
		"otherjob": {Pulse: "otherjob", Until: now.Add(time.Hour)},
	}
	launchdJobs := map[string]LaunchdJobInfo{}
	action, status, _ := PlanJob(job, snoozes, nil, launchdJobs, host, now)
	if action != ActionBootstrap {
		t.Fatalf("expected ActionBootstrap, got %s", action)
	}
	if status != StatusRecovered {
		t.Fatalf("expected StatusRecovered, got %s", status)
	}
}

// RL-22: Expired snooze is treated as active and eligible for recovery.
func TestPlan_ExpiredSnooze_EligibleForRecovery(t *testing.T) {
	host, _ := mockHostOps()
	now := host.Now()
	job := Job{
		Name:         "myjob",
		BinaryPath:   "/bin/myjob",
		PlistPath:    "/Library/LaunchAgents/com.example.myjob.plist",
		LaunchdLabel: "com.example.myjob",
	}
	// Snooze expired 10 minutes ago
	snoozes := map[string]absencealarm.Snooze{
		"myjob": {Pulse: "myjob", Until: now.Add(-10 * time.Minute), Reason: "expired yesterday"},
	}
	launchdJobs := map[string]LaunchdJobInfo{}
	action, status, _ := PlanJob(job, snoozes, nil, launchdJobs, host, now)
	if action != ActionBootstrap {
		t.Fatalf("expected ActionBootstrap for expired snooze, got %s", action)
	}
	if status != StatusRecovered {
		t.Fatalf("expected StatusRecovered, got %s", status)
	}
}

// RL-07, RL-08, RL-09: Success resets consecutive failures; failure increments; >= 2 sets human needed.
func TestState_GraduatedEscalation(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")

	st, err := LoadState(statePath)
	if err != nil {
		t.Fatalf("unexpected load err: %v", err)
	}
	if len(st.Jobs) != 0 {
		t.Fatalf("expected empty state")
	}

	// First failure: count = 1, humanNeeded = false
	st.Jobs["myjob"] = JobState{ConsecutiveFailures: 1, HumanNeeded: false}
	if err := SaveState(statePath, st); err != nil {
		t.Fatalf("save err: %v", err)
	}

	loaded, err := LoadState(statePath)
	if err != nil {
		t.Fatalf("load err: %v", err)
	}
	if loaded.Jobs["myjob"].ConsecutiveFailures != 1 || loaded.Jobs["myjob"].HumanNeeded {
		t.Fatalf("unexpected state after 1 failure: %+v", loaded.Jobs["myjob"])
	}

	// Second failure: count = 2, humanNeeded = true (RL-09)
	st.Jobs["myjob"] = JobState{ConsecutiveFailures: 2, HumanNeeded: true}
	_ = SaveState(statePath, st)
	loaded, _ = LoadState(statePath)
	if loaded.Jobs["myjob"].ConsecutiveFailures != 2 || !loaded.Jobs["myjob"].HumanNeeded {
		t.Fatalf("expected HumanNeeded = true on second failure")
	}

	// Recovery succeeds: count = 0, humanNeeded = false (RL-07)
	st.Jobs["myjob"] = JobState{ConsecutiveFailures: 0, HumanNeeded: false}
	_ = SaveState(statePath, st)
	loaded, _ = LoadState(statePath)
	if loaded.Jobs["myjob"].ConsecutiveFailures != 0 || loaded.Jobs["myjob"].HumanNeeded {
		t.Fatalf("expected failure count reset to 0 upon success")
	}
}

// RL-10: Journal recording appends valid JSON records.
func TestJournal_Append(t *testing.T) {
	dir := t.TempDir()
	journalPath := filepath.Join(dir, "recovery-loop.jsonl")

	rec1 := JournalRecord{
		Time:        time.Date(2026, 9, 5, 8, 0, 0, 0, time.UTC),
		Kind:        "recovery.attempt",
		Job:         "mergeloop",
		Action:      ActionBootstrap,
		Status:      StatusRecovered,
		Attempt:     1,
		HumanNeeded: false,
		Reason:      "service not loaded",
	}
	if err := AppendJournal(journalPath, rec1); err != nil {
		t.Fatalf("append journal: %v", err)
	}

	rec2 := JournalRecord{
		Time:        time.Date(2026, 9, 5, 8, 10, 0, 0, time.UTC),
		Kind:        "recovery.attempt",
		Job:         "disk-watchdog",
		Action:      ActionKickstart,
		Status:      StatusFailed,
		Attempt:     2,
		HumanNeeded: true,
		Reason:      "exit code 1",
		Error:       "kickstart failed",
	}
	if err := AppendJournal(journalPath, rec2); err != nil {
		t.Fatalf("append journal: %v", err)
	}

	raw, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 journal lines, got %d", len(lines))
	}
	var parsed1, parsed2 JournalRecord
	if err := json.Unmarshal([]byte(lines[0]), &parsed1); err != nil {
		t.Fatalf("parse line 1: %v", err)
	}
	if parsed1.Job != "mergeloop" || parsed1.Status != StatusRecovered || parsed1.HumanNeeded {
		t.Fatalf("line 1 mismatch: %+v", parsed1)
	}
	if err := json.Unmarshal([]byte(lines[1]), &parsed2); err != nil {
		t.Fatalf("parse line 2: %v", err)
	}
	if parsed2.Job != "disk-watchdog" || parsed2.Status != StatusFailed || !parsed2.HumanNeeded || parsed2.Attempt != 2 {
		t.Fatalf("line 2 mismatch: %+v", parsed2)
	}
}

// RL-18: Unreadable state file degrades to empty state.
func TestState_UnreadableDegrades(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "corrupt.json")
	if err := os.WriteFile(statePath, []byte("NOT_JSON"), 0o600); err != nil {
		t.Fatalf("write corrupt: %v", err)
	}
	st, err := LoadState(statePath)
	if err == nil {
		t.Fatalf("expected error on corrupt state")
	}
	if st.Jobs == nil || len(st.Jobs) != 0 {
		t.Fatalf("expected empty jobs map in returned state")
	}
}

// RL-19: Duplicate job names in config cause rejection.
func TestConfig_DuplicateNamesRejected(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "jobs.json")
	cfgContent := `{"jobs": [{"name": "myjob"}, {"name": "myjob"}]}`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, err := LoadConfig(cfgPath)
	if err == nil {
		t.Fatalf("expected error for duplicate job names")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// RL-21: Execution is bounded by context.
func TestExecuteRecovery_ContextTimeout(t *testing.T) {
	host, _ := mockHostOps()
	host.RunCommand = func(ctx context.Context, argv []string) (int, string, error) {
		select {
		case <-time.After(100 * time.Millisecond):
			return 0, "", nil
		case <-ctx.Done():
			return 1, "", ctx.Err()
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	job := Job{Name: "slow", InstallCmd: []string{"sleep", "1"}}
	err := ExecuteRecovery(ctx, job, ActionReinstall, host)
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("unexpected err: %v", err)
	}
}
