package instructionlint

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// CheckExclusionRatchet rejects new or enlarged instruction-policy exclusions
// relative to a Git commit. A missing baseline exclusions file is an explicit
// bootstrap: the first policy introduction is reviewable, while later growth
// is blocked.
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
	baseline, exists, err := baselineExclusions(ctx, root, baselineRef)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}

	previous := make(map[string]Exclusion, len(baseline))
	for _, exclusion := range baseline {
		previous[exclusionKey(exclusion.Path, exclusion.Rule, exclusion.Excerpt)] = exclusion
	}
	var violations []Violation
	violationPath := policy.ExclusionsFile
	if violationPath == "" {
		violationPath = ".dear-agent.yml"
	}
	for _, exclusion := range policy.Exclusions {
		key := exclusionKey(exclusion.Path, exclusion.Rule, exclusion.Excerpt)
		before, ok := previous[key]
		if !ok {
			violations = append(violations, exclusionGrowthViolation(violationPath, exclusion, "new exclusion"))
			continue
		}
		if exclusion.Count > before.Count {
			violations = append(violations, exclusionGrowthViolation(violationPath, exclusion, fmt.Sprintf("count increased from %d to %d", before.Count, exclusion.Count)))
		}
	}
	sortViolations(violations)
	return violations, nil
}

func baselineExclusions(ctx context.Context, root, ref string) ([]Exclusion, bool, error) {
	data, exists, err := readGitBlob(ctx, root, ref, ".dear-agent.yml")
	if err != nil || !exists {
		return nil, false, err
	}
	var config repositoryConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, false, fmt.Errorf("instructionlint: parse baseline policy: %w", err)
	}
	policy := config.InstructionPolicy
	if policy.ExclusionsFile == "" {
		if len(policy.Surfaces) == 0 && len(policy.Exclusions) == 0 {
			return nil, false, nil
		}
		if err := validateExclusionsOnly(policy.Exclusions); err != nil {
			return nil, false, fmt.Errorf("instructionlint: invalid baseline exclusions: %w", err)
		}
		return policy.Exclusions, true, nil
	}
	if len(policy.Exclusions) > 0 {
		return nil, false, fmt.Errorf("instructionlint: baseline uses exclusions and exclusions-file together")
	}
	if !cleanRelativePath(policy.ExclusionsFile) {
		return nil, false, fmt.Errorf("instructionlint: baseline exclusions-file must be a clean repository-relative path")
	}
	data, exists, err = readGitBlob(ctx, root, ref, policy.ExclusionsFile)
	if err != nil || !exists {
		return nil, false, err
	}
	exclusions, err := parseExclusions(data)
	if err != nil {
		return nil, false, fmt.Errorf("instructionlint: parse baseline exclusions: %w", err)
	}
	if err := validateExclusionsOnly(exclusions); err != nil {
		return nil, false, fmt.Errorf("instructionlint: invalid baseline exclusions: %w", err)
	}
	return exclusions, true, nil
}

func validateExclusionsOnly(exclusions []Exclusion) error {
	return validatePolicy(Policy{Surfaces: []Surface{{Match: "bootstrap", Owner: "ratchet"}}, Exclusions: exclusions})
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
		return nil, false, nil
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
