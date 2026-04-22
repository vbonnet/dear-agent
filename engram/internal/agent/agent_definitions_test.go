//go:build integration

package agent

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// agentFrontmatter holds parsed YAML frontmatter from agent .md files
type agentFrontmatter struct {
	Name      string
	Model     string
	Isolation string
	Tools     []string
}

// parseAgentFrontmatter parses the YAML frontmatter between --- delimiters
// in agent definition .md files.
func parseAgentFrontmatter(path string) (*agentFrontmatter, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fm := &agentFrontmatter{}
	scanner := bufio.NewScanner(f)
	inFrontmatter := false
	inTools := false

	for scanner.Scan() {
		line := scanner.Text()

		if line == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			break
		}

		if !inFrontmatter {
			continue
		}

		trimmed := strings.TrimSpace(line)

		if inTools {
			if strings.HasPrefix(trimmed, "- ") {
				tool := strings.TrimPrefix(trimmed, "- ")
				fm.Tools = append(fm.Tools, strings.TrimSpace(tool))
				continue
			}
			inTools = false
		}

		if strings.HasPrefix(trimmed, "name:") {
			fm.Name = strings.TrimSpace(strings.TrimPrefix(trimmed, "name:"))
		} else if strings.HasPrefix(trimmed, "model:") {
			fm.Model = strings.TrimSpace(strings.TrimPrefix(trimmed, "model:"))
		} else if strings.HasPrefix(trimmed, "isolation:") {
			fm.Isolation = strings.TrimSpace(strings.TrimPrefix(trimmed, "isolation:"))
		} else if strings.HasPrefix(trimmed, "tools:") {
			inTools = true
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return fm, nil
}

// findAgentsDir finds the .claude/agents directory relative to the repo root.
func findAgentsDir(t *testing.T) string {
	t.Helper()

	// Walk up from core/internal/agent to repo root
	candidates := []string{
		filepath.Join("..", "..", "..", "..", ".claude", "agents"),
		filepath.Join(os.Getenv("HOME"), "src", "ws", "oss", "repos", "engram", ".claude", "agents"),
	}

	// Also check worktree paths
	cwd, err := os.Getwd()
	if err == nil {
		// From cwd, try to find repo root by looking for .claude/agents
		dir := cwd
		for i := 0; i < 10; i++ {
			candidate := filepath.Join(dir, ".claude", "agents")
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				return candidate
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}

	t.Skip("Cannot find .claude/agents directory")
	return ""
}

// expectedAgents defines the 6 expected agent definitions and their properties
var expectedAgents = map[string]struct {
	model        string
	hasWrite     bool
	hasEdit      bool
	hasBash      bool
	hasGrep      bool
	hasGlob      bool
	hasRead      bool
	hasWebSearch bool
	hasWebFetch  bool
}{
	"researcher": {
		model:   "sonnet",
		hasRead: true, hasGrep: true, hasGlob: true,
		hasWebSearch: true, hasWebFetch: true,
	},
	"designer": {
		model:   "opus",
		hasRead: true, hasGrep: true, hasGlob: true,
		hasWrite: true, hasEdit: true,
	},
	"planner": {
		model:   "sonnet",
		hasRead: true, hasGrep: true, hasGlob: true,
		hasWrite: true, hasEdit: true,
	},
	"implementer": {
		model:   "sonnet",
		hasRead: true, hasWrite: true, hasEdit: true,
		hasBash: true, hasGrep: true, hasGlob: true,
	},
	"test-writer": {
		model:   "sonnet",
		hasRead: true, hasWrite: true, hasEdit: true,
		hasBash: true, hasGrep: true, hasGlob: true,
	},
	"reviewer": {
		model:   "sonnet",
		hasRead: true, hasGrep: true, hasGlob: true,
		hasBash: true,
	},
}

// TestAgentDefinitions_AllSixExist verifies all 6 expected agent .md files exist.
func TestAgentDefinitions_AllSixExist(t *testing.T) {
	agentsDir := findAgentsDir(t)

	for name := range expectedAgents {
		path := filepath.Join(agentsDir, name+".md")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("Agent definition missing: %s.md (path: %s)", name, path)
		}
	}
}

// TestAgentDefinitions_IsolationWorktree verifies all agents have isolation: worktree.
func TestAgentDefinitions_IsolationWorktree(t *testing.T) {
	agentsDir := findAgentsDir(t)

	for name := range expectedAgents {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(agentsDir, name+".md")
			fm, err := parseAgentFrontmatter(path)
			if err != nil {
				t.Fatalf("Failed to parse frontmatter for %s: %v", name, err)
			}

			if fm.Isolation != "worktree" {
				t.Errorf("Agent %s isolation = %q, want %q", name, fm.Isolation, "worktree")
			}
		})
	}
}

// TestAgentDefinitions_ModelAssignment verifies model assignments.
// designer=opus, all others=sonnet.
func TestAgentDefinitions_ModelAssignment(t *testing.T) {
	agentsDir := findAgentsDir(t)

	for name, expected := range expectedAgents {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(agentsDir, name+".md")
			fm, err := parseAgentFrontmatter(path)
			if err != nil {
				t.Fatalf("Failed to parse frontmatter for %s: %v", name, err)
			}

			if fm.Model != expected.model {
				t.Errorf("Agent %s model = %q, want %q", name, fm.Model, expected.model)
			}
		})
	}
}

// TestAgentDefinitions_ToolRestrictions verifies each agent has the expected tool set.
func TestAgentDefinitions_ToolRestrictions(t *testing.T) {
	agentsDir := findAgentsDir(t)

	hasTool := func(tools []string, tool string) bool {
		for _, tt := range tools {
			if tt == tool {
				return true
			}
		}
		return false
	}

	for name, expected := range expectedAgents {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(agentsDir, name+".md")
			fm, err := parseAgentFrontmatter(path)
			if err != nil {
				t.Fatalf("Failed to parse frontmatter for %s: %v", name, err)
			}

			checks := []struct {
				tool string
				want bool
			}{
				{"Read", expected.hasRead},
				{"Write", expected.hasWrite},
				{"Edit", expected.hasEdit},
				{"Bash", expected.hasBash},
				{"Grep", expected.hasGrep},
				{"Glob", expected.hasGlob},
				{"WebSearch", expected.hasWebSearch},
				{"WebFetch", expected.hasWebFetch},
			}

			for _, check := range checks {
				got := hasTool(fm.Tools, check.tool)
				if got != check.want {
					if check.want {
						t.Errorf("Agent %s missing expected tool %s", name, check.tool)
					} else {
						t.Errorf("Agent %s has unexpected tool %s", name, check.tool)
					}
				}
			}
		})
	}
}

// TestAgentDefinitions_ResearcherReadOnly verifies researcher has no write tools.
func TestAgentDefinitions_ResearcherReadOnly(t *testing.T) {
	agentsDir := findAgentsDir(t)

	path := filepath.Join(agentsDir, "researcher.md")
	fm, err := parseAgentFrontmatter(path)
	if err != nil {
		t.Fatalf("Failed to parse frontmatter: %v", err)
	}

	writableTools := []string{"Write", "Edit"}
	for _, tool := range writableTools {
		for _, agentTool := range fm.Tools {
			if agentTool == tool {
				t.Errorf("Researcher agent has writable tool %s, should be read-only", tool)
			}
		}
	}
}

// TestAgentDefinitions_ReviewerReadOnly verifies reviewer has no Write/Edit tools
// but does have Bash (for running tests).
func TestAgentDefinitions_ReviewerReadOnly(t *testing.T) {
	agentsDir := findAgentsDir(t)

	path := filepath.Join(agentsDir, "reviewer.md")
	fm, err := parseAgentFrontmatter(path)
	if err != nil {
		t.Fatalf("Failed to parse frontmatter: %v", err)
	}

	writableTools := []string{"Write", "Edit"}
	for _, tool := range writableTools {
		for _, agentTool := range fm.Tools {
			if agentTool == tool {
				t.Errorf("Reviewer agent has writable tool %s, should be read-only for code", tool)
			}
		}
	}

	hasBash := false
	for _, agentTool := range fm.Tools {
		if agentTool == "Bash" {
			hasBash = true
			break
		}
	}
	if !hasBash {
		t.Error("Reviewer agent missing Bash tool (needed for running tests)")
	}
}

// TestAgentDefinitions_NameMatchesFilename verifies each agent's name field
// matches its filename.
func TestAgentDefinitions_NameMatchesFilename(t *testing.T) {
	agentsDir := findAgentsDir(t)

	for name := range expectedAgents {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(agentsDir, name+".md")
			fm, err := parseAgentFrontmatter(path)
			if err != nil {
				t.Fatalf("Failed to parse frontmatter for %s: %v", name, err)
			}

			if fm.Name != name {
				t.Errorf("Agent file %s.md has name=%q in frontmatter, want %q",
					name, fm.Name, name)
			}
		})
	}
}

// TestAgentDefinitions_NoExtraAgents verifies no unexpected agent definitions exist.
func TestAgentDefinitions_NoExtraAgents(t *testing.T) {
	agentsDir := findAgentsDir(t)

	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		t.Fatalf("Failed to read agents directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		agentName := strings.TrimSuffix(name, ".md")
		if _, ok := expectedAgents[agentName]; !ok {
			t.Errorf("Unexpected agent definition: %s (not in expected set)", name)
		}
	}
}

// TestAgentDefinitions_DesignerIsOnlyOpus verifies designer is the only agent using opus.
func TestAgentDefinitions_DesignerIsOnlyOpus(t *testing.T) {
	agentsDir := findAgentsDir(t)

	for name := range expectedAgents {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(agentsDir, name+".md")
			fm, err := parseAgentFrontmatter(path)
			if err != nil {
				t.Fatalf("Failed to parse frontmatter for %s: %v", name, err)
			}

			if name == "designer" {
				if fm.Model != "opus" {
					t.Errorf("Designer should use opus model, got %q", fm.Model)
				}
			} else {
				if fm.Model == "opus" {
					t.Errorf("Agent %s uses opus model, only designer should use opus", name)
				}
			}
		})
	}
}

// TestParseAgentFrontmatter_EdgeCases tests the frontmatter parser with edge cases.
func TestParseAgentFrontmatter_EdgeCases(t *testing.T) {
	t.Run("nonexistent file", func(t *testing.T) {
		_, err := parseAgentFrontmatter("/tmp/nonexistent-agent-file.md")
		if err == nil {
			t.Error("Expected error for nonexistent file")
		}
	})
}
