// Package instructionlint validates retired vocabulary and prohibited command
// guidance on declared active AI instruction surfaces.
package instructionlint

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Result summarizes one successful repository inspection.
type Result struct {
	Files      int
	Exclusions int
}

// Violation is one actionable content finding or stale policy exclusion.
type Violation struct {
	Path        string
	Line        int
	Rule        string
	Excerpt     string
	Replacement string
}

func (v Violation) Error() string {
	location := v.Path
	if v.Line > 0 {
		location = fmt.Sprintf("%s:%d", v.Path, v.Line)
	}
	return fmt.Sprintf("%s: %s: %q; use %s", location, v.Rule, v.Excerpt, v.Replacement)
}

// CheckRepository validates every tracked Markdown path matched by the policy
// in root/.dear-agent.yml. It never mutates source or policy files.
func CheckRepository(root string) (Result, []Violation, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Result{}, nil, fmt.Errorf("instructionlint: absolute repository root: %w", err)
	}
	policy, err := loadPolicy(filepath.Join(absRoot, ".dear-agent.yml"))
	if err != nil {
		return Result{}, nil, err
	}
	paths, err := trackedMarkdown(absRoot)
	if err != nil {
		return Result{}, nil, err
	}
	var findings []Violation
	files := 0
	for _, relative := range paths {
		matches := matchingSurfaces(policy.Surfaces, relative)
		if len(matches) == 0 {
			continue
		}
		if len(matches) > 1 {
			return Result{}, nil, fmt.Errorf("instructionlint: %s matches multiple instruction surfaces: %s", relative, strings.Join(matches, ", "))
		}
		data, err := os.ReadFile(filepath.Join(absRoot, filepath.FromSlash(relative)))
		if err != nil {
			return Result{}, nil, fmt.Errorf("instructionlint: read %s: %w", relative, err)
		}
		files++
		for _, segment := range parseSegments(data) {
			findings = append(findings, evaluateSegment(relative, segment)...)
		}
	}
	findings = applyExclusions(findings, policy.Exclusions)
	sortViolations(findings)
	return Result{Files: files, Exclusions: len(policy.Exclusions)}, findings, nil
}

func trackedMarkdown(root string) ([]string, error) {
	cmd := exec.Command("git", "-C", root, "ls-files", "-z", "--")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("instructionlint: git ls-files: %w", err)
	}
	var paths []string
	for raw := range bytes.SplitSeq(output, []byte{0}) {
		if len(raw) == 0 || !strings.EqualFold(filepath.Ext(string(raw)), ".md") {
			continue
		}
		paths = append(paths, filepath.ToSlash(string(raw)))
	}
	sort.Strings(paths)
	return paths, nil
}

func matchingSurfaces(surfaces []Surface, relative string) []string {
	var matches []string
	for _, surface := range surfaces {
		if globPathMatch(surface.Match, relative) {
			matches = append(matches, surface.Match)
		}
	}
	return matches
}

func applyExclusions(findings []Violation, exclusions []Exclusion) []Violation {
	sortViolations(findings)
	used := make([]int, len(exclusions))
	remaining := make([]Violation, 0, len(findings)+len(exclusions))
	for _, finding := range findings {
		suppressed := false
		for i, exclusion := range exclusions {
			if exclusionKey(finding.Path, finding.Rule, finding.Excerpt) != exclusionKey(exclusion.Path, exclusion.Rule, exclusion.Excerpt) || used[i] >= exclusion.Count {
				continue
			}
			used[i]++
			suppressed = true
			break
		}
		if !suppressed {
			remaining = append(remaining, finding)
		}
	}
	for i, exclusion := range exclusions {
		if used[i] == exclusion.Count {
			continue
		}
		remaining = append(remaining, Violation{
			Path:        exclusion.Path,
			Rule:        "stale-exclusion",
			Excerpt:     fmt.Sprintf("%s expected %d exact occurrence(s), matched %d", exclusion.Rule, exclusion.Count, used[i]),
			Replacement: fmt.Sprintf("remove or update the exclusion owned by %s (%s)", exclusion.Owner, exclusion.Reason),
		})
	}
	sortViolations(remaining)
	return remaining
}

func exclusionKey(path, rule, excerpt string) string {
	return path + "\x00" + rule + "\x00" + strings.TrimSpace(excerpt)
}

func sortViolations(violations []Violation) {
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].Path != violations[j].Path {
			return violations[i].Path < violations[j].Path
		}
		if violations[i].Line != violations[j].Line {
			return violations[i].Line < violations[j].Line
		}
		if violations[i].Rule != violations[j].Rule {
			return violations[i].Rule < violations[j].Rule
		}
		return violations[i].Excerpt < violations[j].Excerpt
	})
}
