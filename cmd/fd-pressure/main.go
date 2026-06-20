// Command fd-pressure samples system FD/vnode/gopls pressure and prints a
// report. Exit code 0 = all metrics within limits; exit code 1 = at least one
// metric has crossed its escalation threshold; exit code 2 = usage/flag error.
//
// Designed to be called by the VROOM Overseer's monitoring loop and by humans
// investigating build failures caused by FD/vnode exhaustion.
//
// Usage:
//
//	fd-pressure                          # human-readable table to stdout
//	fd-pressure --json                   # JSON object to stdout
//	fd-pressure --threshold-fds 0.85     # override FD escalation threshold
//	fd-pressure --threshold-vnodes 0.90  # override vnode escalation threshold
//	fd-pressure --threshold-gopls 5      # override gopls count threshold
//
// Metrics:
//
//	open_fds       Fraction of system-wide FD limit in use (Darwin: kern.num_files / kern.maxfiles)
//	vnodes         Fraction of vnode table in use (Darwin only; 0 on Linux)
//	gopls          Count of running gopls processes
//	disk           Fraction of "/" disk space used
//	memory         Fraction of physical RAM used
//	swap           Fraction of swap space used
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/pkg/vroom/supervisor"
)

func main() {
	code, err := run(os.Args[1:], os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fd-pressure:", err)
		os.Exit(2)
	}
	if code != 0 {
		os.Exit(code)
	}
}

type config struct {
	jsonOutput      bool
	thresholdFDs    float64
	thresholdVnodes float64
	thresholdGopls  int
}

func run(args []string, out io.Writer) (int, error) {
	fs := flag.NewFlagSet("fd-pressure", flag.ContinueOnError)
	fs.SetOutput(out)
	cfg := config{}
	fs.BoolVar(&cfg.jsonOutput, "json", false, "emit JSON instead of human-readable table")
	fs.Float64Var(&cfg.thresholdFDs, "threshold-fds", supervisor.DefaultEscalationThreshold.Fraction,
		"FD fraction threshold for non-zero exit")
	fs.Float64Var(&cfg.thresholdVnodes, "threshold-vnodes", supervisor.DefaultEscalationThreshold.Fraction,
		"vnode fraction threshold for non-zero exit")
	fs.IntVar(&cfg.thresholdGopls, "threshold-gopls", supervisor.DefaultEscalationThreshold.GoplsProcesses,
		"gopls process count threshold for non-zero exit")
	if err := fs.Parse(args); err != nil {
		return 2, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	probe := supervisor.NewSysResourceProbe()
	snap, err := probe.Snapshot(ctx)
	cancel() // explicit cancel; don't defer so os.Exit below doesn't suppress it
	if err != nil {
		return 2, fmt.Errorf("snapshot: %w", err)
	}

	breached := evaluate(snap, cfg)

	if cfg.jsonOutput {
		if err := emitJSON(out, snap, breached); err != nil {
			return 2, err
		}
	} else {
		emitTable(out, snap, breached, cfg)
	}

	if len(breached) > 0 {
		return 1, nil
	}
	return 0, nil
}

type breach struct {
	Metric    string  `json:"metric"`
	Value     float64 `json:"value"`
	Count     int     `json:"count,omitempty"`
	Threshold float64 `json:"threshold,omitempty"`
	ThreshInt int     `json:"threshold_count,omitempty"`
}

func evaluate(snap supervisor.ResourceSnapshot, cfg config) []breach {
	var out []breach
	if snap.OpenFDFraction >= cfg.thresholdFDs {
		out = append(out, breach{Metric: "open_fds", Value: snap.OpenFDFraction, Threshold: cfg.thresholdFDs})
	}
	if snap.VnodeUsedFraction >= cfg.thresholdVnodes {
		out = append(out, breach{Metric: "vnodes", Value: snap.VnodeUsedFraction, Threshold: cfg.thresholdVnodes})
	}
	if snap.GoplsProcesses > cfg.thresholdGopls {
		out = append(out, breach{Metric: "gopls", Count: snap.GoplsProcesses, ThreshInt: cfg.thresholdGopls})
	}
	if snap.DiskUsedFraction >= supervisor.DefaultEscalationThreshold.Fraction {
		out = append(out, breach{Metric: "disk", Value: snap.DiskUsedFraction, Threshold: supervisor.DefaultEscalationThreshold.Fraction})
	}
	if snap.MemoryUsedFraction >= supervisor.DefaultEscalationThreshold.Fraction {
		out = append(out, breach{Metric: "memory", Value: snap.MemoryUsedFraction, Threshold: supervisor.DefaultEscalationThreshold.Fraction})
	}
	if snap.SwapUsedFraction >= supervisor.DefaultEscalationThreshold.SwapFraction {
		out = append(out, breach{Metric: "swap", Value: snap.SwapUsedFraction, Threshold: supervisor.DefaultEscalationThreshold.SwapFraction})
	}
	return out
}

func emitJSON(out io.Writer, snap supervisor.ResourceSnapshot, breaches []breach) error {
	type report struct {
		Snapshot supervisor.ResourceSnapshot `json:"snapshot"`
		Breaches []breach                    `json:"breaches"`
		OK       bool                        `json:"ok"`
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(report{
		Snapshot: snap,
		Breaches: breaches,
		OK:       len(breaches) == 0,
	})
}

func emitTable(out io.Writer, snap supervisor.ResourceSnapshot, breaches []breach, cfg config) {
	prefix := func(v float64, thresh float64) string {
		if v >= thresh {
			return "!"
		}
		return " "
	}
	prefixCount := func(n, thresh int) string {
		if n > thresh {
			return "!"
		}
		return " "
	}

	fmt.Fprintf(out, "fd-pressure report\n")
	fmt.Fprintf(out, "%-20s  %8s  %8s  %s\n", "METRIC", "VALUE", "THRESH", "STATUS")
	fmt.Fprintf(out, "%-20s  %8s  %8s  %s\n", "------", "-----", "------", "------")
	fmt.Fprintf(out, "%s %-19s  %7.1f%%  %7.1f%%  %s\n",
		prefix(snap.OpenFDFraction, cfg.thresholdFDs),
		"open_fds",
		snap.OpenFDFraction*100,
		cfg.thresholdFDs*100,
		statusStr(snap.OpenFDFraction >= cfg.thresholdFDs))
	fmt.Fprintf(out, "%s %-19s  %7.1f%%  %7.1f%%  %s\n",
		prefix(snap.VnodeUsedFraction, cfg.thresholdVnodes),
		"vnodes",
		snap.VnodeUsedFraction*100,
		cfg.thresholdVnodes*100,
		statusStr(snap.VnodeUsedFraction >= cfg.thresholdVnodes))
	fmt.Fprintf(out, "%s %-19s  %8d  %8d  %s\n",
		prefixCount(snap.GoplsProcesses, cfg.thresholdGopls),
		"gopls_processes",
		snap.GoplsProcesses,
		cfg.thresholdGopls,
		statusStr(snap.GoplsProcesses > cfg.thresholdGopls))
	fmt.Fprintf(out, "%s %-19s  %7.1f%%  %7.1f%%  %s\n",
		prefix(snap.DiskUsedFraction, supervisor.DefaultEscalationThreshold.Fraction),
		"disk",
		snap.DiskUsedFraction*100,
		supervisor.DefaultEscalationThreshold.Fraction*100,
		statusStr(snap.DiskUsedFraction >= supervisor.DefaultEscalationThreshold.Fraction))
	fmt.Fprintf(out, "%s %-19s  %7.1f%%  %7.1f%%  %s\n",
		prefix(snap.MemoryUsedFraction, supervisor.DefaultEscalationThreshold.Fraction),
		"memory",
		snap.MemoryUsedFraction*100,
		supervisor.DefaultEscalationThreshold.Fraction*100,
		statusStr(snap.MemoryUsedFraction >= supervisor.DefaultEscalationThreshold.Fraction))
	fmt.Fprintf(out, "%s %-19s  %7.1f%%  %7.1f%%  %s\n",
		prefix(snap.SwapUsedFraction, supervisor.DefaultEscalationThreshold.SwapFraction),
		"swap",
		snap.SwapUsedFraction*100,
		supervisor.DefaultEscalationThreshold.SwapFraction*100,
		statusStr(snap.SwapUsedFraction >= supervisor.DefaultEscalationThreshold.SwapFraction))

	fmt.Fprintln(out)
	if len(breaches) == 0 {
		fmt.Fprintln(out, "Status: OK (all metrics within limits)")
	} else {
		fmt.Fprintf(out, "Status: BREACHED (%d metric(s) exceeded threshold)\n", len(breaches))
		for _, b := range breaches {
			if b.Metric == "gopls" {
				fmt.Fprintf(out, "  ! %s: count=%d (threshold=%d) — run: pkill -x gopls\n",
					b.Metric, b.Count, b.ThreshInt)
			} else {
				fmt.Fprintf(out, "  ! %s: %.1f%% (threshold=%.1f%%)\n",
					b.Metric, b.Value*100, b.Threshold*100)
			}
		}
	}
}

func statusStr(breached bool) string {
	if breached {
		return "BREACHED"
	}
	return "ok"
}
