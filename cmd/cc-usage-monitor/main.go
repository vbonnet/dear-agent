// Command cc-usage-monitor is a Go-native Claude Code usage monitor. It parses
// the local Claude Code transcripts under ~/.claude/projects/**/*.jsonl,
// aggregates token counts and estimated USD cost by session type (worker vs
// supervisor) and by model, emits the aggregates as OpenTelemetry metrics via
// the shared OTLP transport, and raises a burn-rate alert when the projected
// 24-hour cost crosses a threshold.
//
// It exists so the VROOM mesh can watch quota burn BEFORE upgrading supervisors
// to a more expensive model — the upgrade carries real quota risk and must not
// land blind (bead ce-bkxa).
//
// Metrics (emitted only when OTEL_EXPORTER_OTLP_ENDPOINT is set; otherwise the
// meter is a no-op and the binary still prints/alerts normally):
//
//	cc.tokens.input   {session_type}  input tokens consumed
//	cc.tokens.output  {session_type}  output tokens produced
//	cc.cost.usd       {session_type}  estimated USD cost
//
// Burn-rate alert: when the projected 24h cost exceeds --threshold (default
// $50), a WARN line is logged and one record with kind "quota.burn.alert" is
// appended to the VROOM decision trail (default ~/.agm/vroom/trail.jsonl). The
// trail write is best-effort: a failure is reported on stderr but never changes
// the exit code, so a monitoring tick keeps going even when the trail is
// unwritable.
//
// Usage:
//
//	cc-usage-monitor                       # scan, emit metrics, alert if burning
//	cc-usage-monitor --json                # machine-readable report on stdout
//	cc-usage-monitor --threshold 100       # alert above $100/day projected
//	cc-usage-monitor --window 2h           # burn-rate window (default 1h)
//	cc-usage-monitor --root /path/projects # override transcript root
//	cc-usage-monitor --no-emit             # skip OTel emission (scan + alert only)
//
// Exit codes:
//
//	0  scan completed (whether or not an alert fired)
//	1  burn-rate alert fired (projected 24h cost over threshold)
//	2  usage / flag / fatal scan error
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/vbonnet/dear-agent/internal/telemetry"
	"github.com/vbonnet/dear-agent/pkg/vroom/decisiontrail"
)

const (
	exitOK    = 0
	exitAlert = 1
	exitUsage = 2

	defaultThreshold = 50.0      // USD/day projected
	defaultWindow    = time.Hour // burn-rate sampling window
	alertKind        = "quota.burn.alert"
	alertRole        = "overseer"
	serviceName      = "cc-usage-monitor"
	meterScope       = "github.com/vbonnet/dear-agent/cc-usage-monitor"
)

func main() {
	code := run(os.Args[1:], os.Stdout, os.Stderr, time.Now())
	os.Exit(code)
}

type config struct {
	root       string
	trailPath  string
	threshold  float64
	window     time.Duration
	jsonOutput bool
	noEmit     bool
}

func run(args []string, stdout, stderr io.Writer, now time.Time) int {
	fs := flag.NewFlagSet(serviceName, flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfg := config{}
	fs.StringVar(&cfg.root, "root", defaultRoot(), "root dir of Claude Code transcripts")
	fs.StringVar(&cfg.trailPath, "trail", defaultTrail(), "VROOM decision trail to append burn alerts to")
	fs.Float64Var(&cfg.threshold, "threshold", defaultThreshold, "projected 24h USD cost that triggers a burn alert")
	fs.DurationVar(&cfg.window, "window", defaultWindow, "trailing window used to project the 24h burn rate")
	fs.BoolVar(&cfg.jsonOutput, "json", false, "emit the report as JSON on stdout")
	fs.BoolVar(&cfg.noEmit, "no-emit", false, "skip OpenTelemetry metric emission (scan + alert only)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	logger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	report, entries, err := ScanDir(cfg.root)
	if err != nil {
		fmt.Fprintln(stderr, serviceName+":", err)
		return exitUsage
	}

	projected := ProjectedDailyCost(entries, now, cfg.window)

	if !cfg.noEmit {
		if emitErr := emitMetrics(context.Background(), report, stderr); emitErr != nil {
			// Emission is best-effort: a missing/broken collector must not
			// break the scan-and-alert path.
			fmt.Fprintln(stderr, serviceName+": metric emission failed:", emitErr)
		}
	}

	alerted := projected > cfg.threshold
	if alerted {
		logger.Warn("claude code quota burn-rate alert",
			"projected_24h_usd", round2(projected),
			"threshold_usd", cfg.threshold,
			"window", cfg.window.String(),
			"total_cost_usd", round2(report.Total.CostUSD),
		)
		if cfg.trailPath != "" {
			if err := appendBurnAlert(context.Background(), cfg.trailPath, report, projected, cfg); err != nil {
				fmt.Fprintln(stderr, serviceName+": trail append failed:", err)
			}
		}
	}

	if cfg.jsonOutput {
		if err := emitJSON(stdout, report, projected, cfg, alerted); err != nil {
			fmt.Fprintln(stderr, serviceName+":", err)
			return exitUsage
		}
	} else {
		printHuman(stdout, report, projected, cfg, alerted)
	}

	if alerted {
		return exitAlert
	}
	return exitOK
}

// emitMetrics installs the OTel meter and reports the per-session-type
// aggregates as observable gauges, then flushes via Shutdown. When no OTLP
// endpoint is configured InitMeter installs a no-op provider, so this is a
// silent no-op rather than an error.
func emitMetrics(ctx context.Context, report *Report, stderr io.Writer) error {
	shutdown, err := telemetry.InitMeter(serviceName)
	if err != nil {
		return err
	}
	defer func() {
		if shErr := shutdown(ctx); shErr != nil {
			fmt.Fprintln(stderr, serviceName+": meter shutdown failed:", shErr)
		}
	}()

	meter := otel.GetMeterProvider().Meter(meterScope)
	inputG, err := meter.Int64ObservableGauge("cc.tokens.input",
		metric.WithDescription("Claude Code input tokens consumed, by session type"),
		metric.WithUnit("{token}"))
	if err != nil {
		return err
	}
	outputG, err := meter.Int64ObservableGauge("cc.tokens.output",
		metric.WithDescription("Claude Code output tokens produced, by session type"),
		metric.WithUnit("{token}"))
	if err != nil {
		return err
	}
	costG, err := meter.Float64ObservableGauge("cc.cost.usd",
		metric.WithDescription("Estimated Claude Code USD cost, by session type"),
		metric.WithUnit("USD"))
	if err != nil {
		return err
	}

	_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		for st, agg := range report.ByType {
			attrs := metric.WithAttributes(attribute.String("session_type", string(st)))
			o.ObserveInt64(inputG, agg.Input, attrs)
			o.ObserveInt64(outputG, agg.Output, attrs)
			o.ObserveFloat64(costG, agg.CostUSD, attrs)
		}
		return nil
	}, inputG, outputG, costG)
	return err
}

// appendBurnAlert writes one quota.burn.alert record to the decision trail.
func appendBurnAlert(ctx context.Context, path string, report *Report, projected float64, cfg config) error {
	trail, err := decisiontrail.OpenJSONL(path)
	if err != nil {
		return err
	}
	defer trail.Close()

	byType := map[string]float64{}
	for st, agg := range report.ByType {
		byType[string(st)] = round2(agg.CostUSD)
	}
	return trail.Append(ctx, decisiontrail.Record{
		Role: alertRole,
		Kind: alertKind,
		Payload: map[string]any{
			"projected_24h_usd": round2(projected),
			"threshold_usd":     cfg.threshold,
			"window":            cfg.window.String(),
			"total_cost_usd":    round2(report.Total.CostUSD),
			"cost_by_type":      byType,
			"messages":          report.Total.Messages,
		},
	})
}

func emitJSON(w io.Writer, report *Report, projected float64, cfg config, alerted bool) error {
	payload := map[string]any{
		"report":            report,
		"projected_24h_usd": round2(projected),
		"threshold_usd":     cfg.threshold,
		"window":            cfg.window.String(),
		"alerted":           alerted,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func printHuman(w io.Writer, report *Report, projected float64, cfg config, alerted bool) {
	fmt.Fprintf(w, "Claude Code usage — %d files, %d messages\n", report.Files, report.Total.Messages)
	fmt.Fprintf(w, "  total: in=%d out=%d cache_w=%d cache_r=%d  est $%.2f\n",
		report.Total.Input, report.Total.Output, report.Total.CacheWrite, report.Total.CacheRead, report.Total.CostUSD)

	// Deterministic ordering for readability.
	types := make([]string, 0, len(report.ByType))
	for st := range report.ByType {
		types = append(types, string(st))
	}
	sort.Strings(types)
	for _, st := range types {
		a := report.ByType[SessionType(st)]
		fmt.Fprintf(w, "  %-11s in=%d out=%d  est $%.2f  (%d msgs)\n", st, a.Input, a.Output, a.CostUSD, a.Messages)
	}

	fmt.Fprintf(w, "  projected 24h burn: $%.2f (window %s, threshold $%.2f)\n",
		projected, cfg.window.String(), cfg.threshold)
	if alerted {
		fmt.Fprintln(w, "  ⚠ BURN ALERT: projected 24h cost exceeds threshold")
	}
}

func round2(f float64) float64 {
	return float64(int64(f*100+0.5)) / 100
}

func defaultRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".claude/projects"
	}
	return filepath.Join(home, ".claude", "projects")
}

func defaultTrail() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".agm", "vroom", "trail.jsonl")
}
