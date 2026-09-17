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
) (deploy.Result, error) {
	if !a.AbsentOnly {
		required, err := requiredPulsesFor(selected, manifestArtifacts, opts)
		if err != nil {
			return deploy.Result{}, fmt.Errorf("resolve required pulses: %w", err)
		}
		return deploy.DeployValidated(a, opts, func(rendered []byte) error {
			if err := deploy.ValidateRequiredPulseConfigRendered(rendered, required); err != nil {
				return fmt.Errorf("validate pulse config: %w", err)
			}
			return nil
		})
	}

	hostPath := pulseHostPath(a, opts)
	outcome, err := mergePulsesDuringDeploy(a, selected, manifestArtifacts, opts, asJSON, stdout)
	if err != nil {
		return deploy.Result{}, fmt.Errorf("pulse merge: %w", err)
	}
	// #nosec G703 -- hostPath is the manifest-selected deployment target;
	// reading it back is required to report the lock-owned merge result.
	live, err := os.ReadFile(hostPath)
	if err != nil {
		return deploy.Result{}, fmt.Errorf("read merged pulse result: %w", err)
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
	}, nil
}

func validateNormalPulseArtifact(
	a deploy.Artifact,
	selected []deploy.Artifact,
	manifestArtifacts []deploy.Artifact,
	opts deploy.Options,
) error {
	required, err := requiredPulsesFor(selected, manifestArtifacts, opts)
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
