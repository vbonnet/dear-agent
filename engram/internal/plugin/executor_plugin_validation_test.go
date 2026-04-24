package plugin

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// findPluginsDir locates the plugins/ directory by walking up from the test location.
func findPluginsDir(t *testing.T) string {
	t.Helper()

	// Walk up from core/internal/plugin to repo root, then into plugins/
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get cwd: %v", err)
	}

	dir := cwd
	for i := 0; i < 10; i++ {
		candidate := filepath.Join(dir, "plugins")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Fallback to absolute path
	absPath := filepath.Join(os.Getenv("HOME"), "src", "ws", "oss", "repos", "engram", "plugins")
	if info, err := os.Stat(absPath); err == nil && info.IsDir() {
		return absPath
	}

	t.Skip("Cannot find plugins/ directory")
	return ""
}

// TestExecutorPlugin_DirectoryStructure verifies the executor plugin has
// the expected directory structure: SKILL.md, commands/spawn.md, commands/spawn-team.md.
func TestExecutorPlugin_DirectoryStructure(t *testing.T) {
	pluginsDir := findPluginsDir(t)
	executorDir := filepath.Join(pluginsDir, "executor")

	expectedFiles := []string{
		"SKILL.md",
		filepath.Join("commands", "spawn.md"),
		filepath.Join("commands", "spawn-team.md"),
	}

	for _, file := range expectedFiles {
		path := filepath.Join(executorDir, file)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("Executor plugin missing expected file: %s (path: %s)", file, path)
		}
	}
}

// TestExecutorPlugin_SKILLmd_ListsCommands verifies SKILL.md lists both
// spawn and spawn-team commands.
func TestExecutorPlugin_SKILLmd_ListsCommands(t *testing.T) {
	pluginsDir := findPluginsDir(t)
	skillPath := filepath.Join(pluginsDir, "executor", "SKILL.md")

	content, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("Failed to read SKILL.md: %v", err)
	}

	text := string(content)

	expectedCommands := []string{"spawn", "spawn-team"}
	for _, cmd := range expectedCommands {
		if !strings.Contains(text, cmd) {
			t.Errorf("SKILL.md does not mention command %q", cmd)
		}
	}
}

// TestExecutorPlugin_SKILLmd_Frontmatter verifies SKILL.md has correct
// frontmatter with name and commands list.
func TestExecutorPlugin_SKILLmd_Frontmatter(t *testing.T) {
	pluginsDir := findPluginsDir(t)
	skillPath := filepath.Join(pluginsDir, "executor", "SKILL.md")

	f, err := os.Open(skillPath)
	if err != nil {
		t.Fatalf("Failed to open SKILL.md: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inFrontmatter := false
	foundName := false
	foundCommands := false
	commandsList := []string{}
	inCommandsList := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

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

		if inCommandsList {
			if strings.HasPrefix(trimmed, "- ") {
				cmd := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
				commandsList = append(commandsList, cmd)
				continue
			}
			inCommandsList = false
		}

		if strings.HasPrefix(trimmed, "name:") {
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, "name:"))
			if val == "executor" {
				foundName = true
			}
		} else if strings.HasPrefix(trimmed, "commands:") {
			foundCommands = true
			inCommandsList = true
		}
	}

	if !foundName {
		t.Error("SKILL.md frontmatter missing name: executor")
	}
	if !foundCommands {
		t.Error("SKILL.md frontmatter missing commands: section")
	}

	// Verify both commands listed
	hasSpawn := false
	hasSpawnTeam := false
	for _, cmd := range commandsList {
		if cmd == "spawn" {
			hasSpawn = true
		}
		if cmd == "spawn-team" {
			hasSpawnTeam = true
		}
	}
	if !hasSpawn {
		t.Error("SKILL.md commands list missing 'spawn'")
	}
	if !hasSpawnTeam {
		t.Error("SKILL.md commands list missing 'spawn-team'")
	}
}

// TestExecutorPlugin_SpawnMd_AllSixRoles verifies spawn.md references all 6
// agent roles.
func TestExecutorPlugin_SpawnMd_AllSixRoles(t *testing.T) {
	pluginsDir := findPluginsDir(t)
	spawnPath := filepath.Join(pluginsDir, "executor", "commands", "spawn.md")

	content, err := os.ReadFile(spawnPath)
	if err != nil {
		t.Fatalf("Failed to read spawn.md: %v", err)
	}

	text := string(content)

	expectedRoles := []string{
		"researcher",
		"designer",
		"planner",
		"implementer",
		"test-writer",
		"reviewer",
	}

	for _, role := range expectedRoles {
		if !strings.Contains(text, role) {
			t.Errorf("spawn.md does not reference agent role %q", role)
		}
	}
}

// TestExecutorPlugin_SpawnMd_HasRoleArgument verifies spawn.md defines
// the required 'role' argument.
func TestExecutorPlugin_SpawnMd_HasRoleArgument(t *testing.T) {
	pluginsDir := findPluginsDir(t)
	spawnPath := filepath.Join(pluginsDir, "executor", "commands", "spawn.md")

	content, err := os.ReadFile(spawnPath)
	if err != nil {
		t.Fatalf("Failed to read spawn.md: %v", err)
	}

	text := string(content)

	if !strings.Contains(text, "role") {
		t.Error("spawn.md missing 'role' argument definition")
	}
	if !strings.Contains(text, "prompt") {
		t.Error("spawn.md missing 'prompt' argument definition")
	}
}

// TestExecutorPlugin_SpawnTeamMd_HasAgentsArgument verifies spawn-team.md
// defines the required 'agents' argument.
func TestExecutorPlugin_SpawnTeamMd_HasAgentsArgument(t *testing.T) {
	pluginsDir := findPluginsDir(t)
	spawnTeamPath := filepath.Join(pluginsDir, "executor", "commands", "spawn-team.md")

	content, err := os.ReadFile(spawnTeamPath)
	if err != nil {
		t.Fatalf("Failed to read spawn-team.md: %v", err)
	}

	text := string(content)

	if !strings.Contains(text, "agents") {
		t.Error("spawn-team.md missing 'agents' argument definition")
	}
	if !strings.Contains(text, "merge_strategy") {
		t.Error("spawn-team.md missing 'merge_strategy' argument definition")
	}
}

// TestExecutorPlugin_SpawnTeamMd_AllSixRoles verifies spawn-team.md references
// all 6 agent roles.
func TestExecutorPlugin_SpawnTeamMd_AllSixRoles(t *testing.T) {
	pluginsDir := findPluginsDir(t)
	spawnTeamPath := filepath.Join(pluginsDir, "executor", "commands", "spawn-team.md")

	content, err := os.ReadFile(spawnTeamPath)
	if err != nil {
		t.Fatalf("Failed to read spawn-team.md: %v", err)
	}

	text := string(content)

	expectedRoles := []string{
		"researcher",
		"designer",
		"planner",
		"implementer",
		"test-writer",
		"reviewer",
	}

	for _, role := range expectedRoles {
		if !strings.Contains(text, role) {
			t.Errorf("spawn-team.md does not reference agent role %q", role)
		}
	}
}

// TestExecutorPlugin_SpawnMd_WorktreeIsolation verifies spawn.md describes
// worktree-based isolation.
func TestExecutorPlugin_SpawnMd_WorktreeIsolation(t *testing.T) {
	pluginsDir := findPluginsDir(t)
	spawnPath := filepath.Join(pluginsDir, "executor", "commands", "spawn.md")

	content, err := os.ReadFile(spawnPath)
	if err != nil {
		t.Fatalf("Failed to read spawn.md: %v", err)
	}

	text := string(content)

	if !strings.Contains(text, "worktree") {
		t.Error("spawn.md does not mention worktree isolation")
	}
	if !strings.Contains(text, "isolation") {
		t.Error("spawn.md does not mention isolation")
	}
}

// TestExecutorPlugin_SpawnTeamMd_MergeStrategies verifies spawn-team.md
// documents merge strategies.
func TestExecutorPlugin_SpawnTeamMd_MergeStrategies(t *testing.T) {
	pluginsDir := findPluginsDir(t)
	spawnTeamPath := filepath.Join(pluginsDir, "executor", "commands", "spawn-team.md")

	content, err := os.ReadFile(spawnTeamPath)
	if err != nil {
		t.Fatalf("Failed to read spawn-team.md: %v", err)
	}

	text := string(content)

	if !strings.Contains(text, "sequential") {
		t.Error("spawn-team.md does not mention 'sequential' merge strategy")
	}
	if !strings.Contains(text, "conflict") {
		t.Error("spawn-team.md does not mention conflict handling")
	}
}
