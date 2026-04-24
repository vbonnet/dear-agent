package stophook

import (
	"fmt"
	"os"
	"strings"
)

// Severity indicates how serious a finding is.
type Severity int

const (
	SeverityPass Severity = iota
	SeverityWarn
	SeverityBlock
)

func (s Severity) String() string {
	switch s {
	case SeverityPass:
		return "PASS"
	case SeverityWarn:
		return "WARN"
	case SeverityBlock:
		return "BLOCK"
	default:
		return "UNKNOWN"
	}
}

// Finding is a single check result.
type Finding struct {
	Check      string
	Severity   Severity
	Message    string
	Suggestion string
}

// Result aggregates findings from a Stop hook.
type Result struct {
	HookName string
	Findings []Finding
}

// Add appends a finding to the result.
func (r *Result) Add(check string, sev Severity, msg, suggestion string) {
	r.Findings = append(r.Findings, Finding{
		Check:      check,
		Severity:   sev,
		Message:    msg,
		Suggestion: suggestion,
	})
}

// Pass adds a passing check.
func (r *Result) Pass(check, msg string) {
	r.Add(check, SeverityPass, msg, "")
}

// Warn adds a warning finding.
func (r *Result) Warn(check, msg, suggestion string) {
	r.Add(check, SeverityWarn, msg, suggestion)
}

// Block adds a blocking finding.
func (r *Result) Block(check, msg, suggestion string) {
	r.Add(check, SeverityBlock, msg, suggestion)
}

// HasBlocking returns true if any finding has Block severity.
func (r *Result) HasBlocking() bool {
	for _, f := range r.Findings {
		if f.Severity == SeverityBlock {
			return true
		}
	}
	return false
}

// ExitCode returns 2 if blocking findings exist, 0 otherwise.
func (r *Result) ExitCode() int {
	if r.HasBlocking() {
		return 2
	}
	return 0
}

// Report writes human-readable results to stderr.
func (r *Result) Report() {
	var lines []string
	lines = append(lines, fmt.Sprintf("[%s] Session exit validation", r.HookName))

	for _, f := range r.Findings {
		prefix := "  +"
		switch f.Severity {
		case SeverityBlock:
			prefix = "  BLOCK"
		case SeverityWarn:
			prefix = "  WARN "
		case SeverityPass:
			prefix = "  OK   "
		}
		lines = append(lines, fmt.Sprintf("%s %s: %s", prefix, f.Check, f.Message))
		if f.Suggestion != "" {
			lines = append(lines, fmt.Sprintf("         -> %s", f.Suggestion))
		}
	}

	fmt.Fprintln(os.Stderr, strings.Join(lines, "\n"))
}
