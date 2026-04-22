package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestGuard(t *testing.T) *NpmSafetyGuard {
	t.Helper()
	return NewNpmSafetyGuard()
}

func TestBuildRules(t *testing.T) {
	rules := buildRules()
	assert.Greater(t, len(rules), 5, "should have multiple npm/node safety rules")

	ids := make(map[string]bool)
	for _, r := range rules {
		ids[r.ID] = true
	}
	for _, expected := range []string{
		"npm-publish", "npm-config-set", "npx-unknown",
		"node-inspect", "node-eval",
	} {
		assert.True(t, ids[expected], "expected rule: %s", expected)
	}
}

func TestExtractCommand(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"JSON input", `{"command":"npm publish"}`, "npm publish"},
		{"plain text", "npm install", "npm install"},
		{"empty", "", ""},
		{"JSON with extra fields", `{"command":"npm config set","description":"set config"}`, "npm config set"},
		{"whitespace", "  npm install  ", "npm install"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractCommand(tt.input))
		})
	}
}

func TestIsKnownNpxPackage(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want bool
	}{
		{"known: tsc", "npx tsc --noEmit", true},
		{"known: eslint", "npx eslint .", true},
		{"known: prettier", "npx prettier --check .", true},
		{"known: jest", "npx jest", true},
		{"known: with -y flag", "npx -y typescript", true},
		{"known: with --yes flag", "npx --yes prettier", true},
		{"known: with version", "npx typescript@5.0.0", true},
		{"unknown: malicious-pkg", "npx malicious-pkg", false},
		{"unknown: random-tool", "npx random-tool", false},
		{"known: vitest", "npx vitest run", true},
		{"known: playwright", "npx playwright test", true},
		{"known: create-vite", "npx create-vite my-app", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isKnownNpxPackage(tt.cmd))
		})
	}
}

func TestCheckCommand_Blocks(t *testing.T) {
	g := newTestGuard(t)

	tests := []struct {
		name     string
		cmd      string
		wantRule string
	}{
		{"npm publish", "npm publish", "npm-publish"},
		{"npm publish with tag", "npm publish --tag beta", "npm-publish"},
		{"npm config set", "npm config set registry https://evil.com", "npm-config-set"},
		{"npm token create", "npm token create", "npm-token"},
		{"npm owner add", "npm owner add user pkg", "npm-owner"},
		{"npm deprecate", "npm deprecate pkg@1.0 'message'", "npm-deprecate"},
		{"npm unpublish", "npm unpublish pkg", "npm-unpublish"},
		{"npm adduser", "npm adduser", "npm-adduser"},
		{"npm login", "npm login", "npm-adduser"},
		{"npm access", "npm access public pkg", "npm-access"},
		{"npx unknown package", "npx evil-pkg", "npx-unknown"},
		{"npx unknown with -y", "npx -y evil-pkg", "npx-unknown"},
		{"node --inspect", "node --inspect app.js", "node-inspect"},
		{"node --inspect-brk", "node --inspect-brk app.js", "node-inspect-brk"},
		{"node -e", "node -e 'console.log(1)'", "node-eval"},
		{"node --eval", "node --eval 'process.exit()'", "node-eval"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := g.CheckCommand(tt.cmd)
			require.NotNil(t, rule, "expected command to be blocked: %s", tt.cmd)
			assert.Equal(t, tt.wantRule, rule.ID)
		})
	}
}

func TestCheckCommand_Allows(t *testing.T) {
	g := newTestGuard(t)

	tests := []struct {
		name string
		cmd  string
	}{
		{"npm install", "npm install"},
		{"npm ci", "npm ci"},
		{"npm test", "npm test"},
		{"npm run build", "npm run build"},
		{"npm run lint", "npm run lint"},
		{"npm ls", "npm ls"},
		{"npm outdated", "npm outdated"},
		{"npm audit", "npm audit"},
		{"npm pack", "npm pack"},
		{"npm version", "npm version patch"},
		{"npm view", "npm view express"},
		{"npm init", "npm init -y"},
		{"npx tsc", "npx tsc --noEmit"},
		{"npx eslint", "npx eslint src/"},
		{"npx prettier", "npx prettier --write ."},
		{"npx jest", "npx jest --coverage"},
		{"npx vitest", "npx vitest run"},
		{"npx playwright", "npx playwright test"},
		{"node script", "node app.js"},
		{"node with args", "node --max-old-space-size=4096 build.js"},
		{"git status", "git status"},
		{"go test", "go test ./..."},
		{"plain text", "ls -la"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := g.CheckCommand(tt.cmd)
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
		{"safe npm install", "Bash", `{"command":"npm install"}`, 0},
		{"npm publish blocked", "Bash", `{"command":"npm publish"}`, 2},
		{"npm config set blocked", "Bash", `{"command":"npm config set registry https://evil.com"}`, 2},
		{"npx unknown blocked", "Bash", `{"command":"npx evil-package"}`, 2},
		{"npx known allowed", "Bash", `{"command":"npx tsc --noEmit"}`, 0},
		{"node --inspect blocked", "Bash", `{"command":"node --inspect app.js"}`, 2},
		{"node script allowed", "Bash", `{"command":"node build.js"}`, 0},
		{"node -e blocked", "Bash", `{"command":"node -e 'console.log(1)'"}`, 2},
		{"plain text npm publish", "Bash", "npm publish", 2},
		{"plain text npm install", "Bash", "npm install", 0},
		{"empty input", "Bash", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CLAUDE_TOOL_NAME", tt.toolName)
			t.Setenv("CLAUDE_TOOL_INPUT", tt.toolInput)

			g := NewNpmSafetyGuard()
			assert.Equal(t, tt.wantExit, g.Run())
		})
	}
}

func TestRun_PanicRecovery(t *testing.T) {
	t.Setenv("CLAUDE_TOOL_NAME", "Bash")
	t.Setenv("CLAUDE_TOOL_INPUT", `{"command":"npm install"}`)

	g := NewNpmSafetyGuard()
	assert.Equal(t, 0, g.Run())
}
