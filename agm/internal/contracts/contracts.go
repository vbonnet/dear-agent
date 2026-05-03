// Package contracts provides contracts-related functionality.
package contracts

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

//go:embed slo-contracts.yaml
var embeddedContracts embed.FS

// SLOContracts is the top-level structure for all SLO thresholds.
type SLOContracts struct {
	ContractVersion  string           `yaml:"contract_version"`
	SessionLifecycle SessionLifecycle `yaml:"session_lifecycle"`
	ScanLoop         ScanLoop         `yaml:"scan_loop"`
	TrustProtocol    TrustProtocol    `yaml:"trust_protocol"`
	StallDetection   StallDetection   `yaml:"stall_detection"`
	AuditTrail       AuditTrail       `yaml:"audit_trail"`
	Compaction       Compaction       `yaml:"compaction"`
	Retry            Retry            `yaml:"retry"`
	SessionHealth    SessionHealth    `yaml:"session_health"`
	OpsAlerts        OpsAlerts        `yaml:"ops_alerts"`
	Daemon           Daemon           `yaml:"daemon"`
	DaemonAlerts     DaemonAlerts     `yaml:"daemon_alerts"`
}

// SessionLifecycle holds SLOs for session creation, resume, archive, and GC.
type SessionLifecycle struct {
	ResumeReadyTimeout          Duration `yaml:"resume_ready_timeout"`
	BloatSizeThresholdBytes     int64    `yaml:"bloat_size_threshold_bytes"`
	BloatProgressEntryThreshold int      `yaml:"bloat_progress_entry_threshold"`
	SessionScanLimit            int      `yaml:"session_scan_limit"`
	ProcessKillGracePeriod      Duration `yaml:"process_kill_grace_period"`
	GCProtectedRoles            []string `yaml:"gc_protected_roles"`
	CooldownInterval            Duration `yaml:"cooldown_interval"`
	RecentActivityThreshold     Duration `yaml:"recent_activity_threshold"`
	LockTimeout                 Duration `yaml:"lock_timeout"`
	LockPollInterval            Duration `yaml:"lock_poll_interval"`
}

// ScanLoop holds SLOs for the orchestrator scan loop and cross-check.
type ScanLoop struct {
	DefaultScanInterval  Duration `yaml:"default_scan_interval"`
	StuckTimeout         Duration `yaml:"stuck_timeout"`
	ScanGapTimeout       Duration `yaml:"scan_gap_timeout"`
	WorkerCommitLookback Duration `yaml:"worker_commit_lookback"`
	MetricsWindow        Duration `yaml:"metrics_window"`
	TmuxCaptureDepth     int      `yaml:"tmux_capture_depth"`
	SessionListLimit     int      `yaml:"session_list_limit"`
}

// TrustProtocol holds SLOs for the trust scoring system.
type TrustProtocol struct {
	BaseScore        int            `yaml:"base_score"`
	MinScore         int            `yaml:"min_score"`
	MaxScore         int            `yaml:"max_score"`
	EventDeltas      map[string]int `yaml:"event_deltas"`
	MinDispatchScore int            `yaml:"min_dispatch_score"`
	PreferredScore   int            `yaml:"preferred_score"`
	ProbationScore   int            `yaml:"probation_score"`
}

// StallDetection holds SLOs for stall detection and recovery.
type StallDetection struct {
	PermissionTimeout     Duration `yaml:"permission_timeout"`
	NoCommitTimeout       Duration `yaml:"no_commit_timeout"`
	ErrorRepeatThreshold  int      `yaml:"error_repeat_threshold"`
	TmuxCaptureDepth      int      `yaml:"tmux_capture_depth"`
	ErrorMessageMaxLength int      `yaml:"error_message_max_length"`
	SessionScanLimit      int      `yaml:"session_scan_limit"`
}

// AuditTrail holds SLOs for the audit logging subsystem.
type AuditTrail struct {
	MaxLineBufferBytes      int    `yaml:"max_line_buffer_bytes"`
	LogDirectoryPermissions uint32 `yaml:"log_directory_permissions"`
	LogFilePermissions      uint32 `yaml:"log_file_permissions"`
}

// Compaction holds thresholds for auto-compaction triggers.
type Compaction struct {
	ContextThresholdWarn     float64 `yaml:"context_threshold_warn"`
	ContextThresholdCompact  float64 `yaml:"context_threshold_compact"`
	ContextThresholdCritical float64 `yaml:"context_threshold_critical"`
}

// Retry holds retry strategy configuration.
type Retry struct {
	MaxRetries    int        `yaml:"max_retries"`
	BackoffDelays []Duration `yaml:"backoff_delays"`
}

// SessionHealth holds thresholds for session health assessment.
type SessionHealth struct {
	ResponsivenessCriticalTimeout Duration `yaml:"responsiveness_critical_timeout"`
	ResponsivenessWarningTimeout  Duration `yaml:"responsiveness_warning_timeout"`
	ErrorLinesCritical            int      `yaml:"error_lines_critical"`
	ErrorLinesWarning             int      `yaml:"error_lines_warning"`
	CPUWarningPercent             float64  `yaml:"cpu_warning_percent"`
	MemoryWarningMB               int      `yaml:"memory_warning_mb"`
	TrustScoreWarning             int      `yaml:"trust_score_warning"`
}

// OpsAlerts holds thresholds for system-level alert generation.
type OpsAlerts struct {
	LoadThreshold             float64  `yaml:"load_threshold"`
	MemoryThresholdPercent    float64  `yaml:"memory_threshold_percent"`
	DiskThresholdPercent      float64  `yaml:"disk_threshold_percent"`
	PermissionPromptThreshold Duration `yaml:"permission_prompt_threshold"`
}

// Daemon holds daemon polling and retry configuration.
type Daemon struct {
	PollInterval       Duration `yaml:"poll_interval"`
	MaxRetries         int      `yaml:"max_retries"`
	InitialBackoff     Duration `yaml:"initial_backoff"`
	LatencyHistorySize int      `yaml:"latency_history_size"`
}

// DaemonAlerts holds thresholds for daemon alert rules.
type DaemonAlerts struct {
	QueueDepthCritical     int      `yaml:"queue_depth_critical"`
	QueueDepthWarning      int      `yaml:"queue_depth_warning"`
	SuccessRateCritical    float64  `yaml:"success_rate_critical"`
	SuccessRateWarning     float64  `yaml:"success_rate_warning"`
	LatencyCritical        Duration `yaml:"latency_critical"`
	LatencyWarning         Duration `yaml:"latency_warning"`
	PollTimeout            Duration `yaml:"poll_timeout"`
	StateErrorRateCritical float64  `yaml:"state_error_rate_critical"`
	StateErrorRateWarning  float64  `yaml:"state_error_rate_warning"`
}

// Duration wraps time.Duration for YAML unmarshalling of duration strings.
type Duration struct {
	time.Duration
}

// UnmarshalYAML parses a duration string like "5m" or "24h".
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	d.Duration = dur
	return nil
}

// MarshalYAML serialises the duration back to a string.
func (d Duration) MarshalYAML() (interface{}, error) {
	return d.String(), nil
}

var (
	globalContracts *SLOContracts
	loadOnce        sync.Once
	loadErr         error
)

// Load reads SLO contracts from the following sources in priority order:
//  1. ~/.agm/slo-contracts.yaml  (user override)
//  2. Embedded default           (compiled into the binary)
//
// The result is cached after the first call.
func Load() *SLOContracts {
	loadOnce.Do(func() {
		globalContracts, loadErr = loadFromSources()
		if loadErr != nil || globalContracts == nil {
			globalContracts = Defaults()
		}
	})
	return globalContracts
}

// LoadFromFile reads SLO contracts from a specific YAML file.
func LoadFromFile(path string) (*SLOContracts, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read contracts file: %w", err)
	}
	return parse(data)
}

// Defaults returns the hardcoded default SLO contracts.
func Defaults() *SLOContracts {
	return &SLOContracts{
		ContractVersion: "1.0.0",
		SessionLifecycle: SessionLifecycle{
			ResumeReadyTimeout:          Duration{5 * time.Second},
			BloatSizeThresholdBytes:     100 * 1024 * 1024,
			BloatProgressEntryThreshold: 1000,
			SessionScanLimit:            1000,
			ProcessKillGracePeriod:      Duration{2 * time.Second},
			GCProtectedRoles:            []string{"orchestrator", "meta-orchestrator", "overseer"},
			CooldownInterval:            Duration{5 * time.Minute},
			RecentActivityThreshold:     Duration{5 * time.Minute},
			LockTimeout:                 Duration{5 * time.Second},
			LockPollInterval:            Duration{50 * time.Millisecond},
		},
		ScanLoop: ScanLoop{
			DefaultScanInterval:  Duration{5 * time.Minute},
			StuckTimeout:         Duration{5 * time.Minute},
			ScanGapTimeout:       Duration{10 * time.Minute},
			WorkerCommitLookback: Duration{24 * time.Hour},
			MetricsWindow:        Duration{1 * time.Hour},
			TmuxCaptureDepth:     30,
			SessionListLimit:     1000,
		},
		TrustProtocol: TrustProtocol{
			BaseScore: 50,
			MinScore:  0,
			MaxScore:  100,
			EventDeltas: map[string]int{
				"success":              5,
				"false_completion":     -15,
				"stall":                -5,
				"error_loop":           -3,
				"permission_churn":     -1,
				"quality_gate_failure": -10,
				"gc_archived":          0,
			},
			MinDispatchScore: 30,
			PreferredScore:   60,
			ProbationScore:   20,
		},
		StallDetection: StallDetection{
			PermissionTimeout:     Duration{5 * time.Minute},
			NoCommitTimeout:       Duration{15 * time.Minute},
			ErrorRepeatThreshold:  3,
			TmuxCaptureDepth:      50,
			ErrorMessageMaxLength: 100,
			SessionScanLimit:      1000,
		},
		AuditTrail: AuditTrail{
			MaxLineBufferBytes:      1024 * 1024,
			LogDirectoryPermissions: 0755,
			LogFilePermissions:      0644,
		},
		Compaction: Compaction{
			ContextThresholdWarn:     70.0,
			ContextThresholdCompact:  80.0,
			ContextThresholdCritical: 90.0,
		},
		Retry: Retry{
			MaxRetries: 3,
			BackoffDelays: []Duration{
				{1 * time.Minute},
				{3 * time.Minute},
				{10 * time.Minute},
			},
		},
		SessionHealth: SessionHealth{
			ResponsivenessCriticalTimeout: Duration{30 * time.Minute},
			ResponsivenessWarningTimeout:  Duration{10 * time.Minute},
			ErrorLinesCritical:            10,
			ErrorLinesWarning:             3,
			CPUWarningPercent:             90.0,
			MemoryWarningMB:               4096,
			TrustScoreWarning:             20,
		},
		OpsAlerts: OpsAlerts{
			LoadThreshold:             12.0,
			MemoryThresholdPercent:    80.0,
			DiskThresholdPercent:      85.0,
			PermissionPromptThreshold: Duration{5 * time.Minute},
		},
		Daemon: Daemon{
			PollInterval:       Duration{30 * time.Second},
			MaxRetries:         3,
			InitialBackoff:     Duration{5 * time.Second},
			LatencyHistorySize: 100,
		},
		DaemonAlerts: DaemonAlerts{
			QueueDepthCritical:     100,
			QueueDepthWarning:      50,
			SuccessRateCritical:    50.0,
			SuccessRateWarning:     75.0,
			LatencyCritical:        Duration{30 * time.Second},
			LatencyWarning:         Duration{10 * time.Second},
			PollTimeout:            Duration{5 * time.Minute},
			StateErrorRateCritical: 25.0,
			StateErrorRateWarning:  10.0,
		},
	}
}

// ResetForTesting clears the cached contracts so Load() re-reads on next call.
func ResetForTesting() {
	loadOnce = sync.Once{}
	globalContracts = nil
	loadErr = nil
}

func loadFromSources() (*SLOContracts, error) {
	// Priority 1: user override at ~/.agm/slo-contracts.yaml
	home, err := os.UserHomeDir()
	if err == nil {
		userPath := filepath.Join(home, ".agm", "slo-contracts.yaml")
		if data, err := os.ReadFile(userPath); err == nil {
			return parse(data)
		}
	}

	// Priority 2: embedded default
	data, err := fs.ReadFile(embeddedContracts, "slo-contracts.yaml")
	if err != nil {
		return nil, fmt.Errorf("read embedded contracts: %w", err)
	}
	return parse(data)
}

func parse(data []byte) (*SLOContracts, error) {
	var c SLOContracts
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse contracts YAML: %w", err)
	}
	return &c, nil
}
