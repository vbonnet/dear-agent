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
//	    Spawns resume automatically when the timestamp passes.
//
//	memfree < min-free-mem-pct-critical:
//	    Archive the newest active worker session to reclaim memory.
//	    Uses: agm session list --json --filter role:worker
//	    Then: agm session archive <name> --async
//
//	Otherwise: do nothing (spawns self-resume when last-spawn.txt expires).
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
)

func main() {
	interval := flag.Duration("interval", 30*time.Second, "polling interval between system checks")
	maxLoadRatio := flag.Float64("max-load-ratio", 0.9, "fraction of NumCPU load avg that triggers spawn pause")
	minFreeMemPct := flag.Float64("min-free-mem-pct", 10, "free RAM % threshold: below this, pause spawns")
	minFreeMemPctCritical := flag.Float64("min-free-mem-pct-critical", 5, "free RAM % threshold: below this, archive newest worker")
	flag.Parse()

	log.SetFlags(log.Ldate | log.Ltime)
	log.SetOutput(os.Stderr)

	maxLoad := *maxLoadRatio * float64(runtime.NumCPU())
	log.Printf("vroom-governor starting: interval=%s max-load=%.1f min-free-mem=%.0f%% critical-mem=%.0f%%",
		*interval, maxLoad, *minFreeMemPct, *minFreeMemPctCritical)

	spawnFile := spawnFilePath()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	tick(maxLoad, *minFreeMemPct, *minFreeMemPctCritical, *interval, spawnFile)

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tick(maxLoad, *minFreeMemPct, *minFreeMemPctCritical, *interval, spawnFile)
		case <-ctx.Done():
			log.Println("vroom-governor: received signal, exiting")
			return
		}
	}
}

func tick(maxLoad, minFreeMemPct, minFreeMemPctCritical float64, interval time.Duration, spawnFile string) {
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

	loadHigh := loadErr == nil && load > maxLoad
	memLow := memErr == nil && memPct < minFreeMemPct
	memCritical := memErr == nil && memPct < minFreeMemPctCritical

	if memCritical {
		log.Printf("CRITICAL: free RAM %.1f%% < %.0f%% — archiving newest worker session", memPct, minFreeMemPctCritical)
		if err := archiveNewestWorker(); err != nil {
			log.Printf("archive newest worker: %v", err)
		}
	}

	if loadHigh || memLow {
		reason := buildReason(loadHigh, load, maxLoad, memLow, memPct, minFreeMemPct)
		log.Printf("pausing spawns (%s)", reason)
		if err := pauseSpawns(spawnFile, interval); err != nil {
			log.Printf("pause spawns: %v", err)
		}
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

// pauseSpawns writes a future timestamp to last-spawn.txt, causing the
// stagger gate to refuse spawns until that time passes.
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
