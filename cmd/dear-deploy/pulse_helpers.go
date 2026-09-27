package main

import (
	"crypto/sha256"
	"errors"
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
	normalPulseSource []byte,
) (deploy.Result, []byte, bool, error) {
	if !a.AbsentOnly {
		return deployNormalPulseDuringDeploy(a, selected, manifestArtifacts, opts, normalPulseSource)
	}

	hostPath := pulseHostPath(a, opts)
	outcome, renderedJobs, jobsSourceMissing, err := mergePulsesDuringDeploy(
		a, selected, manifestArtifacts, opts, asJSON, stdout,
	)
	if err != nil {
		return deploy.Result{}, nil, false, fmt.Errorf("pulse merge: %w", err)
	}
	// #nosec G703 -- hostPath is the manifest-selected deployment target;
	// reading it back is required to report the lock-owned merge result.
	live, err := os.ReadFile(hostPath)
	if err != nil {
		return deploy.Result{}, nil, false, fmt.Errorf("read merged pulse result: %w", err)
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
	}, renderedJobs, jobsSourceMissing, nil
}

func deployNormalPulseDuringDeploy(
	a deploy.Artifact,
	selected []deploy.Artifact,
	manifestArtifacts []deploy.Artifact,
	opts deploy.Options,
	renderedPulse []byte,
) (deploy.Result, []byte, bool, error) {
	required, renderedJobs, jobsSourceMissing, err := requiredPulsesForNormalPulse(
		selected, manifestArtifacts, opts,
	)
	if err != nil {
		return deploy.Result{}, nil, false, fmt.Errorf("resolve required pulses: %w", err)
	}
	if err := deploy.ValidateRequiredPulseConfigRendered(renderedPulse, required); err != nil {
		return deploy.Result{}, nil, false, fmt.Errorf("validate pulse config: %w", err)
	}
	result, err := deploy.DeployRendered(a, opts, renderedPulse)
	if err != nil {
		return deploy.Result{}, nil, false, err
	}
	return result, renderedJobs, jobsSourceMissing, nil
}

func validateNormalPulseArtifact(
	a deploy.Artifact,
	selected []deploy.Artifact,
	manifestArtifacts []deploy.Artifact,
	opts deploy.Options,
) (bool, []byte, bool, error) {
	required, renderedJobs, jobsSourceMissing, err := requiredPulsesForNormalPulse(
		selected, manifestArtifacts, opts,
	)
	if err != nil {
		return false, nil, false, fmt.Errorf("resolve required pulses: %w", err)
	}
	rendered, err := a.Render(opts.RepoRoot, opts.Home)
	if err != nil {
		if a.Optional && errors.Is(err, os.ErrNotExist) {
			return true, renderedJobs, jobsSourceMissing, nil
		}
		return false, nil, false, fmt.Errorf("render pulse config: %w", err)
	}
	if err := deploy.ValidateRequiredPulseConfigRendered(rendered, required); err != nil {
		return false, nil, false, fmt.Errorf("validate pulse config: %w", err)
	}
	return false, renderedJobs, jobsSourceMissing, nil
}

func validateNormalPulseArtifactCurrent(
	a deploy.Artifact,
	selected []deploy.Artifact,
	manifestArtifacts []deploy.Artifact,
	opts deploy.Options,
) ([]byte, bool, error) {
	required, renderedJobs, jobsSourceMissing, err := requiredPulsesFor(
		selected, manifestArtifacts, opts,
	)
	if err != nil {
		return nil, false, fmt.Errorf("resolve required pulses: %w", err)
	}
	if jobsSourceMissing {
		return nil, true, nil
	}
	status := deploy.Status(a, opts)
	if status.State != deploy.StateOK {
		detail := string(status.State)
		if status.Error != "" {
			detail += ": " + status.Error
		}
		return nil, false, fmt.Errorf(
			"%s is %s; sync %s before publishing %s",
			a.Name,
			detail,
			a.Name,
			jobsArtifactName,
		)
	}

	// Status preserves the generic source/live and create-directory contract,
	// but it renders the source internally. Authorize this jobs publication
	// from one exact live pulse snapshot read after that check: validating a
	// separate source render would permit validate-A/approve-B source swaps.
	// The caller holds the normal pulse publication lock through jobs activation.
	// #nosec G703 -- status.DeployedPath is the manifest-selected live registry.
	live, err := os.ReadFile(status.DeployedPath)
	if err != nil {
		return nil, false, fmt.Errorf("read live pulse config %s: %w", status.DeployedPath, err)
	}
	if err := deploy.ValidateRequiredPulseConfigRendered(live, required); err != nil {
		return nil, false, fmt.Errorf("validate live pulse config: %w", err)
	}
	return renderedJobs, false, nil
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

// normalPulseMutationArtifact returns the manifest-declared normal pulse
// registry whose host lock must cover this mutation. A targeted pulse write, a
// targeted jobs write, and a paired write can each invalidate an observation
// made by another concurrent deploy, so all three selections participate.
// Absent-only pulse publication is additive and already locks inside its merge.
func normalPulseMutationArtifact(
	selected, manifestArtifacts []deploy.Artifact,
) (deploy.Artifact, bool) {
	pulse, ok := artifactNamed(manifestArtifacts, pulseArtifactName)
	if !ok || pulse.AbsentOnly {
		return deploy.Artifact{}, false
	}
	if _, ok := artifactNamed(selected, pulseArtifactName); ok {
		return pulse, true
	}
	if _, ok := artifactNamed(selected, jobsArtifactName); ok {
		return pulse, true
	}
	return deploy.Artifact{}, false
}

type preparedNormalPulseSource struct {
	Artifact      deploy.Artifact
	Rendered      []byte
	SourceMissing bool
}

// prepareNormalPulseSource pins the selected normal pulse source before the
// publication decision. A present snapshot is later validated and deployed
// verbatim under the host lock. An optional missing snapshot can remain a true
// no-op without acquiring or creating that lock, even if the source changes
// after this observation.
func prepareNormalPulseSource(
	selected []deploy.Artifact,
	opts deploy.Options,
) (preparedNormalPulseSource, bool, error) {
	pulse, pulseSelected := artifactNamed(selected, pulseArtifactName)
	if !pulseSelected || pulse.AbsentOnly {
		return preparedNormalPulseSource{}, false, nil
	}
	rendered, err := pulse.Render(opts.RepoRoot, opts.Home)
	if err == nil {
		return preparedNormalPulseSource{Artifact: pulse, Rendered: rendered}, true, nil
	}
	if pulse.Optional && errors.Is(err, os.ErrNotExist) {
		return preparedNormalPulseSource{Artifact: pulse, SourceMissing: true}, true, nil
	}
	return preparedNormalPulseSource{}, false, fmt.Errorf("render pulse config: %w", err)
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
) (deploy.PulseMergeResult, []byte, bool, error) {
	hostPath := pulseHostPath(a, opts)
	required, renderedJobs, jobsSourceMissing, err := requiredPulsesFor(
		selected, manifestArtifacts, opts,
	)
	if err != nil {
		return deploy.PulseMergeResult{}, nil, false, err
	}
	rendered, err := a.Render(opts.RepoRoot, opts.Home)
	if err != nil {
		return deploy.PulseMergeResult{}, nil, false, fmt.Errorf("render pulse defaults: %w", err)
	}
	mode, err := a.FileMode()
	if err != nil {
		return deploy.PulseMergeResult{}, nil, false, fmt.Errorf("resolve pulse registry mode: %w", err)
	}
	outcome, err := deploy.MergeRequiredPulsesRenderedResult(hostPath, rendered, mode, required)
	if err != nil {
		return deploy.PulseMergeResult{}, nil, false, err
	}
	if len(outcome.Added) == 0 {
		if (outcome.LedgerChanged || outcome.Reconciled) && !asJSON {
			fmt.Fprintln(stdout, "  RECONCILED absence-alarm pulse offer ledger")
		}
		return outcome, renderedJobs, jobsSourceMissing, nil
	}
	// In JSON mode the caller folds these names into the document it emits, so
	// nothing is printed here: emitJSON has already written to stdout and
	// appending text would make it undecodable.
	if asJSON {
		return outcome, renderedJobs, jobsSourceMissing, nil
	}
	for _, n := range outcome.Added {
		fmt.Fprintf(stdout, "  MERGED    absence-alarm pulse %s\n", n)
	}
	fmt.Fprintln(stdout, "  Restart absence-alarm for the new pulses to take effect.")
	return outcome, renderedJobs, jobsSourceMissing, nil
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
) (map[string]bool, []byte, bool, error) {
	if a, ok := artifactNamed(manifestArtifacts, jobsArtifactName); ok {
		_, jobsSelected := artifactNamed(selected, jobsArtifactName)
		resolved, err := requiredPulseJobRegistry(a, jobsSelected, opts)
		if err != nil {
			return nil, nil, false, err
		}
		if !resolved.Exists {
			return map[string]bool{}, resolved.Prepared, resolved.SourceMissing, nil
		}
		required, err := deploy.RequiredPulseNamesRendered(resolved.Registry)
		return required, resolved.Prepared, resolved.SourceMissing, err
	}
	required, err := deploy.RequiredPulseNames(opts.RepoRoot)
	return required, nil, false, err
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
) (map[string]bool, []byte, bool, error) {
	pulse, pulseSelected := artifactNamed(selected, pulseArtifactName)
	jobs, jobsSelected := artifactNamed(selected, jobsArtifactName)
	if !pulseSelected || pulse.AbsentOnly || !jobsSelected || jobs.AbsentOnly {
		return requiredPulsesFor(selected, manifestArtifacts, opts)
	}

	resolved, err := requiredPulseJobRegistry(jobs, true, opts)
	if err != nil {
		return nil, nil, false, err
	}
	if resolved.SourceMissing {
		if !resolved.Exists {
			return map[string]bool{}, nil, true, nil
		}
		required, err := deploy.RequiredPulseNamesRendered(resolved.Registry)
		if err != nil {
			return nil, nil, false, err
		}
		return required, nil, true, nil
	}
	rendered := resolved.Prepared
	required, err := deploy.RequiredPulseNamesRendered(rendered)
	if err != nil {
		return nil, nil, false, err
	}

	live, exists, err := readLiveRecoveryJobRegistry(jobs, opts)
	if err != nil {
		return nil, nil, false, err
	}
	if !exists {
		return required, rendered, false, nil
	}
	liveRequired, err := deploy.RequiredPulseNamesRendered(live)
	if err != nil {
		// A readable registry rejected by the runtime parser has no functioning
		// live job set to preserve. The paired normal selection may repair it
		// from the already validated prospective snapshot. Observation errors
		// above remain fail-closed, as do pulse-only and absent-only paths.
		//nolint:nilerr // Parser rejection is the repair condition, not a success-path error leak.
		return required, rendered, false, nil
	}
	for name := range liveRequired {
		required[name] = true
	}
	return required, rendered, false, nil
}

type resolvedRecoveryJobRegistry struct {
	Registry      []byte
	Prepared      []byte
	Exists        bool
	SourceMissing bool
}

// requiredPulseJobRegistry returns the bytes the recovery-loop runtime will be
// authoritative over after this command. An unselected live registry remains
// deployed, while a selected normal registry will be replaced from source.
// Absent-only registries remain operator-owned even when selected. A missing
// live registry falls back to the rendered source that would seed it; other
// observation failures are not absence.
func requiredPulseJobRegistry(
	a deploy.Artifact,
	jobsSelected bool,
	opts deploy.Options,
) (resolvedRecoveryJobRegistry, error) {
	// An unselected registry remains the runtime authority after this command,
	// regardless of whether it is normally source-owned. Absent-only registries
	// are always operator-owned and likewise remain live even when selected.
	if a.AbsentOnly || !jobsSelected {
		live, exists, err := readLiveRecoveryJobRegistry(a, opts)
		if err != nil {
			return resolvedRecoveryJobRegistry{}, err
		}
		if exists {
			return resolvedRecoveryJobRegistry{Registry: live, Exists: true}, nil
		}
	}

	rendered, err := a.Render(opts.RepoRoot, opts.Home)
	if err != nil {
		if a.Optional && errors.Is(err, os.ErrNotExist) {
			if jobsSelected && !a.AbsentOnly {
				live, exists, readErr := readLiveRecoveryJobRegistry(a, opts)
				if readErr != nil {
					return resolvedRecoveryJobRegistry{}, readErr
				}
				return resolvedRecoveryJobRegistry{
					Registry:      live,
					Exists:        exists,
					SourceMissing: true,
				}, nil
			}
			return resolvedRecoveryJobRegistry{}, nil
		}
		return resolvedRecoveryJobRegistry{}, fmt.Errorf("render recovery job registry: %w", err)
	}
	resolved := resolvedRecoveryJobRegistry{Registry: rendered, Exists: true}
	if jobsSelected && !a.AbsentOnly {
		resolved.Prepared = rendered
	}
	return resolved, nil
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
