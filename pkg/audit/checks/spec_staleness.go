package checks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/audit"
)

// SpecStalenessCheck flags package directories whose SPEC.md has not been
// updated since N or more commits landed in that directory. A SPEC.md that
// falls far behind the code it documents stops being useful — new behaviour
// is undocumented and old invariants may have been removed.
//
// This is the second spec-compliance check. spec.coverage detects
// missing SPECs; this check detects SPECs that exist but have gone stale.
//
// Config knobs:
//   - roots: []string — same default as spec.coverage (["pkg", "internal",
//     "tools", "cmd"]). Only directories that already have a SPEC.md are
//     examined.
//   - skip: []string — directory names to skip (same default as
//     spec.coverage).
//   - threshold: int — number of commits to the directory (excluding
//     SPEC.md itself) that must have landed after the SPEC.md's last
//     commit before a finding is emitted. Default: 10.
//
// Severity is P2 — stale documentation is drift, not breakage.
type SpecStalenessCheck struct{}

// Meta returns the check metadata for SpecStalenessCheck.
func (SpecStalenessCheck) Meta() audit.CheckMeta {
	return audit.CheckMeta{
		ID:              "spec.staleness",
		Description:     "SPEC.md has not been updated after N commits to its directory",
		Cadence:         audit.CadenceWeekly,
		SeverityCeiling: audit.SeverityP2,
	}
}

const defaultStalenessThreshold = 10

// Run walks each configured root one level deep, finds directories with a
// SPEC.md, and emits a finding for each where more than threshold commits
// have touched the directory (excluding SPEC.md) since the SPEC.md was
// last committed.
//
// Directories where git has no history for the SPEC.md (brand-new, not yet
// added to the index) are silently skipped.
func (SpecStalenessCheck) Run(ctx context.Context, env audit.Env) (audit.Result, error) {
	roots := stringSliceConfig(env.Config, "roots", defaultSpecRoots)
	skip := stringSetConfig(env.Config, "skip", defaultSpecSkip)
	threshold := intConfig(env.Config, "threshold", defaultStalenessThreshold)

	out := audit.Result{Status: audit.StatusOK}

	// git not present or not a git repo — staleness check is a no-op.
	if runCommand(ctx, env.RepoRoot, "git", "rev-parse", "--git-dir").Err != nil {
		return out, nil //nolint:nilerr // git-absent is a valid skip, not an error to propagate
	}

	var findings []audit.Finding
	for _, root := range roots {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		rootAbs := filepath.Join(env.WorkingDir, root)
		entries, err := os.ReadDir(rootAbs)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			out.Status = audit.StatusError
			return out, fmt.Errorf("audit/spec.staleness: read %s: %w", rootAbs, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if skip[name] || strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") {
				continue
			}
			pkgDir := filepath.Join(rootAbs, name)
			specPath := filepath.Join(pkgDir, "SPEC.md")
			if !fileExists(specPath) {
				continue
			}

			rel := filepath.ToSlash(filepath.Join(root, name))
			relSpec := rel + "/SPEC.md"

			count, err := commitsBehind(ctx, env.RepoRoot, rel, relSpec)
			if err != nil {
				// Non-fatal: log and continue so one bad directory doesn't
				// block the whole check.
				if env.Logger != nil {
					env.Logger.Warn("audit/spec.staleness: git query failed",
						"dir", rel, "err", err)
				}
				continue
			}
			if count < threshold {
				continue
			}

			findings = append(findings, audit.Finding{
				Fingerprint: audit.Fingerprint("spec.staleness", rel),
				Severity:    audit.SeverityP2,
				Title:       fmt.Sprintf("%s/SPEC.md is %d commits behind its directory", rel, count),
				Detail: fmt.Sprintf(
					"%s/SPEC.md has not been updated in %d commits that touched %s/. "+
						"The SPEC may no longer reflect the package's current behaviour, "+
						"public API, or acceptance criteria. Update the SPEC.md to reflect "+
						"recent changes, or add a note explaining why the changes did not "+
						"require a SPEC update.",
					rel, count, rel,
				),
				Path: relSpec,
				Suggested: audit.Remediation{
					Strategy: audit.StrategyNoop,
				},
				Evidence: map[string]any{
					"directory":      rel,
					"commits_behind": count,
					"threshold":      threshold,
				},
			})
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		return findings[i].Path < findings[j].Path
	})
	out.Findings = findings
	return out, nil
}

// commitsBehind returns the number of commits that have touched dirRel (any
// file in the directory) after the most recent commit that touched specRel.
// Both paths are relative to repoRoot. Returns 0 when SPEC.md has no git
// history yet.
func commitsBehind(ctx context.Context, repoRoot, dirRel, specRel string) (int, error) {
	// Find the most recent commit that touched SPEC.md.
	specRes := runCommand(ctx, repoRoot, "git", "log", "-1", "--format=%H", "--", specRel)
	if specRes.Err != nil {
		return 0, fmt.Errorf("git log for SPEC.md: %w", specRes.Err)
	}
	specSHA := strings.TrimSpace(specRes.Stdout)
	if specSHA == "" {
		return 0, nil // SPEC.md not yet committed — not stale
	}

	// Count commits to the directory (all files) since the SPEC.md commit.
	// We pipe through --oneline to get one line per commit, then count.
	logRes := runCommand(ctx, repoRoot,
		"git", "log", "--oneline", specSHA+"..HEAD", "--", dirRel+"/")
	if logRes.Err != nil {
		return 0, fmt.Errorf("git log for dir: %w", logRes.Err)
	}

	lines := strings.Split(strings.TrimSpace(logRes.Stdout), "\n")
	count := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count, nil
}

// intConfig reads an int from env.Config[key]. Non-int values and missing
// keys fall back to the provided default.
func intConfig(cfg map[string]any, key string, fallback int) int { //nolint:unparam // key varies across callers
	v, ok := cfg[key]
	if !ok || v == nil {
		return fallback
	}
	switch n := v.(type) {
	case int:
		if n > 0 {
			return n
		}
	case int64:
		if n > 0 {
			return int(n)
		}
	case float64:
		if n > 0 {
			return int(n)
		}
	}
	return fallback
}

func init() {
	audit.Default.MustRegister(SpecStalenessCheck{})
}
