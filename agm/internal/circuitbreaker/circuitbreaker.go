// Package circuitbreaker implements deterministic safeguards to prevent CPU,
// memory, and disk exhaustion from too many concurrent sessions. It enforces
// up to seven gates before allowing a new worker session to spawn:
//
//  1. MaxWorkers — optional hard cap on concurrent worker sessions (disabled
//     when MaxWorkers <= 0, which is the default; workers are then bounded
//     only by the gates below)
//  2. CPULoad — refuses spawn if 5-min load average exceeds threshold
//     (default: 2×NumCPU; override via AGM_MAX_LOAD5). FAILS CLOSED.
//  3. Memory — refuses spawn if free RAM falls below threshold
//     (default: 10%; override via AGM_MIN_FREE_MEM_PCT). FAILS CLOSED when a
//     reader is wired.
//  4. SpawnStagger — minimum time between consecutive spawns
//  5. Disk — refuses spawn if free disk falls below an absolute reserve
//     (default: 15 GiB; override via AGM_MIN_FREE_DISK_GB). FAILS CLOSED.
//  6. AgentProcs — refuses spawn if the machine-wide count of agent harness
//     processes (claude/codex/agm workers, counted from the live process
//     table, not AGM's session records) reaches the cap (default: NumCPU;
//     override via AGM_MAX_AGENT_PROCS). FAILS CLOSED.
//  7. AdmissionBrake — refuses spawn while a host watchdog has engaged the
//     cross-process brake (pkg/vroom/admission). FAILS CLOSED.
//
// Gates 5, 6, and 7 exist because an ungated mesh saturated the host and forced
// a hard reboot (ce-93lw.18): the old governor only advised, only counted AGM
// sessions, never gated on disk, and had no way to hear that disk-watchdog's
// own remediation was being SIGKILLed. Gates 5–7 run only when their readers
// are wired in (production wires all three).
//
// # Fail-closed semantics
//
// The distinction that keeps fail-closed safe is *not wired* versus *wired and
// broken*. A reader that is absent — a nil MemReader on non-Darwin, or no brake
// reader supplied — means the gate was never asked, and it is skipped. A reader
// that is present and returns an error means we asked a question about a host
// whose failure mode is "probes stop answering" and got no answer, so the spawn
// is refused.
//
// AGM_ADMISSION_ALLOW_UNVERIFIED=1 restores fail-open behaviour for load and
// memory *probe errors only*. It exists because a fail-closed gate with no
// manual override is an outage generator: the first time sysctl is missing on a
// box, nothing can spawn and there is no documented way out. It deliberately
// cannot pass a real threshold breach, and cannot pass the disk, agent-process,
// or admission-brake gates — those are the ce-93lw.18 gates whose entire
// purpose is to be unconditional.
//
// The MaxWorkers gate was previously hard-coded to 3. It is now disabled by
// default (0 = no cap) to support dynamic multi-provider worker fleets where
// different workers run on different model families simultaneously. Set
// AGM_MAX_WORKERS to a positive integer to restore a hard cap.
package circuitbreaker

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// allowUnverifiedEnv opts out of fail-closed handling for load and memory probe
// errors. It is read per-check rather than cached so an operator can set it for
// a single command.
const allowUnverifiedEnv = "AGM_ADMISSION_ALLOW_UNVERIFIED"

// allowUnverified reports whether the operator has explicitly accepted spawning
// without a verified load/memory reading.
func allowUnverified() bool {
	switch os.Getenv(allowUnverifiedEnv) {
	case "1", "true", "TRUE", "yes":
		return true
	default:
		return false
	}
}

// unverifiedProbeResult builds the GateResult for a probe that could not be
// read. It refuses by default and passes only under the documented override,
// which is always named in the message so it cannot be set and forgotten.
func unverifiedProbeResult(gate, what string, err error) GateResult {
	if allowUnverified() {
		return GateResult{
			Gate:   gate,
			Passed: true,
			Message: fmt.Sprintf(
				"could not read %s: %v (failing open — %s is set)",
				what, err, allowUnverifiedEnv,
			),
		}
	}
	return GateResult{
		Gate:   gate,
		Passed: false,
		Message: fmt.Sprintf(
			"could not read %s: %v (failing closed — refusing spawn; an unreadable probe is itself a saturation signal. Set %s=1 to spawn without a verified reading.)",
			what, err, allowUnverifiedEnv,
		),
	}
}

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
	// MaxWorkers is an optional hard cap on concurrent worker sessions.
	// When <= 0 (the default) the gate is disabled and worker count is
	// bounded only by CPULoad and SpawnStagger. Set to a positive integer
	// to restore a hard limit (e.g. for resource-constrained laptops).
	// Override via AGM_MAX_WORKERS env var (0 = disable).
	MaxWorkers int

	// MaxLoad5 is the 5-minute load average ceiling before blocking spawns.
	// Default: 2×NumCPU (scales with the machine; override via AGM_MAX_LOAD5).
	MaxLoad5 float64

	// MinFreeMemPct is the minimum percentage of free RAM required before
	// a spawn is allowed. Default: 10 (i.e. 10% free RAM).
	// Override via AGM_MIN_FREE_MEM_PCT env var.
	MinFreeMemPct float64

	// MinSpawnInterval is the minimum duration between consecutive spawns.
	// Default: 2 minutes.
	MinSpawnInterval time.Duration

	// MinFreeDiskGB is the minimum free disk space, in gigabytes, required on
	// the volume backing agent working directories (worktrees, sandboxes)
	// before a spawn is allowed. Default: 15. Override via
	// AGM_MIN_FREE_DISK_GB env var.
	//
	// Honest accounting on the 15 GiB figure: the host that forced the
	// 2026-07-18 reboot still had 17.6 GiB free when it wedged, so this floor
	// would not have fired during that incident. It is defence in depth against
	// an ordinary fill; the admission brake (gate 7) is what catches the
	// remediation-is-dying shape of ce-93lw.18. Do not read the floor as the fix.
	//
	// This is an absolute reserve, not a percentage of total disk. A
	// percentage floor scales with disk size and gets punitive on large
	// disks: 8% of a 460GB disk is ~37GB, which refused spawns at 23GB free
	// even though that's ample headroom for a worktree checkout plus build
	// cache. An absolute GB floor stays meaningful regardless of total disk
	// size.
	//
	// Unlike the load and memory gates, the disk gate FAILS CLOSED: if free
	// disk cannot be determined the spawn is refused, because an unbounded
	// disk fill is what took the host down (ce-93lw.18) and a blind spawn is
	// the failure mode that must never recur.
	MinFreeDiskGB float64

	// MaxAgentProcs caps the number of agent harness processes (the `claude`
	// and `codex` CLIs and agm-spawned workers) running machine-wide, counted
	// from the live process table rather than AGM's session records. This
	// closes the gap where Dispatch-spawned sessions were invisible to a gate
	// that only counted AGM's own sessions (ce-93lw.18).
	//
	// Default: NumCPU. When <= 0 the gate is disabled. Override via
	// AGM_MAX_AGENT_PROCS. Like the disk gate, it FAILS CLOSED: if the process
	// count cannot be determined the spawn is refused.
	MaxAgentProcs int
}

// DefaultConfig returns a Config with production defaults, applying any
// environment-variable overrides.
//
// The MaxWorkers default is 0 (disabled). Worker count is bounded dynamically
// by CPU load, memory, and spawn stagger. Set AGM_MAX_WORKERS to a positive
// integer to restore the old hard cap of 3 (or any other limit).
//
// MaxLoad5 defaults to 2×NumCPU so the CPU gate fires at meaningful
// oversubscription rather than a fixed value of 50 that never fires on a
// 12-core box.
func DefaultConfig() Config {
	cfg := Config{
		MaxWorkers:       0, // dynamic: no hard cap by default
		MaxLoad5:         float64(runtime.NumCPU()) * 2,
		MinFreeMemPct:    10,
		MinSpawnInterval: 2 * time.Minute,
		MinFreeDiskGB:    15,
		MaxAgentProcs:    runtime.NumCPU(),
	}

	if v := os.Getenv("AGM_MAX_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			cfg.MaxWorkers = n
		}
	}

	if v := os.Getenv("AGM_MAX_LOAD5"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			cfg.MaxLoad5 = f
		}
	}

	if v := os.Getenv("AGM_MIN_FREE_MEM_PCT"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			cfg.MinFreeMemPct = f
		}
	}

	if v := os.Getenv("AGM_MIN_FREE_DISK_GB"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			cfg.MinFreeDiskGB = f
		}
	}

	if v := os.Getenv("AGM_MAX_AGENT_PROCS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			cfg.MaxAgentProcs = n
		}
	}

	return cfg
}

// LoadReader provides the 5-minute load average.
type LoadReader interface {
	Load5() (float64, error)
}

// MemReader provides the percentage of free RAM (0–100).
type MemReader interface {
	FreeMemPct() (float64, error)
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

// DiskReader provides the free disk space, in gigabytes, on the volume
// backing agent working directories.
type DiskReader interface {
	FreeDiskGB() (float64, error)
}

// ProcCounter counts agent harness processes (claude/codex/agm workers)
// running machine-wide, read from the live process table.
type ProcCounter interface {
	CountAgentProcs() (int, error)
}

// GateResult describes the outcome of a single gate check.
type GateResult struct {
	Gate             string // "max_workers", "cpu_load", "memory", "spawn_stagger", "disk", "agent_procs", "admission_brake"
	Passed           bool
	RequiresOverride bool // true only when this exact refusal may be crossed by its scoped override
	Message          string
}

// CheckResult aggregates all gate outcomes.
type CheckResult struct {
	Allowed bool
	Gates   []GateResult
	Load    float64
	Level   DEARLevel
}

// checkOptions carries the optional disk, process, and brake readers injected
// via CheckOption. Keeping them optional preserves Check's original signature so
// existing callers and tests compile unchanged; production wires all three
// through WithDiskReader / WithProcCounter / WithBrakeReader.
type checkOptions struct {
	dr DiskReader
	pc ProcCounter
	br BrakeReader
}

// CheckOption injects an optional admission-control reader into Check.
type CheckOption func(*checkOptions)

// WithDiskReader enables the disk-headroom gate using dr.
func WithDiskReader(dr DiskReader) CheckOption {
	return func(o *checkOptions) { o.dr = dr }
}

// WithProcCounter enables the machine-wide agent-process gate using pc.
func WithProcCounter(pc ProcCounter) CheckOption {
	return func(o *checkOptions) { o.pc = pc }
}

// WithBrakeReader enables the admission-brake gate using br.
func WithBrakeReader(br BrakeReader) CheckOption {
	return func(o *checkOptions) { o.br = br }
}

// Check evaluates all gates. It returns CheckResult with Allowed=true only if
// every gate passes. Gates are always all evaluated so the caller can report
// every violation, not just the first.
//
// mr may be nil; when nil the memory gate is skipped, because a reader that was
// never wired asked no question and so cannot have failed. The disk,
// agent-process, and admission-brake gates run only when their readers are
// supplied via WithDiskReader / WithProcCounter / WithBrakeReader; when supplied
// they FAIL CLOSED on read error (they refuse the spawn), because they guard
// against the unbounded resource fill that took the host down (ce-93lw.18).
//
// The load gate, and the memory gate when a reader is wired, also fail closed
// on read error — see the package doc on AGM_ADMISSION_ALLOW_UNVERIFIED for the
// one documented way to opt out of that.
func Check(cfg Config, lr LoadReader, wc WorkerCounter, st SpawnTimer, mr MemReader, opts ...CheckOption) CheckResult {
	var o checkOptions
	for _, opt := range opts {
		opt(&o)
	}

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

	// Gate 3: memory
	memGate := checkMemory(cfg, mr)
	result.Gates = append(result.Gates, memGate)
	if !memGate.Passed {
		result.Allowed = false
	}

	// Gate 4: spawn stagger
	staggerGate := checkSpawnStagger(cfg, st)
	result.Gates = append(result.Gates, staggerGate)
	if !staggerGate.Passed {
		result.Allowed = false
	}

	// Gate 5: disk headroom (fails closed; runs only when a reader is wired)
	if o.dr != nil {
		diskGate := checkDisk(cfg, o.dr)
		result.Gates = append(result.Gates, diskGate)
		if !diskGate.Passed {
			result.Allowed = false
		}
	}

	// Gate 6: machine-wide agent processes (fails closed; runs only when wired)
	if o.pc != nil {
		procGate := checkAgentProcs(cfg, o.pc)
		result.Gates = append(result.Gates, procGate)
		if !procGate.Passed {
			result.Allowed = false
		}
	}

	// Gate 7: watchdog admission brake (fails closed; runs only when wired)
	if o.br != nil {
		brakeGate := checkAdmissionBrake(o.br)
		result.Gates = append(result.Gates, brakeGate)
		if !brakeGate.Passed {
			result.Allowed = false
		}
	}

	// Read load for DEAR classification (best-effort)
	if load, err := lr.Load5(); err == nil {
		result.Load = load
		result.Level = ClassifyLoad(load)
	}

	return result
}

func checkMaxWorkers(cfg Config, wc WorkerCounter) GateResult {
	// MaxWorkers <= 0 means dynamic allocation — gate is disabled. Worker
	// count is bounded by the CPULoad and SpawnStagger gates instead.
	if cfg.MaxWorkers <= 0 {
		count, _ := wc.CountWorkers() // best-effort for the message
		return GateResult{
			Gate:    "max_workers",
			Passed:  true,
			Message: fmt.Sprintf("workers: %d (no hard cap — dynamic allocation enabled)", count),
		}
	}

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
		return unverifiedProbeResult("cpu_load", "load", err)
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

func checkMemory(cfg Config, mr MemReader) GateResult {
	// No reader wired (non-Darwin) is not the same as a reader that failed:
	// nothing was asked, so nothing can have gone unanswered.
	if mr == nil {
		return GateResult{
			Gate:    "memory",
			Passed:  true,
			Message: "memory reader not wired on this platform (gate skipped)",
		}
	}

	freePct, err := mr.FreeMemPct()
	if err != nil {
		return unverifiedProbeResult("memory", "free memory", err)
	}

	if freePct < cfg.MinFreeMemPct {
		return GateResult{
			Gate:   "memory",
			Passed: false,
			Message: fmt.Sprintf(
				"free RAM too low: %.1f%% (minimum: %.0f%%). Free memory before spawning new sessions.",
				freePct, cfg.MinFreeMemPct,
			),
		}
	}

	return GateResult{
		Gate:    "memory",
		Passed:  true,
		Message: fmt.Sprintf("free RAM: %.1f%% (minimum: %.0f%%)", freePct, cfg.MinFreeMemPct),
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

	now := time.Now()
	if lastSpawn.After(now) {
		// A governor timestamp is not a successful spawn, but the shared timer
		// cannot encode provenance after the timestamp passes. The ordinary
		// stagger therefore remains in force for MinSpawnInterval after the
		// governor hold. Report that effective admission boundary so operators
		// are not told to retry before this gate can actually pass.
		resumeAt := lastSpawn.Add(cfg.MinSpawnInterval)
		remaining := resumeAt.Sub(now)
		return GateResult{
			Gate:   "spawn_stagger",
			Passed: false,
			Message: fmt.Sprintf(
				"spawns paused by resource governor; earliest possible admission is %s (%s until that boundary), if the governor does not extend the hold and all other gates pass. This boundary includes the governor hold and %s spawn safety interval.",
				resumeAt.Format(time.RFC3339), formatDuration(remaining), formatDuration(cfg.MinSpawnInterval),
			),
		}
	}

	elapsed := now.Sub(lastSpawn)
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

// checkDisk refuses a spawn when free disk on the agent working volume is
// below the floor. It FAILS CLOSED: a read error refuses the spawn, because an
// unbounded disk fill is the failure that took the host down (ce-93lw.18).
func checkDisk(cfg Config, dr DiskReader) GateResult {
	freeGB, err := dr.FreeDiskGB()
	if err != nil {
		return GateResult{
			Gate:    "disk",
			Passed:  false,
			Message: fmt.Sprintf("could not read free disk: %v (failing closed — refusing spawn)", err),
		}
	}

	if freeGB < cfg.MinFreeDiskGB {
		return GateResult{
			Gate:   "disk",
			Passed: false,
			Message: fmt.Sprintf(
				"free disk too low: %.1f GB (minimum: %.1f GB). Reclaim disk (merged worktrees, go-build cache, dead sandboxes) before spawning new sessions.",
				freeGB, cfg.MinFreeDiskGB,
			),
		}
	}

	return GateResult{
		Gate:    "disk",
		Passed:  true,
		Message: fmt.Sprintf("free disk: %.1f GB (minimum: %.1f GB)", freeGB, cfg.MinFreeDiskGB),
	}
}

// checkAgentProcs refuses a spawn when the machine-wide count of agent harness
// processes has reached the cap. Counting real PIDs (not AGM's session table)
// makes Dispatch-spawned workers visible to the gate. It FAILS CLOSED: a count
// error refuses the spawn.
func checkAgentProcs(cfg Config, pc ProcCounter) GateResult {
	if cfg.MaxAgentProcs <= 0 {
		return GateResult{
			Gate:    "agent_procs",
			Passed:  true,
			Message: "agent-process cap disabled (MaxAgentProcs <= 0)",
		}
	}

	count, err := pc.CountAgentProcs()
	if err != nil {
		return GateResult{
			Gate:    "agent_procs",
			Passed:  false,
			Message: fmt.Sprintf("could not count agent processes: %v (failing closed — refusing spawn)", err),
		}
	}

	if count >= cfg.MaxAgentProcs {
		return GateResult{
			Gate:   "agent_procs",
			Passed: false,
			Message: fmt.Sprintf(
				"agent-process cap reached: %d/%d machine-wide agent processes. Wait for workers to exit before spawning new sessions.",
				count, cfg.MaxAgentProcs,
			),
		}
	}

	return GateResult{
		Gate:    "agent_procs",
		Passed:  true,
		Message: fmt.Sprintf("agent processes: %d/%d", count, cfg.MaxAgentProcs),
	}
}

// checkAdmissionBrake refuses a spawn while a host watchdog has engaged the
// cross-process brake. This is the gate that would have fired on 2026-07-18:
// disk-watchdog's `agm worktree sweep --execute` was returning `signal: killed`
// every tick, and nothing turned that into a refusal.
//
// It FAILS CLOSED in both directions that matter — an engaged brake refuses,
// and an unreadable brake refuses — and it is not subject to
// AGM_ADMISSION_ALLOW_UNVERIFIED, because a latch an operator cannot read is
// cleared by deleting the file, not by overriding the gate that reads it.
func checkAdmissionBrake(br BrakeReader) GateResult {
	brake, err := br.Brake()
	if err != nil {
		return GateResult{
			Gate:   "admission_brake",
			Passed: false,
			Message: fmt.Sprintf(
				"could not read the admission brake: %v (failing closed — refusing spawn). Inspect or delete %s to clear it.",
				err, brakeLocation(br),
			),
		}
	}

	if brake == nil {
		return GateResult{
			Gate:    "admission_brake",
			Passed:  true,
			Message: "no admission brake engaged",
		}
	}

	return GateResult{
		Gate:             "admission_brake",
		Passed:           false,
		RequiresOverride: true,
		Message: fmt.Sprintf(
			"admission brake engaged by %s: %s (set %s ago, expires %s). Spawns resume when the watchdog sees a healthy host; cross it once with an audited override (agm override approve admission-brake, then --brake-override=\"<reason>\"), or delete %s to clear it.",
			brake.Source,
			brake.Reason,
			formatDuration(brake.Age(time.Now().UTC())),
			brake.ExpiresUTC.Format(time.RFC3339),
			brakeLocation(br),
		),
	}
}

// brakeLocation names the brake file when the reader can report it, so the
// refusal tells an operator exactly what to delete.
func brakeLocation(br BrakeReader) string {
	if namer, ok := br.(brakePathNamer); ok {
		return namer.BrakePath()
	}
	return "the admission brake file"
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
	loadLevel := cr.Level
	if loadLevel == "" {
		loadLevel = "unknown"
	}
	header := fmt.Sprintf("circuit breaker: spawn refused (load level: %s)", loadLevel)
	return header + "\n\n" + strings.Join(reasons, "\n")
}
