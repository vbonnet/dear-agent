// Command jaeger-health checks that a local Jaeger instance is alive and
// receiving traces. It is designed for use as a scheduled-task health probe.
//
// What it checks:
//
//  1. Jaeger responds to /api/services (liveness).
//  2. At least one trace has arrived in the configured lookback window
//     across all active services (readiness).
//
// Exit codes:
//
//	0  healthy  — Jaeger alive and recent traces present
//	1  degraded — Jaeger alive but no traces in the lookback window
//	2  down     — Jaeger unreachable or returned an unexpected error
//	3  usage    — bad flags
//
// Usage:
//
//	jaeger-health [--url http://localhost:16686] [--lookback 1h] [--json]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Report is the machine-readable output emitted with --json.
type Report struct {
	CheckedAt  string   `json:"checked_at"`
	JaegerURL  string   `json:"jaeger_url"`
	Status     string   `json:"status"` // "healthy" | "degraded" | "down"
	Alive      bool     `json:"alive"`
	Services   []string `json:"services"`
	TraceCount int      `json:"trace_count"` // traces seen across all services in the lookback window
	Lookback   string   `json:"lookback"`
	Error      string   `json:"error,omitempty"`
}

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	fs := flag.NewFlagSet("jaeger-health", flag.ContinueOnError)
	jaegerURL := fs.String("url", "http://localhost:16686", "Jaeger HTTP API base URL")
	lookback := fs.String("lookback", "1h", "lookback window for trace check (e.g. 1h, 24h, 168h)")
	timeout := fs.Duration("timeout", 5*time.Second, "per-request HTTP timeout")
	asJSON := fs.Bool("json", false, "emit a JSON report to stdout instead of a human summary")
	if err := fs.Parse(args); err != nil {
		return 3
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout*2)
	defer cancel()

	client := &http.Client{Timeout: *timeout}

	r := Report{
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
		JaegerURL: *jaegerURL,
		Lookback:  *lookback,
	}

	apiURL := strings.TrimSuffix(*jaegerURL, "/")

	// 1. Liveness — can we reach /api/services?
	svcs, err := fetchServices(ctx, client, apiURL)
	if err != nil {
		r.Status = "down"
		r.Error = err.Error()
		return emit(r, *asJSON, fmt.Sprintf("DOWN — Jaeger unreachable at %s: %v", *jaegerURL, err), 2)
	}
	r.Alive = true
	r.Services = svcs

	// 2. Readiness — any traces in the lookback window?
	for _, svc := range svcs {
		n, err := countTraces(ctx, client, apiURL, svc, *lookback)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			fmt.Fprintf(os.Stderr, "warn: trace query for %q failed: %v\n", svc, err)
			continue
		}
		r.TraceCount += n
	}

	if r.TraceCount == 0 {
		r.Status = "degraded"
		svcList := strings.Join(svcs, ", ")
		msg := fmt.Sprintf("DEGRADED — Jaeger alive, services=[%s], but no traces in last %s", svcList, *lookback)
		return emit(r, *asJSON, msg, 1)
	}

	r.Status = "healthy"
	svcList := strings.Join(svcs, ", ")
	msg := fmt.Sprintf("HEALTHY — Jaeger alive, services=[%s], %d traces in last %s",
		svcList, r.TraceCount, *lookback)
	return emit(r, *asJSON, msg, 0)
}

// fetchServices calls /api/services and returns the list of known service names.
func fetchServices(ctx context.Context, c *http.Client, base string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/services", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET /api/services returned %d", resp.StatusCode)
	}
	var sr struct {
		Data   []string `json:"data"`
		Errors []string `json:"errors,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("decode /api/services: %w", err)
	}
	if len(sr.Errors) > 0 {
		return nil, fmt.Errorf("jaeger errors: %s", strings.Join(sr.Errors, "; "))
	}
	return sr.Data, nil
}

// countTraces queries /api/traces for a single service and returns the number
// of traces in the lookback window (capped at the query limit of 100).
func countTraces(ctx context.Context, c *http.Client, base, service, lookback string) (int, error) {
	u, err := url.Parse(base + "/api/traces")
	if err != nil {
		return 0, err
	}
	q := u.Query()
	q.Set("service", service)
	q.Set("limit", "100")
	q.Set("lookback", lookback)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("GET /api/traces returned %d", resp.StatusCode)
	}
	var tr struct {
		Data []struct{} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return 0, fmt.Errorf("decode /api/traces: %w", err)
	}
	return len(tr.Data), nil
}

// emit outputs either a JSON report or a human summary line, then returns the
// exit code so callers can return emit(...) directly.
func emit(r Report, asJSON bool, summary string, code int) int {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(r)
	} else {
		fmt.Fprintln(os.Stdout, summary)
	}
	return code
}
