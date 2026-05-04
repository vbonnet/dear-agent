package collectors

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/aggregator"
)

// LintTrend invokes `golangci-lint run --output-format=json` (or reads
// a precomputed JSON file) and emits one signal per file with
// findings, where Value is the per-file finding count.
type LintTrend struct {
	Repo       string         // path to the Go module root
	InputFile  string         // optional precomputed JSON output
	Exec       Exec           // nil → DefaultExec
	LookPathFn func(string) (string, error) // nil → LookPath
}

// Name implements aggregator.Collector.
func (c *LintTrend) Name() string { return "dear-agent.lint" }

// Kind implements aggregator.Collector.
func (c *LintTrend) Kind() aggregator.Kind { return aggregator.KindLintTrend }

// Collect either reads InputFile or shells out to golangci-lint. The
// JSON shape is golangci-lint's standard output: {"Issues": [{"Pos":
// {"Filename": "..."}, "FromLinter": "..."}, ...]}.
func (c *LintTrend) Collect(ctx context.Context) ([]aggregator.Signal, error) {
	raw, err := c.readJSON(ctx)
	if err != nil {
		return nil, err
	}
	return parseGolangCILint(raw)
}

// readJSON returns the raw JSON to parse, either from InputFile or
// from golangci-lint. golangci-lint exits non-zero when issues are
// found; we accept ExitError as long as the JSON is well-formed.
func (c *LintTrend) readJSON(ctx context.Context) ([]byte, error) {
	if c.InputFile != "" {
		return readFile(c.InputFile)
	}
	if strings.TrimSpace(c.Repo) == "" {
		return nil, fmt.Errorf("collectors.LintTrend: empty Repo and no InputFile")
	}
	lookPath := c.LookPathFn
	if lookPath == nil {
		lookPath = LookPath
	}
	if _, err := lookPath("golangci-lint"); err != nil {
		return nil, &aggregator.ErrToolMissing{Collector: c.Name(), Tool: "golangci-lint"}
	}
	execFn := c.Exec
	if execFn == nil {
		execFn = DefaultExec
	}
	out, err := execFn(ctx, c.Repo, "golangci-lint",
		"run", "--output.json.path=stdout", "./...")
	if err != nil {
		// golangci-lint exits 1 when it found issues; that's the
		// happy path for us — we only fail if the output isn't
		// parseable JSON.
		var ee *exec.ExitError
		if !errors.As(err, &ee) && len(out) == 0 {
			return nil, fmt.Errorf("collectors.LintTrend: %w", err)
		}
	}
	return out, nil
}

type golangCILintIssue struct {
	FromLinter string `json:"FromLinter"`
	Text       string `json:"Text"`
	Pos        struct {
		Filename string `json:"Filename"`
	} `json:"Pos"`
}

type golangCILintOutput struct {
	Issues []golangCILintIssue `json:"Issues"`
}

// parseGolangCILint turns golangci-lint JSON into one signal per file.
// Files with zero findings are not emitted.
//
// golangci-lint v2 prints a human-readable summary after the JSON
// document on stdout (e.g. "1 issues:\n* typecheck: 1"), so we decode
// just the first JSON value from the stream and ignore trailing
// content rather than calling json.Unmarshal on the full buffer.
func parseGolangCILint(raw []byte) ([]aggregator.Signal, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	var out golangCILintOutput
	if err := dec.Decode(&out); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("collectors.LintTrend: parse JSON: %w", err)
	}
	perFile := map[string]int{}
	perFileLinters := map[string]map[string]int{}
	for _, iss := range out.Issues {
		f := iss.Pos.Filename
		if f == "" {
			f = "<unknown>"
		}
		perFile[f]++
		if perFileLinters[f] == nil {
			perFileLinters[f] = map[string]int{}
		}
		perFileLinters[f][iss.FromLinter]++
	}

	sigs := make([]aggregator.Signal, 0, len(perFile))
	for f, count := range perFile {
		md := map[string]any{
			"linters": perFileLinters[f],
		}
		mdJSON, _ := json.Marshal(md)
		sigs = append(sigs, aggregator.Signal{
			Kind:     aggregator.KindLintTrend,
			Subject:  f,
			Value:    float64(count),
			Metadata: string(mdJSON),
		})
	}
	return sigs, nil
}
