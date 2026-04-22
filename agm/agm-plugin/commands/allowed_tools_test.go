package commands

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestAllowedToolsSyntax tests that all skill files use correct allowed-tools syntax.
// The correct syntax is space-separated: Bash(command *) NOT colon-separated: Bash(command:*)
// This matches Claude Code's permission system requirements.
func TestAllowedToolsSyntax(t *testing.T) {
	// Get the directory containing this test file
	testDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	// Find all .md files in the commands directory
	files, err := filepath.Glob(filepath.Join(testDir, "*.md"))
	if err != nil {
		t.Fatalf("Failed to glob markdown files: %v", err)
	}

	if len(files) == 0 {
		t.Skip("No markdown files found in commands directory")
	}

	// Regex to detect colon syntax in allowed-tools patterns
	// Matches patterns like: Bash(something:*) or Bash(foo bar:*)
	colonSyntaxRegex := regexp.MustCompile(`Bash\([^)]+:[^)]*\)`)

	var violations []string

	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Errorf("Failed to read file %s: %v", file, err)
			continue
		}

		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			// Only check lines that contain "allowed-tools:"
			if strings.HasPrefix(strings.TrimSpace(line), "allowed-tools:") {
				// Check if this line contains colon syntax
				if colonSyntaxRegex.MatchString(line) {
					violations = append(violations,
						filepath.Base(file)+":"+
							strings.TrimSpace(strings.SplitN(line, "allowed-tools:", 2)[1]))
				}
			}
		}
	}

	if len(violations) > 0 {
		t.Errorf("Found %d files with incorrect colon syntax in allowed-tools:\n", len(violations))
		for _, v := range violations {
			t.Errorf("  - %s", v)
		}
		t.Errorf("\nCorrect syntax: Bash(command *) NOT Bash(command:*)")
		t.Errorf("The space-separated syntax matches Claude Code's permission system.")
	}
}

// TestAgmExitNoTmuxInAllowedTools ensures agm-exit.md never references tmux in allowed-tools.
// This is a regression test: direct tmux calls trigger permission prompts that block exit.
// The fix is to use `agm get-session-name` instead, which calls tmux internally in the Go binary.
func TestAgmExitNoTmuxInAllowedTools(t *testing.T) {
	testDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(testDir, "agm-exit.md"))
	if err != nil {
		t.Fatalf("Failed to read agm-exit.md: %v", err)
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "allowed-tools:") {
			if strings.Contains(trimmed, "tmux") {
				t.Errorf("agm-exit.md allowed-tools must NOT reference tmux directly.\n"+
					"  Found: %s\n"+
					"  Use Bash(agm get-session-name) instead of Bash(tmux display-message *).\n"+
					"  Direct tmux calls trigger permission prompts that block exit.",
					trimmed)
			}
		}
	}
}

// TestAgmExitArgumentFirst ensures agm-exit.md checks $ARGUMENTS before any detection command.
// This is a regression test: if tmux/agm detection runs first, it can trigger permission prompts
// even when the session name was already provided as an argument.
func TestAgmExitArgumentFirst(t *testing.T) {
	testDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(testDir, "agm-exit.md"))
	if err != nil {
		t.Fatalf("Failed to read agm-exit.md: %v", err)
	}

	lines := strings.Split(string(content), "\n")

	// Find the ordered bullet points in Step 1.
	// The first "- " bullet referencing $ARGUMENTS must come before
	// any bullet referencing tmux or get-session-name.
	var argBulletLine, tmuxBulletLine, sessionNameBulletLine int
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- ") && !strings.HasPrefix(trimmed, "- Else") {
			continue
		}
		if strings.Contains(trimmed, "$ARGUMENTS") && argBulletLine == 0 {
			argBulletLine = i + 1
		}
		if strings.Contains(trimmed, "tmux display-message") && tmuxBulletLine == 0 {
			tmuxBulletLine = i + 1
		}
		if strings.Contains(trimmed, "get-session-name") && sessionNameBulletLine == 0 {
			sessionNameBulletLine = i + 1
		}
	}

	if argBulletLine == 0 {
		t.Fatal("agm-exit.md must have a bullet point referencing $ARGUMENTS in Step 1")
	}

	if tmuxBulletLine != 0 && tmuxBulletLine < argBulletLine {
		t.Errorf("agm-exit.md: tmux bullet (line %d) must come AFTER $ARGUMENTS bullet (line %d)",
			tmuxBulletLine, argBulletLine)
	}
	if sessionNameBulletLine != 0 && sessionNameBulletLine < argBulletLine {
		t.Errorf("agm-exit.md: get-session-name bullet (line %d) must come AFTER $ARGUMENTS bullet (line %d)",
			sessionNameBulletLine, argBulletLine)
	}
}

// TestAgmExitNoBlockedBashCommands ensures agm-exit.md does not use bash commands
// that are blocked by the pretool-bash-blocker hook. These commands trigger permission
// prompts or hook rejections that stall the exit flow.
// Blocked commands: touch, echo, printf, cat, cp, mv, rm, sed, awk, head, tail, grep, find, ls
func TestAgmExitNoBlockedBashCommands(t *testing.T) {
	testDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(testDir, "agm-exit.md"))
	if err != nil {
		t.Fatalf("Failed to read agm-exit.md: %v", err)
	}

	lines := strings.Split(string(content), "\n")

	// Commands blocked by pretool-bash-blocker that should never appear in allowed-tools
	blockedCommands := []struct {
		pattern string
		fix     string
	}{
		{"Bash(touch ", "Use Write(~/.agm/*) instead of Bash(touch *)"},
		{"Bash(echo ", "Output text directly, never via bash echo"},
		{"Bash(printf ", "Output text directly, never via bash printf"},
		{"Bash(cat ", "Use Read tool instead of Bash(cat *)"},
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "allowed-tools:") {
			continue
		}
		for _, blocked := range blockedCommands {
			if strings.Contains(trimmed, blocked.pattern) {
				t.Errorf("agm-exit.md allowed-tools must NOT use %s\n"+
					"  Fix: %s\n"+
					"  These commands are blocked by the pretool-bash-blocker hook.",
					blocked.pattern, blocked.fix)
			}
		}
	}
}

// TestAgmExitUsesWriteForMarker ensures agm-exit.md uses the Write tool (not touch)
// for creating the exit-gate marker file. The touch command is blocked by the
// pretool-bash-blocker hook, causing the exit flow to stall.
func TestAgmExitUsesWriteForMarker(t *testing.T) {
	testDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(testDir, "agm-exit.md"))
	if err != nil {
		t.Fatalf("Failed to read agm-exit.md: %v", err)
	}

	text := string(content)

	// The skill must reference the Write tool for marker creation
	if !strings.Contains(text, "Write") {
		t.Error("agm-exit.md must use the Write tool for creating the exit-gate marker file.\n" +
			"  The touch command is blocked by pretool-bash-blocker.")
	}

	// The allowed-tools must include Write permission for ~/.agm/
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "allowed-tools:") {
			if !strings.Contains(trimmed, "Write(") {
				t.Error("agm-exit.md allowed-tools must include Write(~/.agm/*) for exit-gate marker.\n" +
					"  Bash(touch *) is blocked by pretool-bash-blocker hook.")
			}
		}
	}
}

// TestAllowedToolsPresent tests that all skill files have an allowed-tools field
func TestAllowedToolsPresent(t *testing.T) {
	testDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	files, err := filepath.Glob(filepath.Join(testDir, "*.md"))
	if err != nil {
		t.Fatalf("Failed to glob markdown files: %v", err)
	}

	if len(files) == 0 {
		t.Skip("No markdown files found in commands directory")
	}

	var missing []string

	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Errorf("Failed to read file %s: %v", file, err)
			continue
		}

		// Check if file has frontmatter with allowed-tools
		if !strings.Contains(string(content), "allowed-tools:") {
			missing = append(missing, filepath.Base(file))
		}
	}

	if len(missing) > 0 {
		t.Errorf("Found %d skill files missing allowed-tools field:\n", len(missing))
		for _, m := range missing {
			t.Errorf("  - %s", m)
		}
	}
}
