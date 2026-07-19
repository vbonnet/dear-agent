package instructions_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type localADRScope struct {
	dir       string
	indexName string
}

func TestCodeLocalADRsHaveCompleteConciseLifecycle(t *testing.T) {
	root := repoRoot(t)
	governed := map[string]bool{}
	scopes := []localADRScope{
		{dir: "agm/cmd/agm"},
		{dir: "agm/cmd/agm-daemon/adr", indexName: "README.md"},
		{dir: "agm/cmd/agm-mcp-server/adr", indexName: "README.md"},
		{dir: "agm/internal/dolt/adr", indexName: "README.md"},
		{dir: "agm/internal/evaluation/ADR", indexName: "README.md"},
		{dir: "agm/internal/tmux"},
		{dir: "engram/cmd/engram", indexName: "ADR-INDEX.md"},
		{dir: "engram/retrieval/docs/adrs", indexName: "README.md"},
		{dir: "pkg/engram/docs/adrs", indexName: "README.md"},
		{dir: "pkg/llm/docs"},
		{dir: "tools/dod-enforcer/adr", indexName: "README.md"},
	}
	for _, scope := range scopes {
		t.Run(scope.dir, func(t *testing.T) {
			validateLocalADRScope(t, root, scope, governed)
		})
	}

	aggregates := []string{
		"engram/ecphory/ADR.md",
		"engram/ecphory/ranking/ADR.md",
		"pkg/hash/ADR.md",
		"pkg/monitoring/ADR.md",
		"pkg/telemetry/ADR.md",
		"tools/devlog/ADR.md",
	}
	for _, name := range aggregates {
		t.Run(name, func(t *testing.T) {
			content := readFile(t, filepath.Join(root, filepath.FromSlash(name)))
			statuses := adrStatusPattern.FindAllStringSubmatch(content, -1)
			if len(statuses) != 1 {
				t.Errorf("want one normalized Status line, got %d", len(statuses))
			}
			assertRelativeMarkdownLinksResolve(
				t,
				filepath.Dir(filepath.Join(root, filepath.FromSlash(name))),
				filepath.Base(name),
				content,
			)
			governed[name] = true
		})
	}

	assertEveryCodeLocalADRIsGoverned(t, root, governed)
}

func validateLocalADRScope(t *testing.T, root string, scope localADRScope, governed map[string]bool) {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(scope.dir))
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	records := map[string]adrRecord{}
	ids := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := adrFilePattern.FindStringSubmatch(entry.Name())
		if len(match) != 2 {
			if entry.Name() != scope.indexName && isADRLikeFilename(entry.Name()) {
				t.Errorf("malformed ADR filename: %s", entry.Name())
			}
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
		assertRelativeMarkdownLinksResolve(t, dir, entry.Name(), content)
		records[entry.Name()] = adrRecord{id: match[1], status: statuses[0][1]}
		governed[filepath.ToSlash(filepath.Join(scope.dir, entry.Name()))] = true
	}

	if scope.indexName == "" {
		return
	}
	index := readFile(t, filepath.Join(dir, scope.indexName))
	indexed := map[string]adrRecord{}
	for _, match := range adrIndexPattern.FindAllStringSubmatch(index, -1) {
		if len(match) < 4 {
			t.Errorf("%s: invalid index entry format", scope.indexName)
			continue
		}
		if _, exists := indexed[match[2]]; exists {
			t.Errorf("%s indexes %s more than once", scope.indexName, match[2])
		}
		indexed[match[2]] = adrRecord{id: match[1], status: match[3]}
	}
	assertSameADRRecords(t, records, indexed)
}

func assertEveryCodeLocalADRIsGoverned(t *testing.T, root string, governed map[string]bool) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "testdata", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if strings.HasPrefix(relative, "docs/adr/") ||
			strings.HasPrefix(relative, "agm/docs/adr/") {
			return nil
		}
		base := filepath.Base(relative)
		if base != "ADR.md" && !adrFilePattern.MatchString(base) {
			if base != "ADR-INDEX.md" && isADRLikeFilename(base) {
				t.Errorf("malformed code-local ADR filename: %s", relative)
			}
			return nil
		}
		if !governed[relative] {
			t.Errorf("ungoverned code-local ADR: %s", relative)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func isADRLikeFilename(name string) bool {
	return strings.HasPrefix(strings.ToUpper(name), "ADR-") && strings.HasSuffix(strings.ToLower(name), ".md")
}
