package adrlint

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

func checkRepository(ctx context.Context, root string) (Report, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return Report{}, fmt.Errorf("adrlint: resolve repository root: %w", err)
	}
	policy, err := loadPolicy(filepath.Join(root, ".dear-agent.yml"))
	if err != nil {
		return Report{}, err
	}
	tracked, err := trackedADRFiles(ctx, root)
	if err != nil {
		return Report{}, err
	}
	trackedSet := make(map[string]bool, len(tracked))
	for _, name := range tracked {
		trackedSet[name] = true
	}

	report := Report{}
	governed := map[string]bool{}
	for _, aggregate := range policy.Aggregates {
		governed[aggregate.Path] = true
		if !trackedSet[aggregate.Path] {
			report.Violations = append(report.Violations, Violation{Path: aggregate.Path, Reason: "declared aggregate is not tracked"})
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(aggregate.Path)))
		if readErr != nil {
			return Report{}, fmt.Errorf("adrlint: read %s: %w", aggregate.Path, readErr)
		}
		report.Records++
		report.Violations = append(report.Violations, parseAggregate(root, aggregate.Path, data, policy.MaxRecordLines)...)
	}

	for _, scope := range policy.Scopes {
		records, scopeViolations, scopeErr := validateScope(root, scope, tracked, policy.MaxRecordLines)
		if scopeErr != nil {
			return Report{}, scopeErr
		}
		report.Records += len(records)
		report.Violations = append(report.Violations, scopeViolations...)
		for name := range records {
			governed[filepath.ToSlash(filepath.Join(scope.Path, name))] = true
		}
	}

	for _, name := range tracked {
		if !adrShapedPath(name) || governed[name] || excluded(name, policy.Exclusions) {
			continue
		}
		report.Violations = append(report.Violations, Violation{Path: name, Reason: "ungoverned ADR path; declare a scope, aggregate, or reasoned exclusion"})
	}
	return report, nil
}

func validateScope(root string, scope Scope, tracked []string, maxLines int) (map[string]record, []Violation, error) {
	records := map[string]record{}
	ids := map[string]string{}
	var violations []Violation
	for _, relative := range tracked {
		if filepath.ToSlash(filepath.Dir(relative)) != scope.Path || !adrFilePattern.MatchString(filepath.Base(relative)) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			return nil, nil, fmt.Errorf("adrlint: read %s: %w", relative, err)
		}
		parsed, recordViolations := parseRecord(root, relative, data, maxLines)
		violations = append(violations, recordViolations...)
		if previous, exists := ids[parsed.id]; exists {
			violations = append(violations, Violation{Path: relative, Reason: fmt.Sprintf("duplicate ADR-%s: %s and %s", parsed.id, previous, parsed.filename)})
		} else {
			ids[parsed.id] = parsed.filename
		}
		records[parsed.filename] = parsed
	}
	indexRelative := filepath.ToSlash(filepath.Join(scope.Path, scope.Index))
	indexData, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(indexRelative)))
	if err != nil {
		if os.IsNotExist(err) {
			violations = append(violations, Violation{Path: indexRelative, Reason: "declared ADR index does not exist"})
			return records, violations, nil
		}
		return nil, nil, fmt.Errorf("adrlint: read %s: %w", indexRelative, err)
	}
	violations = append(violations, validateIndex(root, indexRelative, indexData, records, maxLines)...)
	return records, violations, nil
}

func trackedADRFiles(ctx context.Context, root string) ([]string, error) {
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
		return nil, fmt.Errorf("adrlint: discover tracked files: %w: %s", err, strings.TrimSpace(stderr.String()))
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

func adrShapedPath(name string) bool {
	base := filepath.Base(name)
	return base == "ADR.md" || adrFilePattern.MatchString(base)
}

func excluded(name string, exclusions []Exclusion) bool {
	for _, exclusion := range exclusions {
		if globPathMatch(exclusion.Match, name) {
			return true
		}
	}
	return false
}
