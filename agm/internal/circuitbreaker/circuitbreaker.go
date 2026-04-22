// Package circuitbreaker implements deterministic safeguards to prevent CPU
// spikes from too many concurrent sessions. It enforces three gates before
// allowing a new worker session to spawn:
//
//  1. MaxWorkers — hard cap on concurrent worker sessions
//  2. CPULoad — refuses spawn if 5-min load average exceeds threshold
//  3. SpawnStagger — minimum time between consecutive spawns
package circuitbreaker

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// DEARLevel classifies system load for logging/reporting.
type DEARLevel string

// DEAR threshold levels for load classification.
const (
	DEARGreen     DEARLevel = "GREEN"     // load < 40
	DEARYellow    DEARLevel = "YELLOW"    // 40–60
	DEARRed       DEARLevel = "RED"       // 60–100
	DEAREmergency DEARLevel = "EMERGENCY" // > 100
)

// ClassifyLoad returns the DEAR level for a given load average.
func ClassifyLoad(load float64) DEARLevel {
	switch {
	case load > 100:
		return DEAREmergency
	case load > 60:
		return DEARRed
	case load >= 40:
		return DEARYellow
	default:
		return DEARGreen
	}
}

// Config holds circuit breaker thresholds.
type Config struct {
	// MaxWorkers is the hard cap on concurrent worker sessions.
	// Default: 3. Override via AGM_MAX_WORKERS env var.
	MaxWorkers int

	// MaxLoad5 is the 5-minute load average ceiling.
	// A spawn is refused when the current 5-min load exceeds this value.
	// Default: 50.
	MaxLoad5 float64

	// MinSpawnInterval is the minimum duration between consecutive spawns.
	// Default: 2 minutes.
	MinSpawnInterval time.Duration
}

// DefaultConfig returns a Config with production defaults, applying any
// environment-variable overrides.
func DefaultConfig() Config {
	cfg := Config{
		MaxWorkers:       3,
		MaxLoad5:         50,
		MinSpawnInterval: 2 * time.Minute,
	}

	if v := os.Getenv("AGM_MAX_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxWorkers = n
		}
	}

	return cfg
}

// LoadReader provides the 5-minute load average.
type LoadReader interface {
	Load5() (float64, error)
}

// WorkerCounter returns the number of currently active worker sessions.
type WorkerCounter interface {
	CountWorkers() (int, error)
}

// SpawnTimer reads and writes the last-spawn timestamp.
type SpawnTimer interface {
	LastSpawnTime() (time.Time, error)
	RecordSpawn(t time.Time) error
}

// GateResult describes the outcome of a single gate check.
type GateResult struct {
	Gate    string // "max_workers", "cpu_load", "spawn_stagger"
	Passed  bool
	Message string
}

// CheckResult aggregates all gate outcomes.
type CheckResult struct {
	Allowed bool
	Gates   []GateResult
	Load    float64
	Level   DEARLevel
}

// Check evaluates all three gates. It returns CheckResult with Allowed=true
// only if every gate passes. Gates are always all evaluated so the caller
// can report every violation, not just the first.
func Check(cfg Config, lr LoadReader, wc WorkerCounter, st SpawnTimer) CheckResult {
	result := CheckResult{Allowed: true}

	// Gate 1: max workers
	workerGate := checkMaxWorkers(cfg, wc)
	result.Gates = append(result.Gates, workerGate)
	if !workerGate.Passed {
		result.Allowed = false
	}

	// Gate 2: CPU load
	loadGate := checkCPULoad(cfg, lr)
	result.Gates = append(result.Gates, loadGate)
	if !loadGate.Passed {
		result.Allowed = false
	}

	// Gate 3: spawn stagger
	staggerGate := checkSpawnStagger(cfg, st)
	result.Gates = append(result.Gates, staggerGate)
	if !staggerGate.Passed {
		result.Allowed = false
	}

	// Read load for DEAR classification (best-effort)
	if load, err := lr.Load5(); err == nil {
		result.Load = load
		result.Level = ClassifyLoad(load)
	}

	return result
}

func checkMaxWorkers(cfg Config, wc WorkerCounter) GateResult {
	count, err := wc.CountWorkers()
	if err != nil {
		// If we can't count, fail open with a warning
		return GateResult{
			Gate:    "max_workers",
			Passed:  true,
			Message: fmt.Sprintf("could not count workers: %v (failing open)", err),
		}
	}

	if count >= cfg.MaxWorkers {
		return GateResult{
			Gate:   "max_workers",
			Passed: false,
			Message: fmt.Sprintf(
				"worker limit reached: %d/%d active workers. Wait for a session to finish or archive idle sessions with: agm session archive <name>",
				count, cfg.MaxWorkers,
			),
		}
	}

	return GateResult{
		Gate:    "max_workers",
		Passed:  true,
		Message: fmt.Sprintf("workers: %d/%d", count, cfg.MaxWorkers),
	}
}

func checkCPULoad(cfg Config, lr LoadReader) GateResult {
	load, err := lr.Load5()
	if err != nil {
		return GateResult{
			Gate:    "cpu_load",
			Passed:  true,
			Message: fmt.Sprintf("could not read load: %v (failing open)", err),
		}
	}

	if load > cfg.MaxLoad5 {
		level := ClassifyLoad(load)
		return GateResult{
			Gate:   "cpu_load",
			Passed: false,
			Message: fmt.Sprintf(
				"system load too high: %.1f (threshold: %.0f, level: %s). Wait for load to decrease before spawning new sessions.",
				load, cfg.MaxLoad5, level,
			),
		}
	}

	return GateResult{
		Gate:    "cpu_load",
		Passed:  true,
		Message: fmt.Sprintf("load5: %.1f (threshold: %.0f)", load, cfg.MaxLoad5),
	}
}

func checkSpawnStagger(cfg Config, st SpawnTimer) GateResult {
	lastSpawn, err := st.LastSpawnTime()
	if err != nil {
		// No record of last spawn — allow
		return GateResult{
			Gate:    "spawn_stagger",
			Passed:  true,
			Message: "no previous spawn recorded",
		}
	}

	elapsed := time.Since(lastSpawn)
	if elapsed < cfg.MinSpawnInterval {
		remaining := cfg.MinSpawnInterval - elapsed
		return GateResult{
			Gate:   "spawn_stagger",
			Passed: false,
			Message: fmt.Sprintf(
				"spawn too soon: last spawn was %s ago (minimum interval: %s). Wait %s before spawning another session.",
				formatDuration(elapsed), formatDuration(cfg.MinSpawnInterval), formatDuration(remaining),
			),
		}
	}

	return GateResult{
		Gate:    "spawn_stagger",
		Passed:  true,
		Message: fmt.Sprintf("last spawn: %s ago", formatDuration(elapsed)),
	}
}

// formatDuration produces a human-friendly duration string.
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return "0s"
	}
	d = d.Truncate(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	mins := int(d.Minutes())
	secs := int(d.Seconds()) % 60
	if secs == 0 {
		return fmt.Sprintf("%dm", mins)
	}
	return fmt.Sprintf("%dm%ds", mins, secs)
}

// FormatDenied produces a user-friendly error message when spawn is refused.
func FormatDenied(cr CheckResult) string {
	var reasons []string
	for _, g := range cr.Gates {
		if !g.Passed {
			reasons = append(reasons, fmt.Sprintf("  • [%s] %s", g.Gate, g.Message))
		}
	}
	header := fmt.Sprintf("circuit breaker: spawn refused (load level: %s)", cr.Level)
	return header + "\n\n" + strings.Join(reasons, "\n")
}
