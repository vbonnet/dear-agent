package main

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/usage"
)

var (
	usageSince     time.Duration
	usageThreshold float64
	usageRoot      string
)

var usageCmd = &cobra.Command{
	Use:   "usage",
	Short: "Token/cost tracking from Claude Code transcripts",
	Long: `Parse ~/.claude/projects/**/*.jsonl conversation transcripts to aggregate
token counts and estimate USD cost by model and session type (worker vs
supervisor), then project a daily burn rate and optionally alert when it
exceeds a threshold.

This is the "path 2" usage monitor: it reads the JSONL transcripts Claude Code
already writes, so it works immediately with no telemetry/env configuration.

Cost is ESTIMATED from approximate Anthropic public pricing — the transcripts
record token counts, not prices. Worker vs supervisor is a heuristic: sessions
whose cwd is under ~/.agm/sandboxes/ are workers; everything else is supervisor.

Examples:
  agm usage                              # last 24h, human summary
  agm usage --since 6h                   # last 6 hours
  agm usage --alert-threshold 50         # warn if projected daily cost > $50
  agm usage --output json                # compact JSON envelope (agent mode)`,
	RunE: runUsage,
}

func init() {
	usageCmd.Flags().DurationVar(&usageSince, "since", 24*time.Hour, "look-back window (e.g. 24h, 90m, 7d as 168h)")
	usageCmd.Flags().Float64Var(&usageThreshold, "alert-threshold", 0, "alert if projected daily cost (USD) exceeds this value (0 = no alert)")
	usageCmd.Flags().StringVar(&usageRoot, "root", "", "transcripts root (default: ~/.claude/projects)")
	rootCmd.AddCommand(usageCmd)
}

func runUsage(cmd *cobra.Command, args []string) error {
	root := usageRoot
	if root == "" {
		root = usage.DefaultRoot()
	}

	since := time.Now().Add(-usageSince)
	entries, err := usage.Collect(root, since)
	if err != nil {
		return fmt.Errorf("collecting usage from %s: %w", root, err)
	}

	windowHours := usageSince.Hours()
	report := usage.Build(entries, windowHours, usageThreshold)

	if isJSONOutput() {
		return printJSON(report)
	}

	printUsageHuman(cmd, report)

	// In human mode, surface the alert on stderr so it is visible even when
	// stdout is piped/captured.
	if report.Alert {
		fmt.Fprintf(os.Stderr,
			"\n⚠ ALERT: projected daily cost $%.2f exceeds threshold $%.2f\n",
			report.BurnRateDailyUSD, report.AlertThresholdUSD)
	}
	return nil
}

func printUsageHuman(cmd *cobra.Command, r usage.Report) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "Claude Code Usage Report")
	fmt.Fprintln(out, "════════════════════════")
	fmt.Fprintf(out, "Window:     last %.0fh (%d messages)\n", r.WindowHours, r.MessageCount)
	fmt.Fprintf(out, "Tokens:     %s\n", groupThousands(r.TotalTokens))
	fmt.Fprintf(out, "Cost:       $%.2f (estimated)\n", r.TotalCostUSD)
	fmt.Fprintf(out, "Burn rate:  $%.2f/day projected\n", r.BurnRateDailyUSD)
	if r.TopModel != "" {
		fmt.Fprintf(out, "Top model:  %s\n", r.TopModel)
	}

	if len(r.Models) > 0 {
		fmt.Fprintln(out, "\nBy model:")
		for _, name := range sortedByCost(r.Models) {
			b := r.Models[name]
			fmt.Fprintf(out, "  %-28s %16s tokens  $%9.2f\n", name, groupThousands(b.Tokens), b.CostUSD)
		}
	}

	if len(r.Sessions) > 0 {
		fmt.Fprintln(out, "\nBy session type:")
		for _, name := range sortedByCost(r.Sessions) {
			b := r.Sessions[name]
			fmt.Fprintf(out, "  %-28s %16s tokens  $%9.2f\n", name, groupThousands(b.Tokens), b.CostUSD)
		}
	}
}

// groupThousands formats an integer with comma thousands separators, e.g.
// 306567137 -> "306,567,137". The package-level formatTokens helper only groups
// a single separator and mis-renders values above one million.
func groupThousands(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := fmt.Sprintf("%d", n)
	out := make([]byte, 0, len(s)+len(s)/3)
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

// sortedByCost returns bucket keys ordered by descending cost (stable by name).
func sortedByCost(m map[string]usage.Bucket) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]].CostUSD != m[keys[j]].CostUSD {
			return m[keys[i]].CostUSD > m[keys[j]].CostUSD
		}
		return keys[i] < keys[j]
	})
	return keys
}
