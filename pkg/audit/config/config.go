// Package config loads the audits: section of .dear-agent.yml and
// converts it into a Plan the audit Runner can execute.
//
// The loader is intentionally tolerant of missing fields — a repo
// with no audits: block runs the package defaults; a repo with
// audits: {} runs nothing. This matches ADR-011 §5.
//
// The loader does NOT register checks or apply the schema; that is
// the caller's job. It only parses the file and resolves cadence
// rules into a fully-populated audit.Plan.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/vbonnet/dear-agent/pkg/audit"
)

// File is the on-disk shape of .dear-agent.yml. We only describe the
// fields the audit subsystem cares about; other consumers (output
// routing) read their own keys from the same file. yaml.v3 ignores
// unknown keys by default so the structs can stay narrow.
type File struct {
	Version int             `yaml:"version"`
	Repo    string          `yaml:"repo"`
	Audits  *AuditsSection  `yaml:"audits,omitempty"`
}

// AuditsSection mirrors the audits: block from ADR-011 §5.
type AuditsSection struct {
	SeverityPolicy map[string]SeverityRule       `yaml:"severity-policy,omitempty"`
	Schedule       map[string][]ScheduledCheck   `yaml:"schedule,omitempty"`
	Trees          []Tree                        `yaml:"trees,omitempty"`
}

// SeverityRule mirrors the per-severity policy block.
type SeverityRule struct {
	FailRun     bool   `yaml:"fail-run"`
	Remediate   string `yaml:"remediate,omitempty"`
	Notify      bool   `yaml:"notify,omitempty"`
}

// ScheduledCheck is one row under audits.schedule.<cadence>:.
type ScheduledCheck struct {
	Check  string         `yaml:"check"`
	Config map[string]any `yaml:"config,omitempty"`
}

// Tree is one entry under audits.trees: — used by polyglot repos.
type Tree struct {
	Path          string           `yaml:"path"`
	ChecksAdd     []ScheduledCheck `yaml:"checks-add,omitempty"`
	ChecksRemove  []ScheduledCheck `yaml:"checks-remove,omitempty"`
}

// Load reads .dear-agent.yml from the given repo root. Returns nil
// (without error) when the file does not exist — the caller decides
// whether to fall back to defaults or fail.
func Load(repoRoot string) (*File, error) {
	path := filepath.Join(repoRoot, ".dear-agent.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("audit/config: read %s: %w", path, err)
	}
	var f File
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("audit/config: parse %s: %w", path, err)
	}
	if f.Version == 0 {
		f.Version = 1
	}
	return &f, nil
}

// BuildPlan converts a loaded File into an audit.Plan for the given
// cadence. Resolution order:
//   1. Start from the schedule[cadence] checks (or registry defaults
//      when schedule is missing for the cadence).
//   2. For each tree under trees:, apply checks-add and checks-remove
//      as overrides for that tree.
//   3. Repos without trees: get a single TreePlan rooted at repoRoot.
//
// triggeredBy is recorded on the audit_runs row.
func BuildPlan(f *File, repoRoot string, cadence audit.Cadence, registry *audit.Registry, triggeredBy string) (audit.Plan, error) {
	if !cadence.IsValid() {
		return audit.Plan{}, fmt.Errorf("audit/config: invalid cadence %q", cadence)
	}
	if registry == nil {
		registry = audit.Default
	}

	policy := defaultSeverityPolicy()
	if f != nil && f.Audits != nil {
		if err := mergeSeverityPolicy(policy, f.Audits.SeverityPolicy); err != nil {
			return audit.Plan{}, err
		}
	}

	scheduled, err := resolveScheduled(f, cadence, registry)
	if err != nil {
		return audit.Plan{}, err
	}

	plan := audit.Plan{
		Repo:           resolveRepoName(f, repoRoot),
		Cadence:        cadence,
		RepoRoot:       repoRoot,
		TriggeredBy:    triggeredBy,
		SeverityPolicy: policy,
		Trees:          buildTreePlans(f, repoRoot, scheduled),
	}
	if err := validatePlan(plan, registry); err != nil {
		return audit.Plan{}, err
	}
	return plan, nil
}

// resolveRepoName returns the repo name from the config file or
// falls back to the basename of repoRoot.
func resolveRepoName(f *File, repoRoot string) string {
	if f != nil && f.Repo != "" {
		return f.Repo
	}
	return filepath.Base(repoRoot)
}

// resolveScheduled returns the cadence's scheduled-check list,
// honouring the file's schedule[cadence] override and falling back to
// the registry defaults.
func resolveScheduled(f *File, cadence audit.Cadence, registry *audit.Registry) ([]audit.ScheduledCheck, error) {
	if f == nil || f.Audits == nil {
		return defaultsForCadence(cadence, registry), nil
	}
	rows, ok := f.Audits.Schedule[string(cadence)]
	if !ok {
		return defaultsForCadence(cadence, registry), nil
	}
	out := make([]audit.ScheduledCheck, 0, len(rows))
	for _, r := range rows {
		if r.Check == "" {
			return nil, fmt.Errorf("audit/config: schedule.%s entry missing check id", cadence)
		}
		out = append(out, audit.ScheduledCheck{CheckID: r.Check, Config: r.Config})
	}
	return out, nil
}

// buildTreePlans turns the (possibly empty) trees: list into a slice
// of TreePlan, applying per-tree adds/removes against the scheduled
// baseline.
func buildTreePlans(f *File, repoRoot string, scheduled []audit.ScheduledCheck) []audit.TreePlan {
	if f == nil || f.Audits == nil || len(f.Audits.Trees) == 0 {
		return []audit.TreePlan{{WorkingDir: repoRoot, Checks: scheduled}}
	}
	out := make([]audit.TreePlan, 0, len(f.Audits.Trees))
	for _, tree := range f.Audits.Trees {
		dir := tree.Path
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(repoRoot, dir)
		}
		out = append(out, audit.TreePlan{WorkingDir: dir, Checks: mergeChecks(scheduled, tree.ChecksAdd, tree.ChecksRemove)})
	}
	return out
}

// validatePlan asserts every referenced check is registered. Catches
// typos in .dear-agent.yml at config-load time rather than mid-run.
func validatePlan(p audit.Plan, registry *audit.Registry) error {
	for ti, tree := range p.Trees {
		for ci, sc := range tree.Checks {
			if _, ok := registry.Lookup(sc.CheckID); !ok {
				return fmt.Errorf("audit/config: trees[%d].checks[%d]: unknown check %q (did you import a checks package?)", ti, ci, sc.CheckID)
			}
		}
	}
	return nil
}

// mergeChecks returns base + add - remove, deduplicated by CheckID.
// add overrides take precedence over base for matching CheckIDs (the
// caller can supply tree-specific Config that way).
func mergeChecks(base []audit.ScheduledCheck, add, remove []ScheduledCheck) []audit.ScheduledCheck {
	removeIDs := map[string]bool{}
	for _, r := range remove {
		removeIDs[r.Check] = true
	}
	addByID := map[string]ScheduledCheck{}
	for _, a := range add {
		addByID[a.Check] = a
	}

	out := make([]audit.ScheduledCheck, 0, len(base)+len(add))
	seen := map[string]bool{}
	for _, b := range base {
		if removeIDs[b.CheckID] {
			continue
		}
		if a, ok := addByID[b.CheckID]; ok {
			out = append(out, audit.ScheduledCheck{CheckID: a.Check, Config: a.Config})
			seen[b.CheckID] = true
			continue
		}
		out = append(out, b)
		seen[b.CheckID] = true
	}
	for _, a := range add {
		if seen[a.Check] || removeIDs[a.Check] {
			continue
		}
		out = append(out, audit.ScheduledCheck{CheckID: a.Check, Config: a.Config})
	}
	return out
}

// defaultsForCadence returns the registry's "what runs at this
// cadence by default" set.
func defaultsForCadence(c audit.Cadence, r *audit.Registry) []audit.ScheduledCheck {
	checks := r.ChecksForCadence(c)
	out := make([]audit.ScheduledCheck, 0, len(checks))
	for _, ch := range checks {
		out = append(out, audit.ScheduledCheck{CheckID: ch.Meta().ID})
	}
	return out
}

// defaultSeverityPolicy is a copy of audit.DefaultSeverityPolicy()
// kept here so the loader is self-contained.
func defaultSeverityPolicy() map[audit.Severity]audit.SeverityRule {
	src := audit.DefaultSeverityPolicy()
	out := make(map[audit.Severity]audit.SeverityRule, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// mergeSeverityPolicy applies overrides on top of the package
// defaults. Unknown severity keys are an error so a misspelled
// "p0" surfaces immediately.
func mergeSeverityPolicy(dst map[audit.Severity]audit.SeverityRule, src map[string]SeverityRule) error {
	for k, v := range src {
		sev := audit.Severity(k)
		if !sev.IsValid() {
			return fmt.Errorf("audit/config: severity-policy: unknown severity %q", k)
		}
		strategy := audit.Strategy(v.Remediate)
		if !strategy.IsValid() {
			return fmt.Errorf("audit/config: severity-policy[%s].remediate: invalid strategy %q", k, v.Remediate)
		}
		dst[sev] = audit.SeverityRule{
			FailRun:         v.FailRun,
			DefaultStrategy: strategy,
			Notify:          v.Notify,
		}
	}
	return nil
}
