// Command benchmark-query provides a CLI for querying benchmark metrics.
//
// Usage:
//
//	benchmark-query [flags]
//	benchmark-query -metric test_pass_rate_delta
//	benchmark-query -metric session_success_rate -since 24h
//	benchmark-query -list
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/internal/metrics"
)

func main() {
	var (
		metric  string
		since   string
		dir     string
		list    bool
		jsonOut bool
	)
	flag.StringVar(&metric, "metric", "", "metric name to query (test_pass_rate_delta, false_completion_rate, hook_bypass_rate, session_success_rate)")
	flag.StringVar(&since, "since", "", "time window (e.g., 24h, 7d, 30d)")
	flag.StringVar(&dir, "dir", "", "metrics directory (default: ~/.agm/benchmarks)")
	flag.BoolVar(&list, "list", false, "list all available metrics with summaries")
	flag.BoolVar(&jsonOut, "json", false, "output as JSON")
	flag.Parse()

	store, err := metrics.NewStore(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if list {
		listMetrics(store, since, jsonOut)
		return
	}

	if metric == "" {
		listMetrics(store, since, jsonOut)
		return
	}

	filter := metrics.QueryFilter{
		Metric: metrics.MetricName(metric),
	}
	if since != "" {
		d, err := parseDuration(since)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid -since: %v\n", err)
			os.Exit(1)
		}
		filter.Since = time.Now().UTC().Add(-d)
	}

	summary, err := store.Summarize(filter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if summary == nil {
		fmt.Println("no data")
		return
	}

	if jsonOut {
		data, _ := json.MarshalIndent(summary, "", "  ")
		fmt.Println(string(data))
	} else {
		printSummary(summary)
	}
}

func listMetrics(store *metrics.Store, since string, jsonOut bool) {
	allMetrics := []metrics.MetricName{
		metrics.MetricTestPassRateDelta,
		metrics.MetricFalseCompletionRate,
		metrics.MetricHookBypassRate,
		metrics.MetricSessionSuccessRate,
	}

	var filter metrics.QueryFilter
	if since != "" {
		d, err := parseDuration(since)
		if err == nil {
			filter.Since = time.Now().UTC().Add(-d)
		}
	}

	var summaries []*metrics.Summary
	for _, m := range allMetrics {
		filter.Metric = m
		s, err := store.Summarize(filter)
		if err != nil {
			continue
		}
		if s != nil {
			summaries = append(summaries, s)
		}
	}

	if jsonOut {
		data, _ := json.MarshalIndent(summaries, "", "  ")
		fmt.Println(string(data))
		return
	}

	if len(summaries) == 0 {
		fmt.Printf("no metrics data in %s\n", store.Dir())
		return
	}

	fmt.Printf("Benchmark Metrics (%s)\n", store.Dir())
	fmt.Println("─────────────────────────────────────────────────────")
	for _, s := range summaries {
		printSummary(s)
		fmt.Println()
	}
}

func printSummary(s *metrics.Summary) {
	fmt.Printf("%-25s [%s]\n", s.Metric, s.Category)
	fmt.Printf("  count:  %d\n", s.Count)
	fmt.Printf("  mean:   %.4f\n", s.Mean)
	fmt.Printf("  latest: %.4f\n", s.Latest)
	fmt.Printf("  min:    %.4f\n", s.Min)
	fmt.Printf("  max:    %.4f\n", s.Max)
}

func parseDuration(s string) (time.Duration, error) {
	// Support "Nd" for days
	if len(s) > 1 && s[len(s)-1] == 'd' {
		var days int
		if _, err := fmt.Sscanf(s, "%dd", &days); err == nil {
			return time.Duration(days) * 24 * time.Hour, nil
		}
	}
	return time.ParseDuration(s)
}
