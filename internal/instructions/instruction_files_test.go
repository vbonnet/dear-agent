package instructions_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRootInstructionEntrypointsImportAgents(t *testing.T) {
	root := repoRoot(t)
	for _, file := range []string{"CLAUDE.md", "GEMINI.md", "CODEX.md", "AGY.md", "OPENCODE.md"} {
		t.Run(file, func(t *testing.T) {
			content := readFile(t, filepath.Join(root, file))
			assertFirstNonEmptyLine(t, file, content, "@import AGENTS.md")
			assertNoSharedPolicyDuplication(t, file, content)
		})
	}
}

func TestScopedInstructionEntrypointsImportRootAgents(t *testing.T) {
	root := repoRoot(t)
	scoped := map[string]string{
		".claude/CLAUDE.md":  "@import ../AGENTS.md",
		".agents/AGENTS.md":  "@import ../AGENTS.md",
		".deepsec/AGENTS.md": "@import ../AGENTS.md",
	}
	for file, want := range scoped {
		t.Run(file, func(t *testing.T) {
			content := readFile(t, filepath.Join(root, filepath.FromSlash(file)))
			assertFirstNonEmptyLine(t, file, content, want)
		})
	}
}

func TestCanonicalAgentsIsNotImportShim(t *testing.T) {
	root := repoRoot(t)
	content := readFile(t, filepath.Join(root, "AGENTS.md"))
	assertFirstNonEmptyLineNot(t, "AGENTS.md", content, "@import")
	for _, required := range []string{"Core Engineering Principles", "Living Documentation Policy"} {
		if !strings.Contains(content, required) {
			t.Fatalf("AGENTS.md missing required shared section %q", required)
		}
	}
}

func assertNoSharedPolicyDuplication(t *testing.T, file, content string) {
	t.Helper()
	lines := nonEmptyLines(content)
	if len(lines) > 1 {
		forbidden := []string{
			"Core Engineering Principles",
			"Living Documentation Policy",
			"Agent Delegation Enforcement",
			"Source Repository Policy",
		}
		for _, marker := range forbidden {
			if strings.Contains(content, marker) {
				t.Fatalf("%s duplicates shared policy section %q instead of importing AGENTS.md", file, marker)
			}
		}
	}
}

func assertFirstNonEmptyLine(t *testing.T, file, content, want string) {
	t.Helper()
	lines := nonEmptyLines(content)
	if len(lines) == 0 {
		t.Fatalf("%s is empty", file)
	}
	if lines[0] != want {
		t.Fatalf("%s first non-empty line = %q, want %q", file, lines[0], want)
	}
}

func assertFirstNonEmptyLineNot(t *testing.T, file, content, forbiddenPrefix string) {
	t.Helper()
	lines := nonEmptyLines(content)
	if len(lines) == 0 {
		t.Fatalf("%s is empty", file)
	}
	if strings.HasPrefix(lines[0], forbiddenPrefix) {
		t.Fatalf("%s should be canonical content, not an import shim: %q", file, lines[0])
	}
}

func nonEmptyLines(content string) []string {
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
