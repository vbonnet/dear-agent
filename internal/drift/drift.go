// Package drift detects deployment drift: the gap between an artifact's
// source of truth in the repository and the copy actually deployed on the
// host.
//
// # Why this exists
//
// "Merged to main" is not "deployed on the host". Several dear-agent
// artifacts ship by being copied or rendered onto the machine, not by being
// imported as Go packages:
//
//   - Claude Code lifecycle hooks (~/.claude/hooks/*), embedded in the agm
//     binary and written out by `agm admin install-hooks`.
//   - launchd plists (~/Library/LaunchAgents/com.dear-agent.*), rendered from
//     templates by `make install-*-launchagent` / `agm admin install-*`.
//   - Claude Code settings and other chezmoi-managed files.
//
// When a PR changes one of these but the redeploy step is skipped, the fix
// lives in git yet never reaches the machine. PR #456 merged a gopls reaper
// into the stop hook; the hook was never redeployed, leaking processes for two
// days. This package is the cheap, build-free check that catches that class:
// hash the deployed file, hash the source-of-truth file, and report DRIFT when
// they disagree.
//
// The check is deliberately just file hashing — no builds, no network beyond
// an optional `git show`. It is fast enough to run on every daily ops audit
// and as the "deployed" gate of the bead lifecycle (see
// cmd/drift-check/README.md).
package drift

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// schemaVersion identifies the JSON output contract so downstream consumers
// (OTel exporters, the daily ops audit, monitoring) can detect a format
// change. Bump it on any breaking change to Report's shape.
const schemaVersion = "drift-check/v1"

// Status is the outcome of comparing one deploy target.
type Status string

const (
	// StatusOK means the deployed file matches the source of truth.
	StatusOK Status = "ok"
	// StatusDrift means both files exist but their hashes differ — the
	// deployed copy is stale relative to main.
	StatusDrift Status = "drift"
	// StatusMissingDeployed means the source exists but nothing is deployed.
	// For a required target this is drift (the artifact was never installed);
	// for an optional one it is reported as skipped.
	StatusMissingDeployed Status = "missing_deployed"
	// StatusMissingSource means the configured source path does not exist in
	// the repo — a config error, not host drift.
	StatusMissingSource Status = "missing_source"
	// StatusError means the comparison could not be completed (read failure,
	// git error). It is never silently treated as OK.
	StatusError Status = "error"
)

// Target is one artifact to check: a source of truth in the repo and the
// place it is deployed to on the host.
type Target struct {
	// Name is a short human label used in output.
	Name string `yaml:"name" json:"name"`
	// Deployed is the path of the installed copy on the host. It may contain a
	// leading ~ and $ENV references, both expanded before reading.
	Deployed string `yaml:"deployed" json:"deployed"`
	// Source is the path of the source of truth, relative to the repo root.
	Source string `yaml:"source" json:"source"`
	// Tokens maps placeholder strings in the source to their deployed values
	// (e.g. "__USER_HOME__" -> "${HOME}"). They are substituted into the
	// source content before hashing so a templated artifact (a launchd plist)
	// compares equal to its rendered, installed form. Values are $ENV-expanded.
	Tokens map[string]string `yaml:"tokens,omitempty" json:"tokens,omitempty"`
	// Remediation is the command that redeploys this artifact. It is surfaced
	// in output and audit so a human (or a future auto-remediation phase) knows
	// exactly how to close the gap.
	Remediation string `yaml:"remediation,omitempty" json:"remediation,omitempty"`
	// Optional marks targets whose absence on the host is acceptable (e.g. a
	// launchd job a given machine has chosen not to install). A missing
	// deployed file for an optional target is "skipped", not drift.
	Optional bool `yaml:"optional,omitempty" json:"optional,omitempty"`
}

// Config is the set of targets to check.
type Config struct {
	Targets []Target `yaml:"targets" json:"targets"`
}

// TargetResult is the outcome of checking one Target.
type TargetResult struct {
	Name           string `json:"name"`
	DeployedPath   string `json:"deployed_path"`
	SourcePath     string `json:"source_path"`
	Status         Status `json:"status"`
	DeployedSHA256 string `json:"deployed_sha256,omitempty"`
	SourceSHA256   string `json:"source_sha256,omitempty"`
	// Diff is a human-readable description of what changed, populated only on
	// StatusDrift.
	Diff string `json:"diff,omitempty"`
	// Remediation echoes the target's redeploy command, populated whenever the
	// status is actionable (drift or required-missing).
	Remediation string `json:"remediation,omitempty"`
	Error       string `json:"error,omitempty"`
}

// Summary tallies target outcomes for quick scanning and monitoring.
type Summary struct {
	Total   int `json:"total"`
	OK      int `json:"ok"`
	Drift   int `json:"drift"`
	Missing int `json:"missing"` // required targets with no deployed file
	Skipped int `json:"skipped"` // optional targets with no deployed file
	Error   int `json:"error"`
}

// Report is the full result of a drift check — the JSON contract consumed by
// tooling.
type Report struct {
	Schema    string         `json:"schema"`
	CheckedAt string         `json:"checked_at"`
	RepoRoot  string         `json:"repo_root"`
	GitRef    string         `json:"git_ref,omitempty"`
	Summary   Summary        `json:"summary"`
	Targets   []TargetResult `json:"targets"`
}

// HasDrift reports whether the run found anything actionable: a stale deployed
// file or a required artifact that was never installed. Errors and skipped
// optional targets do not count as drift (they are surfaced separately).
func (r Report) HasDrift() bool {
	return r.Summary.Drift > 0 || r.Summary.Missing > 0
}

// Options configures a Check run.
type Options struct {
	// RepoRoot is the directory source paths are resolved against. When empty,
	// Check refuses to run rather than guess.
	RepoRoot string
	// GitRef, when set, reads source files via `git show <ref>:<path>` instead
	// of the working tree — so the check compares against committed main rather
	// than whatever is currently checked out. Empty means "working tree".
	GitRef string
	// Home overrides the home directory used to expand a leading ~ in deployed
	// paths. Empty uses os.UserHomeDir.
	Home string
	// Now stamps Report.CheckedAt. Empty uses the current UTC time. It is
	// injectable so tests are deterministic.
	Now string
}

// Check runs every target in cfg and returns a Report. It returns an error
// only for a setup failure (no repo root); per-target failures are captured as
// StatusError inside the Report so one bad target never hides the others.
func Check(ctx context.Context, cfg Config, opts Options) (Report, error) {
	if opts.RepoRoot == "" {
		return Report{}, fmt.Errorf("drift: RepoRoot is required (the directory source paths resolve against)")
	}
	home := opts.Home
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return Report{}, fmt.Errorf("drift: cannot resolve home dir: %w", err)
		}
		home = h
	}
	now := opts.Now
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339)
	}

	rep := Report{
		Schema:    schemaVersion,
		CheckedAt: now,
		RepoRoot:  opts.RepoRoot,
		GitRef:    opts.GitRef,
	}
	for _, t := range cfg.Targets {
		res := checkTarget(ctx, t, opts.RepoRoot, opts.GitRef, home)
		rep.Targets = append(rep.Targets, res)
		rep.Summary.Total++
		switch res.Status {
		case StatusOK:
			rep.Summary.OK++
		case StatusDrift:
			rep.Summary.Drift++
		case StatusMissingDeployed:
			if t.Optional {
				rep.Summary.Skipped++
			} else {
				rep.Summary.Missing++
			}
		case StatusMissingSource, StatusError:
			rep.Summary.Error++
		}
	}
	return rep, nil
}

func checkTarget(ctx context.Context, t Target, repoRoot, gitRef, home string) TargetResult {
	deployedPath := expandPath(t.Deployed, home)
	res := TargetResult{
		Name:         t.Name,
		DeployedPath: deployedPath,
		SourcePath:   t.Source,
	}

	source, err := readSource(ctx, repoRoot, gitRef, t.Source)
	if err != nil {
		res.Status = StatusMissingSource
		res.Error = err.Error()
		return res
	}
	source = applyTokens(source, t.Tokens, home)
	res.SourceSHA256 = sha256hex(source)

	deployed, err := os.ReadFile(deployedPath)
	if err != nil {
		if os.IsNotExist(err) {
			res.Status = StatusMissingDeployed
			if !t.Optional {
				res.Remediation = t.Remediation
			}
			return res
		}
		res.Status = StatusError
		res.Error = fmt.Sprintf("reading deployed file: %v", err)
		return res
	}
	res.DeployedSHA256 = sha256hex(deployed)

	if res.DeployedSHA256 == res.SourceSHA256 {
		res.Status = StatusOK
		return res
	}
	res.Status = StatusDrift
	res.Diff = lineDiff(deployed, source)
	res.Remediation = t.Remediation
	return res
}

// readSource reads the source of truth, either from the working tree (gitRef
// == "") or from a committed ref via `git show`.
func readSource(ctx context.Context, repoRoot, gitRef, rel string) ([]byte, error) {
	if gitRef == "" {
		path := filepath.Join(repoRoot, rel)
		b, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("source not found in repo: %s", rel)
			}
			return nil, fmt.Errorf("reading source %s: %w", rel, err)
		}
		return b, nil
	}
	// `git show <ref>:<path>` wants a forward-slash, repo-relative path.
	spec := gitRef + ":" + filepath.ToSlash(rel)
	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "show", spec)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git show %s: %w (stderr: %s)", spec, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// homeMapper returns an os.Expand mapping that resolves $HOME/${HOME} to the
// resolved home directory rather than the process environment. This keeps token
// and path expansion consistent with the ~ handling and with Options.Home, so an
// overridden home (tests, multi-user) does not produce false mismatches.
func homeMapper(home string) func(string) string {
	return func(k string) string {
		if k == "HOME" && home != "" {
			return home
		}
		return os.Getenv(k)
	}
}

// applyTokens substitutes each placeholder with its expanded value. This
// renders a templated source (e.g. a plist with __USER_HOME__) into the form
// the host actually has on disk, so the hash compare is meaningful. $HOME
// resolves to the resolved home (honoring Options.Home).
func applyTokens(content []byte, tokens map[string]string, home string) []byte {
	if len(tokens) == 0 {
		return content
	}
	mapper := homeMapper(home)
	s := string(content)
	for placeholder, value := range tokens {
		s = strings.ReplaceAll(s, placeholder, os.Expand(value, mapper))
	}
	return []byte(s)
}

// expandPath turns a configured deployed path into an absolute filesystem
// path: a leading ~ becomes home, then $ENV references are expanded ($HOME
// resolving to the resolved home, not the process environment).
func expandPath(p, home string) string {
	if p == "~" {
		p = home
	} else if strings.HasPrefix(p, "~/") {
		p = filepath.Join(home, p[2:])
	}
	return os.Expand(p, homeMapper(home))
}

func sha256hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// lineDiff renders a compact, human-readable description of how deployed and
// source differ. It trims the common leading and trailing lines so the output
// focuses on the changed region — the common case is a few lines added or
// removed in the middle of an otherwise-identical file. Output is bounded so a
// whole-file replacement does not dump megabytes into a log.
func lineDiff(deployed, source []byte) string {
	const maxLines = 25
	dl := strings.Split(string(deployed), "\n")
	sl := strings.Split(string(source), "\n")

	// Trim common prefix.
	start := 0
	for start < len(dl) && start < len(sl) && dl[start] == sl[start] {
		start++
	}
	// Trim common suffix (without crossing the prefix already consumed).
	de, se := len(dl), len(sl)
	for de > start && se > start && dl[de-1] == sl[se-1] {
		de--
		se--
	}

	var b strings.Builder
	fmt.Fprintf(&b, "deployed: %d line(s), source: %d line(s); first divergence near line %d\n",
		len(dl), len(sl), start+1)
	writeBlock(&b, "- deployed", dl[start:de], maxLines)
	writeBlock(&b, "+ source  ", sl[start:se], maxLines)
	return strings.TrimRight(b.String(), "\n")
}

func writeBlock(b *strings.Builder, prefix string, lines []string, limit int) {
	if len(lines) == 0 {
		return
	}
	shown := lines
	truncated := 0
	if len(lines) > limit {
		shown = lines[:limit]
		truncated = len(lines) - limit
	}
	for _, ln := range shown {
		fmt.Fprintf(b, "%s | %s\n", prefix, ln)
	}
	if truncated > 0 {
		fmt.Fprintf(b, "%s | … (%d more line(s))\n", prefix, truncated)
	}
}
