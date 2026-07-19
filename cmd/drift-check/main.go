// Command drift-check detects deployment drift: artifacts whose source of
// truth in this repo no longer matches the copy deployed on the host.
//
// It is the manual check documented in cmd/drift-check/README.md. It only
// hashes files — no builds, no network
// beyond an optional `git show` — so it is cheap enough to run on every daily
// ops audit and as the "deployed" gate of the bead lifecycle.
//
// Usage:
//
//	drift-check [--config <file>] [--repo-root <dir>] [--git-ref <ref>]
//	            [--json] [--audit] [--quiet]
//
// Exit codes:
//
//	0  No drift — every deployed artifact matches its source of truth.
//	2  Drift detected — a deployed artifact is stale, or a required one is
//	   not installed. Re-run the artifact's remediation command.
//	1  Error — bad config, unreadable repo root, etc.
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/internal/drift"
	"github.com/vbonnet/dear-agent/internal/driftaudit"
)

//go:embed targets.yaml
var defaultConfig []byte

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("drift-check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		configPath = fs.String("config", "", "deploy-target config file (default: built-in targets.yaml)")
		repoRoot   = fs.String("repo-root", "", "repo root that source paths resolve against (default: git toplevel of cwd)")
		gitRef     = fs.String("git-ref", "", "compare against this git ref via `git show` instead of the working tree (e.g. origin/main)")
		asJSON     = fs.Bool("json", false, "emit the structured JSON report (for OTel/monitoring)")
		audit      = fs.Bool("audit", false, "append the run to ~/.local/state/dear-agent/drift-check.log")
		quiet      = fs.Bool("quiet", false, "suppress the per-target OK lines in text output")
	)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: drift-check [flags]\n\n"+
			"Detects deployment drift: deployed artifacts that no longer match their\n"+
			"source of truth in the repo (a fix merged to main but never redeployed).\n\n"+
			"Exit 0 = no drift\n"+
			"Exit 2 = drift detected (re-run the remediation command)\n"+
			"Exit 1 = error\n\n"+
			"flags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 1
	}

	cfgData := defaultConfig
	if *configPath != "" {
		b, err := os.ReadFile(*configPath)
		if err != nil {
			fmt.Fprintf(stderr, "error: reading config: %v\n", err)
			return 1
		}
		cfgData = b
	}
	cfg, err := drift.ParseConfig(cfgData)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	// Create the bounded context before any external command so git invocations
	// (repo-root detection and `git show`) are all guarded by the same timeout —
	// a hung `git rev-parse` (locked index, stuck network mount) fails fast
	// instead of blocking the CLI indefinitely.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	root := *repoRoot
	if root == "" {
		detected, err := gitToplevel(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "error: cannot detect repo root: %v\n", err)
			fmt.Fprintf(stderr, "hint: run inside the dear-agent checkout or pass --repo-root <dir>\n")
			return 1
		}
		root = detected
	}

	rep, err := drift.Check(ctx, cfg, drift.Options{RepoRoot: root, GitRef: *gitRef})
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	if *audit {
		if err := driftaudit.Append(homeDir(), auditRecord(rep)); err != nil {
			// Non-fatal: a missed audit line must never hide real drift.
			fmt.Fprintf(stderr, "warning: could not write audit log: %v\n", err)
		}
	}

	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			fmt.Fprintf(stderr, "error: encoding report: %v\n", err)
			return 1
		}
	} else {
		formatText(rep, stdout, *quiet)
	}

	if rep.HasDrift() {
		return 2
	}
	if rep.Summary.Error > 0 {
		// Drift-free, but a target could not be evaluated — surface as error so
		// an unreadable artifact never masquerades as a clean run.
		return 1
	}
	return 0
}

// auditRecord projects a Report into the audit log shape, keeping only
// actionable targets (drift or required-missing) in the per-target detail.
func auditRecord(rep drift.Report) driftaudit.Record {
	rec := driftaudit.Record{
		Time:     rep.CheckedAt,
		HasDrift: rep.HasDrift(),
		Total:    rep.Summary.Total,
		OK:       rep.Summary.OK,
		Drift:    rep.Summary.Drift,
		Missing:  rep.Summary.Missing,
		Skipped:  rep.Summary.Skipped,
		Errors:   rep.Summary.Error,
		GitRef:   rep.GitRef,
	}
	for _, t := range rep.Targets {
		if t.Status == drift.StatusDrift || (t.Status == drift.StatusMissingDeployed && t.Remediation != "") {
			rec.Drifted = append(rec.Drifted, driftaudit.DriftTarget{
				Name:        t.Name,
				Deployed:    t.DeployedPath,
				Source:      t.SourcePath,
				Status:      string(t.Status),
				Remediation: t.Remediation,
			})
		}
	}
	return rec
}

func formatText(rep drift.Report, w io.Writer, quiet bool) {
	for _, t := range rep.Targets {
		switch t.Status {
		case drift.StatusOK:
			if !quiet {
				fmt.Fprintf(w, "  ok       %s\n", t.Name)
			}
		case drift.StatusDrift:
			fmt.Fprintf(w, "  DRIFT    %s\n", t.Name)
			fmt.Fprintf(w, "           deployed: %s\n", t.DeployedPath)
			fmt.Fprintf(w, "           source:   %s\n", t.SourcePath)
			for line := range strings.SplitSeq(t.Diff, "\n") {
				fmt.Fprintf(w, "           %s\n", line)
			}
			if t.Remediation != "" {
				fmt.Fprintf(w, "           fix: %s\n", t.Remediation)
			}
		case drift.StatusMissingDeployed:
			fmt.Fprintf(w, "  MISSING  %s (not deployed: %s)\n", t.Name, t.DeployedPath)
			if t.Remediation != "" {
				fmt.Fprintf(w, "           fix: %s\n", t.Remediation)
			}
		case drift.StatusMissingSource:
			fmt.Fprintf(w, "  CONFIG   %s — source not in repo: %s\n", t.Name, t.SourcePath)
		case drift.StatusError:
			fmt.Fprintf(w, "  ERROR    %s — %s\n", t.Name, t.Error)
		}
	}

	s := rep.Summary
	fmt.Fprintf(w, "\n%d target(s): %d ok, %d drift, %d missing, %d skipped, %d error\n",
		s.Total, s.OK, s.Drift, s.Missing, s.Skipped, s.Error)
	if rep.HasDrift() {
		fmt.Fprintf(w, "DRIFT DETECTED — a merged fix has not reached the host. Re-run the fix command(s) above.\n")
	} else if s.Error == 0 {
		fmt.Fprintf(w, "No drift: every deployed artifact matches main.\n")
	}
}

func gitToplevel(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}
