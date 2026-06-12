// resource-monitor collects process and memory metrics, detects orphaned
// gopls/llama-server instances, logs JSON lines, and optionally kills orphans.
//
// Usage:
//
//	resource-monitor [--kill] [--log <path>] [--json]
//
// Flags:
//
//	--kill       Kill detected orphans that belong to safe-to-kill targets
//	             (gopls, llama-server). Sends SIGTERM.
//	--log <path> Append a JSON line to <path> (default: ~/.agm/logs/resource-monitor.jsonl)
//	--no-log     Disable JSONL logging.
//	--json       Print the full JSON report to stdout instead of human text.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/vbonnet/dear-agent/internal/resourcemon"
)

const defaultLogFile = "~/.agm/logs/resource-monitor.jsonl"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "resource-monitor: %v\n", err)
		os.Exit(1)
	}
}

func run(argv []string) error {
	kill := false
	jsonOut := false
	noLog := false
	logFile := expandHome(defaultLogFile)

	for i := 0; i < len(argv); i++ {
		switch argv[i] {
		case "--kill":
			kill = true
		case "--json":
			jsonOut = true
		case "--no-log":
			noLog = true
		case "--log":
			if i+1 >= len(argv) {
				return fmt.Errorf("--log requires a path argument")
			}
			i++
			logFile = expandHome(argv[i])
		case "-h", "--help":
			fmt.Print(usage)
			return nil
		default:
			return fmt.Errorf("unknown flag %q (try --help)", argv[i])
		}
	}

	r, err := resourcemon.Collect(kill)
	if err != nil {
		return err
	}

	if !noLog {
		if logErr := resourcemon.LogJSONL(r, logFile); logErr != nil {
			fmt.Fprintf(os.Stderr, "resource-monitor: log write failed: %v\n", logErr)
		}
	}

	if jsonOut {
		data, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	printHuman(r)
	return nil
}

func printHuman(r *resourcemon.Report) {
	fmt.Printf("=== resource-monitor %s ===\n", r.Timestamp.Format("2006-01-02 15:04:05 UTC"))
	fmt.Printf("Memory: %d/%d MB used (%.0f%%)  avail=%d MB\n",
		r.MemTotalMB-r.MemAvailMB, r.MemTotalMB, r.MemUsedPct, r.MemAvailMB)
	fmt.Printf("Swap:   %d/%d MB used\n", r.SwapUsedMB, r.SwapTotalMB)

	if len(r.Groups) > 0 {
		fmt.Println("\nWatched processes:")
		sorted := append([]resourcemon.ProcGroup(nil), r.Groups...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].RSSMB > sorted[j].RSSMB })
		for _, g := range sorted {
			fmt.Printf("  %-20s  count=%-3d  rss=%d MB\n", g.Name, g.Count, g.RSSMB)
		}
	}

	if len(r.Orphans) > 0 {
		fmt.Printf("\nOrphaned processes (%d):\n", len(r.Orphans))
		for _, o := range r.Orphans {
			fmt.Printf("  PID=%-7d  %-20s  %d MB  (%s)\n", o.PID, o.Name, o.RSSMB, o.Reason)
		}
	}

	if len(r.Zombies) > 0 {
		fmt.Printf("\nZombie processes (%d):\n", len(r.Zombies))
		for _, z := range r.Zombies {
			fmt.Printf("  PID=%-7d  %s\n", z.PID, z.Name)
		}
	}

	if len(r.Killed) > 0 {
		fmt.Printf("\nKilled (%d):\n", len(r.Killed))
		for _, k := range r.Killed {
			fmt.Printf("  PID=%-7d  %-20s  %d MB\n", k.PID, k.Name, k.RSSMB)
		}
	}

	if len(r.Alerts) > 0 {
		fmt.Println("\nALERTS:")
		for _, a := range r.Alerts {
			fmt.Printf("  ! %s\n", a)
		}
	} else {
		fmt.Println("\nNo alerts.")
	}
}

func expandHome(path string) string {
	if len(path) >= 2 && path[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

const usage = `resource-monitor — process and memory health check

Usage:
  resource-monitor [flags]

Flags:
  --kill       Kill orphaned gopls/llama-server instances (SIGTERM)
  --log <path> Append JSON line to <path> (default: ~/.agm/logs/resource-monitor.jsonl)
  --no-log     Disable JSONL logging
  --json       Print full JSON report to stdout
  -h, --help   Show this help

Exit codes: 0=ok, 1=error
`
