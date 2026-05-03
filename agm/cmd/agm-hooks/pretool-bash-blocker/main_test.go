package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestBlocker(t *testing.T) *BashBlocker {
	t.Helper()
	b, err := NewBashBlocker()
	require.NoError(t, err, "NewBashBlocker must succeed — patterns YAML must be embedded")
	return b
}

func TestLoadRules(t *testing.T) {
	rules, err := loadRules()
	require.NoError(t, err)
	assert.Greater(t, len(rules), 10, "should load many active rules from YAML")

	// Verify brace-expansion is first (custom rule).
	assert.Equal(t, "brace-expansion", rules[0].ID)

	// Verify some critical rules are present.
	ids := make(map[string]bool)
	for _, r := range rules {
		ids[r.ID] = true
	}
	for _, expected := range []string{
		"subshell-cd", "cd-command", "command-substitution",
		"rm-recursive", "git-checkout-main", "git-no-verify",
		"sensitive-dotdir-access", "sed-in-place",
	} {
		assert.True(t, ids[expected], "expected active rule: %s", expected)
	}
}

func TestRelaxedPatternsNotLoaded(t *testing.T) {
	rules, err := loadRules()
	require.NoError(t, err)

	ids := make(map[string]bool)
	for _, r := range rules {
		ids[r.ID] = true
	}
	for _, relaxed := range []string{
		"inline-env-var-prefix", "command-chaining", "error-suppression-hook",
		"bash-conditional", "for-loop", "file-operations", "text-processing",
		"backtick-substitution",
	} {
		assert.False(t, ids[relaxed], "relaxed rule should not be loaded: %s", relaxed)
	}
}

func TestExtractCommand(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"JSON input", `{"command":"rm -rf /"}`, "rm -rf /"},
		{"plain text", "git status", "git status"},
		{"empty", "", ""},
		{"JSON with extra fields", `{"command":"git push","description":"push"}`, "git push"},
		{"whitespace", "  git status  ", "git status"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractCommand(tt.input))
		})
	}
}

func TestIsDangerousBraceExpansion(t *testing.T) {
	tests := []struct {
		name  string
		cmd   string
		match string
		want  bool
	}{
		{"dangerous: rm in braces", "{rm,-rf} /tmp/foo", "{rm,-rf}", true},
		{"dangerous: flag in braces", "{cat,-n} file.txt", "{cat,-n}", true},
		{"dangerous: chmod in braces", "{chmod,777} file", "{chmod,777}", true},
		{"dangerous: kill in braces", "{kill,-9} 1234", "{kill,-9}", true},
		{"safe: directory names", "ls internal/{verify,dolt}/", "{verify,dolt}", false},
		{"safe: Go packages", "go test ./cmd/{foo,bar}/...", "{foo,bar}", false},
		{"safe: file extensions", "ls *.{go,mod}", "{go,mod}", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isDangerousBraceExpansion(tt.cmd, tt.match))
		})
	}
}

func TestIsExemptBraceExpansion(t *testing.T) {
	tests := []struct {
		name  string
		cmd   string
		match string
		want  bool
	}{
		{"exempt: Go path", "go test ./internal/{verify,dolt}/...", "{verify,dolt}", true},
		{"exempt: directory listing", "ls internal/{config,util}/", "{config,util}", true},
		{"exempt: nested Go path", "GOWORK=off go build ./cmd/{foo,bar}", "{foo,bar}", true},
		{"not exempt: dangerous at start", "{rm,-rf} /tmp", "{rm,-rf}", false},
		{"not exempt: no path context", "echo {a,b,c}", "{a,b,c}", false},
		{"not exempt: dangerous with path", "/{rm,-rf}", "{rm,-rf}", false},
		{"exempt: multiple path segments", "cat src/ws/oss/repos/{engram,ai-tools}/README.md", "{engram,ai-tools}", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isExemptBraceExpansion(tt.cmd, tt.match))
		})
	}
}

func TestCheckCommand_CriticalBlocks(t *testing.T) {
	b := newTestBlocker(t)

	tests := []struct {
		name     string
		cmd      string
		wantRule string
	}{
		{"rm -rf /", "rm -rf /", "rm-recursive"},
		{"rm -r directory", "rm -r /tmp/important", "rm-recursive"},
		{"command substitution $()", "agm send --prompt \"$(cat secret)\"", "command-substitution"},
		{"git checkout main", "git checkout main", "git-checkout-main"},
		{"git --no-verify", "git commit --no-verify -m 'skip hooks'", "git-no-verify"},
		{"git push --no-verify", "git push --no-verify", "git-no-verify"},
		{"sensitive dotdir", "cat ~/.ssh/id_rsa", "sensitive-dotdir-access"},
		{"git stash", "git stash", "git-stash"},
		{"git add .", "git add .", "git-add-broad"},
		{"git add -A", "git add -A", "git-add-broad"},
		{"sed -i", "sed -i 's/foo/bar/' file.txt", "sed-in-place"},
		{"cd command", "cd /tmp", "cd-command"},
		{"subshell cd", "(cd /tmp && pwd)", "subshell-cd"},
		{"find command", "find . -name '*.go'", "find-file-search"},
		{"git switch", "git switch main", "git-switch"},
		{"redirect to /etc", "echo bad > /etc/hosts", "redirect-system-path"},
		{"python -c", "python3 -c 'import os'", "python-oneliner"},
		{"while loop", "while true; do sleep 1; done", "while-loop"},
		{"AGM_SKIP_TEST_GATE", "AGM_SKIP_TEST_GATE=1 go test", "agm-skip-test-gate"},
		{"brace expansion", "echo {a,b,c}", "brace-expansion"},
		{"dangerous brace", "{rm,-rf} /tmp/foo", "brace-expansion"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := b.CheckCommand(tt.cmd)
			require.NotNil(t, rule, "expected command to be blocked: %s", tt.cmd)
			assert.Equal(t, tt.wantRule, rule.ID)
		})
	}
}

func TestCheckCommand_Allows(t *testing.T) {
	b := newTestBlocker(t)

	tests := []struct {
		name string
		cmd  string
	}{
		{"git status", "git status"},
		{"go test", "go test ./..."},
		{"git commit", "git commit -m 'fix bug'"},
		{"git add specific files", "git add main.go util.go"},
		{"git push", "git push origin feature-branch"},
		{"npm install", "npm install"},
		{"Go path brace", "go test ./internal/{verify,dolt}/..."},
		{"python script", "python3 script.py"},
		{"git checkout branch", "git checkout -b feature"},
		{"git checkout file", "git checkout -- file.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := b.CheckCommand(tt.cmd)
			assert.Nil(t, rule, "expected command to be allowed, but blocked by %v", rule)
		})
	}
}

func TestRun_Integration(t *testing.T) {
	tests := []struct {
		name      string
		toolName  string
		toolInput string
		wantExit  int
	}{
		{"non-Bash tool", "Read", "anything", 0},
		{"safe command", "Bash", `{"command":"git status"}`, 0},
		{"rm -rf blocked", "Bash", `{"command":"rm -rf /"}`, 2},
		{"command substitution blocked", "Bash", `{"command":"echo $(date)"}`, 2},
		{"plain text input", "Bash", "git status", 0},
		{"plain text rm -rf", "Bash", "rm -rf /tmp/foo", 2},
		{"Go path brace allowed", "Bash", `{"command":"go test ./internal/{verify,dolt}/..."}`, 0},
		{"dangerous brace blocked", "Bash", `{"command":"{rm,-rf} /tmp/foo"}`, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CLAUDE_TOOL_NAME", tt.toolName)
			t.Setenv("CLAUDE_TOOL_INPUT", tt.toolInput)
			defer func() {
				os.Unsetenv("CLAUDE_TOOL_NAME")
				os.Unsetenv("CLAUDE_TOOL_INPUT")
			}()

			b, err := NewBashBlocker()
			require.NoError(t, err)
			assert.Equal(t, tt.wantExit, b.Run())
		})
	}
}

func TestBraceExpansionRegex(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantMatch bool
	}{
		{"simple brace expansion", "{a,b}", true},
		{"multi-element", "{a,b,c,d}", true},
		{"brace in path", "internal/{verify,dolt}/", true},
		{"no comma", "{single}", false},
		{"no braces", "a,b", false},
		{"JSON-like", `{"key": "value"}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantMatch, braceExpansionRegex.MatchString(tt.input))
		})
	}
}
