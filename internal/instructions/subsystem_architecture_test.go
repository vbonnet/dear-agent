package instructions_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestSubsystemArchitectureLineBudgets(t *testing.T) {
	root := repoRoot(t)
	files := []string{
		"agm/docs/ARCHITECTURE.md",
		"agm/cmd/agm-daemon/ARCHITECTURE.md",
		"agm/cmd/agm-mcp-server/ARCHITECTURE.md",
		"agm/internal/agent/gemini/ARCHITECTURE.md",
		"engram/mcp/ARCHITECTURE.md",
		"internal/sandbox/ARCHITECTURE.md",
	}
	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			content := readFile(t, filepath.Join(root, filepath.FromSlash(file)))
			if lines := strings.Count(strings.TrimSuffix(content, "\n"), "\n") + 1; lines > 300 {
				t.Fatalf("%s has %d lines; living architecture budget is 300", file, lines)
			}
		})
	}
}

func TestAGMHarnessArchitectureMatchesRegistry(t *testing.T) {
	root := repoRoot(t)
	source := readFile(t, filepath.Join(root, "agm/internal/agent/harnesses.go"))
	doc := readFile(t, filepath.Join(root, "agm/docs/ARCHITECTURE.md"))

	active := []string{"claude-code", "codex-cli", "agy", "opencode-cli"}
	deprecated := []string{"gemini-cli"}
	assertStringSliceDeclaration(t, source, "activeHarnesses", active)
	assertStringSliceDeclaration(t, source, "deprecatedHarnesses", deprecated)
	for _, harness := range append(active, deprecated...) {
		if !strings.Contains(doc, "`"+harness+"`") {
			t.Errorf("AGM architecture omits registered harness %q", harness)
		}
	}
	if !strings.Contains(doc, "deprecated: `gemini-cli`") &&
		!strings.Contains(doc, "`gemini-cli` | deprecated") {
		t.Error("AGM architecture does not mark gemini-cli deprecated")
	}
}

func TestAGMMCPArchitectureMatchesRegisteredTools(t *testing.T) {
	root := repoRoot(t)
	source := readFile(t, filepath.Join(root, "agm/cmd/agm-mcp-server/tools.go"))
	doc := readFile(t, filepath.Join(root, "agm/cmd/agm-mcp-server/ARCHITECTURE.md"))
	want := []string{
		"agm_list_sessions",
		"agm_search_sessions",
		"agm_get_session_metadata",
		"agm_archive_session",
		"agm_kill_session",
		"agm_create_session",
		"agm_send_message",
		"agm_list_ops",
		"engram_list_wayfinder_sessions",
		"engram_get_wayfinder_session",
	}
	got := uniqueMatches(source, regexp.MustCompile(`Name:\s+"([^"]+)"`))
	assertSameStrings(t, "AGM MCP source tool inventory", got, want)
	assertDocumentedNames(t, doc, want)
}

func TestEngramMCPArchitectureMatchesRegisteredTools(t *testing.T) {
	root := repoRoot(t)
	source := readFile(t, filepath.Join(root, "engram/mcp/src/index.ts"))
	start := strings.Index(source, "const TOOLS")
	if start < 0 {
		t.Fatal("could not locate Engram MCP TOOLS declaration")
	}
	end := strings.Index(source[start:], "\n];")
	if end < 0 {
		t.Fatal("could not locate Engram MCP TOOLS declaration")
	}
	toolsBlock := source[start : start+end]
	got := uniqueMatches(toolsBlock, regexp.MustCompile(`name:\s+'([^']+)'`))
	want := []string{"engram.retrieve", "engram.plugins.list", "wayfinder.phase.status"}
	assertSameStrings(t, "Engram MCP source tool inventory", got, want)

	doc := readFile(t, filepath.Join(root, "engram/mcp/ARCHITECTURE.md"))
	assertDocumentedNames(t, doc, want)
}

func TestSandboxArchitectureMatchesSelectionOrder(t *testing.T) {
	root := repoRoot(t)
	source := readFile(t, filepath.Join(root, "internal/sandbox/factory.go"))
	doc := readFile(t, filepath.Join(root, "internal/sandbox/ARCHITECTURE.md"))
	for _, marker := range []string{
		"case hasBubblewrap():",
		`info.Recommended = "bubblewrap"`,
		"case info.HasOverlayFS:",
		`info.Recommended = "overlayfs"`,
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("sandbox factory missing selection marker %q", marker)
		}
	}
	if strings.Index(source, "case hasBubblewrap():") > strings.Index(source, "case info.HasOverlayFS:") {
		t.Fatal("sandbox factory no longer prefers bubblewrap before OverlayFS")
	}
	for _, provider := range []string{"bubblewrap", "overlayfs", "gvisor", "apfs", "claudecode-worktree", "mock"} {
		if !strings.Contains(doc, "`"+provider+"`") {
			t.Errorf("sandbox architecture omits provider %q", provider)
		}
	}
}

func TestRetiredSubsystemDesignsStayRetired(t *testing.T) {
	root := repoRoot(t)
	cases := map[string][]string{
		"agm/docs/ARCHITECTURE.md": {
			"Astrocyte Python",
			"OpenAI API (GPT-4",
			"Gemini API adapter",
		},
		"agm/cmd/agm-daemon/ARCHITECTURE.md": {
			"localhost:8765",
			"every 2 seconds",
			"Monitoring Loop (every 2s)",
		},
		"agm/cmd/agm-mcp-server/ARCHITECTURE.md": {
			"5-second TTL",
			"three focused MCP tools",
			"manifest.json files on every query",
		},
		"engram/mcp/ARCHITECTURE.md": {
			"engram_mcp_server.py",
			"sentence_transformers",
			"tools/retrieve.py",
		},
		"internal/sandbox/ARCHITECTURE.md": {
			"Linux 5.11+: Native rootless OverlayFS (optimal)",
			"Other: Mock provider",
		},
	}
	for file, forbidden := range cases {
		t.Run(file, func(t *testing.T) {
			content := readFile(t, filepath.Join(root, filepath.FromSlash(file)))
			for _, phrase := range forbidden {
				if strings.Contains(content, phrase) {
					t.Errorf("%s resurrects retired design phrase %q", file, phrase)
				}
			}
		})
	}

	retiredFiles := []string{
		"agm/cmd/agm-daemon/adr/001-http-api-choice.md",
		"agm/cmd/agm-mcp-server/adr/002-caching-strategy.md",
		"agm/internal/agent/gemini/ADR-002-google-genai-sdk-selection.md",
		"engram/mcp/requirements.txt",
		"engram/mcp/test_basic_tools.sh",
	}
	for _, file := range retiredFiles {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(file))); err == nil {
			t.Errorf("retired artifact still exists: %s", file)
		} else if !os.IsNotExist(err) {
			t.Errorf("inspect retired artifact %s: %v", file, err)
		}
	}
}

func assertStringSliceDeclaration(t *testing.T, source, variable string, want []string) {
	t.Helper()
	pattern := regexp.MustCompile(`var\s+` + regexp.QuoteMeta(variable) + `\s*=\s*\[\]string\{([^}]*)\}`)
	match := pattern.FindStringSubmatch(source)
	if len(match) != 2 {
		t.Fatalf("could not parse %s declaration", variable)
	}
	got := uniqueMatches(match[1], regexp.MustCompile(`"([^"]+)"`))
	assertSameStrings(t, variable, got, want)
}

func uniqueMatches(content string, pattern *regexp.Regexp) []string {
	seen := map[string]struct{}{}
	var result []string
	for _, match := range pattern.FindAllStringSubmatch(content, -1) {
		if _, ok := seen[match[1]]; ok {
			continue
		}
		seen[match[1]] = struct{}{}
		result = append(result, match[1])
	}
	return result
}

func assertSameStrings(t *testing.T, label string, got, want []string) {
	t.Helper()
	got = append([]string(nil), got...)
	want = append([]string(nil), want...)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
}

func assertDocumentedNames(t *testing.T, content string, names []string) {
	t.Helper()
	for _, name := range names {
		if !strings.Contains(content, "`"+name+"`") {
			t.Errorf("architecture omits registered name %q", name)
		}
	}
}
