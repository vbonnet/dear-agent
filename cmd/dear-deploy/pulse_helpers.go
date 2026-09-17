package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"

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
	manifestArtifacts []deploy.Artifact,
	opts deploy.Options,
	asJSON bool,
	stdout io.Writer,
) (deploy.Result, error) {
	if !a.AbsentOnly {
		required, err := requiredPulsesFor(manifestArtifacts, opts)
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
	outcome, err := mergePulsesDuringDeploy(a, manifestArtifacts, opts, asJSON, stdout)
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
	case len(outcome.Added) > 0:
		action = deploy.ActionUpdated
	}
	sum := sha256.Sum256(live)
	return deploy.Result{
		Name:         a.Name,
		DeployedPath: hostPath,
		Action:       action,
		SHA256:       fmt.Sprintf("%x", sum),
	}, nil
}

func validateNormalPulseArtifact(
	a deploy.Artifact,
	manifestArtifacts []deploy.Artifact,
	opts deploy.Options,
) error {
	required, err := requiredPulsesFor(manifestArtifacts, opts)
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
	manifestArtifacts []deploy.Artifact,
	opts deploy.Options,
) error {
	if err := validateNormalPulseArtifact(a, manifestArtifacts, opts); err != nil {
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
