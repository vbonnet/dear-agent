// Command retro-audit reads OTel span JSONL files from the engram traces
// directory, classifies findings, and appends a formatted markdown report to
// the sibling engram-research retrospectives directory (or a path supplied via
// --output).
//
// Usage:
//
//	retro-audit [--traces-dir PATH] [--output PATH] [--lookback DURATION] [--dry-run]
//
// Exit codes: 0 on success (even when findings are present); non-zero only on
// I/O errors.  This command is intentionally standalone — it does NOT import
// pkg/audit so it can be run without the full audit runner.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// span mirrors the on-disk JSONL format written by pkg/otelsetup.
type span struct {
	TraceID    string    `json:"trace_id"`
	SpanID     string    `json:"span_id"`
	Name       string    `json:"name"`
	StartTime  time.Time `json:"start_time"`
	DurationMs float64   `json:"duration_ms"`
	StatusCode string    `json:"status_code"`
	StatusMsg  string    `json:"status_message"`
}

// finding is one classified event from the span stream.
type finding struct {
	TraceID    string
	SpanID     string
	Name       string
	Msg        string
	DurationMs float64
}

func main() {
	var (
		tracesDir string
		output    string
		lookback  time.Duration
		dryRun    bool
	)

	home, _ := os.UserHomeDir()
	defaultTracesDir := filepath.Join(home, ".engram", "traces")
	defaultOutput := defaultRetroOutput()

	flag.StringVar(&tracesDir, "traces-dir", defaultTracesDir, "root directory of span JSONL files")
	flag.StringVar(&output, "output", defaultOutput, "output markdown file path (appended)")
	flag.DurationVar(&lookback, "lookback", 24*time.Hour, "how far back to look for spans")
	flag.BoolVar(&dryRun, "dry-run", false, "print findings to stdout without writing to file")
	flag.Parse()

	cutoff := time.Now().Add(-lookback)

	// Discover all spans.jsonl files.
	files, err := filepath.Glob(filepath.Join(tracesDir, "*", "spans.jsonl"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "retro-audit: glob: %v\n", err)
		os.Exit(1)
	}

	var scanned int
	var errors, slows []finding

	for _, f := range files {
		spans, err := readSpans(f, cutoff)
		if err != nil {
			fmt.Fprintf(os.Stderr, "retro-audit: read %s: %v (skipping)\n", f, err)
			continue
		}
		for _, s := range spans {
			scanned++
			if s.StatusCode == "Error" {
				msg := s.StatusMsg
				if msg == "" {
					msg = "span recorded status=ERROR"
				}
				errors = append(errors, finding{
					TraceID: s.TraceID,
					SpanID:  s.SpanID,
					Name:    s.Name,
					Msg:     msg,
				})
			}
			if s.DurationMs > 5000 {
				slows = append(slows, finding{
					TraceID:    s.TraceID,
					SpanID:     s.SpanID,
					Name:       s.Name,
					DurationMs: s.DurationMs,
				})
			}
		}
	}

	total := len(errors) + len(slows)
	report := formatReport(time.Now(), lookback, scanned, errors, slows)

	fmt.Printf("retro-audit: scanned %d spans, %d findings (%d errors, %d slow)\n",
		scanned, total, len(errors), len(slows))

	if dryRun {
		fmt.Print(report)
		return
	}

	if err := appendToFile(output, report); err != nil {
		fmt.Fprintf(os.Stderr, "retro-audit: write output: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("retro-audit: appended report to %s\n", output)
}

func defaultRetroOutput() string {
	root := os.Getenv("ENGRAM_RESEARCH_ROOT")
	if root == "" {
		root = filepath.Join("..", "engram-research")
	}
	return filepath.Join(root, "retrospectives", "RETRO_LOG.md")
}

// readSpans opens a spans.jsonl file and returns spans that start at or after
// cutoff.
func readSpans(path string, cutoff time.Time) ([]span, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []span
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var s span
		if err := json.Unmarshal(line, &s); err != nil {
			continue
		}
		if s.StartTime.Before(cutoff) {
			continue
		}
		out = append(out, s)
	}
	return out, scanner.Err()
}

// short8 returns the first 8 bytes of s, or the whole string if shorter.
// Used to truncate trace/span IDs to a readable prefix.
func short8(s string) string {
	if len(s) >= 8 {
		return s[:8]
	}
	return s
}

// formatReport builds the markdown section to append.
func formatReport(
	now time.Time,
	lookback time.Duration,
	scanned int,
	errors, slows []finding,
) string {
	var b []byte
	b = fmt.Appendf(b, "## Retro: %s — OTel span audit\n\n", now.UTC().Format("2006-01-02"))
	b = fmt.Appendf(b, "**Lookback:** %s  **Spans scanned:** %d  **Findings:** %d\n\n",
		lookback, scanned, len(errors)+len(slows))

	b = fmt.Appendf(b, "### Errors (P1)\n\n")
	if len(errors) == 0 {
		b = fmt.Appendf(b, "_none_\n")
	} else {
		for _, e := range errors {
			b = fmt.Appendf(b, "- `%s/%s` — **%s**: %s\n",
				short8(e.TraceID), short8(e.SpanID), e.Name, e.Msg)
		}
	}

	b = fmt.Appendf(b, "\n### Slow spans (P2)\n\n")
	if len(slows) == 0 {
		b = fmt.Appendf(b, "_none_\n")
	} else {
		for _, s := range slows {
			b = fmt.Appendf(b, "- `%s/%s` — **%s**: %.0fms\n",
				short8(s.TraceID), short8(s.SpanID), s.Name, s.DurationMs)
		}
	}
	b = fmt.Appendf(b, "\n---\n\n")
	return string(b)
}

// appendToFile creates parent directories as needed, then opens the file in
// append mode and writes content.
func appendToFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}
