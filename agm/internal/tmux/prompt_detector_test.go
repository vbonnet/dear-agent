package tmux

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestContainsPromptPattern(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "Claude cursor pattern",
			content:  "▌",
			expected: true,
		},
		{
			name:     "Claude cursor with text",
			content:  "some text ▌",
			expected: true,
		},
		{
			name:     "Common prompt",
			content:  "> ",
			expected: true,
		},
		{
			name:     "Shell prompt",
			content:  "$ ",
			expected: true,
		},
		{
			name:     "Root prompt",
			content:  "# ",
			expected: true,
		},
		{
			name:     "Prompt with path prefix",
			content:  "user@host:~/dir $ ",
			expected: true,
		},
		{
			name:     "Ends with >",
			content:  "user@host>",
			expected: true,
		},
		{
			name:     "Ends with $",
			content:  "bash-5.1$",
			expected: true,
		},
		{
			name:     "Ends with #",
			content:  "root@host#",
			expected: true,
		},
		{
			name:     "Empty string",
			content:  "",
			expected: false,
		},
		{
			name:     "Whitespace only",
			content:  "   ",
			expected: false,
		},
		{
			name:     "Regular text",
			content:  "hello world",
			expected: false,
		},
		{
			name:     "Hash in middle of text",
			content:  "test #tag here",
			expected: false,
		},
		{
			name:     "Dollar in middle of text",
			content:  "costs $100 today",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsPromptPattern(tt.content)
			if result != tt.expected {
				t.Errorf("containsPromptPattern(%q) = %v, expected %v",
					tt.content, result, tt.expected)
			}
		})
	}
}

func TestContainsClaudePromptPattern(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		// Positive cases - should match Claude prompt
		{
			name:     "Exact Claude prompt",
			content:  "❯",
			expected: true,
		},
		{
			name:     "Claude prompt with whitespace",
			content:  "  ❯  ",
			expected: true,
		},
		{
			name:     "Claude prompt in context",
			content:  "user@host:~/dir ❯",
			expected: true,
		},
		{
			name:     "Multi-line with Claude prompt",
			content:  "some output\nmore output\n❯",
			expected: true,
		},
		// Negative cases - should NOT match bash prompts
		{
			name:     "Bash prompt $ (no space)",
			content:  "$",
			expected: false,
		},
		{
			name:     "Bash prompt > (no space)",
			content:  ">",
			expected: false,
		},
		{
			name:     "Bash prompt # (no space)",
			content:  "#",
			expected: false,
		},
		{
			name:     "Bash prompt $ with space",
			content:  "$ ",
			expected: false,
		},
		{
			name:     "Bash prompt > with space",
			content:  "> ",
			expected: false,
		},
		{
			name:     "Bash prompt # with space",
			content:  "# ",
			expected: false,
		},
		{
			name:     "Bash prompt with path",
			content:  "user@host:~/dir $ ",
			expected: false,
		},
		{
			name:     "Empty string",
			content:  "",
			expected: false,
		},
		{
			name:     "Whitespace only",
			content:  "   ",
			expected: false,
		},
		{
			name:     "Regular text",
			content:  "hello world",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsClaudePromptPattern(tt.content)
			if result != tt.expected {
				t.Errorf("containsClaudePromptPattern(%q) = %v, expected %v",
					tt.content, result, tt.expected)
			}
		})
	}
}

func TestContainsTrustPromptPattern(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		// Positive cases - should match trust prompt
		{
			name:     "Exact trust prompt",
			content:  "Do you trust the files in this folder?",
			expected: true,
		},
		{
			name:     "Trust prompt with whitespace",
			content:  "  Do you trust the files in this folder?  \n",
			expected: true,
		},
		{
			name:     "Trust prompt in multiline output",
			content:  "Some text\nDo you trust the files in this folder?\nMore text",
			expected: true,
		},
		{
			name:     "Trust prompt with surrounding text",
			content:  "Claude Code is asking: Do you trust the files in this folder? Please answer.",
			expected: true,
		},
		// Current Claude Code (2.x) wording (ce-wn4qe).
		{
			name:     "Current-wording trust question",
			content:  "Quick safety check: Is this a project you created or one you trust?",
			expected: true,
		},
		{
			name:     "Current-wording affirmative option without question is not trust evidence",
			content:  " ❯ 1. Yes, I trust this folder\n   2. No, exit\n\n Enter to confirm · Esc to cancel",
			expected: false,
		},
		{
			name: "Real captured new trust prompt",
			content: "Quick safety check: Is this a project you created or one you trust?\n" +
				" ⚠ This folder pre-approves 20 tool permissions in .claude/settings.local.json:\n" +
				"   Bash(git status), Bash(git status *), Bash(git -C * status *)\n" +
				" These will apply without asking. Only proceed if you trust this configuration.\n\n" +
				" Security guide\n\n" +
				" ❯ 1. Yes, I trust this folder\n   2. No, exit\n\n Enter to confirm · Esc to cancel",
			expected: true,
		},
		// Negative cases - should NOT match
		{
			name:     "Empty string",
			content:  "",
			expected: false,
		},
		{
			name:     "Whitespace only",
			content:  "   \n  ",
			expected: false,
		},
		{
			name:     "Claude ready prompt",
			content:  "❯ ",
			expected: false,
		},
		{
			name:     "Bash prompt",
			content:  "$ ",
			expected: false,
		},
		{
			name:     "Random text",
			content:  "Random output from command",
			expected: false,
		},
		{
			name:     "Similar but not exact trust text",
			content:  "Do you trust this folder?", // Missing "the files in"
			expected: false,
		},
		{
			name:     "Partial trust text",
			content:  "trust the files",
			expected: false,
		},
		{
			name:     "Trust word in different context",
			content:  "I trust you completely",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsTrustPromptPattern(tt.content)
			if result != tt.expected {
				t.Errorf("containsTrustPromptPattern(%q) = %v, want %v", tt.content, result, tt.expected)
			}
		})
	}
}

func TestClaudeTrustInputOwnership(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		selectorOwns bool
		dialogOwns   bool
	}{
		{
			name:         "live selector owns the tail",
			content:      " Is this a project you created or one you trust?\n\n ❯ 1. Yes, I trust this folder\n   2. No, exit\n\n Enter to confirm · Esc to cancel",
			selectorOwns: true,
			dialogOwns:   true,
		},
		{
			name: "real current permission summary remains same-dialog body",
			content: "Quick safety check: Is this a project you created or one you trust?\n" +
				"⚠ This folder pre-approves 20 tool permissions in .claude/settings.local.json:\n" +
				"Bash(git status), Bash(git status *), Bash(git -C * status *)\n" +
				"These will apply without asking. Only proceed if you trust this configuration.\n\n" +
				"Security guide\n\n❯ 1. Yes, I trust this folder\n  2. No, exit\nEnter to confirm · Esc to cancel",
			selectorOwns: true,
			dialogOwns:   true,
		},
		{
			name: "mixed bare scoped and MCP rules remain same-dialog body",
			content: "Quick safety check: Is this a project you created or one you trust?\n" +
				"This folder pre-approves project permissions:\n" +
				"Read, Bash(git status), mcp__server__*\n" +
				"These will apply without asking. Only proceed if you trust this configuration.\n" +
				"❯ 1. Yes, I trust this folder\n  2. No, exit",
			selectorOwns: true,
			dialogOwns:   true,
		},
		{
			name: "permission summary without consequence is not an answerable dialog",
			content: "Quick safety check: Is this a project you created or one you trust?\n" +
				"This folder pre-approves project permissions:\nRead, Bash(git status)\n" +
				"❯ 1. Yes, I trust this folder\n  2. No, exit",
		},
		{
			name: "arbitrary text inside permission summary is not an answerable dialog",
			content: "Quick safety check: Is this a project you created or one you trust?\n" +
				"This folder pre-approves project permissions:\nRead\nstartup completed\n" +
				"These will apply without asking. Only proceed if you trust this configuration.\n" +
				"❯ 1. Yes, I trust this folder\n  2. No, exit",
		},
		{
			name: "tool-shaped model output without preapproval warning is not dialog body",
			content: "Quick safety check: Is this a project you created or one you trust?\n" +
				"ModelOutput(fake)\n❯ 1. Yes, I trust this folder\n  2. No, exit",
		},
		{
			name:         "legacy live selector owns the tail",
			content:      "Do you trust the files in this folder?\n❯ 1. Yes, proceed\n  2. No, exit",
			selectorOwns: true,
			dialogOwns:   true,
		},
		{
			name:         "ANSI-styled selected affirmative owns the tail",
			content:      "Quick safety check: Is this a project you created or one you trust?\n\x1b[1m❯\x1b[0m 1. \x1b[38;5;105mYes, I trust this folder\x1b[0m\n  2. No, exit",
			selectorOwns: true,
			dialogOwns:   true,
		},
		{
			name:       "selected negative owns dialog but not affirmative authority",
			content:    "Quick safety check: Is this a project you created or one you trust?\n  1. Yes, I trust this folder\n❯ 2. No, exit\n\nEnter to confirm",
			dialogOwns: true,
		},
		{
			name:       "ANSI selected negative owns dialog but not affirmative authority",
			content:    "Quick safety check: Is this a project you created or one you trust?\n  1. Yes, I trust this folder\n\x1b[1m❯\x1b[0m 2. \x1b[38;5;105mNo, exit\x1b[0m\nEnter to confirm",
			dialogOwns: true,
		},
		{
			// ce-wn4qe :403 — the question is on screen but the numbered options
			// have not rendered yet; must not answer.
			name:       "question before options render blocks input without answer authority",
			content:    "Quick safety check: Is this a project you created or one you trust?",
			dialogOwns: true,
		},
		{
			// ce-wn4qe :145 — an already-answered selector still in the capture
			// with a live composer below must not be treated as answerable.
			name:    "historical selector with composer below does not own input",
			content: "❯ 1. Yes, I trust this folder\n  2. No, exit\n\n────────\n❯ \n────────\n  vbonnet@mac:/tmp/wd",
		},
		{
			// A newer permission selector below the trust option must not be
			// answered by the trust path (auto-approve risk).
			name:    "permission prompt below trust selector does not own trust input",
			content: "❯ 1. Yes, I trust this folder\n  2. No, exit\n\nDo you want to proceed?\n❯ 1. Yes\n  2. No",
		},
		{
			name:    "unanchored affirmative-looking row is a composer draft",
			content: "❯ 1. Yes, I trust this folder",
		},
		{
			name:       "anchored affirmative-looking row without pair blocks but cannot authorize",
			content:    "Quick safety check: Is this a project you created or one you trust?\n❯ 1. Yes, I trust this folder",
			dialogOwns: true,
		},
		{
			name:    "unanchored paired current rows are a composer draft",
			content: "❯ 1. Yes, I trust this folder\n  2. No, exit",
		},
		{
			name:    "prose containing trust chrome below historical selector does not own input",
			content: "❯ 1. Yes, I trust this folder\n  2. No, exit\n\n❯ document the Enter to confirm and Esc to cancel behavior",
		},
		{
			name:    "historical question and selector above live composer do not own input",
			content: "Quick safety check: Is this a project you created or one you trust?\n❯ 1. Yes, I trust this folder\n  2. No, exit\nEnter to confirm\n\nresponse complete\n❯ \n────────",
		},
		{
			name: "historical partial question above structurally proven composer does not own input",
			content: "Quick safety check: Is this a project you created or one you trust?\ntransition complete\n" +
				"❯ \n────────\nvbonnet@mac:/tmp/wd",
		},
		{
			name:    "historical question and ordinary response above bare composer do not own input",
			content: "Quick safety check: Is this a project you created or one you trust?\nmodel response complete\n❯ ",
		},
		{
			name:    "generic selected negative is not a trust dialog",
			content: "  1. Yes\n❯ 2. No, exit\nEnter to confirm",
		},
		{
			name:    "generic legacy proceed selector is not a trust dialog",
			content: "Do you want to proceed?\n❯ 1. Yes, proceed\n  2. No, exit",
		},
		{
			name: "historical legacy question does not authorize newer generic permission",
			content: "Do you trust the files in this folder?\n\npermission request follows\n" +
				"Do you want to proceed?\n❯ 1. Yes, proceed\n  2. No, exit",
		},
		{
			name:       "partial affirmative row blocks readiness but cannot authorize Enter",
			content:    "Quick safety check: Is this a project you created or one you trust?\n❯ 1. Yes, I trust this folder\nEnter to confirm",
			dialogOwns: true,
		},
		{
			name:       "question and bare cursor are a partially rendered dialog",
			content:    "Quick safety check: Is this a project you created or one you trust?\n❯ ",
			dialogOwns: true,
		},
		{
			name: "current warning body and bare cursor are a partially rendered dialog",
			content: "Quick safety check: Is this a project you created or one you trust?\n" +
				"⚠ This folder pre-approves 20 tool permissions in .claude/settings.local.json:\n" +
				"These will apply without asking. Only proceed if you trust this configuration.\n❯ ",
			dialogOwns: true,
		},
		{
			name:       "unselected affirmative and bare cursor are a partially rendered dialog",
			content:    "Quick safety check: Is this a project you created or one you trust?\n  1. Yes, I trust this folder\n❯ ",
			dialogOwns: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TrustSelectorOwnsInput(tt.content); got != tt.selectorOwns {
				t.Errorf("TrustSelectorOwnsInput(%q) = %v, want %v", tt.content, got, tt.selectorOwns)
			}
			if got := TrustDialogOwnsInput(tt.content); got != tt.dialogOwns {
				t.Errorf("TrustDialogOwnsInput(%q) = %v, want %v", tt.content, got, tt.dialogOwns)
			}
		})
	}
}

func TestClaudePromptEligibleIgnoresOnlyLiveTrustDialog(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "ordinary composer is eligible",
			content:  "response complete\n❯ ",
			expected: true,
		},
		{
			name:     "current selected affirmative blocks composer detection",
			content:  "Quick safety check: Is this a project you created or one you trust?\n❯ 1. Yes, I trust this folder\n  2. No, exit\nEnter to confirm",
			expected: false,
		},
		{
			name:     "current selected negative blocks composer detection",
			content:  "Quick safety check: Is this a project you created or one you trust?\n  1. Yes, I trust this folder\n❯ 2. No, exit\nEnter to confirm",
			expected: false,
		},
		{
			name:     "historical trust question above live composer is eligible",
			content:  "Quick safety check: Is this a project you created or one you trust?\n❯ 1. Yes, I trust this folder\n  2. No, exit\nEnter to confirm\n\nresponse complete\n❯ ",
			expected: true,
		},
		{
			name: "historical partial question above structurally proven composer is eligible",
			content: "Quick safety check: Is this a project you created or one you trust?\ntransition complete\n" +
				"❯ \n────────\nvbonnet@mac:/tmp/wd",
			expected: true,
		},
		{
			name:     "live selector remains ineligible until a composer owns the tail",
			content:  "Quick safety check: Is this a project you created or one you trust?\n❯ 1. Yes, I trust this folder\n  2. No, exit",
			expected: false,
		},
		{
			name:     "historical selector followed by non-composer output is ineligible",
			content:  "Quick safety check: Is this a project you created or one you trust?\n❯ 1. Yes, I trust this folder\n  2. No, exit\nEnter to confirm\nresponse still loading",
			expected: false,
		},
		{
			name:     "partially rendered trust question and bare cursor are ineligible",
			content:  "Quick safety check: Is this a project you created or one you trust?\n❯ ",
			expected: false,
		},
		{
			name:     "unselected trust option and bare cursor are ineligible",
			content:  "Quick safety check: Is this a project you created or one you trust?\n  1. Yes, I trust this folder\n❯ ",
			expected: false,
		},
		{
			name: "current warning body and bare cursor are ineligible",
			content: "Quick safety check: Is this a project you created or one you trust?\n" +
				"⚠ This folder pre-approves 20 tool permissions in .claude/settings.local.json:\n" +
				"These will apply without asking. Only proceed if you trust this configuration.\n❯ ",
			expected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := claudePromptEligible(tt.content); got != tt.expected {
				t.Errorf("claudePromptEligible(%q) = %v, want %v", tt.content, got, tt.expected)
			}
		})
	}
}

func TestContainsClaudeTrustAffirmative(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{name: "unanchored legacy affirmative", content: "❯ 1. Yes, proceed\n  2. No, exit", expected: false},
		{name: "anchored legacy affirmative", content: "Do you trust the files in this folder?\n❯ 1. Yes, proceed\n  2. No, exit", expected: true},
		{name: "unanchored current affirmative", content: "❯ 1. Yes, I trust this folder\n  2. No, exit", expected: false},
		{name: "anchored current affirmative", content: "Quick safety check: Is this a project you created or one you trust?\n❯ 1. Yes, I trust this folder\n  2. No, exit", expected: true},
		{name: "ASCII greater-than prose is not a selector", content: "> 1. Yes, proceed", expected: false},
		{name: "guillemet prose is not a selector", content: "» 1. Yes, proceed", expected: false},
		{name: "numbered option without cursor", content: " 1. Yes, I trust this folder", expected: false},
		{name: "selected negative with unselected affirmative", content: " 1. Yes, I trust this folder\n❯ 2. No, exit", expected: false},
		{name: "ANSI-styled selected affirmative", content: "Quick safety check: Is this a project you created or one you trust?\n\x1b[1m❯\x1b[0m 1. \x1b[38;5;105mYes, I trust this folder\x1b[0m\n  2. No, exit", expected: true},
		{name: "selected-looking prose suffix", content: "❯ 1. Yes, I trust this folder (quoted example)", expected: false},
		{name: "question without selector rendered yet", content: "Quick safety check: Is this a project you created or one you trust?", expected: false},
		{name: "empty", content: "", expected: false},
		{name: "unrelated", content: "I trust you completely", expected: false},
		{name: "prose mentions the affirmative phrase", content: "The model replied: Yes, proceed with the migration.", expected: false},
		{name: "prose mentions trust phrase", content: "Answer: Yes, I trust this folder is safe to open.", expected: false},
		{name: "generic legacy proceed selector", content: "Do you want to proceed?\n❯ 1. Yes, proceed\n  2. No, exit", expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ContainsClaudeTrustAffirmative(tt.content); got != tt.expected {
				t.Errorf("ContainsClaudeTrustAffirmative(%q) = %v, want %v", tt.content, got, tt.expected)
			}
		})
	}
}

func TestValidClaudeTrustPermissionSummary(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		expects bool
	}{
		{name: "mixed documented forms", line: "Read, Bash(git status), mcp__server__*", expects: true},
		{name: "nested scoped expression", line: "Bash(echo $(date))", expects: true},
		{name: "bulleted rule", line: "• WebFetch(domain:example.com)", expects: true},
		{name: "arbitrary prose", line: "startup completed", expects: false},
		{name: "empty trailing rule", line: "Read,", expects: false},
		{name: "unbalanced scope", line: "Bash(echo $(date)", expects: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validClaudeTrustPermissionSummary(tt.line); got != tt.expects {
				t.Fatalf("validClaudeTrustPermissionSummary(%q) = %t, want %t", tt.line, got, tt.expects)
			}
		})
	}
}

func TestProbeClaudeInput(t *testing.T) {
	captureFailure := errors.New("capture failed")
	tests := []struct {
		name         string
		capturedPane string
		recaptured   string
		captureErr   error
		livenessErr  error
		denyLiveness bool
		sendErr      error
		autoAnswer   bool
		want         ClaudeInputProbe
		wantErr      bool
		wantSend     bool
	}{
		{
			name:         "fragmented event wake-up still answers fresh affirmative selector",
			capturedPane: "Quick safety check: Is this a project you created or one you trust?\n❯ 1. Yes, I trust this folder\n  2. No, exit\nEnter to confirm",
			autoAnswer:   true,
			want:         ClaudeInputProbe{DialogOwnsInput: true, TrustAnswered: true},
			wantSend:     true,
		},
		{
			name:         "ANSI live affirmative selector authorizes Enter",
			capturedPane: "Quick safety check: Is this a project you created or one you trust?\n\x1b[1m❯\x1b[0m 1. \x1b[38;5;105mYes, I trust this folder\x1b[0m\n  2. No, exit\nEnter to confirm",
			autoAnswer:   true,
			want:         ClaudeInputProbe{DialogOwnsInput: true, TrustAnswered: true},
			wantSend:     true,
		},
		{
			name: "real current permission summary authorizes only its anchored selector",
			capturedPane: "Quick safety check: Is this a project you created or one you trust?\n" +
				"⚠ This folder pre-approves 20 tool permissions in .claude/settings.local.json:\n" +
				"Bash(git status), Bash(git status *), Bash(git -C * status *)\n" +
				"These will apply without asking. Only proceed if you trust this configuration.\n\n" +
				"Security guide\n\n❯ 1. Yes, I trust this folder\n  2. No, exit\nEnter to confirm · Esc to cancel",
			autoAnswer: true,
			want:       ClaudeInputProbe{DialogOwnsInput: true, TrustAnswered: true},
			wantSend:   true,
		},
		{
			name: "mixed bare scoped and MCP rules authorize only their anchored selector",
			capturedPane: "Quick safety check: Is this a project you created or one you trust?\n" +
				"This folder pre-approves project permissions:\n" +
				"Read, Bash(git status), mcp__server__*\n" +
				"These will apply without asking. Only proceed if you trust this configuration.\n" +
				"❯ 1. Yes, I trust this folder\n  2. No, exit",
			autoAnswer: true,
			want:       ClaudeInputProbe{DialogOwnsInput: true, TrustAnswered: true},
			wantSend:   true,
		},
		{
			name:         "historical selector and newer composer report current composer",
			capturedPane: "❯ 1. Yes, I trust this folder\n  2. No, exit\nresponse complete\n❯ ",
			autoAnswer:   true,
			want:         ClaudeInputProbe{ComposerOwnsInput: true},
		},
		{
			name:         "styled ghost text reports current composer",
			capturedPane: "❯ \x1b[2mstart the loop\x1b[0m",
			autoAnswer:   true,
			want:         ClaudeInputProbe{ComposerOwnsInput: true},
		},
		{
			name:         "selected negative does not authorize Enter",
			capturedPane: "Quick safety check: Is this a project you created or one you trust?\n  1. Yes, I trust this folder\n❯ 2. No, exit\nEnter to confirm",
			autoAnswer:   true,
			want:         ClaudeInputProbe{DialogOwnsInput: true},
		},
		{
			name:         "generic legacy proceed dialog does not authorize Enter",
			capturedPane: "Do you want to proceed?\n❯ 1. Yes, proceed\n  2. No, exit",
			autoAnswer:   true,
			want:         ClaudeInputProbe{},
		},
		{
			name:         "paired current wording in a composer draft does not authorize Enter",
			capturedPane: "❯ 1. Yes, I trust this folder\n  2. No, exit",
			autoAnswer:   true,
			want:         ClaudeInputProbe{},
		},
		{
			name: "historical question separated from paired draft does not authorize Enter",
			capturedPane: "Quick safety check: Is this a project you created or one you trust?\n" +
				"startup completed\n❯ 1. Yes, I trust this folder\n  2. No, exit",
			autoAnswer: true,
			want:       ClaudeInputProbe{},
		},
		{
			name:         "historical question and ordinary response expose current bare composer",
			capturedPane: "Quick safety check: Is this a project you created or one you trust?\nmodel response complete\n❯ ",
			autoAnswer:   true,
			want:         ClaudeInputProbe{ComposerOwnsInput: true},
		},
		{
			name:         "partially rendered affirmative blocks readiness without answering",
			capturedPane: "Quick safety check: Is this a project you created or one you trust?\n❯ 1. Yes, I trust this folder",
			autoAnswer:   true,
			want:         ClaudeInputProbe{DialogOwnsInput: true},
		},
		{
			name:         "partially rendered question and cursor block readiness",
			capturedPane: "Quick safety check: Is this a project you created or one you trust?\n❯ ",
			autoAnswer:   true,
			want:         ClaudeInputProbe{DialogOwnsInput: true},
		},
		{
			name: "current warning body and cursor block readiness",
			capturedPane: "Quick safety check: Is this a project you created or one you trust?\n" +
				"⚠ This folder pre-approves 20 tool permissions in .claude/settings.local.json:\n" +
				"These will apply without asking. Only proceed if you trust this configuration.\n❯ ",
			autoAnswer: true,
			want:       ClaudeInputProbe{DialogOwnsInput: true},
		},
		{
			name:         "caller may probe without auto-answering",
			capturedPane: "Quick safety check: Is this a project you created or one you trust?\n❯ 1. Yes, I trust this folder\n  2. No, exit",
			want:         ClaudeInputProbe{DialogOwnsInput: true},
		},
		{
			name:       "capture failure is fail closed",
			captureErr: captureFailure,
			autoAnswer: true,
			wantErr:    true,
		},
		{
			name:         "dialog-shaped shell output cannot authorize Enter",
			capturedPane: "Quick safety check: Is this a project you created or one you trust?\n❯ 1. Yes, I trust this folder\n  2. No, exit",
			autoAnswer:   true,
			denyLiveness: true,
			want:         ClaudeInputProbe{DialogOwnsInput: true},
			wantErr:      true,
		},
		{
			name:         "shell ghost composer cannot prove Claude readiness",
			capturedPane: "❯ \x1b[2mstart the loop\x1b[0m",
			autoAnswer:   true,
			denyLiveness: true,
			want:         ClaudeInputProbe{ComposerOwnsInput: true},
			wantErr:      true,
		},
		{
			name:         "selector changing during liveness is reclassified before Enter",
			capturedPane: "Quick safety check: Is this a project you created or one you trust?\n❯ 1. Yes, I trust this folder\n  2. No, exit",
			recaptured:   "Quick safety check: Is this a project you created or one you trust?\n  1. Yes, I trust this folder\n❯ 2. No, exit",
			autoAnswer:   true,
			want:         ClaudeInputProbe{DialogOwnsInput: true},
		},
		{
			name:         "liveness evidence failure is fail closed",
			capturedPane: "Quick safety check: Is this a project you created or one you trust?\n❯ 1. Yes, I trust this folder\n  2. No, exit",
			autoAnswer:   true,
			livenessErr:  errors.New("ps unavailable"),
			want:         ClaudeInputProbe{DialogOwnsInput: true},
			wantErr:      true,
		},
		{
			name:         "Enter delivery failure is reported",
			capturedPane: "Quick safety check: Is this a project you created or one you trust?\n❯ 1. Yes, I trust this folder\n  2. No, exit",
			autoAnswer:   true,
			sendErr:      errors.New("send failed"),
			want:         ClaudeInputProbe{DialogOwnsInput: true},
			wantErr:      true,
			wantSend:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetPane := activePaneTarget{ID: "%7", RootPID: 123}
			sentTarget := ""
			captures := 0
			runtime := claudeInputProbeRuntime{
				resolve: func(_ context.Context, sessionName string) (activePaneTarget, bool, error) {
					if sessionName != "session" {
						t.Fatalf("resolve session = %q, want session", sessionName)
					}
					return targetPane, true, nil
				},
				capture: func(_ context.Context, gotTarget string) (string, error) {
					if gotTarget != targetPane.ID {
						t.Fatalf("capture target = %q, want %q", gotTarget, targetPane.ID)
					}
					captures++
					if captures > 1 && tt.recaptured != "" {
						return tt.recaptured, tt.captureErr
					}
					return tt.capturedPane, tt.captureErr
				},
				liveness: func(_ context.Context, gotTarget activePaneTarget) (PaneLiveness, error) {
					if gotTarget != targetPane {
						t.Fatalf("liveness target = %+v, want %+v", gotTarget, targetPane)
					}
					return PaneLiveness{SessionExists: true, HarnessAlive: !tt.denyLiveness}, tt.livenessErr
				},
				sendEnter: func(_ context.Context, gotTarget string) error {
					sentTarget = gotTarget
					return tt.sendErr
				},
			}
			got, err := probeClaudeInput(context.Background(), "session", tt.autoAnswer, runtime)
			if (err != nil) != tt.wantErr {
				t.Fatalf("probeClaudeInput() error = %v, wantErr %t", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("probeClaudeInput() = %+v, want %+v", got, tt.want)
			}
			if tt.wantSend {
				if sentTarget != targetPane.ID {
					t.Fatalf("Enter target = %q, want captured pane %q", sentTarget, targetPane.ID)
				}
			} else if sentTarget != "" {
				t.Fatalf("unexpected Enter target %q", sentTarget)
			}
		})
	}
}

func TestWaitForClaudeReadyWithProbe(t *testing.T) {
	t.Parallel()

	t.Run("answers trust then accepts the current composer", func(t *testing.T) {
		t.Parallel()
		calls := 0
		err := waitForClaudeReadyWithProbe(t.Context(), "session", time.Second, time.Millisecond, 0,
			func(_ context.Context, session string, autoAnswer bool) (claudeInputObservation, error) {
				calls++
				if session != "session" {
					t.Fatalf("session = %q", session)
				}
				switch calls {
				case 1:
					if !autoAnswer {
						t.Fatal("first probe did not permit trust answer")
					}
					return claudeInputObservation{probe: ClaudeInputProbe{DialogOwnsInput: true, TrustAnswered: true}}, nil
				default:
					if autoAnswer {
						t.Fatal("later probe tried to answer trust twice")
					}
					return claudeInputObservation{probe: ClaudeInputProbe{ComposerOwnsInput: true}}, nil
				}
			})
		if err != nil {
			t.Fatal(err)
		}
		if calls != 2 {
			t.Fatalf("probe calls = %d, want 2", calls)
		}
	})

	t.Run("selected negative never becomes ready", func(t *testing.T) {
		t.Parallel()
		err := waitForClaudeReadyWithProbe(t.Context(), "session", 5*time.Millisecond, time.Millisecond, 0,
			func(context.Context, string, bool) (claudeInputObservation, error) {
				return claudeInputObservation{probe: ClaudeInputProbe{DialogOwnsInput: true}}, nil
			})
		if err == nil || !strings.Contains(err.Error(), "timeout waiting for Claude to be ready") {
			t.Fatalf("error = %v, want bounded readiness timeout", err)
		}
	})

	t.Run("probe failure is fail closed", func(t *testing.T) {
		t.Parallel()
		wantErr := errors.New("capture unavailable")
		err := waitForClaudeReadyWithProbe(t.Context(), "session", time.Second, time.Millisecond, 0,
			func(context.Context, string, bool) (claudeInputObservation, error) {
				return claudeInputObservation{}, wantErr
			})
		if !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
	})
}

func TestContainsResumeFailurePattern(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		wantMatched string
		wantOK      bool
	}{
		{
			name:        "No conversation found",
			line:        "No conversation found with session ID: 61163f27-a35e-4370-be49-f8456401a872",
			wantMatched: "No conversation found",
			wantOK:      true,
		},
		{
			name:        "No messages returned",
			line:        "Error: No messages returned for this session",
			wantMatched: "No messages returned",
			wantOK:      true,
		},
		{
			name:   "Harness prompt is not a failure",
			line:   "❯ ",
			wantOK: false,
		},
		{
			name:   "Shell prompt is not a failure",
			line:   "vbonnet@mac install-skills %",
			wantOK: false,
		},
		{
			name:   "Empty line",
			line:   "",
			wantOK: false,
		},
		{
			name:   "Unrelated error",
			line:   "fatal: not a git repository",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched, ok := containsResumeFailurePattern(tt.line)
			if ok != tt.wantOK {
				t.Errorf("containsResumeFailurePattern(%q) ok = %v, want %v", tt.line, ok, tt.wantOK)
			}
			if ok && matched != tt.wantMatched {
				t.Errorf("containsResumeFailurePattern(%q) matched = %q, want %q", tt.line, matched, tt.wantMatched)
			}
		})
	}
}

func TestClaudePromptPatterns(t *testing.T) {
	// Verify that all expected patterns are defined
	expectedPatterns := map[string]bool{
		"❯":  false, // Claude Code primary prompt
		"▌":  false,
		"> ": false,
		"$ ": false,
		"# ": false,
	}

	if len(ClaudePromptPatterns) != len(expectedPatterns) {
		t.Errorf("Expected %d patterns, got %d", len(expectedPatterns), len(ClaudePromptPatterns))
	}

	for _, pattern := range ClaudePromptPatterns {
		if _, exists := expectedPatterns[pattern]; !exists {
			t.Errorf("Unexpected pattern: %q", pattern)
		}
		expectedPatterns[pattern] = true
	}

	for pattern, found := range expectedPatterns {
		if !found {
			t.Errorf("Missing expected pattern: %q", pattern)
		}
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "No ANSI codes",
			input:    "Hello, world!",
			expected: "Hello, world!",
		},
		{
			name:     "Color codes",
			input:    "\x1b[31mRed text\x1b[0m",
			expected: "Red text",
		},
		{
			name:     "Bracketed paste mode",
			input:    "\x1b[?2004h\x1b[?1004h",
			expected: "",
		},
		{
			name:     "Complex escape sequences from Claude",
			input:    "\x1b[?2004h\x1b[?1004hContent here",
			expected: "Content here",
		},
		{
			name:     "Multiple CSI sequences",
			input:    "\x1b[38;2;215;119;87m ▐\x1b[48;2;0;0;0m▛███▜\x1b[49m▌\x1b[39m   Claude Code",
			expected: " ▐▛███▜▌   Claude Code",
		},
		{
			name:     "OSC sequences",
			input:    "\x1b]0;Title\x07Normal text",
			expected: "Normal text",
		},
		{
			name:     "Mixed sequences",
			input:    "\x1b[?2026h\r\n\x1b[38;2;215;119;87m ▐\x1b[48;2;0;0;0m▛███▜\x1b[49m▌\x1b[39m   \x1b[1mClaude Code\x1b[22m",
			expected: "\r\n ▐▛███▜▌   Claude Code",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Only escape sequences",
			input:    "\x1b[0m\x1b[31m\x1b[?2004h",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripANSI(tt.input)
			if result != tt.expected {
				t.Errorf("stripANSI(%q) = %q, expected %q",
					tt.input, result, tt.expected)
			}
		})
	}
}

// Integration Tests for WaitForClaudePrompt
// These tests verify the capture-pane polling approach works correctly

// TestWaitForClaudePromptRejectsShellDraft proves prompt-shaped shell text is
// not enough without live Claude process ownership.
func TestWaitForClaudePromptRejectsShellDraft(t *testing.T) {
	// Skip if tmux not available
	skipIfNoTmux(t)

	// Create a test session with Claude prompt
	sessionName := "test-prompt-detection-polling"
	socketPath := GetSocketPath()

	// Clean up any existing session
	exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName).Run()

	// Create session with fake Claude prompt
	cmd := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-s", sessionName)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}
	defer func() {
		exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName).Run()
	}()

	// Typing a bare ❯ into a shell is a user draft, not Claude readiness.
	sendCmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "printf '\\n❯ \\n'; sleep 30", "Enter")
	if err := sendCmd.Run(); err != nil {
		t.Fatalf("Failed to send prompt: %v", err)
	}

	if err := WaitForClaudePrompt(sessionName, time.Second); err == nil {
		t.Fatal("shell-rendered prompt was accepted without live Claude ownership")
	}
}

// TestWaitForClaudePromptTimeout tests timeout behavior
func TestWaitForClaudePromptTimeout(t *testing.T) {
	// Skip if tmux not available
	skipIfNoTmux(t)

	sessionName := "test-prompt-timeout"
	socketPath := GetSocketPath()

	// Clean up any existing session
	exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName).Run()

	// Create session WITHOUT Claude prompt
	cmd := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-s", sessionName)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}
	defer func() {
		exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName).Run()
	}()

	// Should timeout (no prompt)
	start := time.Now()
	err := WaitForClaudePrompt(sessionName, 2*time.Second)
	elapsed := time.Since(start)

	if err == nil {
		t.Error("Expected timeout error, got nil")
	}

	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("Expected timeout error, got: %v", err)
	}

	// Should timeout after approximately 2 seconds
	if elapsed < 1800*time.Millisecond || elapsed > 3000*time.Millisecond {
		t.Errorf("Timeout took %v, expected ~2s", elapsed)
	}
}

// TestCapturePaneReadsHistoricalOutput verifies capture-pane can read output
// that was printed before we started monitoring (the core issue we're fixing).
// This is the REGRESSION TEST that prevents going back to control mode.
func TestCapturePaneReadsHistoricalOutput(t *testing.T) {
	// Skip if tmux not available
	skipIfNoTmux(t)

	sessionName := "test-historical-output"
	socketPath := GetSocketPath()

	// Clean up any existing session
	exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName).Run()

	// Create session
	cmd := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-s", sessionName)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}
	defer func() {
		exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName).Run()
	}()

	// Send output BEFORE we start monitoring
	sendCmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "printf 'Historical output\\n❯ \\n'; sleep 30", "Enter")
	if err := sendCmd.Run(); err != nil {
		t.Fatalf("Failed to send output: %v", err)
	}

	// Wait for command to execute (increased for CI stability)
	time.Sleep(2 * time.Second)

	// Verify capture-pane can read output printed before the reader attached.
	captureCmd := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", sessionName, "-p")
	output, err := captureCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to capture pane: %v", err)
	}

	content := string(output)
	if !strings.Contains(content, "Historical output") {
		t.Errorf("capture-pane didn't read historical output: %q", content)
	}
	if !strings.Contains(content, "❯") {
		t.Errorf("capture-pane didn't read prompt: %q", content)
	}
}

// TestWaitForClaudePromptIgnoresBashPrompts verifies we don't false-positive on bash prompts
func TestWaitForClaudePromptIgnoresBashPrompts(t *testing.T) {
	// Skip if tmux not available
	skipIfNoTmux(t)

	sessionName := "test-bash-prompt"
	socketPath := GetSocketPath()

	// Clean up any existing session
	exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName).Run()

	// Create session with bash prompt (not Claude)
	cmd := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-s", sessionName)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}
	defer func() {
		exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName).Run()
	}()

	// Send bash-style prompts (should NOT match)
	bashPrompts := []string{"$", ">", "#", "user@host:~$ ", "root@host#"}
	for _, prompt := range bashPrompts {
		sendCmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "echo '"+prompt+"'", "Enter")
		if err := sendCmd.Run(); err != nil {
			t.Fatalf("Failed to send bash prompt %q: %v", prompt, err)
		}
	}

	time.Sleep(500 * time.Millisecond)

	// Should timeout (bash prompts should NOT be detected as Claude prompts)
	err := WaitForClaudePrompt(sessionName, 2*time.Second)
	if err == nil {
		t.Error("Expected timeout on bash prompts, but detection succeeded")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("Expected timeout error, got: %v", err)
	}
}

// BenchmarkWaitForClaudePrompt benchmarks the polling performance
func BenchmarkWaitForClaudePrompt(b *testing.B) {
	// Skip if tmux not available
	skipIfNoTmux(b)

	sessionName := "test-prompt-bench"
	socketPath := GetSocketPath()

	// Setup: Create session with prompt
	exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName).Run()
	cmd := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-s", sessionName)
	if err := cmd.Run(); err != nil {
		b.Fatalf("Failed to create test session: %v", err)
	}
	defer func() {
		exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName).Run()
	}()

	sendCmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "❯", "Space")
	if err := sendCmd.Run(); err != nil {
		b.Fatalf("Failed to send prompt: %v", err)
	}

	time.Sleep(500 * time.Millisecond) // Ensure prompt is visible

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := WaitForClaudePrompt(sessionName, 5*time.Second); err != nil {
			b.Errorf("Iteration %d failed: %v", i, err)
		}
	}
}
