package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/vbonnet/dear-agent/pkg/aggregator/salience"
)

func runSalience(_ context.Context, args []string) int {
	return runSalienceWith(args, os.Stdin, os.Stdout, os.Stderr)
}

// runSalienceWith is the testable form of runSalience: explicit IO so
// tests can drive stdin and capture stdout/stderr.
func runSalienceWith(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("salience", flag.ContinueOnError)
	fs.SetOutput(stderr)
	input := fs.String("input", "",
		"path to JSONL file of drift signals; '-' or empty reads stdin")
	window := fs.Duration("window", time.Hour,
		"sliding window for the notification budget (e.g. 1h)")
	capacity := fs.Int("capacity", 10,
		"max budget-counted notifications per window; <=0 disables suppression")
	bypassStr := fs.String("bypass", "high",
		"tier at or above which signals always notify (noise|low|medium|high|critical)")
	asJSON := fs.Bool("json", false, "emit machine-readable JSON")
	keepNoise := fs.Bool("keep-noise", false,
		"include TierNoise signals instead of dropping them before the budget")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	bypass, err := salience.ParseTier(*bypassStr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	r, closer, err := openSalienceInput(*input, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if closer != nil {
		defer func() { _ = closer.Close() }()
	}

	budget := salience.NewNotificationBudget(*window, *capacity)
	budget.BypassTier = bypass

	agg := salience.New()
	agg.Budget = budget
	agg.DropNoise = !*keepNoise

	outcomes, err := agg.LoadJSONL(r)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if len(outcomes) == 0 {
		fmt.Fprintln(stderr, salience.ErrEmptyInput)
		return 1
	}

	if *asJSON {
		return renderSalienceJSON(stdout, stderr, outcomes)
	}
	return renderSalienceText(stdout, outcomes)
}

func openSalienceInput(path string, stdin io.Reader) (io.Reader, io.Closer, error) {
	if path == "" || path == "-" {
		return stdin, nil, nil
	}
	f, err := os.Open(path) // #nosec G304 — operator-supplied input path
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", path, err)
	}
	return f, f, nil
}

func renderSalienceJSON(stdout, stderr io.Writer, outcomes []salience.Outcome) int {
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	payload := struct {
		Outcomes []salience.Outcome `json:"outcomes"`
		Summary  jsonSummary        `json:"summary"`
	}{
		Outcomes: outcomes,
		Summary:  toJSONSummary(salience.Summarize(outcomes)),
	}
	if err := enc.Encode(payload); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

// jsonSummary mirrors salience.Summary but stringifies the non-string
// map keys so encoding/json doesn't object.
type jsonSummary struct {
	Total       int            `json:"total"`
	Notified    int            `json:"notified"`
	Suppressed  int            `json:"suppressed"`
	NotifyRatio float64        `json:"notifyRatio"`
	ByTier      map[string]int `json:"byTier,omitempty"`
	ByKind      map[string]int `json:"byKind,omitempty"`
	ByReason    map[string]int `json:"byReason,omitempty"`
}

func toJSONSummary(s salience.Summary) jsonSummary {
	out := jsonSummary{
		Total:       s.Total,
		Notified:    s.Notified,
		Suppressed:  s.Suppressed,
		NotifyRatio: s.NotifyRatio,
		ByTier:      map[string]int{},
		ByKind:      map[string]int{},
		ByReason:    s.ByReason,
	}
	for t, n := range s.ByTier {
		out.ByTier[t.String()] = n
	}
	for k, n := range s.ByKind {
		out.ByKind[string(k)] = n
	}
	return out
}

func renderSalienceText(stdout io.Writer, outcomes []salience.Outcome) int {
	summary := salience.Summarize(outcomes)

	tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "TIER\tKIND\tSUBJECT\tDECISION\tREASON")
	for _, o := range outcomes {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			o.Signal.Salience, o.Signal.Kind, truncate(o.Signal.Subject, 40),
			decisionLabel(o), o.Reason)
	}
	_ = tw.Flush()

	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "total: %d  notified: %d  suppressed: %d  notify-ratio: %.0f%%\n",
		summary.Total, summary.Notified, summary.Suppressed, 100*summary.NotifyRatio)

	tiers := sortedTiers(summary.ByTier)
	if len(tiers) > 0 {
		fmt.Fprintln(stdout, "by tier:")
		for _, t := range tiers {
			fmt.Fprintf(stdout, "  %-9s %d\n", t.String(), summary.ByTier[t])
		}
	}
	if len(summary.ByReason) > 0 {
		fmt.Fprintln(stdout, "suppressed because:")
		for _, r := range sortedKeys(summary.ByReason) {
			fmt.Fprintf(stdout, "  %-20s %d\n", r, summary.ByReason[r])
		}
	}
	return 0
}

func decisionLabel(o salience.Outcome) string {
	switch {
	case o.Notify:
		return "notify"
	case o.Suppressed:
		return "suppress"
	default:
		return "skip"
	}
}

func truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func sortedTiers(m map[salience.Tier]int) []salience.Tier {
	out := make([]salience.Tier, 0, len(m))
	for t := range m {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] > out[j] })
	return out
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

