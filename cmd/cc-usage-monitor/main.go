// Command cc-usage-monitor reports Claude Code token and cost usage by reading
// the local transcript files under ~/.claude/projects/**/*.jsonl, emitting the
// totals as OpenTelemetry metrics, and raising a burn-rate alert when the
// projected 24-hour spend crosses a threshold.
//
// It is the usage half of the VROOM Overseer's monitoring loop and exists so
// the mesh can see supervisor burn BEFORE any Opus upgrade lifts it — a quota
// blow-out should be visible as a trend, not discovered as a 429 (bead ce-bkxa).
//
// Usage:
//
//	cc-usage-monitor                                  # human-readable summary
//	cc-usage-monitor --json                           # JSON result object
//	cc-usage-monitor --burn-threshold 50              # projected-24h $ alert ceiling
//	cc-usage-monitor --burn-window 1h                 # window used for the projection
//	cc-usage-monitor --projects ~/.claude/projects    # transcript root
//	cc-usage-monitor --trail ~/.agm/vroom/trail.jsonl # where burn alerts land
//
// Metrics emitted (via internal/telemetry.InitMeter, OTLP gRPC to
// OTEL_EXPORTER_OTLP_ENDPOINT; a no-op provider when that env var is unset):
//
//	cc.tokens.input   {session_type}   input tokens
//	cc.tokens.output  {session_type}   output tokens
//	cc.cost.usd       {session_type}   USD cost (explicit costUSD, else estimated)
//
// When --trail is set AND the projected 24h cost exceeds --burn-threshold, one
// "quota.burn.alert" record is appended to the named decision trail and a WARN
// line is logged. The trail write is best-effort: a failure is reported on
// stderr but never changes the exit code.
//
// Exit codes:
//
//	0  scanned cleanly, projection under threshold
//	1  scanned cleanly, projection OVER threshold (burn alert fired)
//	2  usage / flag / scan error
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/vbonnet/dear-agent/internal/telemetry"
	"github.com/vbonnet/dear-agent/pkg/vroom/decisiontrail"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	instrumentationScope = "github.com/vbonnet/dear-agent/cmd/cc-usage-monitor"
	serviceName          = "cc-usage-monitor"

	exitOK    = 0
	exitBurn  = 1
	exitError = 2
)

func main() {
	code, err := run(os.Args[1:], os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cc-usage-monitor:", err)
		os.Exit(exitError)
	}
	os.Exit(code)
}

type config struct {
	jsonOutput    bool
	projectsDir   string
	trailPath     string
	burnThreshold float64
	burnWindow    time.Duration
	inputPrice    float64
	outputPrice   float64
}

func run(args []string, stdout, stderr io.Writer) (int, error) {
	fs := flag.NewFlagSet("cc-usage-monitor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfg := config{}
	fs.BoolVar(&cfg.jsonOutput, "json", false, "emit JSON instead of a human-readable summary")
	fs.StringVar(&cfg.projectsDir, "projects", defaultProjectsDir(),
		"root of Claude Code transcripts (default ~/.claude/projects)")
	fs.StringVar(&cfg.trailPath, "trail", defaultTrailPath(),
		"append a quota.burn.alert record here when projected 24h cost exceeds the threshold")
	fs.Float64Var(&cfg.burnThreshold, "burn-threshold", 50.0,
		"projected 24h cost (USD) above which a burn alert fires")
	fs.DurationVar(&cfg.burnWindow, "burn-window", time.Hour,
		"trailing window used to project the 24h burn rate")
	fs.Float64Var(&cfg.inputPrice, "input-price", DefaultPricing.InputPerMTok,
		"USD per 1M input tokens, used to estimate cost when a transcript line has no costUSD")
	fs.Float64Var(&cfg.outputPrice, "output-price", DefaultPricing.OutputPerMTok,
		"USD per 1M output tokens, used to estimate cost when a transcript line has no costUSD")
	if err := fs.Parse(args); err != nil {
		return exitError, nil //nolint:nilerr // flag already printed the error to stderr
	}

	pricing := Pricing{InputPerMTok: cfg.inputPrice, OutputPerMTok: cfg.outputPrice}

	records, files, err := ScanProjects(cfg.projectsDir, pricing)
	if err != nil {
		return exitError, fmt.Errorf("scanning %q: %w", cfg.projectsDir, err)
	}

	res := aggregate(records, time.Now(), cfg.burnWindow)
	res.FilesScanned = files

	// Metrics first: the OTLP path is the durable record even when the trail
	// write or the human summary is unavailable.
	emitMetrics(context.Background(), res)

	burning := res.Projected24hUSD > cfg.burnThreshold
	if burning {
		logger := log.New(stderr, "", log.LstdFlags)
		logger.Printf("WARN quota.burn.alert projected_24h=$%.2f threshold=$%.2f window=%s window_cost=$%.4f",
			res.Projected24hUSD, cfg.burnThreshold, cfg.burnWindow, res.WindowCostUSD)
		if cfg.trailPath != "" {
			if err := appendBurnAlert(context.Background(), cfg.trailPath, res, cfg.burnThreshold); err != nil {
				fmt.Fprintln(stderr, "cc-usage-monitor: trail append failed:", err)
			}
		}
	}

	if cfg.jsonOutput {
		if err := emitJSON(stdout, res, cfg.burnThreshold, burning); err != nil {
			return exitError, err
		}
	} else {
		printSummary(stdout, res, cfg.burnThreshold, burning)
	}

	if burning {
		return exitBurn, nil
	}
	return exitOK, nil
}

// emitMetrics publishes the per-cohort token and cost counters. Counter
// instrument errors fall back to no-op instruments rather than failing the run.
func emitMetrics(ctx context.Context, res Result) {
	shutdown, err := telemetry.InitMeter(serviceName)
	if err == nil {
		defer func() {
			sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if serr := shutdown(sctx); serr != nil {
				fmt.Fprintln(os.Stderr, "cc-usage-monitor: meter shutdown:", serr)
			}
		}()
	}

	m := otel.GetMeterProvider().Meter(instrumentationScope)
	inputCtr, _ := m.Int64Counter("cc.tokens.input",
		metric.WithDescription("Claude Code input tokens, by session type"), metric.WithUnit("{token}"))
	outputCtr, _ := m.Int64Counter("cc.tokens.output",
		metric.WithDescription("Claude Code output tokens, by session type"), metric.WithUnit("{token}"))
	costCtr, _ := m.Float64Counter("cc.cost.usd",
		metric.WithDescription("Claude Code cost in USD, by session type"), metric.WithUnit("USD"))

	for _, st := range sortedTypes(res.ByType) {
		u := res.ByType[st]
		attrs := metric.WithAttributes(attribute.String("session_type", st))
		inputCtr.Add(ctx, u.InputTokens, attrs)
		outputCtr.Add(ctx, u.OutputTokens, attrs)
		costCtr.Add(ctx, u.CostUSD, attrs)
	}
}

func appendBurnAlert(ctx context.Context, path string, res Result, threshold float64) error {
	trail, err := decisiontrail.OpenJSONL(path)
	if err != nil {
		return err
	}
	defer trail.Close()

	return trail.Append(ctx, decisiontrail.Record{
		Role: "overseer",
		Kind: "quota.burn.alert",
		Payload: map[string]any{
			"projected_24h_usd": res.Projected24hUSD,
			"threshold_usd":     threshold,
			"window":            res.BurnWindow.String(),
			"window_cost_usd":   res.WindowCostUSD,
			"total_cost_usd":    res.Total.CostUSD,
			"by_type":           res.ByType,
			"files_scanned":     res.FilesScanned,
			"records_counted":   res.RecordsCounted,
		},
	})
}

func emitJSON(w io.Writer, res Result, threshold float64, burning bool) error {
	payload := map[string]any{
		"result":        res,
		"threshold_usd": threshold,
		"burn_alert":    burning,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func printSummary(w io.Writer, res Result, threshold float64, burning bool) {
	fmt.Fprintf(w, "cc-usage-monitor: scanned %d files, %d usage records\n", res.FilesScanned, res.RecordsCounted)
	for _, st := range sortedTypes(res.ByType) {
		u := res.ByType[st]
		fmt.Fprintf(w, "  %-11s in=%d out=%d cost=$%.4f\n", st, u.InputTokens, u.OutputTokens, u.CostUSD)
	}
	fmt.Fprintf(w, "  %-11s in=%d out=%d cost=$%.4f\n", "TOTAL", res.Total.InputTokens, res.Total.OutputTokens, res.Total.CostUSD)
	status := "ok"
	if burning {
		status = "BURN ALERT"
	}
	fmt.Fprintf(w, "  burn: window=%s window_cost=$%.4f projected_24h=$%.2f threshold=$%.2f [%s]\n",
		res.BurnWindow, res.WindowCostUSD, res.Projected24hUSD, threshold, status)
}

func sortedTypes(byType map[string]*Usage) []string {
	out := make([]string, 0, len(byType))
	for k := range byType {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func defaultProjectsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".claude/projects"
	}
	return filepath.Join(home, ".claude", "projects")
}

func defaultTrailPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".agm", "vroom", "trail.jsonl")
}
