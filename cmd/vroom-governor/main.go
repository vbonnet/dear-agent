// Command vroom-governor monitors system load and memory on a fixed interval
// and pauses/resumes AGM worker spawns accordingly. On critical memory
// pressure it archives the newest active worker session to free resources.
//
// Usage:
//
//	vroom-governor [--interval 30s] [--max-load-ratio 0.9] \
//	               [--min-free-mem-pct 10] [--min-free-mem-pct-critical 5]
//
// Actions per tick:
//
//	load > max-load-ratio × NumCPU OR memfree < min-free-mem-pct:
//	    Extend ~/.agm/last-spawn.txt by one interval to pause new spawns.
//	    Earliest admission is the timestamp plus AGM's spawn-safety interval,
//	    provided the governor stops extending the hold and all other gates pass.
//
//	memfree < min-free-mem-pct-critical:
//	    Archive the newest active worker session to reclaim memory.
//	    Uses: agm session list --json --filter role:worker
//	    Then: agm session archive <name> --async
//
//	load or memory probe returns an error:
//	    Engage the cross-process admission brake (pkg/vroom/admission) so every
//	    spawn path refuses new work. A probe that cannot answer is itself a
//	    saturation signal (ce-93lw.18): these readings were previously discarded
//	    with `err == nil &&`, so a governor that had gone blind looked exactly
//	    like a governor reporting a healthy host.
//
//	    Threshold breaches keep using the last-spawn.txt pause. That path works
//	    and vroom-dispatch's stagger retry is the right response to it; only the
//	    unreadable case escalates to the brake, because that is the case where we
//	    do not know what we are waiting for.
//
//	Otherwise: release the brake and stop extending last-spawn.txt. The existing
//	hold and AGM's spawn-safety interval still elapse before other gates may
//	admit a spawn.
//
// Exits cleanly on SIGTERM/SIGINT.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/pkg/vroom/admission"
)

func main() {
	interval := flag.Duration("interval", 30*time.Second, "polling interval between system checks")
	maxLoadRatio := flag.Float64("max-load-ratio", 0.9, "fraction of NumCPU load avg that triggers spawn pause")
	minFreeMemPct := flag.Float64("min-free-mem-pct", 10, "free RAM % threshold: below this, pause spawns")
	minFreeMemPctCritical := flag.Float64("min-free-mem-pct-critical", 5, "free RAM % threshold: below this, archive newest worker")
	brakePath := flag.String("brake", admission.DefaultPath(),
		"admission-brake path; engaged when a probe cannot be read, released on a clean tick")
	brakeTTL := flag.Duration("brake-ttl", admission.DefaultTTL,
		"how long an engaged admission brake blocks spawns before expiring on its own")
	flag.Parse()

	log.SetFlags(log.Ldate | log.Ltime)
	log.SetOutput(os.Stderr)

	maxLoad := *maxLoadRatio * float64(runtime.NumCPU())
	log.Printf("vroom-governor starting: interval=%s max-load=%.1f min-free-mem=%.0f%% critical-mem=%.0f%%",
		*interval, maxLoad, *minFreeMemPct, *minFreeMemPctCritical)

	cfg := tickConfig{
		maxLoad:               maxLoad,
		minFreeMemPct:         *minFreeMemPct,
		minFreeMemPctCritical: *minFreeMemPctCritical,
		interval:              *interval,
		spawnFile:             spawnFilePath(),
		brakePath:             *brakePath,
		brakeTTL:              *brakeTTL,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	tick(cfg)

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tick(cfg)
		case <-ctx.Done():
			log.Println("vroom-governor: received signal, exiting")
			return
		}
	}
}

// tickConfig carries one tick's thresholds and file locations.
type tickConfig struct {
	maxLoad               float64
	minFreeMemPct         float64
	minFreeMemPctCritical float64
	interval              time.Duration
	spawnFile             string
	brakePath             string
	brakeTTL              time.Duration
}

// brakeSource identifies this governor in the admission-brake record.
const brakeSource = "vroom-governor"

func tick(cfg tickConfig) {
	load, loadErr := readLoad5()
	memPct, memErr := readFreeMemPct()

	logParts := []string{}
	if loadErr == nil {
		logParts = append(logParts, fmt.Sprintf("load5=%.2f", load))
	} else {
		logParts = append(logParts, fmt.Sprintf("load5=err(%v)", loadErr))
	}
	if memErr == nil {
		logParts = append(logParts, fmt.Sprintf("freemem=%.1f%%", memPct))
	} else {
		logParts = append(logParts, fmt.Sprintf("freemem=err(%v)", memErr))
	}
	log.Printf("tick: %s", strings.Join(logParts, " "))

	loadHigh := loadErr == nil && load > cfg.maxLoad
	memLow := memErr == nil && memPct < cfg.minFreeMemPct
	memCritical := memErr == nil && memPct < cfg.minFreeMemPctCritical

	if memCritical {
		log.Printf("CRITICAL: free RAM %.1f%% < %.0f%% — archiving newest worker session", memPct, cfg.minFreeMemPctCritical)
		if err := archiveNewestWorker(); err != nil {
			log.Printf("archive newest worker: %v", err)
		}
	}

	if loadHigh || memLow {
		reason := buildReason(loadHigh, load, cfg.maxLoad, memLow, memPct, cfg.minFreeMemPct)
		log.Printf("pausing spawns (%s)", reason)
		if err := pauseSpawns(cfg.spawnFile, cfg.interval); err != nil {
			log.Printf("pause spawns: %v", err)
		}
	}

	applyBrake(cfg, unreadableProbeReason(loadErr, memErr), loadHigh || memLow)
}

// unreadableProbeReason returns the brake reason when either probe failed, or
// "" when both read cleanly.
//
// Before ce-93lw.18 these errors were dropped by the `err == nil &&` guards
// above, which made a governor that had gone blind indistinguishable from one
// reporting a healthy host. An unreadable probe is now a refusal signal.
func unreadableProbeReason(loadErr, memErr error) string {
	switch {
	case loadErr != nil && memErr != nil:
		return fmt.Sprintf("load and memory probes both unreadable: load5: %v; freemem: %v", loadErr, memErr)
	case loadErr != nil:
		return fmt.Sprintf("load probe unreadable: %v", loadErr)
	case memErr != nil:
		return fmt.Sprintf("memory probe unreadable: %v", memErr)
	default:
		return ""
	}
}

// applyBrake latches the admission brake when a probe could not be read, and
// releases it on a clean tick that is also within thresholds.
//
// A clean reading that breaches a threshold neither engages nor releases: the
// last-spawn.txt pause above already covers it, and clearing the brake there
// would let this governor overrule a brake disk-watchdog engaged for an
// unrelated reason.
func applyBrake(cfg tickConfig, reason string, thresholdBreached bool) {
	if cfg.brakePath == "" {
		return
	}
	if reason != "" {
		log.Printf("engaging admission brake (%s)", reason)
		if err := admission.Engage(cfg.brakePath, brakeSource, reason, cfg.brakeTTL); err != nil {
			log.Printf("engage admission brake: %v", err)
		}
		return
	}
	if thresholdBreached {
		return
	}
	// Scoped to this source. This governor ticks every 30 seconds while
	// disk-watchdog ticks every 5 minutes, so an unconditional release here
	// would clear a disk brake almost as fast as the watchdog could set one --
	// and a host that is out of disk but not out of CPU is the likeliest shape
	// of the failure this gate exists for.
	if err := admission.ReleaseBySource(cfg.brakePath, brakeSource); err != nil {
		log.Printf("release admission brake: %v", err)
	}
}

func buildReason(loadHigh bool, load, maxLoad float64, memLow bool, memPct, minFree float64) string {
	var parts []string
	if loadHigh {
		parts = append(parts, fmt.Sprintf("load %.2f > %.1f", load, maxLoad))
	}
	if memLow {
		parts = append(parts, fmt.Sprintf("free RAM %.1f%% < %.0f%%", memPct, minFree))
	}
	return strings.Join(parts, ", ")
}

// pauseSpawns writes a future timestamp to last-spawn.txt. The stagger gate
// refuses spawns through that hold and its configured post-hold safety interval.
func pauseSpawns(spawnFile string, pauseDuration time.Duration) error {
	future := time.Now().Add(pauseDuration)
	dir := filepath.Dir(spawnFile)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	return os.WriteFile(spawnFile, []byte(future.Format(time.RFC3339)+"\n"), 0o600)
}

// archiveNewestWorker finds the most-recently-updated active worker session
// and archives it with --async.
func archiveNewestWorker() error {
	sessions, err := listWorkerSessions()
	if err != nil {
		return fmt.Errorf("list worker sessions: %w", err)
	}
	if len(sessions) == 0 {
		log.Println("no active worker sessions to archive")
		return nil
	}
	newest := sessions[0]
	log.Printf("archiving worker session %q (updated: %s)", newest.Name, newest.UpdatedAt)
	out, err := exec.Command("agm", "session", "archive", newest.Name, "--async").CombinedOutput()
	if err != nil {
		return fmt.Errorf("agm session archive %s: %w (output: %s)", newest.Name, err, strings.TrimSpace(string(out)))
	}
	log.Printf("archived %q: %s", newest.Name, strings.TrimSpace(string(out)))
	return nil
}

type sessionSummary struct {
	Name      string   `json:"name"`
	Status    string   `json:"status"`
	Tags      []string `json:"tags"`
	UpdatedAt string   `json:"updated_at"`
}

type listResult struct {
	Sessions []sessionSummary `json:"sessions"`
}

// listWorkerSessions returns active sessions tagged role:worker, sorted by
// UpdatedAt descending (newest first).
func listWorkerSessions() ([]sessionSummary, error) {
	out, err := exec.Command("agm", "session", "list", "--json", "--filter", "role:worker").Output()
	if err != nil {
		return nil, fmt.Errorf("agm session list: %w", err)
	}
	var result listResult
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("parse session list: %w", err)
	}
	// Keep only non-archived, non-offline sessions.
	var active []sessionSummary
	for _, s := range result.Sessions {
		lower := strings.ToLower(s.Status)
		if lower != "archived" && lower != "offline" {
			active = append(active, s)
		}
	}
	// Sort newest first (lexicographic on RFC3339 is correct).
	sort.Slice(active, func(i, j int) bool {
		return active[i].UpdatedAt > active[j].UpdatedAt
	})
	return active, nil
}

// spawnFilePath returns the path to ~/.agm/last-spawn.txt, matching the
// FileSpawnTimer in agm/internal/circuitbreaker/providers.go.
func spawnFilePath() string {
	dir := os.Getenv("AGM_CONFIG_DIR")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".agm")
	}
	return filepath.Join(dir, "last-spawn.txt")
}
