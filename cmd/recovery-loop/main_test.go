package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/recoveryloop"
)

func testHostOps() (recoveryloop.HostOps, *[]string) {
	calls := &[]string{}
	return recoveryloop.HostOps{
		Now: func() time.Time {
			return time.Date(2026, 9, 5, 8, 0, 0, 0, time.UTC)
		},
		FileExists: func(path string) bool {
			return true
		},
		LaunchdList: func(ctx context.Context) (map[string]recoveryloop.LaunchdJobInfo, error) {
			return map[string]recoveryloop.LaunchdJobInfo{
				"com.dear-agent.mergeloop":       {Loaded: true, Status: 0, PID: 100},
				"com.dear-agent.disk-watchdog":   {Loaded: true, Status: 0, PID: 101},
				"com.dear-agent.sandbox-gc":      {Loaded: true, Status: 0, PID: 102},
				"com.dear-agent.token-refresher": {Loaded: true, Status: 0, PID: 103},
				"com.dear-agent.absence-alarm":   {Loaded: true, Status: 0, PID: 104},
			}, nil
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

// RL-11: Exit 0 when all jobs healthy.
func TestCLI_Exit0_AllHealthy(t *testing.T) {
	host, _ := testHostOps()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "jobs.json")
	_ = os.WriteFile(cfgPath, []byte(`{"jobs":[{"name":"j1","launchd_label":"com.dear-agent.mergeloop"}]}`), 0o600)

	var stdout, stderr bytes.Buffer
	code := run([]string{"--config", cfgPath}, &stdout, &stderr, host, nil)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d. stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Status: OK") {
		t.Fatalf("expected Status: OK in stdout: %s", stdout.String())
	}
}

// RL-11: Exit 0 when recovery succeeds.
func TestCLI_Exit0_Recovered(t *testing.T) {
	host, calls := testHostOps()
	// Simulate unloaded job
	host.LaunchdList = func(ctx context.Context) (map[string]recoveryloop.LaunchdJobInfo, error) {
		return map[string]recoveryloop.LaunchdJobInfo{}, nil
	}

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "jobs.json")
	_ = os.WriteFile(cfgPath, []byte(`{"jobs":[{"name":"j1","launchd_label":"com.dear-agent.mergeloop","plist_path":"/Library/LaunchAgents/com.dear-agent.mergeloop.plist"}]}`), 0o600)

	var stdout, stderr bytes.Buffer
	code := run([]string{"--config", cfgPath}, &stdout, &stderr, host, nil)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d. stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "recovered") {
		t.Fatalf("expected recovered in stdout: %s", stdout.String())
	}
	foundBootstrap := false
	for _, call := range *calls {
		if strings.Contains(call, "bootstrap:") {
			foundBootstrap = true
			break
		}
	}
	if !foundBootstrap {
		t.Fatalf("expected bootstrap call, got %v", *calls)
	}
}

// RL-12: Exit 1 when recovery fails.
func TestCLI_Exit1_RecoveryFails(t *testing.T) {
	host, _ := testHostOps()
	host.LaunchctlBootstrap = func(ctx context.Context, plistPath string) error {
		return errors.New("permission denied")
	}
	// Job is unloaded
	host.LaunchdList = func(ctx context.Context) (map[string]recoveryloop.LaunchdJobInfo, error) {
		return map[string]recoveryloop.LaunchdJobInfo{}, nil
	}

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "jobs.json")
	_ = os.WriteFile(cfgPath, []byte(`{"jobs":[{"name":"j1","launchd_label":"com.dear-agent.mergeloop","plist_path":"/Library/LaunchAgents/com.dear-agent.mergeloop.plist"}]}`), 0o600)

	var stdout, stderr bytes.Buffer
	code := run([]string{"--config", cfgPath}, &stdout, &stderr, host, nil)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d. stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Status: ALARM") {
		t.Fatalf("expected Status: ALARM in stdout: %s", stdout.String())
	}
}

// RL-13: Exit 2 on bad flags or invalid config.
func TestCLI_Exit2_UsageError(t *testing.T) {
	host, _ := testHostOps()
	var stdout, stderr bytes.Buffer
	code := run([]string{"--unknown-flag"}, &stdout, &stderr, host, nil)
	if code != 2 {
		t.Fatalf("expected exit 2 on unknown flag, got %d", code)
	}

	code = run([]string{"extra-arg"}, &stdout, &stderr, host, nil)
	if code != 2 {
		t.Fatalf("expected exit 2 on positional arg, got %d", code)
	}

	code = run([]string{"--timeout", "-5s"}, &stdout, &stderr, host, nil)
	if code != 2 {
		t.Fatalf("expected exit 2 on negative timeout, got %d", code)
	}
}

// RL-14: --dry-run plans and reports without executing side effects.
func TestCLI_DryRun_NoExecutionOrWrites(t *testing.T) {
	host, calls := testHostOps()
	host.LaunchdList = func(ctx context.Context) (map[string]recoveryloop.LaunchdJobInfo, error) {
		return map[string]recoveryloop.LaunchdJobInfo{}, nil
	}

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "jobs.json")
	_ = os.WriteFile(cfgPath, []byte(`{"jobs":[{"name":"j1","launchd_label":"com.dear-agent.mergeloop","plist_path":"/plist"}]}`), 0o600)
	statePath := filepath.Join(dir, "state.json")
	hbPath := filepath.Join(dir, "heartbeat.json")
	journalPath := filepath.Join(dir, "journal.jsonl")

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--config", cfgPath,
		"--state", statePath,
		"--heartbeat", hbPath,
		"--journal", journalPath,
		"--dry-run",
	}, &stdout, &stderr, host, nil)

	if code != 0 {
		t.Fatalf("expected exit 0 in dry-run, got %d", code)
	}
	if len(*calls) != 0 {
		t.Fatalf("expected 0 host execution calls in dry-run, got %v", *calls)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("state file should not be created in dry-run")
	}
	if _, err := os.Stat(hbPath); !os.IsNotExist(err) {
		t.Fatalf("heartbeat file should not be created in dry-run")
	}
	if _, err := os.Stat(journalPath); !os.IsNotExist(err) {
		t.Fatalf("journal file should not be created in dry-run")
	}
	if !strings.Contains(stdout.String(), "planned, dry-run") {
		t.Fatalf("expected dry-run note in stdout: %s", stdout.String())
	}
}

// RL-15: --json emits machine-readable report on stdout.
func TestCLI_JSONOutput(t *testing.T) {
	host, _ := testHostOps()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "jobs.json")
	_ = os.WriteFile(cfgPath, []byte(`{"jobs":[{"name":"j1","launchd_label":"com.dear-agent.mergeloop"}]}`), 0o600)

	var stdout, stderr bytes.Buffer
	code := run([]string{"--config", cfgPath, "--json"}, &stdout, &stderr, host, nil)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	var rep recoveryloop.Heartbeat
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("parse JSON stdout: %v\noutput: %s", err, stdout.String())
	}
	if rep.Healthy != 1 || len(rep.Results) != 1 || rep.Results[0].Job != "j1" {
		t.Fatalf("unexpected JSON report content: %+v", rep)
	}
}

// RL-16: Writes heartbeat file on tick completion outside dry-run.
func TestCLI_WritesHeartbeat(t *testing.T) {
	host, _ := testHostOps()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "jobs.json")
	_ = os.WriteFile(cfgPath, []byte(`{"jobs":[{"name":"j1","launchd_label":"com.dear-agent.mergeloop"}]}`), 0o600)
	hbPath := filepath.Join(dir, "heartbeat.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{"--config", cfgPath, "--heartbeat", hbPath}, &stdout, &stderr, host, nil)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	raw, err := os.ReadFile(hbPath)
	if err != nil {
		t.Fatalf("read heartbeat file: %v", err)
	}
	var hb recoveryloop.Heartbeat
	if err := json.Unmarshal(raw, &hb); err != nil {
		t.Fatalf("parse heartbeat JSON: %v", err)
	}
	if hb.Healthy != 1 || len(hb.Results) != 1 {
		t.Fatalf("unexpected heartbeat content: %+v", hb)
	}
}

// RL-09, RL-17: Second failure triggers notification; notification failure does not change exit code.
func TestCLI_SecondFailure_EscalationNotification(t *testing.T) {
	host, _ := testHostOps()
	host.LaunchctlBootstrap = func(ctx context.Context, plistPath string) error {
		return errors.New("bootstrapping failed")
	}
	host.LaunchdList = func(ctx context.Context) (map[string]recoveryloop.LaunchdJobInfo, error) {
		return map[string]recoveryloop.LaunchdJobInfo{}, nil
	}

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "jobs.json")
	_ = os.WriteFile(cfgPath, []byte(`{"jobs":[{"name":"j1","launchd_label":"com.dear-agent.mergeloop","plist_path":"/plist"}]}`), 0o600)
	statePath := filepath.Join(dir, "state.json")

	// Pre-seed 1 failure in state
	seedState := recoveryloop.State{
		Jobs: map[string]recoveryloop.JobState{
			"j1": {ConsecutiveFailures: 1},
		},
	}
	_ = recoveryloop.SaveState(statePath, seedState)

	notified := false
	notifyFn := func(ctx context.Context, title, body string) error {
		notified = true
		if !strings.Contains(title, "HUMAN NEEDED") {
			t.Errorf("expected HUMAN NEEDED in notification title, got %s", title)
		}
		// Return error to test RL-17: notification error reported on stderr without changing exit code
		return errors.New("network unreachable")
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--config", cfgPath,
		"--state", statePath,
	}, &stdout, &stderr, host, notifyFn)

	if code != 1 {
		t.Fatalf("expected exit 1 on recovery failure, got %d", code)
	}
	if !notified {
		t.Fatalf("expected escalation notification to be dispatched on 2nd failure")
	}
	if !strings.Contains(stderr.String(), "recovery-loop: notify: network unreachable") {
		t.Fatalf("expected notify error on stderr: %s", stderr.String())
	}
}
