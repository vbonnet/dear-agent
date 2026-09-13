// Command dear-deploy deploys dear-agent's host artifacts — launchd plists and
// Claude Code hooks — from their source of truth in this repo to their installed
// location on the machine. It is the write-side counterpart to drift-check:
// drift-check tells you a deployed artifact is stale; dear-deploy makes it
// current.
//
// Every write goes through the principle-9 atomic sequence (stage → verify →
// activate, see internal/deploy): a failed deploy never leaves a half-written
// file in place, and there is no bypass flag (ADR-031).
//
// Subcommands:
//
//	dear-deploy list                 list every deployable artifact
//	dear-deploy status [name...]     show deployed state vs the manifest
//	dear-deploy sync   [name...]     deploy artifacts that have drifted (idempotent)
//	dear-deploy install [name...]    (re)install artifacts, even if unchanged
//
// With no names, status/sync/install operate on the whole manifest. Common
// flags: --manifest <file>, --repo-root <dir>, --json, --dry-run (sync/install).
//
// Artifacts come in two kinds. File artifacts (plists, compiled hooks) are
// compared and deployed by byte content. Binary artifacts (Go programs such as
// mergeloop) are status-only: their deployed copy is compared by the
// vcs.revision embedded at build time — what `go version -m` prints — against
// the repo HEAD, so a binary built before a fix landed shows as stale drift.
// sync/install never copy a binary; it is (re)built out of band by its
// remediation command (e.g. `make install-mergeloop`).
//
// Exit codes: 0 success / clean; 2 (status only) drift or a required artifact
// missing; 1 error.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/internal/deploy"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 1
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "list":
		return runList(rest, stdout, stderr)
	case "status":
		return runStatus(rest, stdout, stderr)
	case "sync":
		return runDeploy(cmd, rest, stdout, stderr)
	case "install":
		return runDeploy(cmd, rest, stdout, stderr)
	case "build-install":
		return runBuildInstall(rest, stdout, stderr)
	case "merge-pulses":
		return runMergePulses(rest, stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "dear-deploy: unknown subcommand %q\n\n%s", cmd, usage)
		return 1
	}
}

// commonFlags holds the flags every subcommand shares plus the resolved
// manifest and options, so each runner does the same setup once.
type commonFlags struct {
	manifest string
	repoRoot string
	home     string
	asJSON   bool
}

func (c *commonFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&c.manifest, "manifest", "", "manifest file (default: <repo-root>/deploy/manifest.yaml)")
	fs.StringVar(&c.repoRoot, "repo-root", "", "repo root that source paths resolve against (default: git toplevel of cwd)")
	fs.StringVar(&c.home, "home", "", "home directory for expanding ~ in deployed paths (default: $HOME)")
	fs.BoolVar(&c.asJSON, "json", false, "emit a structured JSON report")
}

// parseArgs parses flags that may appear before, after, or interspersed with
// the positional artifact names. Go's flag package stops at the first
// non-flag token, so a plain fs.Parse would treat `sync hook --dry-run` as
// three positionals; this loops Parse to collect flags from every gap and
// returns the positional names. This lets `dear-deploy sync x --json` work the
// way a user expects.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return positional, nil
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}

// load resolves the repo root, reads and parses the manifest, and selects the
// requested artifacts. It centralises the error reporting so each subcommand
// stays focused on its own output.
func (c *commonFlags) load(names []string, stderr io.Writer) ([]deploy.Artifact, deploy.Options, int) {
	root := c.repoRoot
	if root == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		detected, err := gitToplevel(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "error: cannot detect repo root: %v\n", err)
			fmt.Fprintf(stderr, "hint: run inside the dear-agent checkout or pass --repo-root <dir>\n")
			return nil, deploy.Options{}, 1
		}
		root = detected
	}

	manifestPath := c.manifest
	if manifestPath == "" {
		manifestPath = filepath.Join(root, "deploy", "manifest.yaml")
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: reading manifest: %v\n", err)
		return nil, deploy.Options{}, 1
	}
	m, err := deploy.ParseManifest(data)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return nil, deploy.Options{}, 1
	}
	selected, err := m.Select(names)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return nil, deploy.Options{}, 1
	}

	// Resolve home eagerly so displayed deployed paths are concrete (~ expanded)
	// even in list/status, not just inside Deploy. An empty --home falls back to
	// $HOME exactly as deploy.Options would have.
	home := c.home
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(stderr, "error: cannot resolve home dir: %v\n", err)
			return nil, deploy.Options{}, 1
		}
		home = h
	}
	return selected, deploy.Options{RepoRoot: root, Home: home}, 0
}

func runList(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var c commonFlags
	c.register(fs)
	names, err := parseArgs(fs, args)
	if err != nil {
		return 1
	}
	selected, opts, code := c.load(names, stderr)
	if code != 0 {
		return code
	}

	if c.asJSON {
		type item struct {
			Name        string `json:"name"`
			Kind        string `json:"kind,omitempty"`
			Source      string `json:"source"`
			Deployed    string `json:"deployed"`
			Mode        string `json:"mode"`
			Optional    bool   `json:"optional,omitempty"`
			Remediation string `json:"remediation,omitempty"`
		}
		out := make([]item, 0, len(selected))
		for _, a := range selected {
			mode, _ := a.FileMode()
			kind := string(a.Kind)
			if kind == "" {
				kind = string(deploy.KindFile)
			}
			out = append(out, item{
				Name:        a.Name,
				Kind:        kind,
				Source:      a.Source,
				Deployed:    a.DeployedPath(opts.Home),
				Mode:        fmt.Sprintf("%04o", mode.Perm()),
				Optional:    a.Optional,
				Remediation: a.Remediation,
			})
		}
		return emitJSON(out, stdout, stderr)
	}

	for _, a := range selected {
		mode, _ := a.FileMode()
		opt := ""
		if a.Optional {
			opt = " (optional)"
		}
		if a.IsBinary() {
			opt += " [binary]"
		}
		fmt.Fprintf(stdout, "%s%s\n", a.Name, opt)
		fmt.Fprintf(stdout, "    source:   %s\n", a.Source)
		fmt.Fprintf(stdout, "    deployed: %s  [%04o]\n", a.DeployedPath(opts.Home), mode.Perm())
	}
	fmt.Fprintf(stdout, "\n%d artifact(s)\n", len(selected))
	return 0
}

func runStatus(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var c commonFlags
	c.register(fs)
	names, err := parseArgs(fs, args)
	if err != nil {
		return 1
	}
	selected, opts, code := c.load(names, stderr)
	if code != 0 {
		return code
	}

	results := make([]deploy.StatusResult, 0, len(selected))
	for _, a := range selected {
		results = append(results, deploy.Status(a, opts))
	}

	// A required pulse that a sync would merge is real drift, even though the
	// absent-only registry itself compares clean. Reporting OK here is how a
	// missed migration stays invisible to a deployment audit.
	pendingPulses, pulseErr := pendingPulseNames(selected, opts, stderr)
	for _, n := range pendingPulses {
		results = append(results, deploy.StatusResult{
			Name:         pulseArtifactName + ":" + n,
			State:        deploy.StateDrift,
			DeployedPath: n,
		})
	}
	if pulseErr != nil {
		// "Could not check" is not "clean". A status that exits 0 because it
		// failed to read the registry tells an audit the host is fine when
		// nobody looked.
		results = append(results, deploy.StatusResult{
			Name:  pulseArtifactName + ":merge-check",
			State: deploy.StateError,
			Error: pulseErr.Error(),
		})
	}

	if c.asJSON {
		if rc := emitJSON(results, stdout, stderr); rc != 0 {
			return rc
		}
	} else {
		formatStatus(results, stdout)
	}

	// Exit 2 on actionable state (drift, or a *required* artifact missing),
	// 1 on an evaluation error, 0 clean — matching drift-check's contract so
	// dear-deploy status can serve as the same kind of gate.
	drift, errs := false, false
	for _, r := range results {
		switch r.State {
		case deploy.StateOK:
			// clean — no action needed
		case deploy.StateDrift:
			drift = true
		case deploy.StateMissing, deploy.StateSourceMissing:
			if !r.Optional {
				drift = true
			}
		case deploy.StateError:
			errs = true
		}
	}
	switch {
	case drift:
		return 2
	case errs:
		return 1
	default:
		return 0
	}
}

func formatStatus(results []deploy.StatusResult, w io.Writer) {
	var ok, drift, missing, srcMissing, skipped, errs int
	for _, r := range results {
		switch r.State {
		case deploy.StateOK:
			ok++
			if r.Kind == deploy.KindBinary {
				fmt.Fprintf(w, "  ok        %s (current %s)\n", r.Name, r.DeployedVersion)
			} else {
				fmt.Fprintf(w, "  ok        %s\n", r.Name)
			}
		case deploy.StateDrift:
			drift++
			fmt.Fprintf(w, "  DRIFT     %s\n", r.Name)
			if r.Kind == deploy.KindBinary {
				fmt.Fprintf(w, "            deployed %s -> source %s — %s\n", r.DeployedVersion, r.SourceVersion, r.Detail)
				fmt.Fprintf(w, "            fix: %s\n", binaryFix(r))
			} else {
				fmt.Fprintf(w, "            deployed: %s\n", r.DeployedPath)
				fmt.Fprintf(w, "            fix: dear-deploy sync %s\n", r.Name)
			}
		case deploy.StateMissing:
			if r.Optional {
				skipped++
				fmt.Fprintf(w, "  skipped   %s (optional, not deployed)\n", r.Name)
			} else {
				missing++
				if r.Kind == deploy.KindBinary {
					fmt.Fprintf(w, "  MISSING   %s (NOT DEPLOYED: %s; source %s)\n", r.Name, r.DeployedPath, r.SourceVersion)
					fmt.Fprintf(w, "            fix: %s\n", binaryFix(r))
				} else {
					fmt.Fprintf(w, "  MISSING   %s (not deployed: %s)\n", r.Name, r.DeployedPath)
					fmt.Fprintf(w, "            fix: dear-deploy sync %s\n", r.Name)
				}
			}
		case deploy.StateSourceMissing:
			if r.Optional {
				skipped++
				fmt.Fprintf(w, "  skipped   %s (optional, source not built)\n", r.Name)
			} else {
				srcMissing++
				fmt.Fprintf(w, "  NO-SOURCE %s (source of truth not present)\n", r.Name)
				if r.Remediation != "" {
					fmt.Fprintf(w, "            build: %s\n", r.Remediation)
				}
			}
		case deploy.StateError:
			errs++
			fmt.Fprintf(w, "  ERROR     %s — %s\n", r.Name, r.Error)
		}
	}
	fmt.Fprintf(w, "\n%d artifact(s): %d ok, %d drift, %d missing, %d no-source, %d skipped, %d error\n",
		len(results), ok, drift, missing, srcMissing, skipped, errs)
	if drift+missing == 0 && errs == 0 {
		fmt.Fprintf(w, "In sync: every deployed artifact matches the manifest.\n")
	} else if drift+missing > 0 {
		fmt.Fprintf(w, "OUT OF SYNC — apply each fix line above (file artifacts: `dear-deploy sync`; binaries: their make target).\n")
	}
}

// binaryFix returns the command that redeploys a binary artifact. Binaries are
// built and installed out of band (not by `dear-deploy sync`), so the fix is the
// artifact's own Remediation; absent that, the generic make target.
func binaryFix(r deploy.StatusResult) string {
	if r.Remediation != "" {
		return r.Remediation
	}
	return "make install-" + r.Name
}

// runDeploy implements both `sync` and `install`. They share all machinery; the
// only difference is install forces a rewrite of every selected artifact while
// sync skips ones already matching their source.
func runDeploy(cmd string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(stderr)
	var c commonFlags
	c.register(fs)
	var dryRun bool
	fs.BoolVar(&dryRun, "dry-run", false, "show what would change without writing")
	names, err := parseArgs(fs, args)
	if err != nil {
		return 1
	}
	selected, opts, code := c.load(names, stderr)
	if code != 0 {
		return code
	}
	opts.Force = cmd == "install"

	if dryRun {
		return dryRunDeploy(cmd, selected, opts, c.asJSON, stdout, stderr)
	}

	results := make([]deploy.Result, 0, len(selected))
	var failures []string
	for _, a := range selected {
		r, err := deploy.Deploy(a, opts)
		if err != nil {
			// One bad artifact is reported but does not abort the rest: a
			// failed write is already rolled back (the target is untouched),
			// so continuing cannot corrupt anything.
			fmt.Fprintf(stderr, "  FAILED    %s — %v\n", a.Name, err)
			failures = append(failures, a.Name)
			continue
		}
		results = append(results, r)
	}

	if c.asJSON {
		if rc := emitJSON(results, stdout, stderr); rc != 0 {
			return rc
		}
	} else {
		formatDeploy(cmd, results, stdout)
	}

	// The pulse registry is absent-only, so Deploy reports it unchanged on
	// every host that already has one and a newly required pulse never lands.
	// This is the canonical publication path (the post-merge hook runs
	// `dear-deploy sync`), so the merge belongs here rather than only on the
	// manual launch-agent install target.
	if a, ok := artifactNamed(selected, pulseArtifactName); ok {
		if err := mergePulsesDuringDeploy(a, opts, c.asJSON, stdout, stderr); err != nil {
			// A required pulse that did not land means the recovery loop is
			// unwired for some job. Reporting a successful deploy over that is
			// the failure this whole area exists to remove, so it joins the
			// deploy's own failure list.
			fmt.Fprintf(stderr, "  FAILED    %s pulse merge — %v\n", a.Name, err)
			failures = append(failures, a.Name+" (pulse merge)")
		}
	}

	if len(failures) > 0 {
		fmt.Fprintf(stderr, "\n%s: %d artifact(s) failed: %s\n", cmd, len(failures), strings.Join(failures, ", "))
		return 1
	}
	return 0
}

func dryRunDeploy(cmd string, selected []deploy.Artifact, opts deploy.Options, asJSON bool, stdout, stderr io.Writer) int {
	type plan struct {
		Name         string `json:"name"`
		DeployedPath string `json:"deployed"`
		WouldDo      string `json:"would_do"`
	}
	force := cmd == "install"
	plans := make([]plan, 0, len(selected))
	// An absent-only registry always reports "unchanged", so without this a
	// preview would show nothing while a real sync merged required pulses.
	pendingPulses, pulsePreviewErr := pendingPulseNames(selected, opts, stderr)
	if pulsePreviewErr != nil {
		return 1
	}
	for _, a := range selected {
		// Binaries are status-only — sync/install never copy them into place.
		if a.IsBinary() {
			plans = append(plans, plan{Name: a.Name, DeployedPath: a.DeployedPath(opts.Home), WouldDo: "skip (binary)"})
			continue
		}
		s := deploy.Status(a, opts)
		would := "unchanged"
		switch s.State {
		case deploy.StateMissing:
			would = "install"
		case deploy.StateDrift:
			would = "update"
		case deploy.StateOK:
			if force && !a.AbsentOnly {
				would = "reinstall"
			}
		case deploy.StateSourceMissing:
			if a.Optional {
				would = "skip (no source)"
			} else {
				would = "ERROR: source not built"
			}
		case deploy.StateError:
			would = "ERROR: " + s.Error
		}
		plans = append(plans, plan{Name: a.Name, DeployedPath: s.DeployedPath, WouldDo: would})
	}
	for _, n := range pendingPulses {
		plans = append(plans, plan{
			Name:         pulseArtifactName + ":" + n,
			DeployedPath: n,
			WouldDo:      "merge pulse",
		})
	}
	if asJSON {
		return emitJSON(plans, stdout, stderr)
	}
	fmt.Fprintf(stdout, "[dry-run] %s would:\n", cmd)
	for _, p := range plans {
		fmt.Fprintf(stdout, "  %-18s %s -> %s\n", p.WouldDo, p.Name, p.DeployedPath)
	}
	fmt.Fprintf(stdout, "\n[dry-run] nothing written.\n")
	return 0
}

// pendingPulseNames reports the required pulses a real sync would merge.
//
// The registry is absent-only, so Status and dry-run both classify it OK on
// any host that has one. A preview that cannot see a pending migration is a
// preview that disagrees with the thing it previews, which is how a missed
// migration stays invisible to deployment audits.
func pendingPulseNames(selected []deploy.Artifact, opts deploy.Options, stderr io.Writer) ([]string, error) {
	a, ok := artifactNamed(selected, pulseArtifactName)
	if !ok {
		return nil, nil
	}
	hostPath, defaultsPath := pulsePaths(a, opts)
	required, err := requiredPulsesFor(selected, opts)
	if err != nil {
		fmt.Fprintf(stderr, "  ERROR     cannot resolve required pulses: %v\n", err)
		return nil, err
	}
	pending, err := deploy.PendingPulseMerges(hostPath, defaultsPath, required)
	if err != nil {
		fmt.Fprintf(stderr, "  ERROR     cannot compute pending pulse merges: %v\n", err)
		return nil, err
	}
	return pending, nil
}

func formatDeploy(cmd string, results []deploy.Result, w io.Writer) {
	var changed int
	for _, r := range results {
		if r.Action != deploy.ActionUnchanged && r.Action != deploy.ActionSkipped {
			changed++
		}
		fmt.Fprintf(w, "  %-10s %s -> %s\n", r.Action, r.Name, r.DeployedPath)
	}
	fmt.Fprintf(w, "\n%s complete: %d artifact(s), %d changed.\n", cmd, len(results), changed)
}

func emitJSON(v any, stdout, stderr io.Writer) int {
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(stderr, "error: encoding JSON: %v\n", err)
		return 1
	}
	return 0
}

func gitToplevel(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// runBuildInstall builds a Go binary from the repo and atomically installs it,
// gating on the built revision being origin/main (or an ancestor). This is the
// crash-safe, stale-proof replacement for the post-merge hook's `go install`.
func runBuildInstall(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("build-install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var pkg, target, sourceRef, repoRoot string
	fs.StringVar(&pkg, "pkg", "", "go package to build, relative to repo root (e.g. ./agm/cmd/agm)")
	fs.StringVar(&target, "target", "", "install path (default ~/go/bin/<pkg basename>)")
	fs.StringVar(&sourceRef, "source-ref", "origin/main", "ref the built binary must be, or be an ancestor of")
	fs.StringVar(&repoRoot, "repo-root", "", "repo root to build in (default: git toplevel of cwd)")
	if _, err := parseArgs(fs, args); err != nil {
		return 1
	}
	if pkg == "" {
		fmt.Fprintln(stderr, "build-install: --pkg is required")
		return 1
	}
	if repoRoot == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		root, err := gitToplevel(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "build-install: cannot resolve repo root: %v\n", err)
			return 1
		}
		repoRoot = strings.TrimSpace(root)
	}
	if target == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(stderr, "build-install: cannot resolve home: %v\n", err)
			return 1
		}
		target = filepath.Join(home, "go", "bin", filepath.Base(pkg))
	}

	r, err := deploy.AtomicInstall(pkg, target, sourceRef, deploy.Options{RepoRoot: repoRoot})
	if err != nil {
		fmt.Fprintf(stderr, "build-install FAILED: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "✓ installed %s -> %s (rev %s, gated against %s)\n", r.Name, r.Target, r.Revision, r.SourceRef)
	return 0
}

const usage = `dear-deploy — deploy dear-agent host artifacts (launchd plists, Claude Code hooks).

Usage:
  dear-deploy list                 list every deployable artifact
  dear-deploy status [name...]     show deployed state vs the manifest
  dear-deploy sync   [name...]     deploy artifacts that have drifted (idempotent)
  dear-deploy install [name...]    (re)install artifacts, even if unchanged
  dear-deploy build-install --pkg P   build a Go binary and atomically install it
  dear-deploy merge-pulses         add newly required absence-alarm pulses to the
                                   host config, keeping operator customization

Each write is atomic (stage -> verify -> activate); a failed deploy leaves the
previously-installed artifact untouched. There is no force/bypass flag.

build-install flags:
  --pkg PKG         go package to build, relative to repo root (e.g. ./agm/cmd/agm) [required]
  --target PATH     install path (default: ~/go/bin/<pkg basename>)
  --source-ref REF  the built binary must be REF or an ancestor of it (default: origin/main)
  --repo-root DIR   repo root to build in (default: git toplevel of cwd)

Common flags:
  --manifest FILE   manifest file (default: <repo-root>/deploy/manifest.yaml)
  --repo-root DIR   repo root for source paths (default: git toplevel of cwd)
  --home DIR        home dir for expanding ~ (default: $HOME)
  --json            structured JSON output
  --dry-run         (sync/install) show what would change without writing

Exit codes: 0 ok/clean; 2 (status) drift or required artifact missing; 1 error.
`

// runMergePulses adds newly required absence-alarm pulses to the host config.
//
// The pulse config is absent-only, so no other deploy path will touch a host
// that already has one. Without this step a pulse the recovery job registry
// depends on can live in the repository for months while the running alarm
// never emits it, and nothing reports the gap because every artifact is
// "deployed".
func runMergePulses(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("merge-pulses", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var c commonFlags
	c.register(fs)
	positional, err := parseArgs(fs, args)
	if err != nil {
		return 1
	}
	if len(positional) > 0 {
		// Silently ignoring names would let `merge-pulses some-other-artifact`
		// look like it did something targeted when it always acts on the pulse
		// registry alone.
		fmt.Fprintf(stderr, "dear-deploy: merge-pulses takes no artifact names (got %v)\n", positional)
		return 1
	}

	// Resolve through the manifest, so --manifest and --repo-root behave here
	// exactly as they do on the sync path instead of this command mutating
	// hard-coded locations the caller never selected.
	selected, opts, code := c.load([]string{pulseArtifactName}, stderr)
	if code != 0 {
		return code
	}
	a, ok := artifactNamed(selected, pulseArtifactName)
	if !ok {
		fmt.Fprintf(stderr, "dear-deploy: manifest has no %q artifact\n", pulseArtifactName)
		return 1
	}
	hostPath, defaultsPath := pulsePaths(a, opts)

	required, err := requiredPulsesFor(selected, opts)
	if err != nil {
		fmt.Fprintf(stderr, "dear-deploy: resolve required pulses: %v\n", err)
		return 1
	}
	added, err := deploy.MergeRequiredPulses(hostPath, defaultsPath, required)
	if err != nil {
		fmt.Fprintf(stderr, "dear-deploy: merge pulses: %v\n", err)
		return 1
	}

	if c.asJSON {
		// --json is advertised as common to every subcommand, so it must
		// produce a document here too rather than prose a decoder chokes on.
		return emitJSON(struct {
			Registry string   `json:"registry"`
			Added    []string `json:"added"`
		}{Registry: hostPath, Added: added}, stdout, stderr)
	}
	if len(added) == 0 {
		fmt.Fprintf(stdout, "absence-alarm pulses: already current (%s)\n", hostPath)
		return 0
	}
	fmt.Fprintf(stdout, "absence-alarm pulses: added %d to %s\n", len(added), hostPath)
	for _, n := range added {
		fmt.Fprintf(stdout, "  + %s\n", n)
	}
	fmt.Fprintln(stdout, "Restart absence-alarm for these to take effect.")
	return 0
}

// pulseArtifactName is the manifest entry whose deployed copy is the host's
// absence-alarm pulse registry.
const pulseArtifactName = "absence-alarm-pulses"

// artifactNamed returns the selected artifact with this name, if any.
func artifactNamed(selected []deploy.Artifact, name string) (deploy.Artifact, bool) {
	for _, a := range selected {
		if a.Name == name {
			return a, true
		}
	}
	return deploy.Artifact{}, false
}

// pulsePaths derives the host registry and repository defaults from the
// manifest entry rather than hard-coding them, so a --manifest override that
// relocates either one is honoured instead of silently mutating the standard
// host paths.
func pulsePaths(a deploy.Artifact, opts deploy.Options) (hostPath, defaultsPath string) {
	return a.DeployedPath(opts.Home), filepath.Join(opts.RepoRoot, a.Source)
}

// mergePulsesDuringDeploy folds newly required pulses into the host registry
// as part of a normal sync. It never fails the deploy: a host with a pulse
// registry this cannot parse keeps the registry it has, and says so.
func mergePulsesDuringDeploy(a deploy.Artifact, opts deploy.Options, asJSON bool, stdout, stderr io.Writer) error {
	hostPath, defaultsPath := pulsePaths(a, opts)
	//nolint:gosec // defaultsPath is derived from the manifest artifact's own
	// Source field joined to the operator-supplied repo root, which is exactly
	// the path every other artifact in this command is read from.
	if _, err := os.Stat(defaultsPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	required, err := requiredPulsesFor([]deploy.Artifact{a}, opts)
	if err != nil {
		return err
	}
	added, err := deploy.MergeRequiredPulses(hostPath, defaultsPath, required)
	if err != nil {
		return err
	}
	if len(added) == 0 {
		return nil
	}
	// emitJSON has already written the documented structured document to
	// stdout, so anything appended there would make it undecodable. In JSON
	// mode the human-readable notice goes to stderr instead.
	out := stdout
	if asJSON {
		out = stderr
	}
	for _, n := range added {
		fmt.Fprintf(out, "  MERGED    absence-alarm pulse %s\n", n)
	}
	fmt.Fprintln(out, "  Restart absence-alarm for the new pulses to take effect.")
	return nil
}

// jobsArtifactName is the manifest entry holding the recovery job registry.
const jobsArtifactName = "recovery-loop-jobs"

// requiredPulsesFor resolves the job registry through the manifest when it is
// available, so a --manifest override that relocates or customises the registry
// decides which pulses are required, rather than a hard-coded repository path.
func requiredPulsesFor(selected []deploy.Artifact, opts deploy.Options) (map[string]bool, error) {
	if a, ok := artifactNamed(selected, jobsArtifactName); ok {
		return deploy.RequiredPulseNamesFrom(filepath.Join(opts.RepoRoot, a.Source))
	}
	return deploy.RequiredPulseNames(opts.RepoRoot)
}
