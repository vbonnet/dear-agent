package instructionlint

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// CheckExclusionRatchet rejects removed instruction surfaces and new or
// enlarged exclusions relative to a Git commit. A missing baseline policy is
// an explicit bootstrap; after that, governed inventory and debt only ratchet
// toward stricter coverage.
func CheckExclusionRatchet(ctx context.Context, root, baselineRef string) ([]Violation, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("instructionlint: resolve repository root: %w", err)
	}
	if baselineRef == "" || strings.HasPrefix(baselineRef, "-") || strings.ContainsAny(baselineRef, ":\x00\r\n") {
		return nil, fmt.Errorf("instructionlint: baseline ref must be a non-option commit or ref")
	}
	policy, err := loadPolicy(filepath.Join(root, ".dear-agent.yml"))
	if err != nil {
		return nil, err
	}
	if err := verifyCommit(ctx, root, baselineRef); err != nil {
		return nil, err
	}
	baseline, exists, err := loadBaselinePolicy(ctx, root, baselineRef)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	violations := ratchetViolations(policy, baseline)
	sortViolations(violations)
	return violations, nil
}

func ratchetViolations(policy, baseline Policy) []Violation {
	previous := make(map[string]Exclusion, len(baseline.Exclusions))
	for _, exclusion := range baseline.Exclusions {
		previous[exclusionKey(exclusion.Path, exclusion.Rule, exclusion.Excerpt, exclusion.Context)] = exclusion
	}
	var violations []Violation
	currentSurfaces := make(map[string]bool, len(policy.Surfaces))
	for _, surface := range policy.Surfaces {
		currentSurfaces[surface.Match] = true
	}
	for _, surface := range baseline.Surfaces {
		if !currentSurfaces[surface.Match] {
			violations = append(violations, surfaceRemovalViolation(surface))
		}
	}
	violationPath := policy.ExclusionsFile
	if violationPath == "" {
		violationPath = ".dear-agent.yml"
	}
	for _, exclusion := range policy.Exclusions {
		key := exclusionKey(exclusion.Path, exclusion.Rule, exclusion.Excerpt, exclusion.Context)
		before, ok := previous[key]
		if !ok {
			violations = append(violations, exclusionGrowthViolation(violationPath, exclusion, "new exclusion"))
			continue
		}
		if exclusion.Count > before.Count {
			violations = append(violations, exclusionGrowthViolation(violationPath, exclusion, fmt.Sprintf("count increased from %d to %d", before.Count, exclusion.Count)))
		}
	}
	return violations
}

func loadBaselinePolicy(ctx context.Context, root, ref string) (Policy, bool, error) {
	data, exists, err := readGitBlob(ctx, root, ref, ".dear-agent.yml")
	if err != nil || !exists {
		return Policy{}, false, err
	}
	var config repositoryConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return Policy{}, false, fmt.Errorf("instructionlint: parse baseline policy: %w", err)
	}
	policy := config.InstructionPolicy
	if policy.ExclusionsFile == "" {
		if len(policy.Surfaces) == 0 && len(policy.Exclusions) == 0 {
			return Policy{}, false, nil
		}
		if err := validatePolicy(policy); err != nil {
			return Policy{}, false, fmt.Errorf("instructionlint: invalid baseline policy: %w", err)
		}
		return policy, true, nil
	}
	if len(policy.Exclusions) > 0 {
		return Policy{}, false, fmt.Errorf("instructionlint: baseline uses exclusions and exclusions-file together")
	}
	if !cleanRelativePath(policy.ExclusionsFile) {
		return Policy{}, false, fmt.Errorf("instructionlint: baseline exclusions-file must be a clean repository-relative path")
	}
	data, exists, err = readGitBlob(ctx, root, ref, policy.ExclusionsFile)
	if err != nil || !exists {
		return Policy{}, false, err
	}
	exclusions, err := parseExclusions(data)
	if err != nil {
		return Policy{}, false, fmt.Errorf("instructionlint: parse baseline exclusions: %w", err)
	}
	policy.Exclusions = exclusions
	if err := validatePolicy(policy); err != nil {
		return Policy{}, false, fmt.Errorf("instructionlint: invalid baseline policy: %w", err)
	}
	return policy, true, nil
}

func surfaceRemovalViolation(surface Surface) Violation {
	return Violation{
		Path:        ".dear-agent.yml",
		Rule:        "surface-removal",
		Excerpt:     fmt.Sprintf("%s owned by %s", surface.Match, surface.Owner),
		Replacement: "restore the baseline instruction surface; governed inventory may only expand after bootstrap",
	}
}

func exclusionGrowthViolation(path string, exclusion Exclusion, detail string) Violation {
	return Violation{
		Path:        path,
		Rule:        "exclusion-growth",
		Excerpt:     fmt.Sprintf("%s: %s / %s / %q", detail, exclusion.Path, exclusion.Rule, exclusion.Excerpt),
		Replacement: "remove the new debt or reduce it to the baseline count; exclusions may only shrink after bootstrap",
	}
}

func verifyCommit(ctx context.Context, root, ref string) error {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", ref+"^{commit}")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("instructionlint: resolve baseline ref %q: %w: %s", ref, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func readGitBlob(ctx context.Context, root, ref, path string) ([]byte, bool, error) {
	object := ref + ":" + filepath.ToSlash(path)
	cmd := exec.CommandContext(ctx, "git", "cat-file", "-e", object)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("instructionlint: inspect baseline exclusions %s: %w", object, err)
	}
	cmd = exec.CommandContext(ctx, "git", "show", object)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	data, err := cmd.Output()
	if err != nil {
		return nil, false, fmt.Errorf("instructionlint: read baseline exclusions %s: %w", object, err)
	}
	return data, true, nil
}
