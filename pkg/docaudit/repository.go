package docaudit

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func checkRepository(ctx context.Context, root string, opts Options) (Report, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return Report{}, fmt.Errorf("docaudit: resolve repository root: %w", err)
	}
	policy, err := loadPolicy(filepath.Join(root, ".dear-agent.yml"))
	if err != nil {
		return Report{}, err
	}
	tracked, err := trackedFiles(ctx, root)
	if err != nil {
		return Report{}, err
	}

	report := Report{}
	for _, name := range tracked {
		surface, matchErr := matchingSurface(name, policy.Surfaces)
		if matchErr != nil {
			return Report{}, matchErr
		}
		if surface == nil {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if readErr != nil {
			return Report{}, fmt.Errorf("docaudit: read %s: %w", name, readErr)
		}
		report.Documents++
		kind := classifyMarker(data, surface.MaxAgeDays, opts.AsOf)
		if kind != "" {
			report.Findings = append(report.Findings, Finding{Kind: kind, Path: name, Surface: surface.Name})
		}
	}
	sortFindings(report.Findings)

	baselinePath := filepath.Join(root, filepath.FromSlash(policy.Baseline))
	baseline, err := loadBaselineFile(baselinePath)
	if err != nil {
		return Report{}, err
	}
	report.NewFindings, report.StaleBaseline = compareFindings(report.Findings, baseline)
	if strings.TrimSpace(opts.BaselineRef) != "" {
		basePolicy, policyPresent, policyErr := loadPolicyAtRef(ctx, root, opts.BaselineRef)
		if policyErr != nil {
			return Report{}, policyErr
		}
		if policyPresent {
			if policyErr := validatePolicyRatchet(basePolicy, policy, tracked); policyErr != nil {
				return Report{}, policyErr
			}
		}
		report.AddedBaseline, err = baselineAdditionsAtRef(ctx, root, opts.BaselineRef, policy, baseline)
		if err != nil {
			return Report{}, err
		}
	}
	return report, nil
}

func validatePolicyRatchet(base, current Policy, tracked []string) error {
	for _, name := range tracked {
		baseSurface, err := matchingSurface(name, base.Surfaces)
		if err != nil {
			return err
		}
		if baseSurface == nil {
			continue
		}
		currentSurface, err := matchingSurface(name, current.Surfaces)
		if err != nil {
			return err
		}
		if currentSurface == nil {
			return fmt.Errorf("docaudit: policy weakens governed coverage for tracked path %q", name)
		}
		if currentSurface.MaxAgeDays > baseSurface.MaxAgeDays {
			return fmt.Errorf("docaudit: policy raises max-age-days for %q from %d to %d", name, baseSurface.MaxAgeDays, currentSurface.MaxAgeDays)
		}
	}
	return nil
}

func baselineAdditionsAtRef(ctx context.Context, root, ref string, currentPolicy Policy, current []BaselineEntry) ([]BaselineEntry, error) {
	basePolicy, policyPresent, err := loadPolicyAtRef(ctx, root, ref)
	if err != nil {
		return nil, err
	}
	baseBaselinePath := currentPolicy.Baseline
	if policyPresent {
		baseBaselinePath = basePolicy.Baseline
	}
	base, baselinePresent, err := loadBaselineAtRef(ctx, root, ref, baseBaselinePath)
	if err != nil {
		return nil, err
	}
	// Bootstrap is allowed only when the base revision had no living-docs
	// policy and no baseline at the current path. Renaming the configured
	// baseline therefore cannot disguise additions as a first-time setup.
	if !policyPresent && !baselinePresent {
		return nil, nil
	}
	return addedEntries(current, base), nil
}

func trackedFiles(ctx context.Context, root string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-files", "-z", "--full-name")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("docaudit: discover tracked files: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	parts := bytes.Split(output, []byte{0})
	files := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) > 0 {
			files = append(files, filepath.ToSlash(string(part)))
		}
	}
	sort.Strings(files)
	return files, nil
}

func sortFindings(findings []Finding) {
	sort.Slice(findings, func(i, j int) bool { return findings[i].ID() < findings[j].ID() })
}
