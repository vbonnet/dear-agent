package instructions_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var (
	adrFilePattern              = regexp.MustCompile(`^(?:ADR-([0-9]{3})|([0-9]{4}))-[a-z0-9-]+\.md$`)
	adrTitlePattern             = regexp.MustCompile(`(?m)^# ADR-([0-9]{3,4}): .+$`)
	adrStatusPattern            = regexp.MustCompile(`(?m)^Status: (Accepted|Proposed|Deprecated|Superseded)(?: .*)?$`)
	adrIndexPattern             = regexp.MustCompile(`(?m)^\| \[([0-9]{3,4})\]\(([^)]+\.md)\) \| [^|]+ \| (Accepted|Proposed|Deprecated|Superseded) \|$`)
	markdownLink                = regexp.MustCompile(`\[[^]]+\]\(([^)]+)\)`)
	markdownReferenceLink       = regexp.MustCompile(`\[[^]]+\]\[([^]]+)\]`)
	markdownReferenceDefinition = regexp.MustCompile(`(?m)^[ \t]{0,3}\[([^]]+)\]:[ \t]+<?([^> \t]+)>?(?:[ \t]+.*)?$`)
)

type adrRecord struct {
	id     string
	status string
}

func TestADRDirectoriesHaveUniqueIndexedLifecycle(t *testing.T) {
	root := repoRoot(t)
	for _, relativeDir := range []string{"docs/adr", "agm/docs/adr"} {
		t.Run(relativeDir, func(t *testing.T) {
			dir := filepath.Join(root, filepath.FromSlash(relativeDir))
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}

			records := map[string]adrRecord{}
			ids := map[string]string{}
			for _, entry := range entries {
				if entry.IsDir() || entry.Name() == "README.md" {
					continue
				}
				if filepath.Ext(entry.Name()) != ".md" {
					continue
				}
				match := adrFilePattern.FindStringSubmatch(entry.Name())
				if len(match) != 3 {
					t.Errorf("%s: malformed ADR filename; want ADR-NNN-slug.md or NNNN-slug.md", entry.Name())
					continue
				}
				id := match[1]
				if id == "" {
					id = match[2]
				}
				content := readFile(t, filepath.Join(dir, entry.Name()))
				titles := adrTitlePattern.FindAllStringSubmatch(content, -1)
				if len(titles) != 1 {
					t.Errorf("%s: want one ADR heading, got %d", entry.Name(), len(titles))
					continue
				}
				if titles[0][1] != id {
					t.Errorf("%s: heading ID %s does not match filename ID %s", entry.Name(), titles[0][1], id)
				}
				if previous, exists := ids[id]; exists {
					t.Errorf("duplicate ADR-%s: %s and %s", id, previous, entry.Name())
				} else {
					ids[id] = entry.Name()
				}

				statuses := adrStatusPattern.FindAllStringSubmatch(content, -1)
				if len(statuses) != 1 {
					t.Errorf("%s: want one normalized Status line, got %d", entry.Name(), len(statuses))
					continue
				}
				if statuses[0][1] == "Superseded" {
					successor, ok := adrSuccessorTarget(statuses[0][0], content)
					if !ok {
						t.Errorf("%s: superseded status must link to its live successor", entry.Name())
					} else {
						assertLiveADRSuccessor(t, root, dir, entry.Name(), successor)
					}
				}
				assertRelativeMarkdownLinksResolve(t, dir, entry.Name(), content)
				records[entry.Name()] = adrRecord{id: id, status: statuses[0][1]}
			}

			index := readFile(t, filepath.Join(dir, "README.md"))
			indexed := map[string]adrRecord{}
			for _, match := range adrIndexPattern.FindAllStringSubmatch(index, -1) {
				if _, exists := indexed[match[2]]; exists {
					t.Errorf("README indexes %s more than once", match[2])
				}
				indexed[match[2]] = adrRecord{id: match[1], status: match[3]}
			}
			assertSameADRRecords(t, records, indexed)
		})
	}
}

func assertLiveADRSuccessor(t *testing.T, root, dir, source, target string) {
	t.Helper()
	if err := validateLiveADRSuccessor(root, dir, source, target); err != nil {
		t.Errorf("%s: %v", source, err)
	}
}

func validateLiveADRSuccessor(root, dir, source, target string) error {
	targetPath := filepath.Clean(filepath.Join(dir, filepath.FromSlash(target)))
	if targetPath == filepath.Join(dir, source) {
		return fmt.Errorf("superseded status must point to a different ADR")
	}
	if match := adrFilePattern.FindStringSubmatch(filepath.Base(targetPath)); len(match) != 3 {
		return fmt.Errorf("successor %q is not an ADR record", target)
	}
	targetDir := filepath.Dir(targetPath)
	if !governedADRDirectory(root, targetDir) {
		return fmt.Errorf("successor %q is outside the governed ADR inventories", target)
	}
	indexData, err := os.ReadFile(filepath.Join(targetDir, "README.md"))
	if err != nil {
		return fmt.Errorf("read successor inventory: %w", err)
	}
	indexed := false
	for _, entry := range adrIndexPattern.FindAllStringSubmatch(string(indexData), -1) {
		indexedPath := filepath.Clean(filepath.Join(targetDir, filepath.FromSlash(entry[2])))
		if indexedPath == targetPath {
			indexed = true
			break
		}
	}
	if !indexed {
		return fmt.Errorf("successor %q is not indexed by its governed ADR inventory", target)
	}
	content, err := os.ReadFile(targetPath)
	if err != nil {
		return fmt.Errorf("successor %q cannot be read: %w", target, err)
	}
	statuses := adrStatusPattern.FindAllStringSubmatch(string(content), -1)
	if len(statuses) != 1 || (statuses[0][1] != "Accepted" && statuses[0][1] != "Proposed") {
		return fmt.Errorf("successor %q must be one live Accepted or Proposed ADR", target)
	}
	return nil
}

func governedADRDirectory(root, candidate string) bool {
	for _, relative := range []string{"docs/adr", "agm/docs/adr"} {
		if filepath.Clean(candidate) == filepath.Clean(filepath.Join(root, filepath.FromSlash(relative))) {
			return true
		}
	}
	return false
}

func TestADRFilenamePatternAcceptsOnlyCanonicalWidth(t *testing.T) {
	tests := map[string]bool{
		"ADR-001-example.md":  true,
		"0001-example.md":     true,
		"001-example.md":      false,
		"ADR-0001-example.md": false,
	}
	for name, want := range tests {
		if got := adrFilePattern.MatchString(name); got != want {
			t.Errorf("adrFilePattern.MatchString(%q) = %t, want %t", name, got, want)
		}
	}
}

func TestValidateLiveADRSuccessor(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "docs", "adr")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"README.md":            "| [002](ADR-002-live.md) | Live | Accepted |\n| [003](ADR-003-retired.md) | Retired | Deprecated |\n",
		"ADR-002-live.md":      "# ADR-002: Live\n\nStatus: Accepted\n",
		"ADR-003-retired.md":   "# ADR-003: Retired\n\nStatus: Deprecated\n",
		"ADR-004-unindexed.md": "# ADR-004: Unindexed\n\nStatus: Accepted\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name   string
		target string
		want   string
	}{
		{name: "live indexed successor", target: "ADR-002-live.md"},
		{name: "invalid record", target: "README.md", want: "not an ADR record"},
		{name: "not indexed", target: "ADR-004-unindexed.md", want: "not indexed"},
		{name: "not live", target: "ADR-003-retired.md", want: "must be one live"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLiveADRSuccessor(root, dir, "ADR-001-old.md", tt.target)
			if tt.want == "" && err != nil {
				t.Fatal(err)
			}
			if tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func assertRelativeMarkdownLinksResolve(t *testing.T, dir, name, content string) {
	t.Helper()
	for _, match := range markdownLink.FindAllStringSubmatch(content, -1) {
		assertRelativeMarkdownTargetResolves(t, dir, name, markdownLinkDestination(match[1]))
	}
	for _, match := range markdownReferenceDefinition.FindAllStringSubmatch(content, -1) {
		assertRelativeMarkdownTargetResolves(t, dir, name, match[2])
	}
}

func assertRelativeMarkdownTargetResolves(t *testing.T, dir, name, rawTarget string) {
	t.Helper()
	target := strings.SplitN(rawTarget, "#", 2)[0]
	if target == "" || strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
		return
	}
	if _, err := os.Stat(filepath.Clean(filepath.Join(dir, filepath.FromSlash(target)))); err != nil {
		t.Errorf("%s: relative link %q does not resolve: %v", name, rawTarget, err)
	}
}

func markdownLinkDestination(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "<") {
		if end := strings.Index(raw, ">"); end > 0 {
			return raw[1:end]
		}
	}
	if end := strings.IndexAny(raw, " \t\r\n"); end >= 0 {
		return raw[:end]
	}
	return raw
}

func adrSuccessorTarget(statusLine, content string) (string, bool) {
	if match := markdownLink.FindStringSubmatch(statusLine); len(match) == 2 {
		return markdownLinkDestination(match[1]), true
	}
	definitions := make(map[string]string)
	for _, match := range markdownReferenceDefinition.FindAllStringSubmatch(content, -1) {
		definitions[strings.ToLower(strings.TrimSpace(match[1]))] = match[2]
	}
	for _, match := range markdownReferenceLink.FindAllStringSubmatch(statusLine, -1) {
		if target, ok := definitions[strings.ToLower(strings.TrimSpace(match[1]))]; ok {
			return target, true
		}
	}
	return "", false
}

func TestRelativeMarkdownLinkWithTitleResolves(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ADR-002-live.md"), []byte("# Live\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertRelativeMarkdownLinksResolve(t, dir, "ADR-001-old.md", `[decision](ADR-002-live.md "successor")`)
}

func TestReferenceStyleADRSuccessorResolves(t *testing.T) {
	content := "Status: Superseded by [ADR-002][successor]\n\n[successor]: ADR-002-live.md \"live decision\"\n"
	target, ok := adrSuccessorTarget("Status: Superseded by [ADR-002][successor]", content)
	if !ok || target != "ADR-002-live.md" {
		t.Fatalf("adrSuccessorTarget() = %q, %t, want ADR-002-live.md, true", target, ok)
	}
}

func assertSameADRRecords(t *testing.T, records, indexed map[string]adrRecord) {
	t.Helper()
	var problems []string
	for name, record := range records {
		entry, exists := indexed[name]
		if !exists {
			problems = append(problems, "missing index entry for "+name)
			continue
		}
		if entry != record {
			problems = append(problems, name+": index identity/status differs from record")
		}
	}
	for name := range indexed {
		if _, exists := records[name]; !exists {
			problems = append(problems, "index points to non-record "+name)
		}
	}
	sort.Strings(problems)
	for _, problem := range problems {
		t.Error(problem)
	}
}
