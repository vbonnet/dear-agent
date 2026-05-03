package checks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/vbonnet/dear-agent/pkg/audit"
)

// LintGoCheck wraps `golangci-lint run --out-format=json`. Each
// reported issue becomes one Finding fingerprinted by (file, line,
// linter, message). Severity is P2 by default — lint findings are
// drift, not breakage; auto-fixable rules suggest StrategyAuto so
// the runner can apply `golangci-lint run --fix`.
type LintGoCheck struct{}

// Meta returns the check's identity.
func (LintGoCheck) Meta() audit.CheckMeta {
	return audit.CheckMeta{
		ID:              "lint.go",
		Description:     "golangci-lint must report no issues",
		Cadence:         audit.CadenceDaily,
		SeverityCeiling: audit.SeverityP1,
	}
}

// Run invokes golangci-lint with --output.json.path=stdout and parses
// the structured output. Exit code 1 means "issues found" (not
// error); other non-zero codes are real errors. The flag spelling
// is golangci-lint v2.x — v1 used --out-format=json. dear-agent and
// brain-v2 both pin v2; v1 support is out of scope.
func (LintGoCheck) Run(ctx context.Context, env audit.Env) (audit.Result, error) {
	res := runCommand(ctx, env.WorkingDir, "golangci-lint", "run",
		"--output.json.path=stdout", "--output.text.path=stderr", "./...")
	out := audit.Result{Status: audit.StatusOK, Stdout: res.Stdout, Stderr: res.Stderr}
	if res.Err != nil {
		out.Status = audit.StatusError
		return out, fmt.Errorf("audit/lint.go: invoke golangci-lint: %w", res.Err)
	}
	if res.ExitCode != 0 && res.ExitCode != 1 {
		out.Status = audit.StatusError
		return out, fmt.Errorf("audit/lint.go: golangci-lint exited %d: %s", res.ExitCode, firstNonEmptyLine(res.Stderr))
	}

	report, err := parseLintReport(res.Stdout)
	if err != nil {
		out.Status = audit.StatusError
		return out, fmt.Errorf("audit/lint.go: parse report: %w", err)
	}

	for _, issue := range report.Issues {
		severity := audit.SeverityP2
		if issue.Severity == "error" {
			severity = audit.SeverityP1
		}
		path := issue.Pos.Filename
		line := issue.Pos.Line
		out.Findings = append(out.Findings, audit.Finding{
			Fingerprint: audit.Fingerprint("lint.go", path, fmt.Sprintf("%d", line), issue.FromLinter, issue.Text),
			Severity:    severity,
			Title:       fmt.Sprintf("%s: %s", issue.FromLinter, issue.Text),
			Detail:      issue.Text,
			Path:        path,
			Line:        line,
			Suggested: audit.Remediation{
				Strategy: audit.StrategyAuto,
				Command:  "golangci-lint run --fix ./...",
			},
			Evidence: map[string]any{
				"linter":   issue.FromLinter,
				"severity": issue.Severity,
			},
		})
	}
	return out, nil
}

// lintReport mirrors the subset of golangci-lint's JSON output we
// consume. The full schema includes runtime stats and config dumps
// we deliberately ignore.
type lintReport struct {
	Issues []lintIssue `json:"Issues"`
}

type lintIssue struct {
	FromLinter string  `json:"FromLinter"`
	Text       string  `json:"Text"`
	Severity   string  `json:"Severity"`
	Pos        lintPos `json:"Pos"`
}

type lintPos struct {
	Filename string `json:"Filename"`
	Line     int    `json:"Line"`
}

// parseLintReport unmarshals golangci-lint output. Empty stdout is
// treated as zero issues. golangci-lint v2 emits the JSON object as
// the first line and a trailing "N issues." text line afterward; we
// extract only the JSON object so the trailing summary doesn't
// confuse json.Unmarshal.
func parseLintReport(stdout string) (lintReport, error) {
	if stdout == "" {
		return lintReport{}, nil
	}
	jsonPart := extractJSONObject(stdout)
	if jsonPart == "" {
		return lintReport{}, nil
	}
	var r lintReport
	if err := json.Unmarshal([]byte(jsonPart), &r); err != nil {
		return lintReport{}, err
	}
	return r, nil
}

// extractJSONObject returns the substring spanning the first
// balanced { … } object in s, or "" if none is found. Used to peel
// the JSON payload off golangci-lint v2's mixed text+JSON stdout.
// String-escape handling is sufficient for valid JSON; we are not
// trying to be a tolerant parser, only to skip a trailing summary.
func extractJSONObject(s string) string {
	start := -1
	depth := 0
	inStr := false
	esc := false
	for i, r := range s {
		switch {
		case esc:
			esc = false
		case inStr && r == '\\':
			esc = true
		case r == '"':
			inStr = !inStr
		case inStr:
			// inside a string; ignore braces
		case r == '{':
			if start < 0 {
				start = i
			}
			depth++
		case r == '}':
			depth--
			if depth == 0 && start >= 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

func init() {
	audit.Default.MustRegister(LintGoCheck{})
}
