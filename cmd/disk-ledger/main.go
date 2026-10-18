// Command disk-ledger reports which agent runs are leaking disk and whether
// the reclaim path is actually reclaiming anything.
//
// It exists because free-space thresholds answer the wrong question. The host
// watchdog reported "build caches: 14 found, 0 reaped ... Status: OK" while
// the shared Go build cache alone held 44 GiB, because its reaper only ever
// scanned /tmp and TMPDIR and nothing checked the shared roots at all. A
// collector that reclaims nothing looks exactly like an idle healthy host
// unless something also asserts that the tracked roots are within budget.
//
// Exit codes follow the health-binary contract used by the other *-health
// commands so absence-alarm can consume this directly:
//
//	0  healthy
//	1  degraded (over budget, or leaks attributable to dead owners)
//	2  down     (the reclaim path is silently doing nothing)
//	3  usage error
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/vbonnet/dear-agent/pkg/diskledger"
)

const (
	exitHealthy  = 0
	exitDegraded = 1
	exitDown     = 2
	exitUsage    = 3
)

// report is the machine-readable shape emitted with --json.
type report struct {
	At          time.Time                `json:"at"`
	Roots       []diskledger.RootVerdict `json:"roots"`
	Health      diskledger.ReclaimHealth `json:"reclaim_health"`
	Leaks       []diskledger.Record      `json:"leaks,omitempty"`
	LeakedBytes int64                    `json:"leaked_bytes_total"`
	Exit        int                      `json:"exit"`
}

func main() {
	code, err := run(os.Args[1:], os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "disk-ledger:", err)
	}
	os.Exit(code)
}

func run(args []string, stdout, stderr io.Writer) (int, error) {
	fs := flag.NewFlagSet("disk-ledger", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		asJSON    = fs.Bool("json", false, "Emit the report as JSON")
		ledgerP   = fs.String("ledger", "", "Ledger JSONL path (default ~/.agm/logs/diskledger.jsonl)")
		grace     = fs.Duration("grace", time.Hour, "Ignore open entries younger than this")
		window    = fs.Duration("window", 24*time.Hour, "Window for the bytes-reclaimed check")
		reclaimed = fs.Int64("bytes-reclaimed", -1, "Bytes reclaimed in the window; -1 reads the gc log")
		heartbeat = fs.String("heartbeat", "", "Write a heartbeat file here after a successful check")
	)
	if perr := fs.Parse(args); perr != nil {
		// Usage errors are already rendered to stderr by the flag package;
		// returning them again would double-print.
		return exitUsage, nil //nolint:nilerr // flag package already reported it
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return exitUsage, fmt.Errorf("resolve home: %w", err)
	}

	roots := diskledger.EvaluateRoots(diskledger.DefaultRootBudgets(home))

	ledgerPath := *ledgerP
	if ledgerPath == "" {
		if ledgerPath, err = diskledger.DefaultPath(); err != nil {
			return exitUsage, err
		}
	}
	recs, err := diskledger.ReadRecords(ledgerPath)
	if err != nil {
		return exitUsage, fmt.Errorf("read ledger: %w", err)
	}
	// An owner is live if a sandbox/worktree for it still resolves to a
	// running session. Without a session store we fall back to "assume dead
	// after grace", which is safe here because Reconcile only ever REPORTS.
	leaks := diskledger.Reconcile(diskledger.OpenEntries(recs), nil, *grace)

	bytesReclaimed := *reclaimed
	if bytesReclaimed < 0 {
		bytesReclaimed = reclaimedFromGCLog(filepath.Join(home, ".agm", "logs", "gc.jsonl"), *window)
	}

	health := diskledger.CheckReclaimHealth(roots, bytesReclaimed, *window)

	rep := report{At: time.Now(), Roots: roots, Health: health, Leaks: leaks}
	for _, l := range leaks {
		rep.LeakedBytes += l.LeakedBytes
	}
	rep.Exit = exitFor(health, leaks)

	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			return exitUsage, err
		}
	} else {
		writeText(stdout, rep)
	}

	if *heartbeat != "" {
		// Best-effort: a heartbeat that cannot be written must not change the
		// health verdict, but its absence is itself what absence-alarm watches,
		// so the failure is reported rather than swallowed.
		if herr := writeHeartbeat(*heartbeat, rep); herr != nil {
			fmt.Fprintf(stderr, "disk-ledger: write heartbeat: %v\n", herr)
		}
	}
	return rep.Exit, nil
}

// exitFor maps a health verdict to the health-binary exit contract.
func exitFor(h diskledger.ReclaimHealth, leaks []diskledger.Record) int {
	switch {
	case h.Status == diskledger.StatusSilentGC:
		// The collector is collecting nothing while roots are over budget.
		// This is the condition that hid a 44 GiB leak behind "Status: OK".
		return exitDown
	case h.Status == diskledger.StatusPressure, len(leaks) > 0:
		return exitDegraded
	default:
		return exitHealthy
	}
}

func writeText(w io.Writer, rep report) {
	fmt.Fprintln(w, "disk-ledger report")
	sorted := append([]diskledger.RootVerdict(nil), rep.Roots...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].AllocatedBytes > sorted[j].AllocatedBytes })
	for _, r := range sorted {
		state := "ok"
		switch {
		case r.Missing:
			state = "absent"
		case r.OverBudget:
			state = fmt.Sprintf("OVER by %s", gib(r.OverBy()))
		}
		fmt.Fprintf(w, "  %-16s %10s  [budget %8s]  %s\n", r.Name, gib(r.AllocatedBytes), gib(r.BudgetBytes), state)
	}
	fmt.Fprintf(w, "  reclaimed in %s: %s\n", rep.Health.Window, gib(rep.Health.BytesReclaimed))
	if len(rep.Leaks) > 0 {
		fmt.Fprintf(w, "  attributable leaks: %d run(s), %s\n", len(rep.Leaks), gib(rep.LeakedBytes))
		for _, l := range rep.Leaks {
			fmt.Fprintf(w, "    %-24s owner=%-24s %8s  %s\n", l.RunID, l.Owner, gib(l.LeakedBytes), l.Path)
		}
	}
	fmt.Fprintf(w, "\nStatus: %s", rep.Health.Status)
	if rep.Health.Detail != "" {
		fmt.Fprintf(w, " (%s)", rep.Health.Detail)
	}
	fmt.Fprintln(w)
}

func gib(b int64) string {
	if b == 0 {
		return "0"
	}
	return fmt.Sprintf("%.1f GiB", float64(b)/(1<<30))
}

// reclaimedFromGCLog sums bytes_reclaimed from gc.jsonl over the window.
// An unreadable or absent log yields 0, which is the conservative answer: it
// makes an over-budget host report a silent GC rather than assume reclaim it
// cannot prove.
func reclaimedFromGCLog(path string, window time.Duration) int64 {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	cutoff := time.Now().Add(-window)
	var total int64
	dec := json.NewDecoder(f)
	for dec.More() {
		var e struct {
			Timestamp      time.Time `json:"timestamp"`
			BytesReclaimed int64     `json:"bytes_reclaimed"`
		}
		if err := dec.Decode(&e); err != nil {
			break
		}
		if e.Timestamp.After(cutoff) {
			total += e.BytesReclaimed
		}
	}
	return total
}

func writeHeartbeat(path string, rep report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(map[string]any{
		"at":                 rep.At,
		"exit":               rep.Exit,
		"status":             rep.Health.Status,
		"leaked_bytes_total": rep.LeakedBytes,
	})
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
