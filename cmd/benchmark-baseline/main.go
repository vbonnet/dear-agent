// Command benchmark-baseline captures a pre-flywheel baseline by combining
// (a) a structural benchmark run using the SWE-Bench Lite fixture suite with
// the StubExecutor, and (b) an operational snapshot from the Jaeger tracing
// API.
//
// The output is a single JSON file intended to be committed to
// benchmarks/baselines/ so that future flywheel cycles have a measurable
// starting point.
//
// Usage:
//
//	benchmark-baseline \
//	  [--jaeger-url http://localhost:16686] \
//	  [--lookback 720h] \
//	  [--suite swe-bench-lite] \
//	  [--output benchmarks/baselines/pre-flywheel.json]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/pkg/benchmarks"
)

// Baseline is the top-level output written to disk.
type Baseline struct {
	Version     string               `json:"version"`
	CapturedAt  time.Time            `json:"captured_at"`
	GitSHA      string               `json:"git_sha,omitempty"`
	Benchmark   *benchmarks.Results  `json:"benchmark"`
	Operational *OperationalSnapshot `json:"operational"`
	Notes       []string             `json:"notes,omitempty"`
}

// OperationalSnapshot holds metrics scraped from Jaeger.
type OperationalSnapshot struct {
	JaegerURL string         `json:"jaeger_url"`
	Lookback  string         `json:"lookback"`
	Services  []string       `json:"services"`
	Spans     SpanMetrics    `json:"spans"`
	Notes     []string       `json:"notes,omitempty"`
}

// SpanMetrics aggregates span-level data into summary metrics.
type SpanMetrics struct {
	TraceCount    int            `json:"trace_count"`
	SpanCount     int            `json:"span_count"`
	ByOperation   map[string]int `json:"by_operation"`
	BySessionState map[string]int `json:"by_session_state,omitempty"`
	ErrorCount    int            `json:"error_count"`
	SuccessRate   float64        `json:"success_rate"`
	AvgDurationMs float64        `json:"avg_duration_ms"`
}

// jaegerTag is a single key/value pair in a Jaeger span.
type jaegerTag struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

// jaegerSpan holds the fields we read from a Jaeger span object.
type jaegerSpan struct {
	OperationName string      `json:"operationName"`
	Duration      int64       `json:"duration"` // microseconds
	Tags          []jaegerTag `json:"tags"`
}

// jaegerTrace holds one trace from the Jaeger HTTP response.
type jaegerTrace struct {
	TraceID string       `json:"traceID"`
	Spans   []jaegerSpan `json:"spans"`
}

// jaegerResp is a partial decode of the Jaeger HTTP query response.
type jaegerResp struct {
	Data []jaegerTrace `json:"data"`
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("benchmark-baseline", flag.ContinueOnError)
	var (
		jaegerURL = fs.String("jaeger-url", "http://localhost:16686", "Jaeger HTTP API base URL")
		lookback  = fs.String("lookback", "720h", "Jaeger lookback window (Go duration, e.g. 720h = 30 days)")
		suiteName = fs.String("suite", "swe-bench-lite", "benchmark suite (swe-bench-lite|swe-bench-verified|swe-atlas|vibe-bench)")
		output    = fs.String("output", "benchmarks/baselines/pre-flywheel.json", "output path (use - for stdout)")
		verbose   = fs.Bool("verbose", false, "debug logging")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	ctx := context.Background()

	// 1. Structural benchmark (fixture dataset + stub executor).
	benchResults, err := runStructural(ctx, logger, benchmarks.Suite(*suiteName))
	if err != nil {
		logger.Error("structural benchmark failed", "err", err)
		return 1
	}

	// 2. Operational snapshot from Jaeger (best-effort).
	ops := scrapeJaeger(ctx, logger, *jaegerURL, *lookback)

	baseline := &Baseline{
		Version:     "1",
		CapturedAt:  time.Now().UTC(),
		GitSHA:      headSHA(),
		Benchmark:   benchResults,
		Operational: ops,
		Notes: []string{
			"stub executor used — solve_rate=0 is the day-0 structural marker, not a real score",
			"rerun with a real TaskExecutor wired to measure true solve rate improvement",
		},
	}

	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		logger.Error("marshal baseline", "err", err)
		return 1
	}

	if *output == "-" {
		fmt.Println(string(data))
		return 0
	}

	if dir := dirOf(*output); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			logger.Error("create output dir", "path", dir, "err", err)
			return 1
		}
	}
	if err := os.WriteFile(*output, data, 0o600); err != nil {
		logger.Error("write baseline", "path", *output, "err", err)
		return 1
	}

	logger.Info("baseline written", "path", *output)
	printSummary(os.Stdout, baseline)
	return 0
}

func runStructural(ctx context.Context, logger *slog.Logger, suite benchmarks.Suite) (*benchmarks.Results, error) {
	b, err := benchmarks.DefaultRegistry.Get(suite)
	if err != nil {
		return nil, fmt.Errorf("suite %q not registered: %w", suite, err)
	}
	logger.Info("running fixture benchmark", "suite", suite)
	results, err := b.Run(ctx, benchmarks.RunConfig{
		Mode:  benchmarks.ModeDearAgent,
		Model: "stub",
	})
	if err != nil {
		return nil, err
	}
	s := results.Summary
	logger.Info("fixture benchmark done",
		"total", s.Total, "solved", s.Solved,
		"errored", s.Errored, "solve_rate", s.SolveRate,
	)
	return results, nil
}

func scrapeJaeger(ctx context.Context, logger *slog.Logger, baseURL, lookback string) *OperationalSnapshot {
	snap := &OperationalSnapshot{JaegerURL: baseURL, Lookback: lookback}

	for _, svc := range []string{"agm", "dear-agent"} {
		traces, err := fetchJaegerTraces(ctx, baseURL, svc, lookback)
		if err != nil {
			logger.Debug("jaeger query skipped", "service", svc, "err", err)
			continue
		}
		if len(traces.Data) == 0 {
			continue
		}
		snap.Services = append(snap.Services, svc)
		mergeTraces(&snap.Spans, traces)
	}

	if snap.Spans.TraceCount == 0 {
		snap.Notes = append(snap.Notes,
			fmt.Sprintf("no traces found in %s lookback — OTel end-to-end not yet wired", lookback),
		)
	} else {
		snap.Notes = append(snap.Notes,
			fmt.Sprintf("%d traces / %d spans across %d service(s) (pre-flywheel baseline)",
				snap.Spans.TraceCount, snap.Spans.SpanCount, len(snap.Services)),
		)
	}
	return snap
}

func fetchJaegerTraces(ctx context.Context, baseURL, service, lookback string) (*jaegerResp, error) {
	u, err := url.Parse(baseURL + "/api/traces")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("service", service)
	q.Set("limit", "1000")
	q.Set("lookback", lookback)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var jr jaegerResp
	if err := json.Unmarshal(body, &jr); err != nil {
		return nil, err
	}
	return &jr, nil
}

func mergeTraces(m *SpanMetrics, jr *jaegerResp) {
	if m.ByOperation == nil {
		m.ByOperation = make(map[string]int)
	}
	if m.BySessionState == nil {
		m.BySessionState = make(map[string]int)
	}

	var totalDurUs int64
	for _, t := range jr.Data {
		m.TraceCount++
		for _, s := range t.Spans {
			m.SpanCount++
			m.ByOperation[s.OperationName]++
			totalDurUs += s.Duration
			applySpanTags(m, s.Tags)
		}
	}

	if m.SpanCount > 0 {
		m.AvgDurationMs = float64(totalDurUs) / float64(m.SpanCount) / 1000.0
		nonError := m.SpanCount - m.ErrorCount
		if nonError < 0 {
			nonError = 0
		}
		m.SuccessRate = float64(nonError) / float64(m.SpanCount)
	}
}

// applySpanTags updates m with signal from a single span's tag list.
func applySpanTags(m *SpanMetrics, tags []jaegerTag) {
	for _, tag := range tags {
		switch tag.Key {
		case "session.state":
			if v, ok := tag.Value.(string); ok {
				m.BySessionState[v]++
			}
		case "otel.status_code":
			if v, ok := tag.Value.(string); ok && strings.EqualFold(v, "error") {
				m.ErrorCount++
			}
		case "error":
			if v, ok := tag.Value.(bool); ok && v {
				m.ErrorCount++
			}
		}
	}
}

func headSHA() string {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func dirOf(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[:i]
	}
	return "."
}

func printSummary(w io.Writer, b *Baseline) {
	fmt.Fprintf(w, "\n=== Pre-Flywheel Baseline (%s) ===\n", b.CapturedAt.Format("2006-01-02"))
	fmt.Fprintf(w, "Git SHA     : %s\n", b.GitSHA)
	if bm := b.Benchmark; bm != nil {
		s := bm.Summary
		fmt.Fprintf(w, "Suite       : %s  mode=%s\n", bm.Suite, bm.Mode)
		fmt.Fprintf(w, "Tasks       : %d total, %d solved, %d errored  (solve_rate=%.1f%%)\n",
			s.Total, s.Solved, s.Errored, s.SolveRate*100)
	}
	if op := b.Operational; op != nil {
		sm := op.Spans
		fmt.Fprintf(w, "Jaeger      : %d traces / %d spans  services=%v\n",
			sm.TraceCount, sm.SpanCount, op.Services)
		if sm.SpanCount > 0 {
			fmt.Fprintf(w, "              success_rate=%.1f%%  avg_dur=%.2fms  errors=%d\n",
				sm.SuccessRate*100, sm.AvgDurationMs, sm.ErrorCount)
		}
		for _, n := range op.Notes {
			fmt.Fprintf(w, "              note: %s\n", n)
		}
	}
	for _, n := range b.Notes {
		fmt.Fprintf(w, "Note        : %s\n", n)
	}
	fmt.Fprintln(w)
}
