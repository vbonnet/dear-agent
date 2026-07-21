// Package instructionlint validates retired vocabulary and prohibited command
// guidance on declared active AI instruction surfaces.
package instructionlint

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	Context     string
	Replacement string
}

func (v Violation) Error() string {
	location := v.Path
	if v.Line > 0 {
		location = fmt.Sprintf("%s:%d", v.Path, v.Line)
	}
	return fmt.Sprintf("%s: %s: %q; use %s", location, v.Rule, v.Excerpt, v.Replacement)
}

// CheckRepository validates every tracked path matched by the policy
// in root/.dear-agent.yml. It never mutates source or policy files.
func CheckRepository(ctx context.Context, root string) (Result, []Violation, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Result{}, nil, fmt.Errorf("instructionlint: absolute repository root: %w", err)
	}
	policy, err := loadPolicy(filepath.Join(absRoot, ".dear-agent.yml"))
	if err != nil {
		return Result{}, nil, err
	}
	paths, err := trackedFiles(ctx, absRoot)
	if err != nil {
		return Result{}, nil, err
	}
	tracked := make(map[string]bool, len(paths))
	for _, relative := range paths {
		tracked[relative] = true
	}
	var findings []Violation
	files := 0
	matchedSurface := make(map[string]int, len(policy.Surfaces))
	for _, relative := range paths {
		matches := matchingSurfaces(policy.Surfaces, relative)
		if len(matches) == 0 {
			continue
		}
		if len(matches) > 1 {
			return Result{}, nil, fmt.Errorf("instructionlint: %s matches multiple instruction surfaces: %s", relative, strings.Join(matches, ", "))
		}
		matchedSurface[matches[0]]++
		absolute := filepath.Join(absRoot, filepath.FromSlash(relative))
		symlink, err := inspectGovernedPath(absRoot, absolute, relative, policy.Surfaces, tracked)
		if err != nil {
			return Result{}, nil, err
		}
		if symlink {
			files++
			continue
		}
		data, err := os.ReadFile(absolute)
		if err != nil {
			return Result{}, nil, fmt.Errorf("instructionlint: read %s: %w", relative, err)
		}
		files++
		var fileFindings []Violation
		fileFindings = append(fileFindings, importViolations(relative, data, policy.Surfaces, tracked)...)
		segments, err := instructionSegments(relative, data)
		if err != nil {
			return Result{}, nil, fmt.Errorf("instructionlint: parse %s: %w", relative, err)
		}
		for _, segment := range segments {
			fileFindings = append(fileFindings, evaluateSegment(relative, segment)...)
		}
		findings = append(findings, bindFindingContexts(fileFindings, data)...)
	}
	for _, surface := range policy.Surfaces {
		if matchedSurface[surface.Match] == 0 {
			return Result{}, nil, fmt.Errorf("instructionlint: instruction surface %q owned by %q matches no tracked paths", surface.Match, surface.Owner)
		}
	}
	findings = applyExclusions(findings, policy.Exclusions)
	sortViolations(findings)
	return Result{Files: files, Exclusions: len(policy.Exclusions)}, findings, nil
}

func bindFindingContexts(findings []Violation, data []byte) []Violation {
	for i := range findings {
		findings[i].Context = contextFingerprint(data, findings[i].Line)
	}
	return findings
}

func contextFingerprint(data []byte, line int) string {
	lines := strings.Split(string(data), "\n")
	if line < 1 || line > len(lines) {
		return ""
	}
	index := line - 1
	window := []string{strings.TrimSpace(lines[index])}
	for before := index - 1; before >= 0; before-- {
		if value := strings.TrimSpace(lines[before]); value != "" {
			window = append([]string{value}, window...)
			break
		}
	}
	for after := index + 1; after < len(lines); after++ {
		if value := strings.TrimSpace(lines[after]); value != "" {
			window = append(window, value)
			break
		}
	}
	fingerprintInput := fmt.Sprintf("line:%d\n%s", line, strings.Join(window, "\n"))
	digest := sha256.Sum256([]byte(fingerprintInput))
	return fmt.Sprintf("%x", digest)
}

func inspectGovernedPath(root, absolute, relative string, surfaces []Surface, tracked map[string]bool) (bool, error) {
	info, err := os.Lstat(absolute)
	if err != nil {
		return false, fmt.Errorf("instructionlint: inspect %s: %w", relative, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return false, nil
	}
	if err := validateGovernedSymlink(root, absolute, relative, surfaces, tracked); err != nil {
		return false, err
	}
	return true, nil
}

func validateGovernedSymlink(root, absolute, relative string, surfaces []Surface, tracked map[string]bool) error {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("instructionlint: resolve repository root for instruction symlink %s: %w", relative, err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return fmt.Errorf("instructionlint: resolve instruction symlink %s: %w", relative, err)
	}
	targetRelative, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil {
		return fmt.Errorf("instructionlint: locate instruction symlink target %s: %w", relative, err)
	}
	if targetRelative == ".." || filepath.IsAbs(targetRelative) || strings.HasPrefix(targetRelative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("instructionlint: instruction symlink %s resolves outside repository", relative)
	}
	target := filepath.ToSlash(targetRelative)
	matches := matchingSurfaces(surfaces, target)
	if !tracked[target] || len(matches) != 1 {
		return fmt.Errorf("instructionlint: instruction symlink %s target %s must be tracked and match exactly one instruction surface", relative, target)
	}
	return nil
}

func instructionSegments(relative string, data []byte) ([]Segment, error) {
	extension := strings.ToLower(filepath.Ext(relative))
	if extension == ".yml" || extension == ".yaml" || extension == ".json" {
		return parseYAMLSegments(data)
	}
	if extension == ".go" {
		return parseGoPromptSegments(data)
	}
	if strings.Contains(filepath.ToSlash(relative), "/hooks/") && extension == "" {
		return parseScriptSegments(data), nil
	}
	return parseSegments(data), nil
}

var instructionImport = regexp.MustCompile(`(?m)^\s*@import\s+(\S+)\s*$`)

func importViolations(relative string, data []byte, surfaces []Surface, tracked map[string]bool) []Violation {
	var violations []Violation
	for _, match := range instructionImport.FindAllSubmatch(data, -1) {
		target := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(relative), filepath.FromSlash(string(match[1])))))
		if target == "." || target == ".." || strings.HasPrefix(target, "../") ||
			!tracked[target] || len(matchingSurfaces(surfaces, target)) != 1 {
			violations = append(violations, Violation{
				Path:        relative,
				Rule:        "ungoverned-import",
				Excerpt:     strings.TrimSpace(string(match[0])),
				Replacement: "import exactly one declared instruction-policy surface",
			})
		}
	}
	return violations
}

func trackedFiles(ctx context.Context, root string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "ls-files", "-z", "--")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("instructionlint: git ls-files: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var paths []string
	for raw := range bytes.SplitSeq(output, []byte{0}) {
		if len(raw) == 0 {
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
			if exclusionKey(finding.Path, finding.Rule, finding.Excerpt, finding.Context) != exclusionKey(exclusion.Path, exclusion.Rule, exclusion.Excerpt, exclusion.Context) || used[i] >= exclusion.Count {
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

func exclusionKey(path, rule, excerpt, context string) string {
	return path + "\x00" + rule + "\x00" + strings.TrimSpace(excerpt) + "\x00" + context
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
