package docaudit

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func loadBaselineFile(file string) ([]BaselineEntry, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("docaudit: read baseline: %w", err)
	}
	return parseBaseline(data)
}

func parseBaseline(data []byte) ([]BaselineEntry, error) {
	var entries []BaselineEntry
	previous := ""
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		kindText, pathText, ok := strings.Cut(line, "\t")
		kind := FindingKind(kindText)
		if !ok || !knownFindingKinds[kind] || !validBaselinePath(pathText) {
			return nil, fmt.Errorf("docaudit: invalid baseline entry at line %d", lineNumber)
		}
		entry := BaselineEntry{Kind: kind, Path: pathText}
		id := entry.ID()
		if previous != "" && id <= previous {
			return nil, fmt.Errorf("docaudit: baseline entries must be unique and sorted at line %d", lineNumber)
		}
		entries = append(entries, entry)
		previous = id
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("docaudit: scan baseline: %w", err)
	}
	return entries, nil
}

func validBaselinePath(name string) bool {
	if name == "" || strings.ContainsAny(name, "\r\n\t") || filepath.IsAbs(name) || filepath.ToSlash(filepath.Clean(name)) != name {
		return false
	}
	return name != ".." && !strings.HasPrefix(name, "../")
}

func loadBaselineAtRef(ctx context.Context, root, ref, baselinePath string) ([]BaselineEntry, bool, error) {
	data, present, err := loadFileAtRef(ctx, root, ref, baselinePath)
	if err != nil || !present {
		return nil, present, err
	}
	entries, err := parseBaseline(data)
	return entries, true, err
}

func loadFileAtRef(ctx context.Context, root, ref, filePath string) ([]byte, bool, error) {
	if ref != strings.TrimSpace(ref) || ref == "" || strings.HasPrefix(ref, "-") {
		return nil, false, fmt.Errorf("docaudit: invalid baseline ref %q", ref)
	}
	verify := exec.CommandContext(ctx, "git", "rev-parse", "--verify", ref+"^{commit}")
	verify.Dir = root
	verify.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if output, err := verify.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		return nil, false, fmt.Errorf("docaudit: resolve baseline ref %q: %w: %s", ref, err, strings.TrimSpace(string(output)))
	}
	object := ref + ":" + filepath.ToSlash(filePath)
	exists := exec.CommandContext(ctx, "git", "cat-file", "-e", object)
	exists.Dir = root
	exists.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if err := exists.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("docaudit: inspect baseline at %q: %w", ref, err)
	}
	show := exec.CommandContext(ctx, "git", "show", object)
	show.Dir = root
	show.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	data, err := show.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, false, ctx.Err()
		}
		return nil, false, fmt.Errorf("docaudit: read baseline at %q: %w", ref, err)
	}
	return data, true, nil
}

func compareFindings(findings []Finding, baseline []BaselineEntry) (newFindings []Finding, stale []BaselineEntry) {
	current := make(map[string]bool, len(findings))
	for _, finding := range findings {
		current[finding.ID()] = true
	}
	allowed := make(map[string]bool, len(baseline))
	for _, entry := range baseline {
		allowed[entry.ID()] = true
		if !current[entry.ID()] {
			stale = append(stale, entry)
		}
	}
	for _, finding := range findings {
		if !allowed[finding.ID()] {
			newFindings = append(newFindings, finding)
		}
	}
	return newFindings, stale
}

func addedEntries(current, base []BaselineEntry) []BaselineEntry {
	prior := make(map[string]bool, len(base))
	for _, entry := range base {
		prior[entry.ID()] = true
	}
	var added []BaselineEntry
	for _, entry := range current {
		if !prior[entry.ID()] {
			added = append(added, entry)
		}
	}
	sort.Slice(added, func(i, j int) bool { return added[i].ID() < added[j].ID() })
	return added
}
