// Absence-alarm cross-watch: reading the absence-alarm heartbeat file to decide
// whether the independent monitoring scheduler is still completing ticks (ce-x2h49).
//
// This is the watch-the-watcher closure (AA-16 consumer, answering ce-2a3ma):
// absence-alarm pulses on disk-watchdog's output log, and disk-watchdog closes the
// loop by checking that absence-alarm's heartbeat records a positive tick_time
// within the configured window (DW-48..DW-51).

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	defaultAbsenceMaxAge      = 30 * time.Minute
	absenceClockSkewTolerance = 5 * time.Minute
)

// absenceHealth represents what a tick concluded about the absence-alarm scheduler.
type absenceHealth struct {
	Stale     bool
	TickTime  time.Time
	Age       time.Duration
	AgeString string
	Reason    string
}

func defaultAbsenceHeartbeatPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "state", "dear-agent", "absence-alarm.heartbeat.json")
}

// checkAbsenceAlarmHealth classifies absence-alarm scheduler liveness from its heartbeat file.
// A missing, unreadable, corrupt, future-skewed, or over-age heartbeat is classified as stale.
func checkAbsenceAlarmHealth(cfg config, now time.Time) *absenceHealth {
	if cfg.absenceHeartbeatPath == "" || cfg.absenceMaxAge <= 0 {
		return nil
	}

	data, err := os.ReadFile(cfg.absenceHeartbeatPath)
	if err != nil {
		return &absenceHealth{
			Stale:  true,
			Reason: fmt.Sprintf("absence-alarm heartbeat %s is unreadable (%v); scheduler liveness cannot be confirmed", cfg.absenceHeartbeatPath, err),
		}
	}

	var hb struct {
		TickTime string `json:"tick_time"`
	}
	if err := json.Unmarshal(data, &hb); err != nil {
		return &absenceHealth{
			Stale:  true,
			Reason: fmt.Sprintf("absence-alarm heartbeat %s is invalid JSON (%v); scheduler liveness cannot be confirmed", cfg.absenceHeartbeatPath, err),
		}
	}
	if hb.TickTime == "" {
		return &absenceHealth{
			Stale:  true,
			Reason: fmt.Sprintf("absence-alarm heartbeat %s contains no tick_time; scheduler liveness cannot be confirmed", cfg.absenceHeartbeatPath),
		}
	}

	tickTime, err := time.Parse(time.RFC3339Nano, hb.TickTime)
	if err != nil {
		tickTime, err = time.Parse(time.RFC3339, hb.TickTime)
	}
	if err != nil {
		return &absenceHealth{
			Stale:  true,
			Reason: fmt.Sprintf("absence-alarm heartbeat %s has invalid tick_time %q (%v); scheduler liveness cannot be confirmed", cfg.absenceHeartbeatPath, hb.TickTime, err),
		}
	}

	// Clock skew: a heartbeat timestamped in the future beyond tolerance (5m) is an alarm.
	if tickTime.After(now.Add(absenceClockSkewTolerance)) {
		return &absenceHealth{
			Stale:    true,
			TickTime: tickTime,
			Reason:   fmt.Sprintf("absence-alarm heartbeat %s timestamp %s is in the future (>5m clock skew)", cfg.absenceHeartbeatPath, hb.TickTime),
		}
	}

	age := max(0, now.Sub(tickTime))
	ageStr := age.Round(time.Second).String()
	if age > cfg.absenceMaxAge {
		return &absenceHealth{
			Stale:     true,
			TickTime:  tickTime,
			Age:       age,
			AgeString: ageStr,
			Reason:    fmt.Sprintf("absence-alarm is stale: last tick %s ago (window %s)", ageStr, cfg.absenceMaxAge),
		}
	}

	return &absenceHealth{
		Stale:     false,
		TickTime:  tickTime,
		Age:       age,
		AgeString: ageStr,
	}
}
