package recoveryloop

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/pkg/absencealarm"
)

// ActionType is the remediation action to perform on a job.
type ActionType string

// Remediation action types.
const (
	ActionNone      ActionType = "none"
	ActionReinstall ActionType = "reinstall"
	ActionBootstrap ActionType = "bootstrap"
	ActionKickstart ActionType = "kickstart"
)

// RecoveryStatus is the classified status of a job after evaluation/recovery.
type RecoveryStatus string

// Recovery status classifications.
const (
	StatusHealthy   RecoveryStatus = "healthy"
	StatusSnoozed   RecoveryStatus = "snoozed"
	StatusRecovered RecoveryStatus = "recovered"
	StatusFailed    RecoveryStatus = "failed"
)

// Job defines one critical job to monitor and self-heal.
type Job struct {
	Name         string   `json:"name"`
	LaunchdLabel string   `json:"launchd_label,omitempty"`
	PlistPath    string   `json:"plist_path,omitempty"`
	BinaryPath   string   `json:"binary_path,omitempty"`
	InstallCmd   []string `json:"install_cmd,omitempty"`
	Pulse        string   `json:"pulse,omitempty"`
}

// Config is the configuration document for recovery-loop.
type Config struct {
	Jobs []Job `json:"jobs"`
}

// Result is the evaluated or executed outcome for one job.
type Result struct {
	Job         string         `json:"job"`
	Status      RecoveryStatus `json:"status"`
	Action      ActionType     `json:"action"`
	Attempt     int            `json:"attempt"`
	HumanNeeded bool           `json:"human_needed"`
	Reason      string         `json:"reason,omitempty"`
	Error       string         `json:"error,omitempty"`
}

// LaunchdJobInfo contains launchctl list details for a single job.
type LaunchdJobInfo struct {
	PID    int
	Status int
	Label  string
	Loaded bool
}

// HostOps defines injectable host operations for testing.
type HostOps struct {
	Now                func() time.Time
	FileExists         func(path string) bool
	LaunchdList        func(ctx context.Context) (map[string]LaunchdJobInfo, error)
	LaunchctlBootout   func(ctx context.Context, label string) error
	LaunchctlBootstrap func(ctx context.Context, plistPath string) error
	LaunchctlKickstart func(ctx context.Context, label string) error
	RunCommand         func(ctx context.Context, argv []string) (exitCode int, output string, err error)
}

// DefaultJobs returns the canonical set of critical jobs when no config is provided.
func DefaultJobs() []Job {
	return []Job{
		{
			Name:         "mergeloop",
			LaunchdLabel: "com.dear-agent.mergeloop",
			PlistPath:    "~/Library/LaunchAgents/com.dear-agent.mergeloop.plist",
			BinaryPath:   "~/go/bin/mergeloop",
			InstallCmd:   []string{"go", "install", "./cmd/mergeloop"},
			Pulse:        "mergeloop-loaded",
		},
		{
			Name:         "disk-watchdog",
			LaunchdLabel: "com.dear-agent.disk-watchdog",
			PlistPath:    "~/Library/LaunchAgents/com.dear-agent.disk-watchdog.plist",
			BinaryPath:   "~/go/bin/disk-watchdog",
			InstallCmd:   []string{"go", "install", "./cmd/disk-watchdog"},
			Pulse:        "disk-watchdog-tick",
		},
		{
			Name:         "sandbox-gc",
			LaunchdLabel: "com.dear-agent.sandbox-gc",
			PlistPath:    "~/Library/LaunchAgents/com.dear-agent.sandbox-gc.plist",
			BinaryPath:   "~/go/bin/agm",
			InstallCmd:   []string{"go", "install", "./agm/cmd/agm"},
			Pulse:        "sandbox-gc-tick",
		},
		{
			Name:         "token-refresher",
			LaunchdLabel: "com.dear-agent.token-refresher",
			PlistPath:    "~/Library/LaunchAgents/com.dear-agent.token-refresher.plist",
			BinaryPath:   "~/go/bin/token-refresher",
			Pulse:        "token-refresher-tick",
		},
		{
			Name:         "absence-alarm",
			LaunchdLabel: "com.dear-agent.absence-alarm",
			PlistPath:    "~/Library/LaunchAgents/com.dear-agent.absence-alarm.plist",
			BinaryPath:   "~/go/bin/absence-alarm",
			InstallCmd:   []string{"go", "install", "./cmd/absence-alarm"},
			Pulse:        "absence-alarm-heartbeat",
		},
	}
}

// ExpandHome expands leading ~ or ~/ in path.
func ExpandHome(s string) string {
	if s == "~" || strings.HasPrefix(s, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(s, "~"), "/"))
		}
	}
	return s
}

// LoadConfig reads and validates the critical jobs configuration.
func LoadConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read jobs config %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse jobs config %s: %w", path, err)
	}
	if len(cfg.Jobs) == 0 {
		return nil, fmt.Errorf("jobs config %s contains no jobs", path)
	}
	seen := make(map[string]bool, len(cfg.Jobs))
	for i := range cfg.Jobs {
		j := &cfg.Jobs[i]
		if j.Name == "" {
			return nil, fmt.Errorf("jobs config %s: job at index %d has empty name", path, i)
		}
		if seen[j.Name] {
			return nil, fmt.Errorf("jobs config %s: duplicate job name %q", path, j.Name)
		}
		seen[j.Name] = true
		j.PlistPath = ExpandHome(j.PlistPath)
		j.BinaryPath = ExpandHome(j.BinaryPath)
		for k, arg := range j.InstallCmd {
			j.InstallCmd[k] = ExpandHome(arg)
		}
	}
	return &cfg, nil
}

// ParseLaunchdList parses output of `launchctl list`.
func ParseLaunchdList(out string) map[string]LaunchdJobInfo {
	jobs := make(map[string]LaunchdJobInfo)
	scanner := bufio.NewScanner(strings.NewReader(out))
	// Header: PID Status Label - skip if present
	_ = scanner.Scan()
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		label := fields[2]
		pid := 0
		if fields[0] != "-" {
			pid, _ = strconv.Atoi(fields[0])
		}
		status, _ := strconv.Atoi(fields[1])
		jobs[label] = LaunchdJobInfo{
			PID:    pid,
			Status: status,
			Label:  label,
			Loaded: true,
		}
	}
	return jobs
}

// DefaultHostOps returns real host implementations.
func DefaultHostOps() HostOps {
	uid := os.Getuid()
	guiDomain := fmt.Sprintf("gui/%d", uid)
	return HostOps{
		Now: time.Now,
		FileExists: func(path string) bool {
			_, err := os.Stat(path)
			return err == nil
		},
		LaunchdList: func(ctx context.Context) (map[string]LaunchdJobInfo, error) {
			cmd := exec.CommandContext(ctx, "launchctl", "list")
			out, err := cmd.Output()
			if err != nil {
				return nil, fmt.Errorf("launchctl list: %w", err)
			}
			return ParseLaunchdList(string(out)), nil
		},
		LaunchctlBootout: func(ctx context.Context, label string) error {
			target := fmt.Sprintf("%s/%s", guiDomain, label)
			cmd := exec.CommandContext(ctx, "launchctl", "bootout", target)
			return cmd.Run()
		},
		LaunchctlBootstrap: func(ctx context.Context, plistPath string) error {
			cmd := exec.CommandContext(ctx, "launchctl", "bootstrap", guiDomain, plistPath)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("launchctl bootstrap: %w (%s)", err, strings.TrimSpace(string(out)))
			}
			return nil
		},
		LaunchctlKickstart: func(ctx context.Context, label string) error {
			target := fmt.Sprintf("%s/%s", guiDomain, label)
			cmd := exec.CommandContext(ctx, "launchctl", "kickstart", "-k", target)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("launchctl kickstart: %w (%s)", err, strings.TrimSpace(string(out)))
			}
			return nil
		},
		RunCommand: func(ctx context.Context, argv []string) (int, string, error) {
			if len(argv) == 0 {
				return 2, "", errors.New("empty command")
			}
			cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
			out, err := cmd.CombinedOutput()
			output := strings.TrimSpace(string(out))
			if err != nil {
				if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
					return exitErr.ExitCode(), output, err
				}
				return 1, output, err
			}
			return 0, output, nil
		},
	}
}

// LoadAbsenceAlarms reads the absence-alarm journal and returns pulses that are alarming.
func LoadAbsenceAlarms(journalPath string) (map[string]bool, error) {
	alarming := make(map[string]bool)
	f, err := os.Open(journalPath)
	if err != nil {
		if os.IsNotExist(err) {
			return alarming, nil
		}
		return nil, fmt.Errorf("open absence journal %s: %w", journalPath, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec absencealarm.JournalRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if rec.Kind == "absence.alarm" || rec.Status == absencealarm.StatusAbsent || rec.Status == absencealarm.StatusUndetermined {
			alarming[rec.Pulse] = true
		}
	}
	return alarming, scanner.Err()
}

// IsJobSnoozed checks if a job is covered by an active, unexpired snooze (RL-05, RL-22).
func IsJobSnoozed(job Job, snoozes map[string]absencealarm.Snooze, now time.Time) (absencealarm.Snooze, bool) {
	candidates := []string{job.Name, job.Pulse, job.LaunchdLabel}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if sn, ok := snoozes[c]; ok {
			if sn.Until.After(now) {
				return sn, true
			}
		}
	}
	return absencealarm.Snooze{}, false
}

// PlanJob determines the required recovery action for a job without executing it.
func PlanJob(
	job Job,
	snoozes map[string]absencealarm.Snooze,
	alarmingPulses map[string]bool,
	launchdJobs map[string]LaunchdJobInfo,
	host HostOps,
	now time.Time,
) (ActionType, RecoveryStatus, string) {
	if sn, snoozed := IsJobSnoozed(job, snoozes, now); snoozed {
		return ActionNone, StatusSnoozed, fmt.Sprintf("snoozed until %s: %s", sn.Until.Format(time.RFC3339), sn.Reason)
	}

	// RL-01: missing binary check
	if job.BinaryPath != "" && !host.FileExists(job.BinaryPath) {
		if len(job.InstallCmd) > 0 {
			return ActionReinstall, StatusRecovered, fmt.Sprintf("binary %s does not exist on disk", job.BinaryPath)
		}
	}

	// Launchd checks
	if job.LaunchdLabel != "" {
		info, loaded := launchdJobs[job.LaunchdLabel]
		// RL-02, RL-06: unloaded launchd job
		if !loaded {
			return ActionBootstrap, StatusRecovered, fmt.Sprintf("launchd job %s is not loaded", job.LaunchdLabel)
		}
		// RL-03: exit code 78 (EX_CONFIG) or -9 (SIGKILL / code signing mismatch)
		if info.Status == 78 || info.Status == -9 {
			return ActionBootstrap, StatusRecovered, fmt.Sprintf("launchd job %s exited with status %d (LWCR/codesigning issue)", job.LaunchdLabel, info.Status)
		}
		// RL-04: pulse absent/undetermined or non-zero exit when not running
		if (job.Pulse != "" && alarmingPulses[job.Pulse]) || (info.PID == 0 && info.Status != 0) {
			return ActionKickstart, StatusRecovered, fmt.Sprintf("launchd job %s is loaded but pulse %q is alarming (last exit status %d)", job.LaunchdLabel, job.Pulse, info.Status)
		}
	}

	return ActionNone, StatusHealthy, "job is healthy"
}

// ExecuteRecovery executes the planned action bounded by context (RL-21).
func ExecuteRecovery(ctx context.Context, job Job, action ActionType, host HostOps) error {
	switch action {
	case ActionReinstall:
		if len(job.InstallCmd) == 0 {
			return errors.New("no install command configured")
		}
		code, out, err := host.RunCommand(ctx, job.InstallCmd)
		if err != nil || code != 0 {
			return fmt.Errorf("reinstall command %v failed (exit %d): %w (output: %s)", job.InstallCmd, code, err, out)
		}
		return nil
	case ActionBootstrap:
		if job.PlistPath == "" {
			return fmt.Errorf("no plist path configured for %s", job.Name)
		}
		// Best-effort bootout first to clear stale LWCR state if already loaded
		if bErr := host.LaunchctlBootout(ctx, job.LaunchdLabel); bErr != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		if err := host.LaunchctlBootstrap(ctx, job.PlistPath); err != nil {
			return fmt.Errorf("bootstrap %s: %w", job.PlistPath, err)
		}
		return nil
	case ActionKickstart:
		if job.LaunchdLabel == "" {
			return fmt.Errorf("no launchd label configured for %s", job.Name)
		}
		if err := host.LaunchctlKickstart(ctx, job.LaunchdLabel); err != nil {
			return fmt.Errorf("kickstart %s: %w", job.LaunchdLabel, err)
		}
		return nil
	case ActionNone:
		return nil
	default:
		return fmt.Errorf("unknown action type %q", action)
	}
}
