package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/internal/drift"
)

// driftConfigRelPath is the deploy-target config, relative to the repo root.
// The deployed gate reuses drift-check's single source of truth so the two
// tools always describe the same artifacts the same way — there is no second
// list to keep in sync.
const driftConfigRelPath = "cmd/drift-check/targets.yaml"

// DeployFinding is one deploy target that a bead's merged change touched but
// which is not current on the host — the "merged != deployed" gap made
// concrete for a single artifact.
type DeployFinding struct {
	Target      string
	Deployed    string
	Status      string // "drift" (stale on host) | "missing" (never installed)
	Remediation string
}

// deployedGate is the "deployed" gate of the bead lifecycle documented in
// cmd/drift-check/README.md. Given the files a bead's *merged* PRs
// changed, it finds the deploy targets whose source was touched and checks
// whether each one is current on the host. A stale (drift) or never-installed
// (missing, required) target is returned as a finding that blocks the close —
// closing the gap where a fix is merged to main but never reached the machine.
//
// It is deliberately fail-open on its own infrastructure: when the
// deploy-target config or a dear-agent checkout is not available where the
// close runs, the gate records why it was skipped and returns no findings
// rather than blocking a legitimate close. A *genuine* drift, by contrast,
// always blocks — that is the whole point. The abandon-reason override still
// releases both gates.
func deployedGate(ctx context.Context, repoRoot, gitRef string, changedFiles []string) (findings []DeployFinding, checked int, skipNote string) {
	if repoRoot == "" {
		return nil, 0, "no repo root resolved (run from a dear-agent checkout or pass --repo-root)"
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, driftConfigRelPath))
	if err != nil {
		// Not a dear-agent checkout (or an unusual layout): nothing to verify.
		return nil, 0, "deploy-target config " + driftConfigRelPath + " not found under " + repoRoot
	}
	cfg, err := drift.ParseConfig(data)
	if err != nil {
		return nil, 0, "deploy-target config invalid: " + err.Error()
	}

	matched := matchTargets(changedFiles, cfg)
	if len(matched) == 0 {
		// The merged change touched no deploy target — the gate ran and is clean.
		return nil, 0, ""
	}

	rep, err := drift.Check(ctx, drift.Config{Targets: matched}, drift.Options{RepoRoot: repoRoot, GitRef: gitRef})
	if err != nil {
		return nil, len(matched), "drift check could not run: " + err.Error()
	}

	for _, t := range rep.Targets {
		switch t.Status {
		case drift.StatusDrift:
			findings = append(findings, DeployFinding{
				Target: t.Name, Deployed: t.DeployedPath, Status: "drift", Remediation: t.Remediation,
			})
		case drift.StatusMissingDeployed:
			// internal/drift leaves Remediation empty for an *optional* missing
			// target (a machine may legitimately not install every job); only a
			// required artifact carries one. Block solely on the required case.
			if t.Remediation != "" {
				findings = append(findings, DeployFinding{
					Target: t.Name, Deployed: t.DeployedPath, Status: "missing", Remediation: t.Remediation,
				})
			}
		case drift.StatusOK, drift.StatusMissingSource, drift.StatusError:
			// Not close blockers: StatusOK is a current host; StatusMissingSource
			// and StatusError are config or read problems in the repo itself, not
			// host drift. A clean host or a config bug must never wedge a bead.
		}
	}
	return findings, len(matched), ""
}

// matchTargets returns the deploy targets whose source-of-truth file is among
// changedFiles, in config order with no duplicates (config sources are unique).
// Paths are normalised to forward slashes so a gh file list entry ("a/b.go")
// matches a config source declared the same way.
func matchTargets(changedFiles []string, cfg drift.Config) []drift.Target {
	changed := make(map[string]bool, len(changedFiles))
	for _, f := range changedFiles {
		f = strings.TrimSpace(f)
		if f != "" {
			changed[filepath.ToSlash(f)] = true
		}
	}
	var out []drift.Target
	for _, t := range cfg.Targets {
		if changed[filepath.ToSlash(t.Source)] {
			out = append(out, t)
		}
	}
	return out
}

// dedupRemediations returns the distinct, non-empty remediation commands across
// findings, preserving first-seen order — several stale hooks often share one
// redeploy command (`agm admin install-hooks`), so the block shows it once.
func dedupRemediations(findings []DeployFinding) []string {
	seen := make(map[string]bool, len(findings))
	var out []string
	for _, f := range findings {
		if f.Remediation != "" && !seen[f.Remediation] {
			seen[f.Remediation] = true
			out = append(out, f.Remediation)
		}
	}
	return out
}

// detectRepoRoot resolves the dear-agent checkout the deployed gate reads source
// files from — the git toplevel of the current directory. It returns "" on any
// failure; deployedGate treats an empty root as "skip, do not block".
func detectRepoRoot() string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel").Output()
	if ctx.Err() != nil {
		return ""
	}
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
