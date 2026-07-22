package adrlint

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
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

	governed := governedADRPaths(policy, tracked)
	report, err := checkAggregates(root, policy, trackedSet, governed)
	if err != nil {
		return Report{}, err
	}

	for _, scope := range policy.Scopes {
		records, scopeViolations, scopeErr := validateScope(root, scope, tracked, trackedSet, governed, effectiveMaxLines(policy.MaxLines, scope.MaxLines))
		if scopeErr != nil {
			return Report{}, scopeErr
		}
		report.Records += len(records)
		report.Violations = append(report.Violations, scopeViolations...)
		for name := range records {
			governed[filepath.ToSlash(filepath.Join(scope.Path, name))] = true
		}
	}

	report.Violations = append(report.Violations, ungovernedADRViolations(tracked, governed, policy.Exclusions)...)
	return report, nil
}

func governedADRPaths(policy Policy, tracked []string) map[string]bool {
	governed := make(map[string]bool, len(policy.Aggregates))
	for _, aggregate := range policy.Aggregates {
		governed[aggregate.Path] = true
	}
	for _, scope := range policy.Scopes {
		for _, name := range tracked {
			base := filepath.Base(name)
			if filepath.ToSlash(filepath.Dir(name)) == scope.Path && governedRecordFilename(base) && base != scope.Index {
				governed[name] = true
			}
		}
	}
	return governed
}

func governedRecordFilename(name string) bool {
	return adrFilePattern.MatchString(name) || adrLikeFilename(name)
}

func checkAggregates(root string, policy Policy, trackedSet, governed map[string]bool) (Report, error) {
	report := Report{}
	for _, aggregate := range policy.Aggregates {
		if !trackedSet[aggregate.Path] {
			report.Violations = append(report.Violations, Violation{Path: aggregate.Path, Reason: "declared aggregate is not tracked"})
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(aggregate.Path)))
		if err != nil {
			return Report{}, fmt.Errorf("adrlint: read %s: %w", aggregate.Path, err)
		}
		report.Records++
		report.Violations = append(report.Violations, sizeViolations(aggregate.Path, data, effectiveMaxLines(policy.MaxLines, aggregate.MaxLines))...)
		report.Violations = append(report.Violations, parseAggregate(root, aggregate.Path, data, governed)...)
	}
	return report, nil
}

func ungovernedADRViolations(tracked []string, governed map[string]bool, exclusions []Exclusion) []Violation {
	var violations []Violation
	for _, name := range tracked {
		if adrShapedPath(name) && !governed[name] && !excluded(name, exclusions) {
			violations = append(violations, Violation{Path: name, Reason: "ungoverned ADR path; declare a scope, aggregate, or reasoned exclusion"})
		}
	}
	return violations
}

func validateScope(root string, scope Scope, tracked []string, trackedSet, governed map[string]bool, maxLines int) (map[string]record, []Violation, error) {
	records := map[string]record{}
	ids := map[string]string{}
	var violations []Violation
	for _, relative := range tracked {
		if filepath.ToSlash(filepath.Dir(relative)) != scope.Path {
			continue
		}
		base := filepath.Base(relative)
		if !adrFilePattern.MatchString(base) {
			if base != scope.Index && adrLikeFilename(base) {
				violations = append(violations, Violation{Path: relative, Reason: "malformed ADR filename; want ADR-NNN-slug.md or NNNN-slug.md"})
			}
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			return nil, nil, fmt.Errorf("adrlint: read %s: %w", relative, err)
		}
		violations = append(violations, sizeViolations(relative, data, maxLines)...)
		parsed, recordViolations := parseRecord(root, relative, data, governed)
		violations = append(violations, recordViolations...)
		identity, identityErr := strconv.Atoi(parsed.id)
		if identityErr != nil {
			return nil, nil, fmt.Errorf("adrlint: normalize ADR identity %q: %w", parsed.id, identityErr)
		}
		identityKey := strconv.Itoa(identity)
		if previous, exists := ids[identityKey]; exists {
			violations = append(violations, Violation{Path: relative, Reason: fmt.Sprintf("duplicate ADR identity %d: %s and %s", identity, previous, parsed.filename)})
		} else {
			ids[identityKey] = parsed.filename
		}
		records[parsed.filename] = parsed
	}
	indexRelative := filepath.ToSlash(filepath.Join(scope.Path, scope.Index))
	if !trackedSet[indexRelative] {
		violations = append(violations, Violation{Path: indexRelative, Reason: "declared ADR index is not tracked"})
		return records, violations, nil
	}
	indexData, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(indexRelative)))
	if err != nil {
		if os.IsNotExist(err) {
			violations = append(violations, Violation{Path: indexRelative, Reason: "declared ADR index does not exist"})
			return records, violations, nil
		}
		return nil, nil, fmt.Errorf("adrlint: read %s: %w", indexRelative, err)
	}
	violations = append(violations, sizeViolations(indexRelative, indexData, maxLines)...)
	violations = append(violations, validateIndex(root, indexRelative, indexData, records)...)
	return records, violations, nil
}

func sizeViolations(relative string, data []byte, maxLines int) []Violation {
	lines := bytes.Count(data, []byte{'\n'})
	if len(data) > 0 && data[len(data)-1] != '\n' {
		lines++
	}
	if lines <= maxLines {
		return nil
	}
	return []Violation{{Path: relative, Reason: fmt.Sprintf("%d lines exceeds the %d-line ADR review budget", lines, maxLines)}}
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
	return base == "ADR.md" || governedRecordFilename(base)
}

func adrLikeFilename(name string) bool {
	lower := strings.ToLower(name)
	return lower != "adr-index.md" && strings.HasSuffix(lower, ".md") && adrLikePrefix.MatchString(name)
}

func effectiveMaxLines(fallback, override int) int {
	if override > 0 {
		return override
	}
	return fallback
}

func excluded(name string, exclusions []Exclusion) bool {
	for _, exclusion := range exclusions {
		if globPathMatch(exclusion.Match, name) {
			return true
		}
	}
	return false
}
