// Command hook-analyzer analyzes bash blocker hook denial logs to identify
// false positives and propose pattern improvements.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/engram/hooks-bin/internal/analyzer"
)

func main() {
	logPath := flag.String("log-path", "~/.claude/hooks/logs/pretool-bash-blocker.log", "path to the hook denial log")
	output := flag.String("output", "text", "output format: text or json")
	topN := flag.Int("top", 10, "show top N patterns by FP rate")
	since := flag.String("since", "", "time filter: e.g. 7d, 24h, 1h30m")
	pattern := flag.String("pattern", "", "analyze a specific pattern only")
	noTranscripts := flag.Bool("no-transcripts", false, "skip transcript correlation")
	flag.Parse()

	*logPath = expandPath(*logPath)

	var sinceTime *time.Time
	if *since != "" {
		d, err := analyzer.ParseTimeDelta(*since)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid --since value %q: %v\n", *since, err)
			os.Exit(1)
		}
		t := time.Now().Add(-d)
		sinceTime = &t
	}

	// Step 1: Parse log.
	denials, _, stats, err := analyzer.ParseLog(*logPath, sinceTime)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing log: %v\n", err)
		os.Exit(1)
	}

	// Count lines for the report (use total invocations as proxy since
	// the log parser doesn't expose raw line count).
	lineCount := stats.TotalInvocations

	// Step 2: Classify denials.
	var classified []analyzer.ClassifiedDenial
	if !*noTranscripts {
		cache := analyzer.NewTranscriptCache(32)
		classified = analyzer.ClassifyDenials(denials, cache)
	} else {
		classified = make([]analyzer.ClassifiedDenial, len(denials))
		for i, d := range denials {
			classified[i] = analyzer.ClassifiedDenial{
				Denial:  d,
				Outcome: analyzer.OutcomeUnknown,
			}
		}
	}

	// Step 3: Analyze patterns.
	patterns := analyzer.AnalyzePatterns(classified)
	patterns = analyzer.ProposePatternFixes(patterns, classified)

	// Step 4: Filter to specific pattern if requested.
	if *pattern != "" {
		var filtered []analyzer.PatternAnalysis
		for _, pa := range patterns {
			if strings.EqualFold(pa.PatternName, *pattern) {
				filtered = append(filtered, pa)
			}
		}
		patterns = filtered
	}

	// Step 5: Generate report.
	report := analyzer.GenerateReport(stats, patterns, *logPath, lineCount)

	// Step 6: Output.
	switch *output {
	case "json":
		data, err := analyzer.FormatJSONReport(report)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error formatting JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
	case "text":
		fmt.Print(analyzer.FormatTextReport(report, *topN))
	default:
		fmt.Fprintf(os.Stderr, "unknown output format: %s (use 'text' or 'json')\n", *output)
		os.Exit(1)
	}
}

// expandPath replaces a leading ~ with the user's home directory.
func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return home + p[1:]
	}
	return p
}
