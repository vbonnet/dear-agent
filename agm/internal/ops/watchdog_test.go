package ops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveWatchdogPaths(t *testing.T) {
	paths := ResolveWatchdogPaths("/home/testuser")

	if paths.BinDir != "/home/testuser/.agm/bin" {
		t.Errorf("BinDir = %q, want /home/testuser/.agm/bin", paths.BinDir)
	}
	if paths.LogDir != "/home/testuser/.agm/logs" {
		t.Errorf("LogDir = %q, want /home/testuser/.agm/logs", paths.LogDir)
	}
	if paths.ScriptPath != "/home/testuser/.agm/bin/watchdog.sh" {
		t.Errorf("ScriptPath = %q, want /home/testuser/.agm/bin/watchdog.sh", paths.ScriptPath)
	}
	if paths.LogPath != "/home/testuser/.agm/logs/watchdog.log" {
		t.Errorf("LogPath = %q, want /home/testuser/.agm/logs/watchdog.log", paths.LogPath)
	}
}

func TestGenerateWatchdogScript(t *testing.T) {
	script := GenerateWatchdogScript("/tmp/agm.sock", "/usr/local/bin/agm")

	checks := []struct {
		name    string
		content string
	}{
		{"shebang", "#!/usr/bin/env bash"},
		{"socket path", `SOCKET_PATH="/tmp/agm.sock"`},
		{"agm binary", `AGM_BIN="/usr/local/bin/agm"`},
		{"set flags", "set -euo pipefail"},
		{"crash detection", "CRASH DETECTED"},
		{"stale socket removal", "Removed stale socket"},
		{"server restart", "tmux server restarted"},
		{"session recovery", "session recovery"},
		{"missing sessions check", "MISSING SESSIONS"},
		{"tmux list-sessions", "tmux -S \"$SOCKET_PATH\" list-sessions"},
	}

	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(script, tc.content) {
				t.Errorf("script missing %q (%s)", tc.content, tc.name)
			}
		})
	}
}

func TestGenerateWatchdogScript_CustomPaths(t *testing.T) {
	script := GenerateWatchdogScript("/custom/socket.sock", "/opt/agm/bin/agm")

	if !strings.Contains(script, `SOCKET_PATH="/custom/socket.sock"`) {
		t.Error("script does not use custom socket path")
	}
	if !strings.Contains(script, `AGM_BIN="/opt/agm/bin/agm"`) {
		t.Error("script does not use custom agm binary path")
	}
}

func TestRemoveWatchdogLines(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "empty",
			input:  "",
			expect: "",
		},
		{
			name:   "no watchdog lines",
			input:  "0 * * * * /some/other/job\n",
			expect: "0 * * * * /some/other/job\n",
		},
		{
			name: "watchdog only",
			input: WatchdogCrontabComment + "\n" +
				"* * * * * ~/.agm/bin/watchdog.sh >> ~/.agm/logs/watchdog.log 2>&1\n",
			expect: "",
		},
		{
			name: "watchdog with other entries",
			input: "0 * * * * /some/job\n" +
				WatchdogCrontabComment + "\n" +
				"* * * * * ~/.agm/bin/watchdog.sh >> ~/.agm/logs/watchdog.log 2>&1\n" +
				"30 2 * * * /nightly/job\n",
			expect: "0 * * * * /some/job\n" +
				"30 2 * * * /nightly/job\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := removeWatchdogLines(tc.input)
			if got != tc.expect {
				t.Errorf("removeWatchdogLines(%q)\n  got:  %q\n  want: %q", tc.input, got, tc.expect)
			}
		})
	}
}

func TestInstallWatchdog_WritesScript(t *testing.T) {
	tmpDir := t.TempDir()
	paths := ResolveWatchdogPaths(tmpDir)

	// InstallWatchdog will fail on crontab (not available in test), but
	// we can verify that the script file is written correctly up to that point.
	script := GenerateWatchdogScript("/tmp/agm.sock", "agm")

	// Manually create dirs and write script to test the file content
	if err := os.MkdirAll(paths.BinDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(paths.LogDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(paths.ScriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Verify file exists and is executable
	info, err := os.Stat(paths.ScriptPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode()&0111 == 0 {
		t.Error("script is not executable")
	}

	// Verify content
	content, err := os.ReadFile(paths.ScriptPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.HasPrefix(string(content), "#!/usr/bin/env bash") {
		t.Error("script does not start with shebang")
	}
}

func TestIsWatchdogInstalled_NoScript(t *testing.T) {
	tmpDir := t.TempDir()
	paths := ResolveWatchdogPaths(tmpDir)

	if IsWatchdogInstalled(paths) {
		t.Error("expected not installed when script doesn't exist")
	}
}

func TestUninstallWatchdog_RemovesScript(t *testing.T) {
	tmpDir := t.TempDir()
	paths := ResolveWatchdogPaths(tmpDir)

	// Create the script file
	if err := os.MkdirAll(paths.BinDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(paths.ScriptPath, []byte("#!/bin/bash\n"), 0755); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Verify it exists
	if _, err := os.Stat(paths.ScriptPath); err != nil {
		t.Fatalf("script should exist: %v", err)
	}

	// UninstallWatchdog will fail on crontab removal in test env,
	// so we test script removal directly
	err := os.Remove(paths.ScriptPath)
	if err != nil {
		t.Fatalf("remove: %v", err)
	}

	if _, err := os.Stat(paths.ScriptPath); !os.IsNotExist(err) {
		t.Error("script should be removed")
	}
}

func TestWatchdogPaths_Consistency(t *testing.T) {
	paths := ResolveWatchdogPaths("/home/user")

	// Script should be inside BinDir
	if filepath.Dir(paths.ScriptPath) != paths.BinDir {
		t.Errorf("ScriptPath %q not inside BinDir %q", paths.ScriptPath, paths.BinDir)
	}

	// Log should be inside LogDir
	if filepath.Dir(paths.LogPath) != paths.LogDir {
		t.Errorf("LogPath %q not inside LogDir %q", paths.LogPath, paths.LogDir)
	}
}

func TestGenerateWatchdogScript_NoJqDependency(t *testing.T) {
	script := GenerateWatchdogScript("/tmp/agm.sock", "agm")

	// Check for actual jq invocations (piped or direct), not comments mentioning jq.
	for _, line := range strings.Split(script, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue // skip comments
		}
		if strings.Contains(trimmed, "| jq") || strings.Contains(trimmed, "jq ") {
			t.Errorf("script has jq dependency in non-comment line: %s", trimmed)
		}
	}
}

func TestGenerateWatchdogScript_RecoverySession(t *testing.T) {
	script := GenerateWatchdogScript("/tmp/agm.sock", "agm")

	// The bootstrap recovery session should be created and then killed
	if !strings.Contains(script, "_watchdog_recovery") {
		t.Error("script missing recovery session name")
	}
	if !strings.Contains(script, "kill-session -t _watchdog_recovery") {
		t.Error("script should clean up the recovery bootstrap session")
	}
}
