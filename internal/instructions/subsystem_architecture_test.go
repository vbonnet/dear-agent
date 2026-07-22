package instructions_test

import (
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
			if lines := strings.Count(content, "\n") + 1; lines > 300 {
				t.Fatalf("%s has %d lines; living architecture budget is 300", file, lines)
			}
		})
	}
}

func TestAGMHarnessArchitectureMatchesRegistry(t *testing.T) {
	root := repoRoot(t)
	source := readFile(t, filepath.Join(root, "agm/internal/agent/harnesses.go"))
	doc := readFile(t, filepath.Join(root, "agm/docs/ARCHITECTURE.md"))

	active := []string{"claude-code", "codex-cli", "agy", "opencode-cli", "pi-cli"}
	deprecated := []string{"gemini-cli"}
	assertStringSliceDeclaration(t, source, "activeHarnesses", active)
	assertStringSliceDeclaration(t, source, "deprecatedHarnesses", deprecated)
	for _, harness := range active {
		if !strings.Contains(doc, "| `"+harness+"` | active |") {
			t.Errorf("AGM architecture does not mark active harness %q active", harness)
		}
	}
	for _, harness := range deprecated {
		if !strings.Contains(doc, "| `"+harness+"` | deprecated") {
			t.Errorf("AGM architecture does not mark deprecated harness %q deprecated", harness)
		}
	}
}

func TestAGMDaemonArchitectureMatchesDeliveryAuthority(t *testing.T) {
	root := repoRoot(t)
	source := readFile(t, filepath.Join(root, "agm/internal/daemon/daemon.go"))
	doc := readFile(t, filepath.Join(root, "agm/cmd/agm-daemon/ARCHITECTURE.md"))
	for _, marker := range []string{
		"slo.Daemon.PollInterval.Duration",
		"session.CheckSessionDelivery(recipientManifest.Tmux.SessionName)",
	} {
		if !strings.Contains(source, marker) {
			t.Fatalf("daemon source missing delivery marker %q", marker)
		}
	}
	for _, fact := range []string{"`session.CheckSessionDelivery` is the sole readiness authority", "`~/.config/agm/message_queue.db`"} {
		if !strings.Contains(doc, fact) {
			t.Errorf("daemon architecture omits runtime fact %q", fact)
		}
	}
}

func TestAGMMCPArchitectureMatchesRegisteredTools(t *testing.T) {
	root := repoRoot(t)
	mainSource := readFile(t, filepath.Join(root, "agm/cmd/agm-mcp-server/main.go"))
	toolSource := readFile(t, filepath.Join(root, "agm/cmd/agm-mcp-server/tools.go"))
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
	wantRegistrations := []string{
		"ListSessions",
		"SearchSessions",
		"GetSessionMetadata",
		"ArchiveSession",
		"KillSession",
		"CreateSession",
		"SendMessage",
		"ListOps",
		"ListWayfinderSessions",
		"GetWayfinderSession",
	}
	registrations := uniqueMatches(mainSource, regexp.MustCompile(`add([A-Za-z0-9]+)Tool\(server,\s*cfg\)`))
	assertSameStrings(t, "AGM MCP registered helpers", registrations, wantRegistrations)
	declarations := uniqueMatches(toolSource, regexp.MustCompile(`func add([A-Za-z0-9]+)Tool\(`))
	declared := make(map[string]struct{}, len(declarations))
	for _, declaration := range declarations {
		declared[declaration] = struct{}{}
	}
	for _, registration := range registrations {
		if _, ok := declared[registration]; !ok {
			t.Errorf("registered helper add%sTool has no declaration", registration)
		}
	}
	got := uniqueMatches(toolSource, regexp.MustCompile(`Name:\s*"([^"]+)"`))
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
	for _, provider := range []string{"bubblewrap", "overlayfs", "gvisor", "apfs", "mock"} {
		if !strings.Contains(doc, "`"+provider+"`") {
			t.Errorf("sandbox architecture omits provider %q", provider)
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
