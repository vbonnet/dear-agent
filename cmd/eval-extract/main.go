// Command eval-extract runs the production trace → eval case pipeline.
//
// It reads completed traces (the kind the DEAR Audit phase produces), keeps the
// problematic ones — errored tool calls, error/stall outcomes, low eval scores,
// stale memory reads — and writes each as a regression eval case into a
// discoverable evals/ dataset. This is the "close the loop" step of the
// telemetry/eval flywheel: the eval suite grows from real failures.
//
// Input formats (auto-detected):
//   - a .json file holding a single Trace object or a JSON array of Traces
//   - a .jsonl file with one Trace per line
//   - a directory: every *.json / *.jsonl file in it is read with the above rules
//
// Usage:
//
//	eval-extract -in <traces.jsonl> [-out evals] [-dry-run]
//	eval-extract -in <dir-of-traces> -out evals
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/evalcase"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "eval-extract:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("eval-extract", flag.ContinueOnError)
	fs.SetOutput(out)
	var (
		in           = fs.String("in", "", "path to a trace JSON/JSONL file or a directory of them (required)")
		outDir       = fs.String("out", "evals", "eval dataset root directory; cases land in <out>/cases/")
		dryRun       = fs.Bool("dry-run", false, "classify and report but do not write any eval cases")
		minEvalScore = fs.Float64("min-eval-score", evalcase.DefaultClassifierConfig().MinEvalScore, "eval scores below this flag the trace")
		maxRetries   = fs.Int("max-tool-retries", evalcase.DefaultClassifierConfig().MaxToolRetries, "tool retry_count >= this flags a stall (0 disables)")
		minRelevance = fs.Float64("min-memory-relevance", evalcase.DefaultClassifierConfig().MinMemoryRelevance, "memory read relevance below this flags a memory error (0 disables)")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *in == "" {
		fs.Usage()
		return fmt.Errorf("missing required -in")
	}

	traces, err := loadTraces(*in)
	if err != nil {
		return err
	}
	if len(traces) == 0 {
		fmt.Fprintln(out, "no traces found")
		return nil
	}

	cfg := evalcase.ClassifierConfig{
		MinEvalScore:       *minEvalScore,
		MaxToolRetries:     *maxRetries,
		MinMemoryRelevance: *minRelevance,
	}

	if *dryRun {
		return dryRunReport(out, traces, cfg)
	}

	store := evalcase.NewFileStore(*outDir)
	p := evalcase.NewPipeline(store)
	p.Classifier = cfg

	res, err := p.Run(context.Background(), traces)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "scanned=%d problematic=%d generated=%d skipped=%d\n",
		res.Scanned, res.Problematic, res.Generated, res.Skipped)
	if res.Generated > 0 || res.Skipped > 0 {
		fmt.Fprintf(out, "eval cases written under %s\n", store.CasesDir())
	}
	for _, c := range res.Cases {
		fmt.Fprintf(out, "  %-20s %-16s %s\n", c.ID, c.Classification, firstLine(c.ActualBehavior))
	}
	return nil
}

func dryRunReport(out io.Writer, traces []evalcase.Trace, cfg evalcase.ClassifierConfig) error {
	problematic := 0
	for _, t := range traces {
		v := cfg.Classify(t)
		if !v.Problematic {
			continue
		}
		problematic++
		fmt.Fprintf(out, "  %-20s %-16s %s\n", t.TraceID, v.Primary, strings.Join(v.Reasons, "; "))
	}
	fmt.Fprintf(out, "scanned=%d problematic=%d (dry-run: nothing written)\n", len(traces), problematic)
	return nil
}

// loadTraces reads traces from a file or a directory of files.
func loadTraces(path string) ([]evalcase.Trace, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if !info.IsDir() {
		return loadTraceFile(path)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", path, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch filepath.Ext(e.Name()) {
		case ".json", ".jsonl":
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	var traces []evalcase.Trace
	for _, name := range names {
		ts, err := loadTraceFile(filepath.Join(path, name))
		if err != nil {
			return nil, err
		}
		traces = append(traces, ts...)
	}
	return traces, nil
}

// loadTraceFile reads one file. A .jsonl file is parsed line by line; any other
// file is parsed as either a single Trace object or a JSON array of Traces,
// detected from the first non-whitespace byte.
func loadTraceFile(path string) ([]evalcase.Trace, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if filepath.Ext(path) == ".jsonl" {
		return parseJSONL(data, path)
	}
	trimmed := strings.TrimLeftFunc(string(data), func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	if strings.HasPrefix(trimmed, "[") {
		var traces []evalcase.Trace
		if err := json.Unmarshal(data, &traces); err != nil {
			return nil, fmt.Errorf("parse %s as trace array: %w", path, err)
		}
		return traces, nil
	}
	var t evalcase.Trace
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse %s as trace object: %w", path, err)
	}
	return []evalcase.Trace{t}, nil
}

func parseJSONL(data []byte, path string) ([]evalcase.Trace, error) {
	var traces []evalcase.Trace
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var t evalcase.Trace
		if err := json.Unmarshal([]byte(line), &t); err != nil {
			return nil, fmt.Errorf("parse %s line %d: %w", path, i+1, err)
		}
		traces = append(traces, t)
	}
	return traces, nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
