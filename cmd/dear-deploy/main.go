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
// Run `dear-deploy --help` for the subcommand and flag inventory. It is not
// repeated here: this comment and cmd/dear-deploy/README.md had both drifted to
// a four-command list while the binary had five, and a catalog that disagrees
// with the binary is worse than no catalog.
//
// With no names, status/sync/install operate on the whole manifest.
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
func (c *commonFlags) load(names []string, stderr io.Writer) ([]deploy.Artifact, []deploy.Artifact, deploy.Options, int) {
	root := c.repoRoot
	if root == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		detected, err := gitToplevel(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "error: cannot detect repo root: %v\n", err)
			fmt.Fprintf(stderr, "hint: run inside the dear-agent checkout or pass --repo-root <dir>\n")
			return nil, nil, deploy.Options{}, 1
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
		return nil, nil, deploy.Options{}, 1
	}
	m, err := deploy.ParseManifest(data)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return nil, nil, deploy.Options{}, 1
	}
	selected, err := m.Select(names)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return nil, nil, deploy.Options{}, 1
	}

	// Resolve home eagerly so displayed deployed paths are concrete (~ expanded)
	// even in list/status, not just inside Deploy. An empty --home falls back to
	// $HOME exactly as deploy.Options would have.
	home := c.home
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(stderr, "error: cannot resolve home dir: %v\n", err)
			return nil, nil, deploy.Options{}, 1
		}
		home = h
	}
	return selected, m.Artifacts, deploy.Options{RepoRoot: root, Home: home}, 0
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
	selected, _, opts, code := c.load(names, stderr)
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
	selected, manifestArtifacts, opts, code := c.load(names, stderr)
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
	pendingPulses, pulseMergeSelected, ledgerChanged, pulseErr := pendingPulseNames(
		selected, manifestArtifacts, opts, stderr,
	)
	pulseIndex := statusResultIndex(results, pulseArtifactName)
	if len(pendingPulses) > 0 || pulseMergeSelected && ledgerChanged {
		detail := "pending pulse-ledger update"
		if len(pendingPulses) > 0 {
			detail = "pending required pulses: " + strings.Join(pendingPulses, ", ")
		}
		if pulseIndex >= 0 {
			// Absent-only status is normally OK once the operator registry exists.
			// Pending required pulses are real artifact drift, but the artifact's
			// selector must remain its manifest name rather than a synthetic child.
			if results[pulseIndex].State == deploy.StateOK {
				results[pulseIndex].State = deploy.StateDrift
			}
			results[pulseIndex].Detail = detail
			results[pulseIndex].Remediation = "dear-deploy sync " + pulseArtifactName
		} else {
			results = append(results, deploy.StatusResult{
				Name:         pulseArtifactName,
				State:        deploy.StateDrift,
				DeployedPath: pulseArtifactPath(manifestArtifacts, opts),
				Remediation:  "dear-deploy sync " + pulseArtifactName,
				Detail:       detail,
			})
		}
	}
	if pulseErr != nil {
		// "Could not check" is not "clean". A status that exits 0 because it
		// failed to read the registry tells an audit the host is fine when
		// nobody looked.
		errorName, errorPath := pulseDiagnosticArtifact(selected, manifestArtifacts, opts)
		errorIndex := statusResultIndex(results, errorName)
		if errorIndex >= 0 {
			results[errorIndex].State = deploy.StateError
			results[errorIndex].Error = pulseErr.Error()
			results[errorIndex].Detail = "required-pulse evaluation failed"
		} else {
			results = append(results, deploy.StatusResult{
				Name:         errorName,
				State:        deploy.StateError,
				DeployedPath: errorPath,
				Error:        pulseErr.Error(),
				Detail:       "required-pulse evaluation failed",
			})
		}
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
	case errs:
		return 1
	case drift:
		return 2
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
			} else if r.Remediation != "" {
				fmt.Fprintf(w, "            deployed: %s\n", r.DeployedPath)
				fmt.Fprintf(w, "            fix: %s\n", r.Remediation)
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
		if r.Detail != "" && (r.Kind != deploy.KindBinary || r.State != deploy.StateDrift) {
			fmt.Fprintf(w, "            detail: %s\n", r.Detail)
		}
	}
	fmt.Fprintf(w, "\n%d artifact(s): %d ok, %d drift, %d missing, %d no-source, %d skipped, %d error\n",
		len(results), ok, drift, missing, srcMissing, skipped, errs)
	if drift+missing+srcMissing == 0 && errs == 0 {
		fmt.Fprintf(w, "In sync: every deployed artifact matches the manifest.\n")
	} else if drift+missing+srcMissing > 0 {
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
	selected, manifestArtifacts, opts, code := c.load(names, stderr)
	if code != 0 {
		return code
	}
	opts.Force = cmd == "install"

	if dryRun {
		return dryRunDeploy(cmd, selected, manifestArtifacts, opts, c.asJSON, stdout, stderr)
	}

	results := make([]deploy.Result, 0, len(selected))
	var failures []string

	// Publish the pulse registry before recovery jobs regardless of manifest
	// ordering. An absent-only registry uses the locked additive protocol; a
	// normal artifact uses validated exact-source generic deployment. Treating
	// the reserved name as inherently absent-only would silently ignore a
	// custom manifest's ownership contract.
	var pulseResult *deploy.Result
	var pulseDependencyErr error
	pulseHandled := false
	if a, ok := artifactNamed(selected, pulseArtifactName); ok {
		pulseHandled = true
		result, err := deployPulseDuringDeploy(a, selected, manifestArtifacts, opts, c.asJSON, stdout)
		if err != nil {
			fmt.Fprintf(stderr, "  FAILED    %s — %v\n", a.Name, err)
			failures = append(failures, a.Name)
			pulseDependencyErr = err
		} else {
			pulseResult = &result
		}
	} else if _, ok := artifactNamed(selected, jobsArtifactName); ok {
		preview, _, _, err := pendingPulseNames(selected, manifestArtifacts, opts, stderr)
		switch {
		case err != nil:
			pulseDependencyErr = err
		case len(preview) > 0:
			pulseDependencyErr = fmt.Errorf(
				"required pulse state is not current (%s); sync %s first",
				strings.Join(preview, ", "),
				pulseArtifactName,
			)
		}
	}

	for _, a := range selected {
		if a.Name == pulseArtifactName && pulseHandled {
			// The prepass already used the ownership mode declared by this
			// artifact and completed before any dependent recovery jobs.
			if pulseResult != nil {
				results = append(results, *pulseResult)
			}
			continue
		}
		if a.Name == jobsArtifactName && pulseDependencyErr != nil {
			fmt.Fprintf(stderr, "  FAILED    %s pulse dependency — %v\n", a.Name, pulseDependencyErr)
			failures = append(failures, a.Name+" (pulse dependency)")
			continue
		}
		r, err := deploy.Deploy(a, opts)
		if err != nil {
			// One bad artifact is reported but does not abort independent work.
			// Activation is an atomic replace: an error leaves either the prior
			// bytes or the complete intended bytes, never a partial artifact. A
			// post-rename durability error is retried before "unchanged" can be
			// reported on the next sync.
			fmt.Fprintf(stderr, "  FAILED    %s — %v\n", a.Name, err)
			failures = append(failures, a.Name)
			continue
		}
		results = append(results, r)
	}

	if c.asJSON {
		// sync/install --json predates pulse migration and its top-level array is
		// consumed by automation. The synthetic pulse Result above carries the
		// compatible installed/updated/unchanged receipt without versioning the
		// output contract underneath existing callers.
		if rc := emitJSON(results, stdout, stderr); rc != 0 {
			return rc
		}
	} else {
		formatDeploy(cmd, results, stdout)
	}

	if len(failures) > 0 {
		fmt.Fprintf(stderr, "\n%s: %d artifact(s) failed: %s\n", cmd, len(failures), strings.Join(failures, ", "))
		return 1
	}
	return 0
}

type deployPlan struct {
	Name         string `json:"name"`
	DeployedPath string `json:"deployed"`
	WouldDo      string `json:"would_do"`
	Detail       string `json:"detail,omitempty"`
}

func dryRunDeploy(
	cmd string,
	selected, manifestArtifacts []deploy.Artifact,
	opts deploy.Options,
	asJSON bool,
	stdout, stderr io.Writer,
) int {
	force := cmd == "install"
	plans := make([]deployPlan, 0, len(selected))
	// An absent-only registry always reports "unchanged", so without this a
	// preview would show nothing while a real sync merged required pulses.
	pendingPulses, pulseMergeSelected, ledgerChanged, pulsePreviewErr := pendingPulseNames(
		selected, manifestArtifacts, opts, stderr,
	)
	if pulsePreviewErr != nil {
		return 1
	}
	jobsBlocked := !pulseMergeSelected && len(pendingPulses) > 0
	hasErrors := false
	for _, a := range selected {
		p, artifactErr := planArtifact(a, opts, force, jobsBlocked)
		p = planPulsePreview(a, p, pulseMergeSelected, pendingPulses, ledgerChanged)
		plans = append(plans, p)
		hasErrors = hasErrors || artifactErr
	}
	if !pulseMergeSelected && len(pendingPulses) > 0 {
		hasErrors = true
		plans = append(plans, deployPlan{
			Name:         pulseArtifactName,
			DeployedPath: pulseArtifactPath(manifestArtifacts, opts),
			WouldDo:      "ERROR: sync " + pulseArtifactName + " first",
			Detail:       "pending required pulses: " + strings.Join(pendingPulses, ", "),
		})
	}
	if asJSON {
		if rc := emitJSON(plans, stdout, stderr); rc != 0 {
			return rc
		}
	} else {
		fmt.Fprintf(stdout, "[dry-run] %s would:\n", cmd)
		for _, p := range plans {
			fmt.Fprintf(stdout, "  %-18s %s -> %s\n", p.WouldDo, p.Name, p.DeployedPath)
			if p.Detail != "" {
				fmt.Fprintf(stdout, "                     detail: %s\n", p.Detail)
			}
		}
		fmt.Fprintf(stdout, "\n[dry-run] nothing written.\n")
	}
	if hasErrors {
		return 1
	}
	return 0
}

func planArtifact(a deploy.Artifact, opts deploy.Options, force, jobsBlocked bool) (deployPlan, bool) {
	// Binaries are status-only — sync/install never copy them into place.
	if a.IsBinary() {
		return deployPlan{Name: a.Name, DeployedPath: a.DeployedPath(opts.Home), WouldDo: "skip (binary)"}, false
	}
	s := deploy.Status(a, opts)
	would := "unchanged"
	hasError := false
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
			hasError = true
		}
	case deploy.StateError:
		would = "ERROR: " + s.Error
		hasError = true
	}
	if a.Name == jobsArtifactName && jobsBlocked {
		would = "ERROR: required pulse state is not current"
		hasError = true
	}
	return deployPlan{Name: a.Name, DeployedPath: s.DeployedPath, WouldDo: would}, hasError
}

// pendingPulseNames reports the required pulses a real sync would merge and
// also checks that a jobs-only selection is safe to publish against the live
// registry. The first bool reports whether this command selected an absent-
// only pulse artifact and can therefore perform the pending merge itself. The
// second reports a ledger-only adoption that is actionable only for that pulse
// selection. A normal pulse artifact is validated as an exact replacement and
// emits no merge plan.
//
// The registry is absent-only, so Status and dry-run both classify it OK on
// any host that has one. A preview that cannot see a pending migration is a
// preview that disagrees with the thing it previews, which is how a missed
// migration stays invisible to deployment audits.
func pendingPulseNames(
	selected, manifestArtifacts []deploy.Artifact,
	opts deploy.Options,
	stderr io.Writer,
) ([]string, bool, bool, error) {
	a, pulseSelected := artifactNamed(selected, pulseArtifactName)
	if pulseSelected && !a.AbsentOnly {
		if err := validateNormalPulseArtifact(a, selected, manifestArtifacts, opts); err != nil {
			fmt.Fprintf(stderr, "  ERROR     cannot validate pulse artifact: %v\n", err)
			return nil, false, false, err
		}
		return nil, false, false, nil
	}
	if !pulseSelected {
		if _, jobsSelected := artifactNamed(selected, jobsArtifactName); !jobsSelected {
			return nil, false, false, nil
		}
		var ok bool
		a, ok = artifactNamed(manifestArtifacts, pulseArtifactName)
		if !ok {
			err := fmt.Errorf("manifest declares %s without %s", jobsArtifactName, pulseArtifactName)
			fmt.Fprintf(stderr, "  ERROR     cannot resolve required pulses: %v\n", err)
			return nil, false, false, err
		}
		if !a.AbsentOnly {
			if err := validateNormalPulseArtifactCurrent(a, selected, manifestArtifacts, opts); err != nil {
				fmt.Fprintf(stderr, "  ERROR     cannot publish %s: %v\n", jobsArtifactName, err)
				return nil, false, false, err
			}
			return nil, false, false, nil
		}
	}
	hostPath := pulseHostPath(a, opts)
	required, err := requiredPulsesFor(selected, manifestArtifacts, opts)
	if err != nil {
		fmt.Fprintf(stderr, "  ERROR     cannot resolve required pulses: %v\n", err)
		return nil, pulseSelected, false, err
	}
	rendered, err := a.Render(opts.RepoRoot, opts.Home)
	if err != nil {
		fmt.Fprintf(stderr, "  ERROR     cannot render pulse defaults: %v\n", err)
		return nil, pulseSelected, false, err
	}
	preview, err := deploy.PendingPulseMergePreviewRendered(hostPath, rendered, required)
	if err != nil {
		fmt.Fprintf(stderr, "  ERROR     cannot compute pending pulse merges: %v\n", err)
		return nil, pulseSelected, false, err
	}
	return preview.Added, pulseSelected, preview.LedgerChanged, nil
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

Each file write is staged and verified before atomic activation. Failures before
activation leave the prior artifact untouched; a pulse-ledger failure after
registry activation leaves a pending transaction for the next sync to reconcile.
There is no force/bypass flag.

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
	selected, manifestArtifacts, opts, code := c.load([]string{pulseArtifactName}, stderr)
	if code != 0 {
		return code
	}
	a, ok := artifactNamed(selected, pulseArtifactName)
	if !ok {
		fmt.Fprintf(stderr, "dear-deploy: manifest has no %q artifact\n", pulseArtifactName)
		return 1
	}
	if !a.AbsentOnly {
		fmt.Fprintf(stderr, "dear-deploy: %s is not absent-only; use sync or install for exact-source deployment\n", pulseArtifactName)
		return 1
	}
	hostPath := pulseHostPath(a, opts)

	required, err := requiredPulsesFor(selected, manifestArtifacts, opts)
	if err != nil {
		fmt.Fprintf(stderr, "dear-deploy: resolve required pulses: %v\n", err)
		return 1
	}
	rendered, err := a.Render(opts.RepoRoot, opts.Home)
	if err != nil {
		fmt.Fprintf(stderr, "dear-deploy: render pulse defaults: %v\n", err)
		return 1
	}
	mode, err := a.FileMode()
	if err != nil {
		fmt.Fprintf(stderr, "dear-deploy: resolve pulse registry mode: %v\n", err)
		return 1
	}
	outcome, err := deploy.MergeRequiredPulsesRenderedResult(hostPath, rendered, mode, required)
	if err != nil {
		fmt.Fprintf(stderr, "dear-deploy: merge pulses: %v\n", err)
		return 1
	}
	return emitMergePulsesOutcome(outcome, hostPath, c.asJSON, stdout, stderr)
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

// pulseHostPath derives the live registry from the selected manifest artifact
// rather than hard-coding the standard host location.
func pulseHostPath(a deploy.Artifact, opts deploy.Options) string {
	return a.DeployedPath(opts.Home)
}

// mergePulsesDuringDeploy folds newly required pulses into the host registry
// as part of a normal sync. A host with a registry this cannot safely update
// keeps the registry it has and makes the deploy fail loud.
func mergePulsesDuringDeploy(
	a deploy.Artifact,
	selected []deploy.Artifact,
	manifestArtifacts []deploy.Artifact,
	opts deploy.Options,
	asJSON bool,
	stdout io.Writer,
) (deploy.PulseMergeResult, error) {
	hostPath := pulseHostPath(a, opts)
	required, err := requiredPulsesFor(selected, manifestArtifacts, opts)
	if err != nil {
		return deploy.PulseMergeResult{}, err
	}
	rendered, err := a.Render(opts.RepoRoot, opts.Home)
	if err != nil {
		return deploy.PulseMergeResult{}, fmt.Errorf("render pulse defaults: %w", err)
	}
	mode, err := a.FileMode()
	if err != nil {
		return deploy.PulseMergeResult{}, fmt.Errorf("resolve pulse registry mode: %w", err)
	}
	outcome, err := deploy.MergeRequiredPulsesRenderedResult(hostPath, rendered, mode, required)
	if err != nil {
		return deploy.PulseMergeResult{}, err
	}
	if len(outcome.Added) == 0 {
		if (outcome.LedgerChanged || outcome.Reconciled) && !asJSON {
			fmt.Fprintln(stdout, "  RECONCILED absence-alarm pulse offer ledger")
		}
		return outcome, nil
	}
	// In JSON mode the caller folds these names into the document it emits, so
	// nothing is printed here: emitJSON has already written to stdout and
	// appending text would make it undecodable.
	if asJSON {
		return outcome, nil
	}
	for _, n := range outcome.Added {
		fmt.Fprintf(stdout, "  MERGED    absence-alarm pulse %s\n", n)
	}
	fmt.Fprintln(stdout, "  Restart absence-alarm for the new pulses to take effect.")
	return outcome, nil
}

// jobsArtifactName is the manifest entry holding the recovery job registry.
const jobsArtifactName = "recovery-loop-jobs"

// requiredPulsesFor resolves the job registry through the manifest and command
// selection. A selected normal job registry is about to be replaced from
// source; an unselected or absent-only registry remains live and authoritative.
// This also keeps --manifest relocation authoritative instead of consulting a
// hard-coded repository path.
func requiredPulsesFor(
	selected, manifestArtifacts []deploy.Artifact,
	opts deploy.Options,
) (map[string]bool, error) {
	if a, ok := artifactNamed(manifestArtifacts, jobsArtifactName); ok {
		_, jobsSelected := artifactNamed(selected, jobsArtifactName)
		registry, err := requiredPulseJobRegistry(a, jobsSelected, opts)
		if err != nil {
			return nil, err
		}
		return deploy.RequiredPulseNamesRendered(registry)
	}
	return deploy.RequiredPulseNames(opts.RepoRoot)
}

// requiredPulseJobRegistry returns the bytes the recovery-loop runtime will be
// authoritative over after this command. An unselected live registry remains
// deployed, while a selected normal registry will be replaced from source.
// Absent-only registries remain operator-owned even when selected. A missing
// live registry falls back to the rendered source that would seed it; other
// observation failures are not absence.
func requiredPulseJobRegistry(a deploy.Artifact, jobsSelected bool, opts deploy.Options) ([]byte, error) {
	// An unselected registry remains the runtime authority after this command,
	// regardless of whether it is normally source-owned. Absent-only registries
	// are always operator-owned and likewise remain live even when selected.
	if a.AbsentOnly || !jobsSelected {
		livePath := a.DeployedPath(opts.Home)
		// #nosec G703 -- livePath is the manifest-selected deployed artifact;
		// observing that exact runtime input is the purpose of this boundary.
		_, err := os.Lstat(livePath)
		switch {
		case err == nil:
			// #nosec G703 -- see the manifest-selected path justification above.
			live, readErr := os.ReadFile(livePath)
			if readErr != nil {
				return nil, fmt.Errorf("read live recovery job registry %s: %w", livePath, readErr)
			}
			return live, nil
		case !os.IsNotExist(err):
			return nil, fmt.Errorf("inspect live recovery job registry %s: %w", livePath, err)
		}
	}

	rendered, err := a.Render(opts.RepoRoot, opts.Home)
	if err != nil {
		return nil, fmt.Errorf("render recovery job registry: %w", err)
	}
	return rendered, nil
}
