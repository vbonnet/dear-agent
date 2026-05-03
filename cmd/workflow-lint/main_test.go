package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/workflow"
)

func writeWorkflow(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "wf.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLintFileCleanWorkflow(t *testing.T) {
	path := writeWorkflow(t, `
name: clean
version: "1"
nodes:
  - id: a
    kind: ai
    ai:
      role: research
      prompt: hi
`)
	if findings := lintFile(path, lintConfig{}); len(findings) != 0 {
		t.Errorf("expected no findings, got %v", findings)
	}
}

func TestLintFileFlagsHardcodedModel(t *testing.T) {
	path := writeWorkflow(t, `
name: legacy
version: "1"
nodes:
  - id: a
    kind: ai
    ai:
      model: claude-opus-3
      prompt: hi
`)
	cfg := lintConfig{
		checkDeprecated:    true,
		deprecatedModelSet: toSet([]string{"claude-opus-3"}),
	}
	findings := lintFile(path, cfg)
	if len(findings) == 0 {
		t.Fatalf("expected findings, got none")
	}
	joined := strings.Join(findings, "\n")
	if !strings.Contains(joined, "DEPRECATED") {
		t.Errorf("expected DEPRECATED finding, got: %s", joined)
	}
	if !strings.Contains(joined, "claude-opus-3") {
		t.Errorf("expected model id in finding, got: %s", joined)
	}
}

func TestLintFileWarnsOnRoleless(t *testing.T) {
	path := writeWorkflow(t, `
name: roleless
version: "1"
nodes:
  - id: a
    kind: ai
    ai:
      model: some-model
      prompt: hi
`)
	findings := lintFile(path, lintConfig{})
	if len(findings) == 0 {
		t.Fatal("expected WARN finding for roleless node")
	}
	if !strings.Contains(findings[0], "WARN") {
		t.Errorf("expected WARN, got: %s", findings[0])
	}
}

func TestLintFileFlagsModelOverride(t *testing.T) {
	path := writeWorkflow(t, `
name: override
version: "1"
nodes:
  - id: a
    kind: ai
    ai:
      role: research
      model_override: gpt-4
      prompt: hi
`)
	cfg := lintConfig{
		checkDeprecated:    true,
		deprecatedModelSet: toSet([]string{"gpt-4"}),
	}
	findings := lintFile(path, cfg)
	if len(findings) == 0 {
		t.Fatalf("expected findings, got none")
	}
	joined := strings.Join(findings, "\n")
	if !strings.Contains(joined, "model_override") {
		t.Errorf("expected model_override finding: %s", joined)
	}
}

func TestLintFileLoopChildLinted(t *testing.T) {
	path := writeWorkflow(t, `
name: looped
version: "1"
nodes:
  - id: lp
    kind: loop
    loop:
      until: Outputs.inner == done
      nodes:
        - id: inner
          kind: ai
          ai:
            model: gemini-1.5-pro
            prompt: hi
`)
	cfg := lintConfig{
		checkDeprecated:    true,
		deprecatedModelSet: toSet([]string{"gemini-1.5-pro"}),
	}
	findings := lintFile(path, cfg)
	if len(findings) == 0 {
		t.Fatalf("expected findings for loop child, got none")
	}
	joined := strings.Join(findings, "\n")
	if !strings.Contains(joined, "lp/inner") {
		t.Errorf("expected lp/inner reference, got: %s", joined)
	}
}

func TestLintFileBadYAMLBubblesUp(t *testing.T) {
	path := writeWorkflow(t, "name: [broken")
	findings := lintFile(path, lintConfig{})
	if len(findings) == 0 || !strings.Contains(findings[0], "ERROR") {
		t.Errorf("expected ERROR finding, got %v", findings)
	}
}

func TestLintFileTouchesEveryAINode(t *testing.T) {
	// Smoke test: sanity-check that all AI nodes are visited so a
	// future test author sees the full surface walked.
	path := writeWorkflow(t, `
name: many
version: "1"
nodes:
  - id: a
    kind: ai
    ai: {prompt: x}
  - id: b
    kind: ai
    ai: {prompt: y}
`)
	w, err := workflow.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	walkAINodes(w.Nodes, func(_ string, n *workflow.Node) {
		if n.Kind == workflow.KindAI {
			seen++
		}
	})
	if seen != 2 {
		t.Errorf("expected 2 AI nodes visited, got %d", seen)
	}
}

func TestSplitCSV(t *testing.T) {
	got := splitCSV("a, b ,, c ")
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
