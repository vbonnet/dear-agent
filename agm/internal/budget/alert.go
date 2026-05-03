package budget

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// Alert represents a budget threshold alert.
type Alert struct {
	SessionID string
	Level     Level
	Message   string
	Timestamp time.Time
}

// CheckAndAlert checks the budget status and logs warnings if thresholds are exceeded.
// If the session is over its critical threshold and AutoCompactSignal is enabled,
// writes a signal file that external tooling can watch for auto-compaction.
func CheckAndAlert(status Status, cfg Config, signalDir string) *Alert {
	if status.Level == LevelOK {
		return nil
	}

	alert := &Alert{
		SessionID: status.SessionID,
		Level:     status.Level,
		Timestamp: time.Now(),
	}

	switch status.Level {
	case LevelWarning:
		alert.Message = fmt.Sprintf(
			"session %s: context budget warning — %.1f%% used (threshold: %.0f%% for %s role)",
			status.SessionID, status.PercentageUsed, status.Threshold, status.Role,
		)
		log.Printf("WARNING: %s", alert.Message)

	case LevelCritical:
		alert.Message = fmt.Sprintf(
			"session %s: context budget CRITICAL — %.1f%% used (critical: %.0f%%)",
			status.SessionID, status.PercentageUsed, cfg.CriticalPercent,
		)
		log.Printf("CRITICAL: %s", alert.Message)

		if cfg.AutoCompactSignal && signalDir != "" {
			writeCompactSignal(status.SessionID, signalDir)
		}
	}

	return alert
}

// writeCompactSignal writes a signal file to trigger auto-compaction.
// The file is named "compact-{sessionID}" and contains the timestamp.
func writeCompactSignal(sessionID string, signalDir string) {
	if err := os.MkdirAll(signalDir, 0o700); err != nil {
		log.Printf("budget: failed to create signal dir %s: %v", signalDir, err)
		return
	}

	signalPath := filepath.Join(signalDir, fmt.Sprintf("compact-%s", sessionID))
	content := fmt.Sprintf("compact requested at %s\nusage exceeded critical threshold\n",
		time.Now().Format(time.RFC3339))

	if err := os.WriteFile(signalPath, []byte(content), 0o600); err != nil {
		log.Printf("budget: failed to write compact signal %s: %v", signalPath, err)
	}
}

// SignalDir returns the default signal directory path for budget alerts.
func SignalDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/agm-signals"
	}
	return filepath.Join(home, ".agm", "signals")
}
