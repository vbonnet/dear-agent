package checks

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/audit"
)

// SpecCoverageCheck flags top-level package directories under tracked
// roots that lack a sibling SPEC.md. The repo convention is that each
// package area documents itself with a SPEC.md; this check makes the
// convention enforceable.
//
// Spec compliance decomposes into coverage, staleness, and conformance; this
// is the cheapest check.
//
// Config knobs:
//   - roots: []string — directories whose direct children must each
//     own a SPEC.md. Default: ["pkg", "internal", "tools", "cmd"].
//     Repos that organise differently override the list.
//   - skip: []string — names (not paths) of directories under any
//     root that should be ignored. Default: ["vendor", "testdata",
//     "node_modules", ".git"]. Names starting with "_" are always
//     skipped (Go convention for non-package directories).
//
// Severity is P3 — a missing SPEC is informational drift, not
// breakage. The default cadence is weekly because the signal moves
// slowly (a directory's SPEC status changes only when a package is
// added or removed).
type SpecCoverageCheck struct{}

var (
	defaultSpecRoots = []string{"pkg", "internal", "tools", "cmd"}
	defaultSpecSkip  = []string{"vendor", "testdata", "node_modules", ".git"}
)

// Meta returns the check's identity.
func (SpecCoverageCheck) Meta() audit.CheckMeta {
	return audit.CheckMeta{
		ID:              "spec.coverage",
		Description:     "Top-level package directories must own a SPEC.md",
		Cadence:         audit.CadenceWeekly,
		SeverityCeiling: audit.SeverityP2,
	}
}

// Run walks each configured root one level deep and emits one finding
// per direct subdirectory that contains Go source but no SPEC.md.
// Roots that do not exist are silently skipped — a repo without a
// `cmd/` tree should not be penalised for the convention's absence.
func (SpecCoverageCheck) Run(ctx context.Context, env audit.Env) (audit.Result, error) {
	roots := stringSliceConfig(env.Config, "roots", defaultSpecRoots)
	skip := stringSetConfig(env.Config, "skip", defaultSpecSkip)

	out := audit.Result{Status: audit.StatusOK}

	var findings []audit.Finding
	for _, root := range roots {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		rootAbs := filepath.Join(env.WorkingDir, root)
		entries, err := os.ReadDir(rootAbs)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			out.Status = audit.StatusError
			return out, fmt.Errorf("audit/spec.coverage: read %s: %w", rootAbs, err)
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
			has, err := hasNonTestGoFile(pkgDir)
			if err != nil {
				out.Status = audit.StatusError
				return out, fmt.Errorf("audit/spec.coverage: scan %s: %w", pkgDir, err)
			}
			if !has {
				continue
			}
			specPath := filepath.Join(pkgDir, "SPEC.md")
			if fileExists(specPath) {
				continue
			}
			rel := filepath.ToSlash(filepath.Join(root, name))
			findings = append(findings, audit.Finding{
				Fingerprint: audit.Fingerprint("spec.coverage", rel),
				Severity:    audit.SeverityP3,
				Title:       fmt.Sprintf("%s/ has no SPEC.md", rel),
				Detail: fmt.Sprintf(
					"Package directory %s/ contains Go source but no SPEC.md. "+
						"The repo convention is that each top-level package area documents "+
						"its purpose, public API, and acceptance criteria in a sibling SPEC.md "+
						"so future readers (human or AI) can model the package without "+
						"reading every file. Add %s/SPEC.md describing the package's "+
						"role; see pkg/hash/SPEC.md for a minimal example.",
					rel, rel,
				),
				Path: rel + "/SPEC.md",
				Suggested: audit.Remediation{
					Strategy: audit.StrategyNoop,
				},
				Evidence: map[string]any{
					"directory": rel,
					"root":      root,
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

// hasNonTestGoFile reports whether dir contains at least one .go file
// other than _test.go. Subdirectories are not searched: this check is
// scoped to the package directly under the configured root.
func hasNonTestGoFile(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		return true, nil
	}
	return false, nil
}

// fileExists reports whether path resolves to a regular file. Used in
// place of os.Stat to keep the call site self-documenting; we do not
// care *why* a stat fails (missing, permission, etc.) — any failure
// means "no SPEC.md to count" and the check moves on.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}

// stringSliceConfig reads a []string from env.Config[key], tolerating
// the YAML-decoded shape map[string]any with []any children.
func stringSliceConfig(cfg map[string]any, key string, fallback []string) []string {
	v, ok := cfg[key]
	if !ok || v == nil {
		return fallback
	}
	switch s := v.(type) {
	case []string:
		if len(s) == 0 {
			return fallback
		}
		return s
	case []any:
		out := make([]string, 0, len(s))
		for _, item := range s {
			if str, ok := item.(string); ok && str != "" {
				out = append(out, str)
			}
		}
		if len(out) == 0 {
			return fallback
		}
		return out
	}
	return fallback
}

// stringSetConfig reads a []string config value as a set for fast
// lookup. The fallback is also wrapped into a set.
func stringSetConfig(cfg map[string]any, key string, fallback []string) map[string]bool {
	src := stringSliceConfig(cfg, key, fallback)
	out := make(map[string]bool, len(src))
	for _, name := range src {
		out[name] = true
	}
	return out
}

func init() {
	audit.Default.MustRegister(SpecCoverageCheck{})
}
