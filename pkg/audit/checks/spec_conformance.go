package checks

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/audit"
)

// SpecConformanceCheck verifies that file/package paths referenced in SPEC.md
// files actually exist in the codebase. A SPEC.md that references a path that
// no longer exists has rotted — it may describe an API, design doc, or
// acceptance-criteria location that has been moved or deleted.
//
// This is the third cheap spec-compliance check:
//   - Tier 1 (spec.coverage):   detect missing SPECs
//   - Tier 2 (spec.staleness):  detect stale SPECs
//   - Tier 3 (spec.conformance): detect broken path references in SPECs
//
// Cheap mode: regex-extracts backtick-wrapped strings that contain a path
// separator (/) and checks whether each resolves to an existing path under
// env.RepoRoot. Paths with known false-positive prefixes (URLs, shell
// expansions, Go import paths that differ from disk layout) are skipped.
//
// Config knobs:
//   - roots: []string — same default as spec.coverage.
//   - skip: []string — directory names to skip.
//
// Severity is P2 — a dead path reference is drift, not a runtime failure.
type SpecConformanceCheck struct{}

// Meta returns the check metadata for SpecConformanceCheck.
func (SpecConformanceCheck) Meta() audit.CheckMeta {
	return audit.CheckMeta{
		ID:              "spec.conformance",
		Description:     "Paths referenced in SPEC.md must exist in the repo",
		Cadence:         audit.CadenceWeekly,
		SeverityCeiling: audit.SeverityP2,
	}
}

// reBacktickPath matches backtick-delimited tokens that contain at least one
// slash — the minimal indicator that a token is meant as a file/dir path.
var reBacktickPath = regexp.MustCompile("`([^`]+/[^`]*)`")

// Run walks each configured root one level deep, reads SPEC.md for every
// directory that has one, extracts backtick-wrapped path references, and
// emits one finding per reference that does not resolve to an existing path
// under env.RepoRoot.
func (SpecConformanceCheck) Run(ctx context.Context, env audit.Env) (audit.Result, error) {
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
			if os.IsNotExist(err) {
				continue
			}
			out.Status = audit.StatusError
			return out, fmt.Errorf("audit/spec.conformance: read %s: %w", rootAbs, err)
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
			dead, err := deadPathRefs(specPath, env.RepoRoot)
			if err != nil {
				if env.Logger != nil {
					env.Logger.Warn("audit/spec.conformance: read failed",
						"spec", specPath, "err", err)
				}
				continue
			}
			for _, ref := range dead {
				findings = append(findings, audit.Finding{
					Fingerprint: audit.Fingerprint("spec.conformance", rel+":"+ref),
					Severity:    audit.SeverityP2,
					Title:       fmt.Sprintf("%s/SPEC.md references missing path %q", rel, ref),
					Detail: fmt.Sprintf(
						"%s/SPEC.md contains a backtick-wrapped path reference `%s` that "+
							"does not exist under the repo root. The SPEC may describe a file "+
							"that was moved, renamed, or deleted. Update the reference or "+
							"remove the stale mention.",
						rel, ref,
					),
					Path: rel + "/SPEC.md",
					Suggested: audit.Remediation{
						Strategy: audit.StrategyNoop,
					},
					Evidence: map[string]any{
						"directory": rel,
						"dead_ref":  ref,
					},
				})
			}
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		return findings[i].Title < findings[j].Title
	})
	out.Findings = findings
	return out, nil
}

// deadPathRefs reads specFile, extracts backtick-wrapped tokens that look like
// file/directory paths, and returns the subset that do not exist under
// repoRoot. URL references and Go module import paths are filtered out because
// they are not disk paths.
func deadPathRefs(specFile, repoRoot string) ([]string, error) {
	f, err := os.Open(specFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	seen := map[string]struct{}{}
	var dead []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		for _, m := range reBacktickPath.FindAllStringSubmatch(line, -1) {
			token := m[1]
			if _, ok := seen[token]; ok {
				continue
			}
			seen[token] = struct{}{}
			if shouldSkipPathToken(token) {
				continue
			}
			abs := filepath.Join(repoRoot, filepath.FromSlash(token))
			if _, err := os.Stat(abs); os.IsNotExist(err) {
				dead = append(dead, token)
			}
		}
	}
	return dead, scanner.Err()
}

// shouldSkipPathToken returns true for tokens that are not disk paths and
// would produce false positives if we tried to stat them.
func shouldSkipPathToken(token string) bool {
	// URLs.
	if strings.HasPrefix(token, "http://") || strings.HasPrefix(token, "https://") {
		return true
	}
	// Shell/env expansions and globs.
	if strings.ContainsAny(token, "*?${}[]") {
		return true
	}
	// Go module import paths look like "github.com/..." or "golang.org/...".
	// They are not relative disk paths.
	for _, host := range []string{"github.com/", "golang.org/", "google.golang.org/",
		"gopkg.in/", "go.uber.org/", "go.opentelemetry.io/"} {
		if strings.HasPrefix(token, host) {
			return true
		}
	}
	// Absolute paths (start with /) are not repo-relative; skip.
	if strings.HasPrefix(token, "/") {
		return true
	}
	// Tokens starting with "~/" are home-relative; not repo paths.
	if strings.HasPrefix(token, "~/") {
		return true
	}
	return false
}

func init() {
	audit.Default.MustRegister(SpecConformanceCheck{})
}
