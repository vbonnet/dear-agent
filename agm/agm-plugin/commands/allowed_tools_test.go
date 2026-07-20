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

// TestAgmExitArgumentFirst ensures agm-exit.md checks $ARGUMENTS before detection.
func TestAgmExitArgumentFirst(t *testing.T) {
	testDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(testDir, "agm-exit.md"))
	if err != nil {
		t.Fatalf("Failed to read agm-exit.md: %v", err)
	}

	text := string(content)
	bodyStart := strings.Index(text, "# Archive and exit")
	if bodyStart < 0 {
		t.Fatal("agm-exit.md missing command body")
	}
	body := text[bodyStart:]
	argumentIndex := strings.Index(body, "$ARGUMENTS")
	detectionIndex := strings.Index(body, "agm get-session-name")
	if argumentIndex < 0 {
		t.Fatal("agm-exit.md must reference $ARGUMENTS")
	}
	if detectionIndex < 0 {
		t.Fatal("agm-exit.md must provide typed session-name detection")
	}
	if detectionIndex < argumentIndex {
		t.Error("agm-exit.md must inspect $ARGUMENTS before running agm get-session-name")
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

func TestAgmExitDelegatesMechanicsToTypedArchive(t *testing.T) {
	testDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(testDir, "agm-exit.md"))
	if err != nil {
		t.Fatalf("Failed to read agm-exit.md: %v", err)
	}

	text := string(content)
	for _, required := range []string{"agm session get", "agm session archive", "merged", "deployed", "verified"} {
		if !strings.Contains(text, required) {
			t.Errorf("agm-exit.md missing required typed delivery contract %q", required)
		}
	}
	for _, forbidden := range []string{"git status", "git log", "exit-gate", "Write(", "archive <session-name> --force"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("agm-exit.md reimplements or bypasses typed archival with %q", forbidden)
		}
	}
}

func TestPluginCommandsUseFileInputsForUntrustedText(t *testing.T) {
	testDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	tests := []struct {
		file      string
		required  []string
		forbidden []string
	}{
		{file: "agm-send.md", required: []string{"--prompt-file", "Write(/tmp/agm-send-*", "rm -f -- <path>"}, forbidden: []string{`--prompt "{MESSAGE}"`, "properly escape quotes", "/private/tmp"}},
		{file: "wiki-query-save.md", required: []string{"--query-file", "--answer-file", "Write(/tmp/agm-wiki-*", "rm -f -- <question-file> <answer-file>"}, forbidden: []string{`--query "{QUESTION}"`, `--answer "{ANSWER}"`, "/private/tmp"}},
	}
	for _, test := range tests {
		content, readErr := os.ReadFile(filepath.Join(testDir, test.file))
		if readErr != nil {
			t.Fatalf("read %s: %v", test.file, readErr)
		}
		text := string(content)
		for _, required := range test.required {
			if !strings.Contains(text, required) {
				t.Errorf("%s missing safe-input contract %q", test.file, required)
			}
		}
		for _, forbidden := range test.forbidden {
			if strings.Contains(text, forbidden) {
				t.Errorf("%s contains unsafe-input contract %q", test.file, forbidden)
			}
		}
	}
}

func TestPluginCommandsMatchCurrentReadOnlyAndResumeBehavior(t *testing.T) {
	testDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	resume, err := os.ReadFile(filepath.Join(testDir, "agm-resume.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(resume), "interactive selection behavior") || !strings.Contains(string(resume), "Require an identifier") {
		t.Fatal("agm-resume.md still promises the unimplemented interactive picker")
	}
	lint, err := os.ReadFile(filepath.Join(testDir, "wiki-lint.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(lint), "agm wiki lint --no-append") {
		t.Fatal("wiki-lint.md does not preserve its read-only contract")
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
		if filepath.Base(file) == "SPEC.md" {
			continue
		}
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
