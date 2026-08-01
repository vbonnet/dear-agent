// Command flywheel-drift detects state drift in the AGM/dear-agent flywheel.
//
// Two checks run on every invocation:
//
//   - drift/stale_beads: open beads with no activity for >7 days
//   - drift/otel_traces: Jaeger traces for the "agm" service in the last 24h
//
// Usage:
//
//	flywheel-drift [flags]
//	flywheel-drift --json                      # machine-readable JSON
//	flywheel-drift --jaeger-url URL            # custom Jaeger base URL
//	flywheel-drift --stale-days N              # staleness threshold (default 7)
//	flywheel-drift --otel-hours N              # OTel lookback window (default 24)
//	flywheel-drift --otel-service SERVICE      # Jaeger service name (default "agm")
//
// Exit codes: 0 healthy, 1 warnings, 2 errors.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	healthchecker "github.com/vbonnet/dear-agent/pkg/health-checker"
)

func main() {
	os.Exit(run())
}

func run() int {
	var (
		jaegerURL = flag.String("jaeger-url", "http://localhost:16686", "Jaeger base URL")
		staleDays = flag.Int("stale-days", 7, "days before an open bead is considered stale")
		otelHours = flag.Int("otel-hours", 24, "hours to look back for OTel traces")
		otelSvc   = flag.String("otel-service", "agm", "OTel service name to check in Jaeger")
		asJSON    = flag.Bool("json", false, "emit machine-readable JSON")
	)
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	runner := healthchecker.NewRunner(
		&beadsStaleCheck{staleDays: *staleDays},
		&jaegerHealthCheck{
			baseURL:  *jaegerURL,
			service:  *otelSvc,
			lookback: time.Duration(*otelHours) * time.Hour,
		},
	).WithParallel(true)

	results, err := runner.RunAll(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error running checks: %v\n", err)
		return 1
	}

	if *asJSON {
		exitCode, encErr := writeJSONReport(os.Stdout, results)
		if encErr != nil {
			fmt.Fprintf(os.Stderr, "encode: %v\n", encErr)
			return 1
		}
		return exitCode
	} else {
		printReport(results)
	}

	return healthchecker.Summarize(results).ExitCode()
}

// jsonResult is a JSON-serializable view of a healthchecker.Result that omits
// the Fix.Apply function field, which cannot be marshaled.
type jsonResult struct {
	Name     string               `json:"name"`
	Category string               `json:"category"`
	Status   healthchecker.Status `json:"status"`
	Message  string               `json:"message"`
	Fixable  bool                 `json:"fixable,omitempty"`
}

func writeJSONReport(w io.Writer, results []healthchecker.Result) (int, error) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(toJSONResults(results)); err != nil {
		return 0, err
	}
	return healthchecker.Summarize(results).ExitCode(), nil
}

func toJSONResults(results []healthchecker.Result) []jsonResult {
	out := make([]jsonResult, len(results))
	for i, r := range results {
		out[i] = jsonResult{
			Name:     r.Name,
			Category: r.Category,
			Status:   r.Status,
			Message:  r.Message,
			Fixable:  r.Fixable,
		}
	}
	return out
}

func printReport(results []healthchecker.Result) {
	summary := healthchecker.Summarize(results)
	fmt.Printf("Flywheel Drift Report — %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("Status: %s  (%d checks: %d ok, %d warn, %d error)\n\n",
		summary.OverallStatus(), summary.Total, summary.Passed, summary.Warnings, summary.Errors)
	for _, r := range results {
		var icon string
		switch r.Status {
		case healthchecker.StatusOK:
			icon = "✓"
		case healthchecker.StatusInfo:
			icon = "ℹ"
		case healthchecker.StatusWarning:
			icon = "⚠"
		case healthchecker.StatusError:
			icon = "✗"
		}
		fmt.Printf("  %s [%s/%s] %s\n", icon, r.Category, r.Name, r.Message)
	}
	fmt.Println()
}

// beadEntry is the subset of the bd JSON we need.
type beadEntry struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	UpdatedAt time.Time `json:"updated_at"`
}

// beadsStaleCheck flags open beads that have had no activity for > staleDays.
type beadsStaleCheck struct {
	staleDays int
}

func (c *beadsStaleCheck) Name() string     { return "stale_beads" }
func (c *beadsStaleCheck) Category() string { return "drift" }

func (c *beadsStaleCheck) Run(ctx context.Context) healthchecker.Result {
	out, err := exec.CommandContext(ctx, "bd", "list", "--status", "open", "--json").Output()
	if err != nil {
		return healthchecker.Result{
			Name:     c.Name(),
			Category: c.Category(),
			Status:   healthchecker.StatusError,
			Message:  fmt.Sprintf("bd list failed: %v", err),
		}
	}

	beads, parseErr := parseBeads(out)
	if parseErr != nil {
		return healthchecker.Result{
			Name:     c.Name(),
			Category: c.Category(),
			Status:   healthchecker.StatusError,
			Message:  fmt.Sprintf("parse bd output: %v", parseErr),
		}
	}

	threshold := time.Now().Add(-time.Duration(c.staleDays) * 24 * time.Hour)
	stale := filterStale(beads, threshold)

	if len(stale) == 0 {
		return healthchecker.Result{
			Name:     c.Name(),
			Category: c.Category(),
			Status:   healthchecker.StatusOK,
			Message:  fmt.Sprintf("all %d open beads updated within %d days", len(beads), c.staleDays),
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%d stale bead(s) (no activity >%dd):", len(stale), c.staleDays)
	for _, b := range stale {
		ageDays := int(time.Since(b.UpdatedAt).Hours() / 24)
		fmt.Fprintf(&sb, "\n      %s (%dd) — %s", b.ID, ageDays, b.Title)
	}
	msg := sb.String()
	return healthchecker.Result{
		Name:     c.Name(),
		Category: c.Category(),
		Status:   healthchecker.StatusWarning,
		Message:  msg,
	}
}

// parseBeads unmarshals the bd JSON output into a slice of beadEntry.
func parseBeads(data []byte) ([]beadEntry, error) {
	var beads []beadEntry
	if err := json.Unmarshal(data, &beads); err != nil {
		return nil, err
	}
	return beads, nil
}

// filterStale returns beads whose UpdatedAt is before the threshold.
func filterStale(beads []beadEntry, threshold time.Time) []beadEntry {
	var out []beadEntry
	for _, b := range beads {
		if b.UpdatedAt.Before(threshold) {
			out = append(out, b)
		}
	}
	return out
}

// jaegerHealthCheck queries Jaeger for recent traces from a named service.
type jaegerHealthCheck struct {
	baseURL  string
	service  string
	lookback time.Duration
}

func (c *jaegerHealthCheck) Name() string     { return "otel_traces" }
func (c *jaegerHealthCheck) Category() string { return "drift" }

type jaegerTracesResp struct {
	Data []struct{} `json:"data"`
}

func (c *jaegerHealthCheck) Run(ctx context.Context) healthchecker.Result {
	now := time.Now()
	start := now.Add(-c.lookback)
	url := fmt.Sprintf("%s/api/traces?service=%s&start=%d&end=%d&limit=1",
		c.baseURL, c.service, start.UnixMicro(), now.UnixMicro())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return healthchecker.Result{
			Name:     c.Name(),
			Category: c.Category(),
			Status:   healthchecker.StatusError,
			Message:  fmt.Sprintf("build request: %v", err),
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return healthchecker.Result{
			Name:     c.Name(),
			Category: c.Category(),
			Status:   healthchecker.StatusWarning,
			Message:  fmt.Sprintf("jaeger unreachable (%s): %v — is Jaeger running?", c.baseURL, err),
		}
	}
	defer resp.Body.Close()

	var jResp jaegerTracesResp
	if err := json.NewDecoder(resp.Body).Decode(&jResp); err != nil {
		return healthchecker.Result{
			Name:     c.Name(),
			Category: c.Category(),
			Status:   healthchecker.StatusError,
			Message:  fmt.Sprintf("decode jaeger response: %v", err),
		}
	}

	if len(jResp.Data) == 0 {
		return healthchecker.Result{
			Name:     c.Name(),
			Category: c.Category(),
			Status:   healthchecker.StatusWarning,
			Message:  fmt.Sprintf("no traces from %q in the last %.0fh — OTel pipeline may be down", c.service, c.lookback.Hours()),
		}
	}
	return healthchecker.Result{
		Name:     c.Name(),
		Category: c.Category(),
		Status:   healthchecker.StatusOK,
		Message:  fmt.Sprintf("service %q is reporting traces (%.0fh window)", c.service, c.lookback.Hours()),
	}
}
