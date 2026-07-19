package instructions_test

import (
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
	successorPattern            = regexp.MustCompile(`^Status: Superseded .*\[[^]]+\]\(([^)#]+\.md)(?:#[^)]*)?\).*$`)
	adrIndexPattern             = regexp.MustCompile(`(?m)^\| \[([0-9]{3,4})\]\(([^)]+\.md)\) \| [^|]+ \| (Accepted|Proposed|Deprecated|Superseded) \|$`)
	markdownLink                = regexp.MustCompile(`\[[^]]+\]\(([^)]+)\)`)
	markdownReferenceDefinition = regexp.MustCompile(`(?m)^[ \t]{0,3}\[[^]]+\]:[ \t]+<?([^> \t]+)>?(?:[ \t]+.*)?$`)
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
					successor := successorPattern.FindStringSubmatch(statuses[0][0])
					if len(successor) != 2 {
						t.Errorf("%s: superseded status must link to its live successor", entry.Name())
					} else {
						assertLiveADRSuccessor(t, root, dir, entry.Name(), successor[1])
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
	targetPath := filepath.Clean(filepath.Join(dir, filepath.FromSlash(target)))
	if targetPath == filepath.Join(dir, source) {
		t.Errorf("%s: superseded status must point to a different ADR", source)
		return
	}
	if match := adrFilePattern.FindStringSubmatch(filepath.Base(targetPath)); len(match) != 3 {
		t.Errorf("%s: successor %q is not an ADR record", source, target)
		return
	}
	targetDir := filepath.Dir(targetPath)
	if !governedADRDirectory(root, targetDir) {
		t.Errorf("%s: successor %q is outside the governed ADR inventories", source, target)
		return
	}
	index := readFile(t, filepath.Join(targetDir, "README.md"))
	indexed := false
	for _, entry := range adrIndexPattern.FindAllStringSubmatch(index, -1) {
		indexedPath := filepath.Clean(filepath.Join(targetDir, filepath.FromSlash(entry[2])))
		if indexedPath == targetPath {
			indexed = true
			break
		}
	}
	if !indexed {
		t.Errorf("%s: successor %q is not indexed by its governed ADR inventory", source, target)
		return
	}
	content, err := os.ReadFile(targetPath)
	if err != nil {
		t.Errorf("%s: successor %q cannot be read: %v", source, target, err)
		return
	}
	statuses := adrStatusPattern.FindAllStringSubmatch(string(content), -1)
	if len(statuses) != 1 || (statuses[0][1] != "Accepted" && statuses[0][1] != "Proposed") {
		t.Errorf("%s: successor %q must be one live Accepted or Proposed ADR", source, target)
	}
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

func assertRelativeMarkdownLinksResolve(t *testing.T, dir, name, content string) {
	t.Helper()
	matches := markdownLink.FindAllStringSubmatch(content, -1)
	matches = append(matches, markdownReferenceDefinition.FindAllStringSubmatch(content, -1)...)
	for _, match := range matches {
		target := strings.SplitN(match[1], "#", 2)[0]
		if target == "" || strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
			continue
		}
		if _, err := os.Stat(filepath.Clean(filepath.Join(dir, filepath.FromSlash(target)))); err != nil {
			t.Errorf("%s: relative link %q does not resolve: %v", name, match[1], err)
		}
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
