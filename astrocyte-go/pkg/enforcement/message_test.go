package enforcement

import (
	"strings"
	"testing"
)

func TestGenerateRejectionMessage(t *testing.T) {
	tests := []struct {
		name           string
		pattern        *Pattern
		command        string
		wantContains   []string
		wantNotContain []string
	}{
		{
			name: "basic message with tier1_example",
			pattern: &Pattern{
				ID:          "cd-chaining",
				Reason:      "Command chaining with cd",
				Alternative: "Use tool-specific -C flag (e.g., git -C /path)",
				Tier1Example: "❌ BAD: cd /repo && git push\n✅ GOOD: git -C /repo push",
			},
			command: "cd /repo && git push",
			wantContains: []string{
				"Violation Detected: cd-chaining",
				"Command: cd /repo && git push",
				"Reason: Command chaining with cd",
				"Correct approach: Use tool-specific -C flag",
				"❌ BAD: cd /repo && git push",
				"✅ GOOD: git -C /repo push",
				"Please revise your approach",
			},
		},
		{
			name: "message with examples list",
			pattern: &Pattern{
				ID:          "cat-file-read",
				Reason:      "Using cat to read files",
				Alternative: "Use Read tool: Read(file_path='...')",
				Examples: []string{
					"cat file.txt",
					"cat /path/to/file.log",
				},
			},
			command: "cat config.yaml",
			wantContains: []string{
				"Violation Detected: cat-file-read",
				"Command: cat config.yaml",
				"Reason: Using cat to read files",
				"Correct approach: Use Read tool",
				"Examples of violations:",
				"cat file.txt",
				"cat /path/to/file.log",
			},
		},
		{
			name: "message without command",
			pattern: &Pattern{
				ID:          "grep-search",
				Reason:      "Using bash grep",
				Alternative: "Use Grep tool: Grep(pattern='...', path='...')",
				Tier1Example: "❌ BAD: grep 'TODO' file.txt\n✅ GOOD: Grep(pattern='TODO', path='file.txt')",
			},
			command: "",
			wantContains: []string{
				"Violation Detected: grep-search",
				"Reason: Using bash grep",
				"Correct approach: Use Grep tool",
			},
			wantNotContain: []string{
				"Command:",
			},
		},
		{
			name: "message with no examples or tier1_example",
			pattern: &Pattern{
				ID:          "test-pattern",
				Reason:      "Test violation",
				Alternative: "Use correct approach",
			},
			command: "test command",
			wantContains: []string{
				"Violation Detected: test-pattern",
				"Command: test command",
				"Reason: Test violation",
				"Correct approach: Use correct approach",
				"Please revise your approach",
			},
			wantNotContain: []string{
				"Example:",
				"Examples of violations:",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateRejectionMessage(tt.pattern, tt.command)

			// Check for expected content
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("GenerateRejectionMessage() missing expected content:\nwant substring: %q\ngot: %s", want, got)
				}
			}

			// Check for unwanted content
			for _, notWant := range tt.wantNotContain {
				if strings.Contains(got, notWant) {
					t.Errorf("GenerateRejectionMessage() contains unwanted content:\nunwanted substring: %q\ngot: %s", notWant, got)
				}
			}
		})
	}
}

func TestGenerateShortRejectionMessage(t *testing.T) {
	tests := []struct {
		name    string
		pattern *Pattern
		want    string
	}{
		{
			name: "basic short message",
			pattern: &Pattern{
				ID:     "cd-chaining",
				Reason: "Command chaining with cd",
			},
			want: "[cd-chaining] Command chaining with cd",
		},
		{
			name: "long reason",
			pattern: &Pattern{
				ID:     "grep-search",
				Reason: "Using bash grep instead of the Grep tool which is more efficient and provides better output",
			},
			want: "[grep-search] Using bash grep instead of the Grep tool which is more efficient and provides better output",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateShortRejectionMessage(tt.pattern)
			if got != tt.want {
				t.Errorf("GenerateShortRejectionMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenerateRejectionMessageWithSeverity(t *testing.T) {
	tests := []struct {
		name         string
		pattern      *Pattern
		command      string
		wantContains []string
	}{
		{
			name: "critical severity",
			pattern: &Pattern{
				ID:          "git-force-push",
				Reason:      "Force pushing to protected branch",
				Alternative: "Use normal push or rebase",
				Severity:    "critical",
				Tier1Example: "❌ BAD: git push --force\n✅ GOOD: git push",
			},
			command: "git push --force origin main",
			wantContains: []string{
				"Violation Detected: git-force-push [CRITICAL]",
				"⚠️  CRITICAL: This violation may cause data loss or system instability",
				"Please revise your approach immediately",
			},
		},
		{
			name: "high severity",
			pattern: &Pattern{
				ID:          "cd-chaining",
				Reason:      "Command chaining with cd",
				Alternative: "Use tool-specific -C flag",
				Severity:    "high",
				Tier1Example: "❌ BAD: cd /repo && git push\n✅ GOOD: git -C /repo push",
			},
			command: "cd /repo && git push",
			wantContains: []string{
				"Violation Detected: cd-chaining [HIGH]",
				"⚠️  This is a high-severity violation",
				"Please revise your approach and try again",
			},
		},
		{
			name: "medium severity (default footer)",
			pattern: &Pattern{
				ID:          "cat-file-read",
				Reason:      "Using cat to read files",
				Alternative: "Use Read tool",
				Severity:    "medium",
			},
			command: "cat file.txt",
			wantContains: []string{
				"Violation Detected: cat-file-read [MEDIUM]",
				"Please revise your approach and try again",
			},
		},
		{
			name: "includes tier1_example",
			pattern: &Pattern{
				ID:          "grep-search",
				Reason:      "Using bash grep",
				Alternative: "Use Grep tool",
				Severity:    "high",
				Tier1Example: "❌ BAD: grep 'TODO' file.txt\n✅ GOOD: Grep(pattern='TODO')",
			},
			command: "grep 'TODO' *.go",
			wantContains: []string{
				"Example:",
				"❌ BAD: grep 'TODO' file.txt",
				"✅ GOOD: Grep(pattern='TODO')",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateRejectionMessageWithSeverity(tt.pattern, tt.command)

			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("GenerateRejectionMessageWithSeverity() missing expected content:\nwant substring: %q\ngot: %s", want, got)
				}
			}
		})
	}
}

func TestGenerateRejectionMessageFormat(t *testing.T) {
	// Test that the message follows the expected structure
	pattern := &Pattern{
		ID:          "cd-chaining",
		Reason:      "Command chaining with cd",
		Alternative: "Use tool-specific -C flag",
		Tier1Example: "❌ BAD: cd /repo && git push\n✅ GOOD: git -C /repo push",
	}
	command := "cd /repo && git push"

	msg := GenerateRejectionMessage(pattern, command)

	// Check that sections appear in the correct order
	headerIdx := strings.Index(msg, "Violation Detected:")
	commandIdx := strings.Index(msg, "Command:")
	reasonIdx := strings.Index(msg, "Reason:")
	approachIdx := strings.Index(msg, "Correct approach:")
	exampleIdx := strings.Index(msg, "Example:")
	footerIdx := strings.Index(msg, "Please revise")

	if headerIdx == -1 || commandIdx == -1 || reasonIdx == -1 || approachIdx == -1 || exampleIdx == -1 || footerIdx == -1 {
		t.Errorf("Message missing required sections")
	}

	// Verify order
	if !(headerIdx < commandIdx && commandIdx < reasonIdx && reasonIdx < approachIdx && approachIdx < exampleIdx && exampleIdx < footerIdx) {
		t.Errorf("Message sections not in expected order")
	}
}

func TestGenerateRejectionMessageRealPattern(t *testing.T) {
	// Test with a real pattern from the bash-anti-patterns.yaml
	pattern := &Pattern{
		ID:          "cd-chaining",
		Regex:       `cd\s+[^\s]+\s+&&`,
		Reason:      "Command chaining with cd",
		Alternative: "Use tool-specific -C flag (e.g., git -C /path)",
		Examples: []string{
			"cd /repo && git push",
			"cd /dir && npm test",
			"cd ~/src && make build",
		},
		Severity:     "high",
		Tier1Example: "❌ BAD: cd /repo && git push\n✅ GOOD: git -C /repo push",
	}

	command := "cd /home/user/project && git push origin main"
	msg := GenerateRejectionMessage(pattern, command)

	// Verify all key components are present
	expectedParts := []string{
		"cd-chaining",
		"Command chaining with cd",
		"Use tool-specific -C flag",
		"cd /home/user/project && git push origin main",
		"❌ BAD:",
		"✅ GOOD:",
	}

	for _, part := range expectedParts {
		if !strings.Contains(msg, part) {
			t.Errorf("Message missing expected part: %q", part)
		}
	}
}
