package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/vbonnet/dear-agent/internal/deploy"
)

func statusResultIndex(results []deploy.StatusResult, name string) int {
	for i := range results {
		if results[i].Name == name {
			return i
		}
	}
	return -1
}

func planPulsePreview(
	a deploy.Artifact,
	plan deployPlan,
	pulseSelected bool,
	pending []string,
	ledgerChanged bool,
) deployPlan {
	if a.Name != pulseArtifactName || !pulseSelected {
		return plan
	}
	if plan.WouldDo == "install" {
		if len(pending) > 0 {
			plan.Detail = "seed pulse registry with default pulses: " + strings.Join(pending, ", ")
		}
		return plan
	}
	if len(pending) > 0 {
		plan.WouldDo = "merge pulses"
		plan.Detail = "pending required pulses: " + strings.Join(pending, ", ")
		return plan
	}
	if ledgerChanged {
		plan.WouldDo = "update pulse ledger"
		plan.Detail = "canonicalize current default-pulse offer state"
	}
	return plan
}

func emitMergePulsesOutcome(
	outcome deploy.PulseMergeResult,
	hostPath string,
	asJSON bool,
	stdout, stderr io.Writer,
) int {
	if asJSON {
		// --json is advertised as common to every subcommand, so it must
		// produce a document here too rather than prose a decoder chokes on.
		return emitJSON(struct {
			Registry      string   `json:"registry"`
			Added         []string `json:"added"`
			Created       bool     `json:"created"`
			LedgerChanged bool     `json:"ledger_changed"`
			Reconciled    bool     `json:"reconciled"`
		}{
			Registry:      hostPath,
			Added:         outcome.Added,
			Created:       outcome.Created,
			LedgerChanged: outcome.LedgerChanged,
			Reconciled:    outcome.Reconciled,
		}, stdout, stderr)
	}
	if outcome.Created {
		fmt.Fprintf(stdout, "absence-alarm pulses: seeded %d into %s\n", len(outcome.Added), hostPath)
		for _, name := range outcome.Added {
			fmt.Fprintf(stdout, "  + %s\n", name)
		}
		fmt.Fprintln(stdout, "Restart absence-alarm for these to take effect.")
		return 0
	}
	if len(outcome.Added) == 0 && !outcome.LedgerChanged && !outcome.Reconciled {
		fmt.Fprintf(stdout, "absence-alarm pulses: already current (%s)\n", hostPath)
		return 0
	}
	if len(outcome.Added) == 0 {
		fmt.Fprintf(stdout, "absence-alarm pulses: reconciled offer-ledger state (%s)\n", hostPath)
		return 0
	}
	fmt.Fprintf(stdout, "absence-alarm pulses: added %d to %s\n", len(outcome.Added), hostPath)
	for _, name := range outcome.Added {
		fmt.Fprintf(stdout, "  + %s\n", name)
	}
	fmt.Fprintln(stdout, "Restart absence-alarm for these to take effect.")
	return 0
}

// pulseArtifactPath returns the manifest-declared live path for aggregate
// status/dry-run diagnostics. Result names remain valid artifact selectors;
// nested pulse names belong in Detail, not in this path or Name.
func pulseArtifactPath(manifestArtifacts []deploy.Artifact, opts deploy.Options) string {
	if a, ok := artifactNamed(manifestArtifacts, pulseArtifactName); ok {
		return pulseHostPath(a, opts)
	}
	return ""
}

// pulseDiagnosticArtifact chooses a real manifest selector for dependency
// errors. A malformed manifest may declare recovery jobs without a pulse
// artifact; in that case the jobs artifact owns the invalid dependency and is
// the only selectable row on which the error can be reported.
func pulseDiagnosticArtifact(
	selected, manifestArtifacts []deploy.Artifact,
	opts deploy.Options,
) (string, string) {
	if a, ok := artifactNamed(manifestArtifacts, pulseArtifactName); ok {
		return a.Name, a.DeployedPath(opts.Home)
	}
	if a, ok := artifactNamed(selected, jobsArtifactName); ok {
		return a.Name, a.DeployedPath(opts.Home)
	}
	return pulseArtifactName, ""
}

// deployPulseDuringDeploy publishes the pulse artifact before any recovery-job
// registry. The manifest's absent-only bit chooses the publication semantics:
// operator-owned registries merge under the pulse lock, while ordinary managed
// files receive generic exact-source deployment after domain validation.
func deployPulseDuringDeploy(
	a deploy.Artifact,
	selected []deploy.Artifact,
	manifestArtifacts []deploy.Artifact,
	opts deploy.Options,
	asJSON bool,
	stdout io.Writer,
) (deploy.Result, []byte, error) {
	if !a.AbsentOnly {
		return deployNormalPulseDuringDeploy(a, selected, manifestArtifacts, opts)
	}

	hostPath := pulseHostPath(a, opts)
	outcome, err := mergePulsesDuringDeploy(a, selected, manifestArtifacts, opts, asJSON, stdout)
	if err != nil {
		return deploy.Result{}, nil, fmt.Errorf("pulse merge: %w", err)
	}
	// #nosec G703 -- hostPath is the manifest-selected deployment target;
	// reading it back is required to report the lock-owned merge result.
	live, err := os.ReadFile(hostPath)
	if err != nil {
		return deploy.Result{}, nil, fmt.Errorf("read merged pulse result: %w", err)
	}
	action := deploy.ActionUnchanged
	switch {
	case outcome.Created:
		action = deploy.ActionInstalled
	case len(outcome.Added) > 0 || outcome.LedgerChanged || outcome.Reconciled:
		action = deploy.ActionUpdated
	}
	detail := ""
	if len(outcome.Added) == 0 && (outcome.LedgerChanged || outcome.Reconciled) {
		detail = "pulse ledger state reconciled"
	}
	sum := sha256.Sum256(live)
	return deploy.Result{
		Name:         a.Name,
		DeployedPath: hostPath,
		Action:       action,
		SHA256:       fmt.Sprintf("%x", sum),
		Detail:       detail,
	}, nil, nil
}

func deployNormalPulseDuringDeploy(
	a deploy.Artifact,
	selected []deploy.Artifact,
	manifestArtifacts []deploy.Artifact,
	opts deploy.Options,
) (deploy.Result, []byte, error) {
	required, renderedJobs, err := requiredPulsesForNormalPulse(
		selected, manifestArtifacts, opts,
	)
	if err != nil {
		return deploy.Result{}, nil, fmt.Errorf("resolve required pulses: %w", err)
	}
	result, err := deploy.DeployValidated(a, opts, func(rendered []byte) error {
		if err := deploy.ValidateRequiredPulseConfigRendered(rendered, required); err != nil {
			return fmt.Errorf("validate pulse config: %w", err)
		}
		return nil
	})
	if err != nil {
		return deploy.Result{}, nil, err
	}
	if renderedJobs != nil && result.Action == deploy.ActionSkipped {
		return deploy.Result{}, nil, fmt.Errorf(
			"pulse source is unavailable; refusing to publish %s",
			jobsArtifactName,
		)
	}
	return result, renderedJobs, nil
}

func validateNormalPulseArtifact(
	a deploy.Artifact,
	selected []deploy.Artifact,
	manifestArtifacts []deploy.Artifact,
	opts deploy.Options,
) error {
	required, _, err := requiredPulsesForNormalPulse(selected, manifestArtifacts, opts)
	if err != nil {
		return fmt.Errorf("resolve required pulses: %w", err)
	}
	rendered, err := a.Render(opts.RepoRoot, opts.Home)
	if err != nil {
		return fmt.Errorf("render pulse config: %w", err)
	}
	if err := deploy.ValidateRequiredPulseConfigRendered(rendered, required); err != nil {
		return fmt.Errorf("validate pulse config: %w", err)
	}
	return nil
}

func validateNormalPulseArtifactCurrent(
	a deploy.Artifact,
	selected []deploy.Artifact,
	manifestArtifacts []deploy.Artifact,
	opts deploy.Options,
) error {
	if err := validateNormalPulseArtifact(a, selected, manifestArtifacts, opts); err != nil {
		return err
	}
	status := deploy.Status(a, opts)
	if status.State == deploy.StateOK {
		return nil
	}
	detail := string(status.State)
	if status.Error != "" {
		detail += ": " + status.Error
	}
	return fmt.Errorf(
		"%s is %s; sync %s before publishing %s",
		a.Name,
		detail,
		a.Name,
		jobsArtifactName,
	)
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

// requiredPulsesForNormalPulse returns the requirements a normal pulse
// replacement must preserve. When both registries are selected normal files,
// pulses publish first, so the prospective registry must cover both the live
// jobs that remain authoritative until their write succeeds and the rendered
// jobs snapshot this command will publish afterward. The returned snapshot is
// deployed verbatim after pulse publication to keep validation and activation
// on the same render.
func requiredPulsesForNormalPulse(
	selected, manifestArtifacts []deploy.Artifact,
	opts deploy.Options,
) (map[string]bool, []byte, error) {
	pulse, pulseSelected := artifactNamed(selected, pulseArtifactName)
	jobs, jobsSelected := artifactNamed(selected, jobsArtifactName)
	if !pulseSelected || pulse.AbsentOnly || !jobsSelected || jobs.AbsentOnly {
		required, err := requiredPulsesFor(selected, manifestArtifacts, opts)
		return required, nil, err
	}

	rendered, err := jobs.Render(opts.RepoRoot, opts.Home)
	if err != nil {
		return nil, nil, fmt.Errorf("render recovery job registry: %w", err)
	}
	required, err := deploy.RequiredPulseNamesRendered(rendered)
	if err != nil {
		return nil, nil, err
	}

	live, exists, err := readLiveRecoveryJobRegistry(jobs, opts)
	if err != nil {
		return nil, nil, err
	}
	if !exists {
		return required, rendered, nil
	}
	liveRequired, err := deploy.RequiredPulseNamesRendered(live)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve pulses from live recovery job registry: %w", err)
	}
	for name := range liveRequired {
		required[name] = true
	}
	return required, rendered, nil
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
		live, exists, err := readLiveRecoveryJobRegistry(a, opts)
		if err != nil {
			return nil, err
		}
		if exists {
			return live, nil
		}
	}

	rendered, err := a.Render(opts.RepoRoot, opts.Home)
	if err != nil {
		return nil, fmt.Errorf("render recovery job registry: %w", err)
	}
	return rendered, nil
}

func readLiveRecoveryJobRegistry(a deploy.Artifact, opts deploy.Options) ([]byte, bool, error) {
	livePath := a.DeployedPath(opts.Home)
	// #nosec G703 -- livePath is the manifest-selected deployed artifact;
	// observing that exact runtime input is the purpose of this boundary.
	_, err := os.Lstat(livePath)
	switch {
	case err == nil:
		// #nosec G703 -- see the manifest-selected path justification above.
		live, readErr := os.ReadFile(livePath)
		if readErr != nil {
			return nil, false, fmt.Errorf("read live recovery job registry %s: %w", livePath, readErr)
		}
		return live, true, nil
	case os.IsNotExist(err):
		return nil, false, nil
	default:
		return nil, false, fmt.Errorf("inspect live recovery job registry %s: %w", livePath, err)
	}
}
