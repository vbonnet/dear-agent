package checks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/audit"
)

// TestCheck wraps `go test ./...` for the daily cadence. It runs
// with -json so a single failed test produces one finding, not the
// whole stderr blob. Severity is P1: a broken test is a quality
// regression and should block release, but the build itself is
// (presumably) green so it is not a P0.
//
// Config knobs:
//   - race: bool — pass -race to the test runner. Default false.
//   - timeout: string — passed verbatim to -timeout (e.g. "2m"). Default empty.
type TestCheck struct{}

// Meta returns the check's identity.
func (TestCheck) Meta() audit.CheckMeta {
	return audit.CheckMeta{
		ID:              "test",
		Description:     "go test ./... must pass for the configured packages",
		Cadence:         audit.CadenceDaily,
		SeverityCeiling: audit.SeverityP1,
	}
}

// Run executes go test ./... -json and parses each JSON record. Each
// failed test becomes one Finding fingerprinted by (package, test
// name) so reruns of the same broken test collapse.
func (TestCheck) Run(ctx context.Context, env audit.Env) (audit.Result, error) {
	args := []string{"test", "-json", "./..."}
	if v, _ := env.Config["race"].(bool); v {
		args = append(args, "-race")
	}
	if to, _ := env.Config["timeout"].(string); to != "" {
		args = append(args, "-timeout", to)
	}

	res := runCommand(ctx, env.WorkingDir, "go", args...)
	out := audit.Result{Status: audit.StatusOK, Stdout: res.Stdout, Stderr: res.Stderr}
	if res.Err != nil {
		out.Status = audit.StatusError
		return out, fmt.Errorf("audit/test: invoke go test: %w", res.Err)
	}

	failures := parseTestFailures(res.Stdout)
	for _, fail := range failures {
		out.Findings = append(out.Findings, audit.Finding{
			Fingerprint: audit.Fingerprint("test", fail.Package, fail.Test),
			Severity:    audit.SeverityP1,
			Title:       fmt.Sprintf("test failed: %s/%s", fail.Package, fail.Test),
			Detail:      truncate(fail.Output, 4000),
			Path:        fail.Package,
			Suggested: audit.Remediation{
				Strategy: audit.StrategyPR,
				Title:    fmt.Sprintf("Investigate failing test %s.%s", fail.Package, fail.Test),
				Body:     fail.Output,
			},
			Evidence: map[string]any{
				"package": fail.Package,
				"test":    fail.Test,
			},
		})
	}
	return out, nil
}

// testEvent mirrors the subset of cmd/test2json events we care about.
// See `go doc test2json` for the schema; we intentionally ignore
// fields we do not consume.
type testEvent struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test,omitempty"`
	Output  string `json:"Output,omitempty"`
}

// testFailure is one rolled-up failure: one row per (package, test).
type testFailure struct {
	Package string
	Test    string
	Output  string
}

// parseTestFailures reads test2json output line-by-line and returns
// one entry per failed test. Output is the concatenation of the
// "output" events for that test, useful as remediation context.
//
// Build failures (Action="fail" with empty Test) bubble up as
// package-level entries with Test="<build>" — we still want to know.
func parseTestFailures(stdout string) []testFailure {
	type key struct{ pkg, test string }
	bufs := map[key]*strings.Builder{}
	failed := map[key]bool{}

	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev testEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		k := key{pkg: ev.Package, test: ev.Test}
		if ev.Action == "output" && ev.Output != "" {
			b, ok := bufs[k]
			if !ok {
				b = &strings.Builder{}
				bufs[k] = b
			}
			b.WriteString(ev.Output)
		}
		if ev.Action == "fail" {
			if ev.Test == "" {
				k.test = "<build>"
			}
			failed[k] = true
		}
	}

	out := make([]testFailure, 0, len(failed))
	for k := range failed {
		var output string
		if b, ok := bufs[key{pkg: k.pkg, test: k.test}]; ok {
			output = b.String()
		}
		out = append(out, testFailure{Package: k.pkg, Test: k.test, Output: output})
	}
	return out
}

func init() {
	audit.Default.MustRegister(TestCheck{})
}
