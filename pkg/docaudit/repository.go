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
		base, present, baseErr := loadBaselineAtRef(ctx, root, opts.BaselineRef, policy.Baseline)
		if baseErr != nil {
			return Report{}, baseErr
		}
		if present {
			report.AddedBaseline = addedEntries(baseline, base)
		}
	}
	return report, nil
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
