package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestInstallHooksHelpIsHarnessNeutral(t *testing.T) {
	for _, text := range []string{installHooksCmd.Short, installHooksCmd.Long} {
		for _, forbidden := range []string{
			"Install Claude Code hooks",
			"Claude Code hooks that notify",
		} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("install-hooks help contains Claude-only wording %q in:\n%s", forbidden, text)
			}
		}
	}
	if !strings.Contains(installHooksCmd.Long, ".opencode/") {
		t.Fatalf("install-hooks help should mention OpenCode hook manifest location:\n%s", installHooksCmd.Long)
	}
	if !strings.Contains(installHooksCmd.Long, ".pi/") || !strings.Contains(installHooksCmd.Long, "private authorization extension") {
		t.Fatalf("install-hooks help should explain Pi hook and authorization surfaces:\n%s", installHooksCmd.Long)
	}
}

func TestSessionStartHookAssociatesClaudeUUIDBeforeReadyState(t *testing.T) {
	hook, err := hooksFS.ReadFile("hooks/session-start-agm-state-ready")
	if err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "agm-calls.log")
	fakeAGM := filepath.Join(binDir, "agm")
	if err := os.WriteFile(fakeAGM, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$HOOK_CALL_LOG\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("bash", "-c", string(hook))
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"AGM_SESSION_NAME=current-claude",
		// A newly launched child may inherit its parent's UUID. The payload UUID
		// identifies the session that actually invoked this hook and must win.
		"CLAUDE_SESSION_ID=550e8400-e29b-41d4-a716-446655440099",
		"HOOK_CALL_LOG="+logPath,
		"BASH_ENV=",
	)
	cmd.Stdin = strings.NewReader(`{"session_id":"550e8400-e29b-41d4-a716-446655440000"}`)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("session-start hook failed: %v\n%s", err, output)
	}

	calls := waitForHookCalls(t, logPath, 2)
	want := []string{
		"session associate current-claude --uuid 550e8400-e29b-41d4-a716-446655440000",
		"session state set current-claude READY --source hook",
	}
	if got := calls; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("hook calls = %#v, want %#v", got, want)
	}
}

func TestSessionStartHookRetriesAssociationUntilRegistration(t *testing.T) {
	hook, err := hooksFS.ReadFile("hooks/session-start-agm-state-ready")
	if err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	stateDir := t.TempDir()
	logPath := filepath.Join(stateDir, "agm-calls.log")
	countPath := filepath.Join(stateDir, "associate-count")
	fakeAGM := filepath.Join(binDir, "agm")
	fakeScript := `#!/bin/sh
if [ "$1 $2" = "session associate" ]; then
    count=0
    [ ! -f "$HOOK_COUNT_FILE" ] || count=$(cat "$HOOK_COUNT_FILE")
    count=$((count + 1))
    printf '%s\n' "$count" > "$HOOK_COUNT_FILE"
    printf '%s\n' "$*" >> "$HOOK_CALL_LOG"
    [ "$count" -ge 3 ] || exit 1
    exit 0
fi
printf '%s\n' "$*" >> "$HOOK_CALL_LOG"
`
	if err := os.WriteFile(fakeAGM, []byte(fakeScript), 0o700); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("bash", "-c", string(hook))
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"AGM_SESSION_NAME=detached-claude",
		"CLAUDE_SESSION_ID=550e8400-e29b-41d4-a716-446655440001",
		"HOOK_CALL_LOG="+logPath,
		"HOOK_COUNT_FILE="+countPath,
		"BASH_ENV=",
	)
	cmd.Stdin = strings.NewReader(`{}`)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("session-start hook failed: %v\n%s", err, output)
	}

	calls := waitForHookCalls(t, logPath, 4)
	wantAssociate := "session associate detached-claude --uuid 550e8400-e29b-41d4-a716-446655440001"
	wantReady := "session state set detached-claude READY --source hook"
	if calls[0] != wantAssociate || calls[1] != wantAssociate || calls[2] != wantAssociate || calls[3] != wantReady {
		t.Fatalf("hook retry calls = %#v, want three associations then READY", calls)
	}
}

func TestSessionStartHookRetryWindowCoversMaximumStartup(t *testing.T) {
	t.Parallel()

	hook, err := hooksFS.ReadFile("hooks/session-start-agm-state-ready")
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`while \[ "\$attempt" -lt ([0-9]+) \]`).FindSubmatch(hook)
	if len(match) != 2 {
		t.Fatal("SessionStart hook retry bound is not explicit")
	}
	attempts, err := strconv.Atoi(string(match[1]))
	if err != nil {
		t.Fatal(err)
	}
	if attempts < 91 {
		t.Fatalf("SessionStart hook retries %d times, want at least 91 to cover the 90-second startup window", attempts)
	}
}

func TestSessionStartHookHasSingleCanonicalSource(t *testing.T) {
	t.Parallel()

	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := findDearAgentRootFrom(workingDir)
	if repoRoot == "" {
		t.Fatal("could not resolve dear-agent repository root")
	}
	legacyPath := filepath.Join(repoRoot, "agm", "hooks", "cmd", "session-start-agm-state-ready")
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		if err != nil {
			t.Fatalf("inspect retired SessionStart hook: %v", err)
		}
		t.Fatalf("retired SessionStart hook still exists at %s", legacyPath)
	}
}

func waitForHookCalls(t *testing.T, logPath string, want int) []string {
	t.Helper()
	// The hook is asynchronous by contract. Allow enough scheduling slack for
	// the full CLI package, whose real tmux/process tests can heavily contend on
	// CI hosts, while still failing far before the 120-second production retry
	// window.
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		calls, err := os.ReadFile(logPath)
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(calls)), "\n")
			if len(lines) >= want {
				return lines
			}
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d hook calls", want)
	return nil
}

func TestAddHookRegistration(t *testing.T) {
	tests := []struct {
		name      string
		initial   map[string]interface{}
		reg       hookRegistration
		wantAdded bool
	}{
		{
			name:    "add to empty hooks map",
			initial: map[string]interface{}{},
			reg: hookRegistration{
				Event:   "PostToolUse",
				Command: "~/.claude/hooks/posttool-agm-state-notify",
				Timeout: 5,
			},
			wantAdded: true,
		},
		{
			name: "skip duplicate command",
			initial: map[string]interface{}{
				"PostToolUse": []interface{}{
					map[string]interface{}{
						"hooks": []interface{}{
							map[string]interface{}{
								"command": "~/.claude/hooks/posttool-agm-state-notify",
								"type":    "command",
							},
						},
					},
				},
			},
			reg: hookRegistration{
				Event:   "PostToolUse",
				Command: "~/.claude/hooks/posttool-agm-state-notify",
				Timeout: 5,
			},
			wantAdded: false,
		},
		{
			name: "add to existing event with other hooks",
			initial: map[string]interface{}{
				"PostToolUse": []interface{}{
					map[string]interface{}{
						"hooks": []interface{}{
							map[string]interface{}{
								"command": "some-other-hook",
								"type":    "command",
							},
						},
					},
				},
			},
			reg: hookRegistration{
				Event:   "PostToolUse",
				Command: "~/.claude/hooks/posttool-agm-state-notify",
				Timeout: 5,
			},
			wantAdded: true,
		},
		{
			name:    "add with matcher",
			initial: map[string]interface{}{},
			reg: hookRegistration{
				Event:   "PreToolUse",
				Command: "~/.claude/hooks/agm-pretool-test-session-guard",
				Timeout: 5,
				Matcher: "Bash",
			},
			wantAdded: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := addHookRegistration(tt.initial, tt.reg)
			if got != tt.wantAdded {
				t.Errorf("addHookRegistration() = %v, want %v", got, tt.wantAdded)
			}

			if tt.wantAdded {
				// Verify the hook was added to the correct event
				eventGroups, ok := tt.initial[tt.reg.Event].([]interface{})
				if !ok || len(eventGroups) == 0 {
					t.Fatal("hook event array not found after adding")
				}

				// Check last group has our command
				lastGroup := eventGroups[len(eventGroups)-1].(map[string]interface{})
				hooks := lastGroup["hooks"].([]interface{})
				lastHook := hooks[0].(map[string]interface{})
				if lastHook["command"] != tt.reg.Command {
					t.Errorf("command = %v, want %v", lastHook["command"], tt.reg.Command)
				}
				if tt.reg.Matcher != "" {
					if lastGroup["matcher"] != tt.reg.Matcher {
						t.Errorf("matcher = %v, want %v", lastGroup["matcher"], tt.reg.Matcher)
					}
				}
			}
		})
	}
}

func TestRegisterHooksInSettings(t *testing.T) {
	// Create a temp directory to act as home
	tmpHome := t.TempDir()
	claudeDir := filepath.Join(tmpHome, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}

	t.Run("creates settings.json if not exists", func(t *testing.T) {
		home := t.TempDir()
		if err := os.MkdirAll(filepath.Join(home, ".claude"), 0755); err != nil {
			t.Fatal(err)
		}

		regs := []hookRegistration{
			{
				Event:   "PostToolUse",
				Command: "~/.claude/hooks/posttool-agm-state-notify",
				Timeout: 5,
			},
		}

		count, err := registerHooksInSettings(home, regs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 1 {
			t.Errorf("registered count = %d, want 1", count)
		}

		// Verify settings.json was created
		data, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
		if err != nil {
			t.Fatal(err)
		}

		var settings map[string]interface{}
		if err := json.Unmarshal(data, &settings); err != nil {
			t.Fatal(err)
		}

		hooksMap, ok := settings["hooks"].(map[string]interface{})
		if !ok {
			t.Fatal("hooks key not found in settings")
		}
		postTool, ok := hooksMap["PostToolUse"].([]interface{})
		if !ok || len(postTool) != 1 {
			t.Fatal("PostToolUse not found or wrong length")
		}
	})

	t.Run("preserves existing settings", func(t *testing.T) {
		home := t.TempDir()
		if err := os.MkdirAll(filepath.Join(home, ".claude"), 0755); err != nil {
			t.Fatal(err)
		}

		// Write initial settings with existing data
		initial := map[string]interface{}{
			"model": "claude-opus-4-6",
			"hooks": map[string]interface{}{
				"PostToolUse": []interface{}{
					map[string]interface{}{
						"hooks": []interface{}{
							map[string]interface{}{
								"command": "existing-hook",
								"type":    "command",
							},
						},
					},
				},
			},
		}
		data, _ := json.MarshalIndent(initial, "", "  ")
		if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"), data, 0600); err != nil {
			t.Fatal(err)
		}

		regs := []hookRegistration{
			{
				Event:   "PostToolUse",
				Command: "~/.claude/hooks/posttool-agm-state-notify",
				Timeout: 5,
			},
		}

		count, err := registerHooksInSettings(home, regs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 1 {
			t.Errorf("registered count = %d, want 1", count)
		}

		// Verify model field preserved
		data, _ = os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
		var settings map[string]interface{}
		json.Unmarshal(data, &settings)
		if settings["model"] != "claude-opus-4-6" {
			t.Error("existing model field was lost")
		}

		// Verify both hooks present
		hooksMap := settings["hooks"].(map[string]interface{})
		postTool := hooksMap["PostToolUse"].([]interface{})
		if len(postTool) != 2 {
			t.Errorf("PostToolUse length = %d, want 2", len(postTool))
		}
	})

	t.Run("idempotent - no duplicates on second run", func(t *testing.T) {
		home := t.TempDir()
		if err := os.MkdirAll(filepath.Join(home, ".claude"), 0755); err != nil {
			t.Fatal(err)
		}

		regs := []hookRegistration{
			{
				Event:   "PostToolUse",
				Command: "~/.claude/hooks/posttool-agm-state-notify",
				Timeout: 5,
			},
			{
				Event:   "PreToolUse",
				Command: "~/.claude/hooks/pretool-agm-mode-tracker",
				Timeout: 5,
			},
		}

		// First run
		count1, err := registerHooksInSettings(home, regs)
		if err != nil {
			t.Fatalf("first run error: %v", err)
		}
		if count1 != 2 {
			t.Errorf("first run count = %d, want 2", count1)
		}

		// Second run - should add nothing
		count2, err := registerHooksInSettings(home, regs)
		if err != nil {
			t.Fatalf("second run error: %v", err)
		}
		if count2 != 0 {
			t.Errorf("second run count = %d, want 0", count2)
		}
	})

	t.Run("registers all AGM hooks", func(t *testing.T) {
		home := t.TempDir()
		if err := os.MkdirAll(filepath.Join(home, ".claude"), 0755); err != nil {
			t.Fatal(err)
		}

		regs := []hookRegistration{
			{Event: "PostToolUse", Command: "~/.claude/hooks/posttool-agm-state-notify", Timeout: 5},
			{Event: "PreToolUse", Command: "~/.claude/hooks/pretool-agm-mode-tracker", Timeout: 5},
			{Event: "PreToolUse", Command: "~/.claude/hooks/agm-pretool-test-session-guard", Timeout: 5},
			{Event: "SessionStart", Command: "~/.claude/hooks/session-start/agm-state-ready", Timeout: 5},
			{Event: "SessionStart", Command: "~/.claude/hooks/session-start/agm-plan-continuity", Timeout: 10},
		}

		count, err := registerHooksInSettings(home, regs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 5 {
			t.Errorf("registered count = %d, want 5", count)
		}

		data, _ := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
		var settings map[string]interface{}
		json.Unmarshal(data, &settings)

		hooksMap := settings["hooks"].(map[string]interface{})

		// Check event counts
		postTool := hooksMap["PostToolUse"].([]interface{})
		if len(postTool) != 1 {
			t.Errorf("PostToolUse groups = %d, want 1", len(postTool))
		}
		preTool := hooksMap["PreToolUse"].([]interface{})
		if len(preTool) != 2 {
			t.Errorf("PreToolUse groups = %d, want 2", len(preTool))
		}
		sessionStart := hooksMap["SessionStart"].([]interface{})
		if len(sessionStart) != 2 {
			t.Errorf("SessionStart groups = %d, want 2", len(sessionStart))
		}
	})
}
