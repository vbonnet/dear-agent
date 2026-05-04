package analyzer

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// GenerateReport assembles the top-level Report from parsed stats and pattern analyses.
func GenerateReport(stats HookLogStats, patterns []PatternAnalysis, logPath string, lineCount int) Report {
	var totalFP, totalTP, totalWasted int
	for _, pa := range patterns {
		totalFP += pa.FalsePositives
		totalTP += pa.TruePositives
		for _, fp := range pa.ExampleFPs {
			totalWasted += fp.WastedCalls
		}
	}

	var overallFPRate float64
	if totalFP+totalTP > 0 {
		overallFPRate = float64(totalFP) / float64(totalFP+totalTP)
	}

	return Report{
		GeneratedAt:      time.Now(),
		LogPath:          logPath,
		LogLineCount:     lineCount,
		TimeRange:        stats.TimeRange,
		Stats:            stats,
		Patterns:         patterns,
		OverallFPRate:    overallFPRate,
		TotalWastedCalls: totalWasted,
	}
}

// FormatTextReport renders a human-readable text report showing the top N patterns.
//nolint:gocyclo // reason: linear report formatting with many sections
func FormatTextReport(r Report, topN int) string {
	var b strings.Builder

	// Header.
	b.WriteString("=== Hook Analyzer Report ===\n")
	fmt.Fprintf(&b, "Generated: %s\n", r.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "Log: %s (%d lines)\n", r.LogPath, r.LogLineCount)
	if !r.TimeRange[0].IsZero() && !r.TimeRange[1].IsZero() {
		fmt.Fprintf(&b, "Time range: %s to %s\n",
			r.TimeRange[0].Format("2006-01-02 15:04"),
			r.TimeRange[1].Format("2006-01-02 15:04"))
	}
	b.WriteString("\n")

	// Summary stats.
	b.WriteString("--- Summary ---\n")
	fmt.Fprintf(&b, "Total invocations: %d\n", r.Stats.TotalInvocations)
	fmt.Fprintf(&b, "Total denials:     %d\n", r.Stats.TotalDenials)
	fmt.Fprintf(&b, "Total approvals:   %d\n", r.Stats.TotalApprovals)
	fmt.Fprintf(&b, "Unique sessions:   %d\n", r.Stats.UniqueSessionIDs)
	fmt.Fprintf(&b, "Overall FP rate:   %.1f%%\n", r.OverallFPRate*100)
	fmt.Fprintf(&b, "Total wasted calls (from examples): %d\n", r.TotalWastedCalls)
	b.WriteString("\n")

	// Per-pattern table.
	n := topN
	if n > len(r.Patterns) {
		n = len(r.Patterns)
	}
	if n == 0 {
		b.WriteString("No patterns to display.\n")
		return b.String()
	}

	fmt.Fprintf(&b, "--- Top %d Patterns by FP Rate ---\n", n)
	fmt.Fprintf(&b, "%-50s %8s %6s %6s %8s\n", "Pattern", "Denials", "FPs", "TPs", "FP Rate")
	b.WriteString(strings.Repeat("-", 84) + "\n")

	for i := 0; i < n; i++ {
		pa := r.Patterns[i]
		name := pa.PatternName
		if len(name) > 50 {
			name = name[:47] + "..."
		}
		fmt.Fprintf(&b, "%-50s %8d %6d %6d %7.1f%%\n",
			name, pa.TotalDenials, pa.FalsePositives, pa.TruePositives, pa.FalsePositiveRate*100)

		// Show example FP commands (up to 3).
		fpLimit := 3
		if fpLimit > len(pa.ExampleFPs) {
			fpLimit = len(pa.ExampleFPs)
		}
		for j := 0; j < fpLimit; j++ {
			cmd := pa.ExampleFPs[j].Denial.Command
			if len(cmd) > 70 {
				cmd = cmd[:67] + "..."
			}
			fmt.Fprintf(&b, "  FP: %s\n", cmd)
		}
	}

	// Proposed fixes section.
	var fixes []PatternAnalysis
	for i := 0; i < n; i++ {
		if r.Patterns[i].ProposedFix != nil {
			fixes = append(fixes, r.Patterns[i])
		}
	}
	if len(fixes) > 0 {
		b.WriteString("\n--- Proposed Fixes ---\n")
		for _, pa := range fixes {
			fix := pa.ProposedFix
			fmt.Fprintf(&b, "\nPattern: %s\n", pa.PatternName)
			fmt.Fprintf(&b, "  Original:  %s\n", fix.OriginalRegex)
			fmt.Fprintf(&b, "  Proposed:  %s\n", fix.ProposedRegex)
			fmt.Fprintf(&b, "  FPs fixed: %d  TPs preserved: %d  TPs lost: %d\n",
				fix.FPsFixed, fix.TPsPreserved, fix.TPsLost)
			for _, ex := range fix.ExampleFixed {
				if len(ex) > 70 {
					ex = ex[:67] + "..."
				}
				fmt.Fprintf(&b, "  Would allow: %s\n", ex)
			}
		}
	}

	return b.String()
}

// FormatJSONReport marshals the Report to indented JSON.
func FormatJSONReport(r Report) ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}
