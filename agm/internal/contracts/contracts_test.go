package contracts

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	d := Defaults()

	if d.ContractVersion != "1.0.0" {
		t.Errorf("ContractVersion = %q, want %q", d.ContractVersion, "1.0.0")
	}

	// Session Lifecycle
	sl := d.SessionLifecycle
	if sl.ResumeReadyTimeout.Duration != 5*time.Second {
		t.Errorf("ResumeReadyTimeout = %v, want 5s", sl.ResumeReadyTimeout.Duration)
	}
	if sl.BloatSizeThresholdBytes != 100*1024*1024 {
		t.Errorf("BloatSizeThresholdBytes = %d, want %d", sl.BloatSizeThresholdBytes, 100*1024*1024)
	}
	if sl.CooldownInterval.Duration != 5*time.Minute {
		t.Errorf("CooldownInterval = %v, want 5m", sl.CooldownInterval.Duration)
	}
	if sl.RecentActivityThreshold.Duration != 5*time.Minute {
		t.Errorf("RecentActivityThreshold = %v, want 5m", sl.RecentActivityThreshold.Duration)
	}
	if sl.LockTimeout.Duration != 5*time.Second {
		t.Errorf("LockTimeout = %v, want 5s", sl.LockTimeout.Duration)
	}
	if sl.LockPollInterval.Duration != 50*time.Millisecond {
		t.Errorf("LockPollInterval = %v, want 50ms", sl.LockPollInterval.Duration)
	}

	// Compaction
	cp := d.Compaction
	if cp.ContextThresholdWarn != 70.0 {
		t.Errorf("ContextThresholdWarn = %v, want 70.0", cp.ContextThresholdWarn)
	}
	if cp.ContextThresholdCompact != 80.0 {
		t.Errorf("ContextThresholdCompact = %v, want 80.0", cp.ContextThresholdCompact)
	}
	if cp.ContextThresholdCritical != 90.0 {
		t.Errorf("ContextThresholdCritical = %v, want 90.0", cp.ContextThresholdCritical)
	}

	// Retry
	rt := d.Retry
	if rt.MaxRetries != 3 {
		t.Errorf("Retry.MaxRetries = %d, want 3", rt.MaxRetries)
	}
	if len(rt.BackoffDelays) != 3 {
		t.Fatalf("Retry.BackoffDelays len = %d, want 3", len(rt.BackoffDelays))
	}
	if rt.BackoffDelays[0].Duration != 1*time.Minute {
		t.Errorf("BackoffDelays[0] = %v, want 1m", rt.BackoffDelays[0].Duration)
	}
	if rt.BackoffDelays[1].Duration != 3*time.Minute {
		t.Errorf("BackoffDelays[1] = %v, want 3m", rt.BackoffDelays[1].Duration)
	}
	if rt.BackoffDelays[2].Duration != 10*time.Minute {
		t.Errorf("BackoffDelays[2] = %v, want 10m", rt.BackoffDelays[2].Duration)
	}

	// Session Health
	sh := d.SessionHealth
	if sh.ResponsivenessCriticalTimeout.Duration != 30*time.Minute {
		t.Errorf("ResponsivenessCriticalTimeout = %v, want 30m", sh.ResponsivenessCriticalTimeout.Duration)
	}
	if sh.ResponsivenessWarningTimeout.Duration != 10*time.Minute {
		t.Errorf("ResponsivenessWarningTimeout = %v, want 10m", sh.ResponsivenessWarningTimeout.Duration)
	}
	if sh.ErrorLinesCritical != 10 {
		t.Errorf("ErrorLinesCritical = %d, want 10", sh.ErrorLinesCritical)
	}
	if sh.ErrorLinesWarning != 3 {
		t.Errorf("ErrorLinesWarning = %d, want 3", sh.ErrorLinesWarning)
	}
	if sh.CPUWarningPercent != 90.0 {
		t.Errorf("CPUWarningPercent = %v, want 90.0", sh.CPUWarningPercent)
	}
	if sh.MemoryWarningMB != 4096 {
		t.Errorf("MemoryWarningMB = %d, want 4096", sh.MemoryWarningMB)
	}
	if sh.TrustScoreWarning != 20 {
		t.Errorf("TrustScoreWarning = %d, want 20", sh.TrustScoreWarning)
	}

	// Ops Alerts
	oa := d.OpsAlerts
	if oa.LoadThreshold != 12.0 {
		t.Errorf("LoadThreshold = %v, want 12.0", oa.LoadThreshold)
	}
	if oa.MemoryThresholdPercent != 80.0 {
		t.Errorf("MemoryThresholdPercent = %v, want 80.0", oa.MemoryThresholdPercent)
	}
	if oa.DiskThresholdPercent != 85.0 {
		t.Errorf("DiskThresholdPercent = %v, want 85.0", oa.DiskThresholdPercent)
	}
	if oa.PermissionPromptThreshold.Duration != 5*time.Minute {
		t.Errorf("PermissionPromptThreshold = %v, want 5m", oa.PermissionPromptThreshold.Duration)
	}

	// Daemon
	dm := d.Daemon
	if dm.PollInterval.Duration != 30*time.Second {
		t.Errorf("Daemon.PollInterval = %v, want 30s", dm.PollInterval.Duration)
	}
	if dm.MaxRetries != 3 {
		t.Errorf("Daemon.MaxRetries = %d, want 3", dm.MaxRetries)
	}
	if dm.InitialBackoff.Duration != 5*time.Second {
		t.Errorf("Daemon.InitialBackoff = %v, want 5s", dm.InitialBackoff.Duration)
	}
	if dm.LatencyHistorySize != 100 {
		t.Errorf("Daemon.LatencyHistorySize = %d, want 100", dm.LatencyHistorySize)
	}

	// Daemon Alerts
	da := d.DaemonAlerts
	if da.QueueDepthCritical != 100 {
		t.Errorf("QueueDepthCritical = %d, want 100", da.QueueDepthCritical)
	}
	if da.QueueDepthWarning != 50 {
		t.Errorf("QueueDepthWarning = %d, want 50", da.QueueDepthWarning)
	}
	if da.SuccessRateCritical != 50.0 {
		t.Errorf("SuccessRateCritical = %v, want 50.0", da.SuccessRateCritical)
	}
	if da.SuccessRateWarning != 75.0 {
		t.Errorf("SuccessRateWarning = %v, want 75.0", da.SuccessRateWarning)
	}
	if da.LatencyCritical.Duration != 30*time.Second {
		t.Errorf("LatencyCritical = %v, want 30s", da.LatencyCritical.Duration)
	}
	if da.LatencyWarning.Duration != 10*time.Second {
		t.Errorf("LatencyWarning = %v, want 10s", da.LatencyWarning.Duration)
	}
	if da.PollTimeout.Duration != 5*time.Minute {
		t.Errorf("PollTimeout = %v, want 5m", da.PollTimeout.Duration)
	}
	if da.StateErrorRateCritical != 25.0 {
		t.Errorf("StateErrorRateCritical = %v, want 25.0", da.StateErrorRateCritical)
	}
	if da.StateErrorRateWarning != 10.0 {
		t.Errorf("StateErrorRateWarning = %v, want 10.0", da.StateErrorRateWarning)
	}
}

func TestLoadEmbedded(t *testing.T) {
	ResetForTesting()
	c := Load()
	if c == nil {
		t.Fatal("Load() returned nil")
	}
	d := Defaults()
	if c.Compaction.ContextThresholdWarn != d.Compaction.ContextThresholdWarn {
		t.Errorf("ContextThresholdWarn mismatch: YAML=%v Defaults=%v",
			c.Compaction.ContextThresholdWarn, d.Compaction.ContextThresholdWarn)
	}
	if c.Daemon.PollInterval.Duration != d.Daemon.PollInterval.Duration {
		t.Errorf("Daemon.PollInterval mismatch: YAML=%v Defaults=%v",
			c.Daemon.PollInterval.Duration, d.Daemon.PollInterval.Duration)
	}
	if c.DaemonAlerts.QueueDepthCritical != d.DaemonAlerts.QueueDepthCritical {
		t.Errorf("QueueDepthCritical mismatch: YAML=%d Defaults=%d",
			c.DaemonAlerts.QueueDepthCritical, d.DaemonAlerts.QueueDepthCritical)
	}
}

func TestLoadCached(t *testing.T) {
	ResetForTesting()
	c1 := Load()
	c2 := Load()
	if c1 != c2 {
		t.Error("Load() should return cached instance on second call")
	}
}

func writeTestYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadFromFile_NewSections(t *testing.T) {
	path := writeTestYAML(t, "contract_version: \"2.0.0\"\ncompaction:\n  context_threshold_warn: 60.0\n  context_threshold_compact: 75.0\n  context_threshold_critical: 85.0\ndaemon:\n  poll_interval: 1m\n  max_retries: 5\n  initial_backoff: 10s\n  latency_history_size: 200\n")

	c, err := LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.ContractVersion != "2.0.0" {
		t.Errorf("ContractVersion = %q, want 2.0.0", c.ContractVersion)
	}
	if c.Compaction.ContextThresholdWarn != 60.0 {
		t.Errorf("ContextThresholdWarn = %v, want 60.0", c.Compaction.ContextThresholdWarn)
	}
	if c.Daemon.PollInterval.Duration != 1*time.Minute {
		t.Errorf("Daemon.PollInterval = %v, want 1m", c.Daemon.PollInterval.Duration)
	}
	if c.Daemon.MaxRetries != 5 {
		t.Errorf("Daemon.MaxRetries = %d, want 5", c.Daemon.MaxRetries)
	}
	if c.Daemon.LatencyHistorySize != 200 {
		t.Errorf("Daemon.LatencyHistorySize = %d, want 200", c.Daemon.LatencyHistorySize)
	}
}

func TestLoadFromFile_NotFound(t *testing.T) {
	_, err := LoadFromFile("/nonexistent/path.yaml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoadFromFile_InvalidYAML(t *testing.T) {
	path := writeTestYAML(t, "{{{{invalid yaml")
	_, err := LoadFromFile(path)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestDurationUnmarshal(t *testing.T) {
	path := writeTestYAML(t, "stall_detection:\n  permission_timeout: 3m30s\n  no_commit_timeout: 1h15m\n  error_repeat_threshold: 1\n  tmux_capture_depth: 10\n  error_message_max_length: 50\n  session_scan_limit: 100\n")

	c, err := LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.StallDetection.PermissionTimeout.Duration != 3*time.Minute+30*time.Second {
		t.Errorf("PermissionTimeout = %v, want 3m30s", c.StallDetection.PermissionTimeout.Duration)
	}
	if c.StallDetection.NoCommitTimeout.Duration != time.Hour+15*time.Minute {
		t.Errorf("NoCommitTimeout = %v, want 1h15m", c.StallDetection.NoCommitTimeout.Duration)
	}
}

func TestResetForTesting(t *testing.T) {
	ResetForTesting()
	c1 := Load()
	ResetForTesting()
	c2 := Load()
	if c1 == c2 {
		t.Error("After ResetForTesting, Load() should return a new instance")
	}
}
