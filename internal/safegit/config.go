// Package safegit config.go loads the optional per-repo .safe-merge.yml file
// that drives the P4 safe-merge gates:
//
//   - expected_reviewers: bot reviewers (e.g. Gemini Code Assist) that must have
//     submitted a review newer than the head SHA push before a merge is allowed.
//     A review that never arrives — or one that predates the last push — is a
//     distinct, surfaced outcome, never a silent pass.
//   - flaky_checks: checks that may be retried once (the "flake valve"); a second
//     failure is treated as a real failure that blocks the merge.
//   - approval_policy: the auto-approve decision taxonomy (bead ce-onj5).
//     AUTO-APPROVE by default; HUMAN required only for the named high-stakes
//     categories (security boundaries, money/spend, product decisions,
//     irreversible infra, …). Unlike the two gates above it does not affect P4
//     reviewer/flake gating; it is the policy consumed by the VROOM escalation
//     classifier via Config.EscalationPolicy. Absent ⇒ the built-in
//     escalation.DefaultApprovalPolicy is used.
//
// The file is optional: if it is absent, the P4 gates are skipped silently and
// safe-merge falls back to its built-in gates.
package safegit

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/vroom/escalation"
	"gopkg.in/yaml.v3"
)

// ConfigFileName is the per-repo safe-merge config file, looked up at the repo
// root (or wherever --config points).
const ConfigFileName = ".safe-merge.yml"

// DefaultReviewerTimeoutMinutes is the wait applied to an expected reviewer that
// has not set its own timeout_minutes before "never arrived" is surfaced.
const DefaultReviewerTimeoutMinutes = 30

// Config is the parsed .safe-merge.yml. All sections are optional; an empty file
// (or one with only unrelated keys) yields a zero Config that gates on nothing.
type Config struct {
	ExpectedReviewers []ExpectedReviewer `yaml:"expected_reviewers"`
	FlakyChecks       []FlakyCheck       `yaml:"flaky_checks"`
	// ApprovalPolicy is the auto-approve decision taxonomy (DEAR retro
	// 2026-06-12): AUTO-APPROVE by default, HUMAN only for the named high-stakes
	// categories. It is independent of the P4 reviewer/flake gates — a config
	// that carries only an approval_policy still reports IsEmpty (no P4 gating)
	// while supplying the taxonomy to the VROOM escalation classifier. When
	// absent, the built-in escalation.DefaultApprovalPolicy is used. See
	// EscalationPolicy.
	ApprovalPolicy *ApprovalPolicy `yaml:"approval_policy"`
}

// ApprovalPolicy mirrors escalation.ApprovalPolicy as the .safe-merge.yml
// schema. It is converted to the in-memory escalation type by
// Config.EscalationPolicy.
type ApprovalPolicy struct {
	// AutoApproveDefault is the posture when no human_required category matches.
	AutoApproveDefault bool `yaml:"auto_approve_default"`
	// HumanRequired lists the categories that force a human; first match wins.
	HumanRequired []HumanRequiredCategory `yaml:"human_required"`
}

// HumanRequiredCategory is one bucket of the taxonomy only a human may decide.
type HumanRequiredCategory struct {
	// Name is the coarse topic recorded for audit (e.g. "security-boundary").
	Name string `yaml:"name"`
	// Reason is the human-readable explanation recorded on the verdict.
	Reason string `yaml:"reason"`
	// Patterns are case-insensitive regexp sources; any match selects the bucket.
	Patterns []string `yaml:"patterns"`
}

// ExpectedReviewer is a bot (or human) login whose review is required before
// merge.
type ExpectedReviewer struct {
	// Login is the exact GitHub login of the reviewer, e.g.
	// "gemini-code-assist[bot]".
	Login string `yaml:"login"`
	// TimeoutMinutes is how long to wait for the review before surfacing
	// "never arrived". Zero falls back to DefaultReviewerTimeoutMinutes.
	TimeoutMinutes int `yaml:"timeout_minutes"`
	// RequireNewerThanPush requires the review to postdate the head SHA push
	// time; a review that predates the last push is surfaced as stale.
	RequireNewerThanPush bool `yaml:"require_newer_than_push"`
}

// Timeout returns the configured reviewer timeout, applying the default when
// unset.
func (r ExpectedReviewer) Timeout() int {
	if r.TimeoutMinutes <= 0 {
		return DefaultReviewerTimeoutMinutes
	}
	return r.TimeoutMinutes
}

// FlakyCheck names a check that may be retried before its failure is treated as
// real.
type FlakyCheck struct {
	// Name is matched (as a substring) against the failing check name.
	Name string `yaml:"name"`
	// MaxRetries is the number of sanctioned reruns; the first rerun is
	// attempt 1, and the (MaxRetries+1)th failure is a real block. Zero means
	// no retries (the check is named flaky but gets no second chance).
	MaxRetries int `yaml:"max_retries"`
}

// IsEmpty reports whether the config gates on nothing (no reviewers, no flaky
// checks). Callers use it to skip the P4 gates silently.
func (c *Config) IsEmpty() bool {
	return c == nil || (len(c.ExpectedReviewers) == 0 && len(c.FlakyChecks) == 0)
}

// LoadConfig loads .safe-merge.yml. When path is non-empty it is read directly
// (a missing file is then an error — an explicit --config override that points
// at nothing is a mistake worth surfacing). When path is empty, ConfigFileName
// is looked up in dir; a missing file there returns (nil, nil) so the P4 gates
// are skipped silently.
func LoadConfig(dir, path string) (*Config, error) {
	target := path
	explicit := path != ""
	if !explicit {
		target = filepath.Join(dir, ConfigFileName)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		if !explicit && errors.Is(err, fs.ErrNotExist) {
			// No repo-root config: skip P4 silently.
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", target, err)
	}

	cfg, err := ParseConfig(data)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", target, err)
	}
	return cfg, nil
}

// LoadRepoConfig loads .safe-merge.yml from the current git repository root,
// returning (nil, nil) when the file is absent (callers then fall back to
// built-in defaults). It is the convenience used by consumers — such as the
// VROOM escalation classifier — that want the repo's config without computing
// the root themselves.
func LoadRepoConfig() (*Config, error) {
	return LoadConfig(repoRoot(), "")
}

// repoRoot returns the git working-tree root for the current directory, used to
// locate ConfigFileName. It falls back to "." when not inside a git repo so a
// loose checkout still finds a config in the working directory.
func repoRoot() string {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "."
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "."
	}
	return root
}

// ParseConfig decodes a .safe-merge.yml document. It is strict about unknown
// fields so a typo (e.g. expected_reviewer vs expected_reviewers) fails loudly
// rather than silently disabling a gate the operator believed was running.
// Exported for testing.
func ParseConfig(data []byte) (*Config, error) {
	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		// An empty document decodes to io.EOF; treat that as an empty config.
		if errors.Is(err, fs.ErrNotExist) {
			return &cfg, nil
		}
		if err.Error() == "EOF" {
			return &cfg, nil
		}
		return nil, err
	}

	for i, r := range cfg.ExpectedReviewers {
		if r.Login == "" {
			return nil, fmt.Errorf("expected_reviewers[%d] has no login", i)
		}
	}
	for i, f := range cfg.FlakyChecks {
		if f.Name == "" {
			return nil, fmt.Errorf("flaky_checks[%d] has no name", i)
		}
		if f.MaxRetries < 0 {
			return nil, fmt.Errorf("flaky_checks[%q] has negative max_retries", f.Name)
		}
	}
	// Validate the approval_policy by compiling it: a bad category name or an
	// uncompilable pattern must fail loudly here, never silently disable a gate
	// the operator believed was running.
	if cfg.ApprovalPolicy != nil {
		if _, err := cfg.ApprovalPolicy.toEscalation().Compile(); err != nil {
			return nil, err
		}
	}
	return &cfg, nil
}

// EscalationPolicy returns the in-memory escalation.ApprovalPolicy to gate on:
// the parsed approval_policy block when present, otherwise the built-in
// escalation.DefaultApprovalPolicy. This is the single conversion point from the
// .safe-merge.yml schema to the VROOM escalation classifier's taxonomy.
func (c *Config) EscalationPolicy() escalation.ApprovalPolicy {
	if c == nil || c.ApprovalPolicy == nil {
		return escalation.DefaultApprovalPolicy()
	}
	return c.ApprovalPolicy.toEscalation()
}

// toEscalation maps the YAML schema onto the canonical escalation type.
func (p *ApprovalPolicy) toEscalation() escalation.ApprovalPolicy {
	out := escalation.ApprovalPolicy{AutoApproveDefault: p.AutoApproveDefault}
	for _, c := range p.HumanRequired {
		out.HumanRequired = append(out.HumanRequired, escalation.HumanRequiredCategory{
			Name:     c.Name,
			Reason:   c.Reason,
			Patterns: c.Patterns,
		})
	}
	return out
}
