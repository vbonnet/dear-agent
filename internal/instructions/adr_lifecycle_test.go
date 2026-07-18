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
	adrFilePattern   = regexp.MustCompile(`^(?:ADR-)?([0-9]{3,4})-[a-z0-9-]+\.md$`)
	adrTitlePattern  = regexp.MustCompile(`(?m)^# ADR-([0-9]{3,4}): .+$`)
	adrStatusPattern = regexp.MustCompile(`(?m)^Status: (Accepted|Proposed|Deprecated|Superseded)(?: .*)?$`)
	adrIndexPattern  = regexp.MustCompile(`(?m)^\| \[([0-9]{3,4})\]\(([^)]+\.md)\) \| [^|]+ \| (Accepted|Proposed|Deprecated|Superseded) \|$`)
	markdownLink     = regexp.MustCompile(`\[[^]]+\]\(([^)]+)\)`)
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
				match := adrFilePattern.FindStringSubmatch(entry.Name())
				if len(match) != 2 {
					continue
				}
				content := readFile(t, filepath.Join(dir, entry.Name()))
				titles := adrTitlePattern.FindAllStringSubmatch(content, -1)
				if len(titles) != 1 {
					t.Errorf("%s: want one ADR heading, got %d", entry.Name(), len(titles))
					continue
				}
				if titles[0][1] != match[1] {
					t.Errorf("%s: heading ID %s does not match filename ID %s", entry.Name(), titles[0][1], match[1])
				}
				if previous, exists := ids[match[1]]; exists {
					t.Errorf("duplicate ADR-%s: %s and %s", match[1], previous, entry.Name())
				} else {
					ids[match[1]] = entry.Name()
				}

				statuses := adrStatusPattern.FindAllStringSubmatch(content, -1)
				if len(statuses) != 1 {
					t.Errorf("%s: want one normalized Status line, got %d", entry.Name(), len(statuses))
					continue
				}
				if lines := strings.Count(content, "\n") + 1; lines > 250 {
					t.Errorf("%s: %d lines exceeds the ADR review budget of 250", entry.Name(), lines)
				}
				assertRelativeMarkdownLinksResolve(t, dir, entry.Name(), content)
				records[entry.Name()] = adrRecord{id: match[1], status: statuses[0][1]}
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

func assertRelativeMarkdownLinksResolve(t *testing.T, dir, name, content string) {
	t.Helper()
	for _, match := range markdownLink.FindAllStringSubmatch(content, -1) {
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
