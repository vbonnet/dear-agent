package checks

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/pkg/audit"
)

// OTelSpansCheck reads JSONL span files from the engram traces directory and
// emits findings for error-status spans (P1) and critically-slow spans (P2).
// It registers itself with audit.Default in init() so the runner picks it up
// without explicit wiring.
//
// Config keys (all optional):
//
//	traces_dir   string  — root to search for */spans.jsonl  (default: ~/.engram/traces)
//	lookback     string  — time.ParseDuration window         (default: "24h")
//	slow_threshold_ms float64 — ms above which a span is slow (default: 5000)
type OTelSpansCheck struct{}

// Meta returns the check's identity.
func (OTelSpansCheck) Meta() audit.CheckMeta {
	return audit.CheckMeta{
		ID:              "otel.span-errors",
		Cadence:         audit.CadenceDaily,
		SeverityCeiling: audit.SeverityP1,
		Description:     "OTel spans must not contain error-status or critically-slow operations within the lookback window",
	}
}

// jsonlSpan is a minimal local mirror of otelsetup.JSONLSpan — only the
// fields the check needs.  We avoid importing pkg/otelsetup to keep the
// dependency graph clean; the on-disk JSON format is stable.
type jsonlSpan struct {
	TraceID    string    `json:"trace_id"`
	SpanID     string    `json:"span_id"`
	Name       string    `json:"name"`
	StartTime  time.Time `json:"start_time"`
	DurationMs float64   `json:"duration_ms"`
	StatusCode string    `json:"status_code"`
	StatusMsg  string    `json:"status_message"`
}

// Run discovers spans.jsonl files under tracesDir, decodes spans within the
// lookback window, and emits findings.
func (c OTelSpansCheck) Run(ctx context.Context, env audit.Env) (audit.Result, error) {
	out := audit.Result{Status: audit.StatusOK}

	tracesDir, err := c.resolveTracesDir(env)
	if err != nil {
		out.Status = audit.StatusError
		return out, fmt.Errorf("audit/otel.span-errors: resolve traces dir: %w", err)
	}

	lookback, err := c.resolveLookback(env)
	if err != nil {
		out.Status = audit.StatusError
		return out, fmt.Errorf("audit/otel.span-errors: parse lookback: %w", err)
	}

	slowThreshold := c.resolveSlowThreshold(env)
	cutoff := env.Time().Add(-lookback)

	files, err := filepath.Glob(filepath.Join(tracesDir, "*", "spans.jsonl"))
	if err != nil {
		out.Status = audit.StatusError
		return out, fmt.Errorf("audit/otel.span-errors: glob: %w", err)
	}

	for _, f := range files {
		findings, err := c.processFile(ctx, f, cutoff, slowThreshold)
		if err != nil {
			if env.Logger != nil {
				env.Logger.Warn("otel.span-errors: skipping file", "file", f, "err", err)
			}
			continue
		}
		out.Findings = append(out.Findings, findings...)
	}

	return out, nil
}

// processFile reads one spans.jsonl file and returns findings for spans
// within the lookback window.
func (c OTelSpansCheck) processFile(
	_ context.Context,
	path string,
	cutoff time.Time,
	slowThresholdMs float64,
) ([]audit.Finding, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var findings []audit.Finding
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var s jsonlSpan
		if err := json.Unmarshal(line, &s); err != nil {
			continue // skip malformed lines
		}
		if s.StartTime.Before(cutoff) {
			continue // outside the lookback window
		}
		if s.StatusCode == "Error" {
			msg := s.StatusMsg
			if msg == "" {
				msg = "span recorded status=ERROR"
			}
			findings = append(findings, audit.Finding{
				CheckID:     "otel.span-errors",
				Fingerprint: audit.Fingerprint("otel.span-errors", s.TraceID, s.SpanID),
				Severity:    audit.SeverityP1,
				Title:       "OTel error span: " + s.Name,
				Detail:      msg,
				Evidence: map[string]any{
					"trace_id": s.TraceID,
					"span_id":  s.SpanID,
					"file":     path,
				},
			})
		}
		if s.DurationMs > slowThresholdMs {
			findings = append(findings, audit.Finding{
				CheckID:     "otel.span-errors",
				Fingerprint: audit.Fingerprint("otel.slow-span", s.TraceID, s.SpanID),
				Severity:    audit.SeverityP2,
				Title:       fmt.Sprintf("Slow OTel span: %s (%.0fms)", s.Name, s.DurationMs),
				Detail:      fmt.Sprintf("span duration %.0fms exceeds threshold %.0fms", s.DurationMs, slowThresholdMs),
				Evidence: map[string]any{
					"trace_id":    s.TraceID,
					"span_id":     s.SpanID,
					"duration_ms": s.DurationMs,
					"file":        path,
				},
			})
		}
	}
	return findings, scanner.Err()
}

// resolveTracesDir returns the traces directory from env.Config, falling back
// to ~/.engram/traces.
func (OTelSpansCheck) resolveTracesDir(env audit.Env) (string, error) {
	if d, _ := env.Config["traces_dir"].(string); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".engram", "traces"), nil
}

// resolveLookback parses the "lookback" config key, defaulting to 24h.
func (OTelSpansCheck) resolveLookback(env audit.Env) (time.Duration, error) {
	if s, _ := env.Config["lookback"].(string); s != "" {
		return time.ParseDuration(s)
	}
	return 24 * time.Hour, nil
}

// resolveSlowThreshold reads "slow_threshold_ms", defaulting to 5000.
func (OTelSpansCheck) resolveSlowThreshold(env audit.Env) float64 {
	if f, _ := env.Config["slow_threshold_ms"].(float64); f > 0 {
		return f
	}
	return 5000.0
}

func init() {
	audit.Default.MustRegister(OTelSpansCheck{})
}
