// Package earslint validates requirements written in the EARS
// (Easy Approach to Requirements Syntax) notation inside SPEC.md files.
//
// EARS constrains every requirement to one of a small set of sentence
// templates, which makes requirements unambiguous and machine-checkable.
// This package is deliberately regex-based: it is a fast, deterministic
// replacement for an LLM "quality rubric" gate, not a natural-language
// understanding engine.
//
// It is usable both as a library (Lint, LintFile) and via the
// cmd/ears-lint CLI.
package earslint

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// Severity classifies a Finding.
type Severity string

const (
	// SeverityError marks a line that looks like a requirement but does not
	// conform to any configured EARS pattern.
	SeverityError Severity = "error"
	// SeverityWarning marks a softer issue (e.g. a file with no requirements).
	SeverityWarning Severity = "warning"
)

// Finding is a single problem discovered while linting.
type Finding struct {
	File     string   `json:"file"`
	Line     int      `json:"line"` // 1-based; 0 means file-level
	Column   int      `json:"column,omitempty"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Text     string   `json:"text,omitempty"` // the offending requirement text
}

func (f Finding) String() string {
	loc := f.File
	if f.Line > 0 {
		loc = fmt.Sprintf("%s:%d", f.File, f.Line)
	}
	return fmt.Sprintf("%s: %s: %s", loc, f.Severity, f.Message)
}

// Result is the outcome of linting a single file.
type Result struct {
	File              string    `json:"file"`
	TotalRequirements int       `json:"total_requirements"` // candidate requirements detected
	ValidRequirements int       `json:"valid_requirements"`
	Findings          []Finding `json:"findings"`
}

// NonConforming returns the number of candidate requirements that matched no
// EARS pattern.
func (r Result) NonConforming() int {
	return r.TotalRequirements - r.ValidRequirements
}

// Failed reports whether the result should be treated as a gate failure.
//
// A file always fails if it contains zero valid EARS requirements. When strict
// is true it additionally fails if any candidate requirement is non-conforming.
// In non-strict mode non-conforming requirements are still reported (as
// Findings) but do not, on their own, fail the gate.
func (r Result) Failed(strict bool) bool {
	if r.ValidRequirements == 0 {
		return true
	}
	if strict && r.NonConforming() > 0 {
		return true
	}
	return false
}

// HasErrors reports whether the result contains any error-severity finding.
func (r Result) HasErrors() bool {
	for _, f := range r.Findings {
		if f.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Linter validates requirements against a set of EARS patterns.
type Linter struct {
	cfg      Config
	patterns []compiledPattern
	// detect matches lines that are *candidate* requirements (contain the
	// requirement keyword, e.g. "shall"). Candidates that match no pattern
	// are reported as errors.
	detect *regexp.Regexp
	// requirementLine decides whether a given line should be treated as a
	// requirement at all (vs. prose). By default we only inspect lines that
	// contain the requirement keyword.
}

type compiledPattern struct {
	name string
	re   *regexp.Regexp
}

// New builds a Linter from a Config. An empty/zero Config falls back to
// DefaultConfig so callers can pass Config{} for default behavior.
func New(cfg Config) (*Linter, error) {
	if len(cfg.Patterns) == 0 {
		cfg = DefaultConfig()
	}
	if cfg.RequirementKeyword == "" {
		cfg.RequirementKeyword = DefaultConfig().RequirementKeyword
	}

	l := &Linter{cfg: cfg}
	for _, p := range cfg.Patterns {
		re, err := regexp.Compile(p.Regex)
		if err != nil {
			return nil, fmt.Errorf("pattern %q: invalid regex: %w", p.Name, err)
		}
		l.patterns = append(l.patterns, compiledPattern{name: p.Name, re: re})
	}

	// A candidate requirement is any line containing the requirement keyword
	// as a whole word, case-insensitively.
	kw := regexp.QuoteMeta(cfg.RequirementKeyword)
	detect, err := regexp.Compile(`(?i)\b` + kw + `\b`)
	if err != nil {
		return nil, fmt.Errorf("requirement keyword %q: %w", cfg.RequirementKeyword, err)
	}
	l.detect = detect
	return l, nil
}

// isCandidate reports whether a line should be treated as a requirement.
func (l *Linter) isCandidate(line string) bool {
	return l.detect.MatchString(line)
}

// matches reports whether a requirement line conforms to any pattern.
func (l *Linter) matches(line string) (string, bool) {
	for _, p := range l.patterns {
		if p.re.MatchString(line) {
			return p.name, true
		}
	}
	return "", false
}

// Lint reads from r, treating it as the contents of the named file, and
// returns a Result. It never returns an error for malformed requirements —
// those are reported as Findings; it only errors on I/O failure.
func (l *Linter) Lint(name string, r io.Reader) (Result, error) {
	res := Result{File: name}
	scanner := bufio.NewScanner(r)
	// Allow long lines (SPEC requirements can be verbose).
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	lineNo := 0
	inFence := false
	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()
		trimmed := strings.TrimSpace(raw)

		// Skip fenced code blocks — code samples often contain "shall"-like
		// tokens that are not requirements.
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}

		candidate := NormalizeRequirementLine(trimmed)
		if candidate == "" || !l.isCandidate(candidate) {
			continue
		}

		res.TotalRequirements++
		if _, ok := l.matches(candidate); ok {
			res.ValidRequirements++
			continue
		}

		res.Findings = append(res.Findings, Finding{
			File:     name,
			Line:     lineNo,
			Severity: SeverityError,
			Message:  "requirement does not match any EARS pattern",
			Text:     candidate,
		})
	}
	if err := scanner.Err(); err != nil {
		return res, fmt.Errorf("reading %s: %w", name, err)
	}

	if res.ValidRequirements == 0 {
		res.Findings = append(res.Findings, Finding{
			File:     name,
			Line:     0,
			Severity: SeverityError,
			Message:  "no valid EARS requirements found",
		})
	}

	return res, nil
}

// LintFile lints the file at path.
func (l *Linter) LintFile(path string) (Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return Result{File: path}, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()
	return l.Lint(path, f)
}

// NormalizeRequirementLine removes supported Markdown presentation syntax so
// every EARS consumer matches and inventories the same canonical line.
func NormalizeRequirementLine(s string) string {
	// Strip list markers: "- ", "* ", "+ ", "1. ", "2) ".
	s = listMarker.ReplaceAllString(s, "")
	// Strip heading markers.
	s = strings.TrimLeft(s, "#")
	s = strings.TrimSpace(s)
	// Remove bold/italic emphasis markers but keep the inner words. Strip
	// underscores and asterisks only when they sit at a word boundary (i.e.
	// act as emphasis delimiters), so snake_case identifiers (user_id) and
	// spaced math/wildcard asterisks (a * b) survive intact in the reported
	// finding text and in pattern matching. The `+` runs cover **bold**/__bold__.
	s = leadingUnderscore.ReplaceAllString(s, "")
	s = trailingUnderscore.ReplaceAllString(s, "")
	s = leadingAsterisk.ReplaceAllString(s, "")
	s = trailingAsterisk.ReplaceAllString(s, "")
	// Backticks are never part of an identifier, so strip code spans entirely.
	s = strings.ReplaceAll(s, "`", "")
	// Drop a trailing requirement id like "(REQ-1)" left at the very end? Keep
	// as-is; patterns are anchored loosely enough to tolerate it.
	return strings.TrimSpace(s)
}

var (
	listMarker         = regexp.MustCompile(`^(\s*[-*+]\s+|\s*\d+[.)]\s+)`)
	leadingUnderscore  = regexp.MustCompile(`\b_+`)
	trailingUnderscore = regexp.MustCompile(`_+\b`)
	leadingAsterisk    = regexp.MustCompile(`\*+\b`)
	trailingAsterisk   = regexp.MustCompile(`\b\*+`)
)
