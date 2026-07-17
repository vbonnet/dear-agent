package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/config"
	"github.com/vbonnet/dear-agent/internal/sandbox"
)

func TestShouldEnableSandbox(t *testing.T) {
	tests := []struct {
		name           string
		cfgEnabled     bool
		enableFlag     bool
		disableFlag    bool
		expectedResult bool
	}{
		{
			name:           "no-sandbox flag disables even when config enabled",
			cfgEnabled:     true,
			enableFlag:     false,
			disableFlag:    true,
			expectedResult: false,
		},
		{
			name:           "default config (enabled=true), no flags = sandbox ON",
			cfgEnabled:     true,
			enableFlag:     false,
			disableFlag:    false,
			expectedResult: true,
		},
		{
			name:           "config disabled, no flags = sandbox OFF",
			cfgEnabled:     false,
			enableFlag:     false,
			disableFlag:    false,
			expectedResult: false,
		},
		{
			name:           "no-sandbox overrides config",
			cfgEnabled:     true,
			enableFlag:     false,
			disableFlag:    true,
			expectedResult: false,
		},
		{
			name:           "enable flag still works for backward compat",
			cfgEnabled:     false,
			enableFlag:     true,
			disableFlag:    false,
			expectedResult: true,
		},
		{
			name:           "disable flag takes precedence over enable",
			cfgEnabled:     true,
			enableFlag:     true,
			disableFlag:    true,
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore original cfg
			originalCfg := cfg
			defer func() { cfg = originalCfg }()

			// Set test config
			cfg = &config.Config{
				Sandbox: config.SandboxConfig{
					Enabled: tt.cfgEnabled,
				},
			}

			result := shouldEnableSandbox(tt.enableFlag, tt.disableFlag)
			if result != tt.expectedResult {
				t.Errorf("shouldEnableSandbox() = %v, want %v", result, tt.expectedResult)
			}
		})
	}
}

func TestSandboxIntegration_Documentation(t *testing.T) {
	t.Log("Task 1.4: AGM Sandbox Integration - Session Creation (sandbox-by-default)")
	t.Log("")
	t.Log("IMPLEMENTATION:")
	t.Log("1. Sandbox is ON by default (config.Sandbox.Enabled=true)")
	t.Log("2. --sandbox flag REMOVED (breaking change)")
	t.Log("3. --no-sandbox flag disables sandbox")
	t.Log("4. --sandbox-provider selects provider (auto, overlayfs, apfs, claudecode-worktree, mock)")
	t.Log("5. SandboxSpec type added for provider-agnostic configuration")
	t.Log("6. ClaudeCodeProvider wraps Claude Code native worktree isolation")
	t.Log("")
	t.Log("FLAGS:")
	t.Log("--no-sandbox        Disable sandbox isolation (sandbox is ON by default)")
	t.Log("--sandbox-provider  Specify provider (auto, overlayfs, apfs, claudecode-worktree, mock)")
	t.Log("")
	t.Log("BEHAVIOR:")
	t.Log("- Default: Sandbox enabled (config.Sandbox.Enabled=true)")
	t.Log("- If --no-sandbox: Sandbox disabled")
	t.Log("- If sandbox enabled: workDir changed to sandbox merged path")
	t.Log("- If error during creation: Sandbox cleaned up automatically")
}

// withEmptySandboxRepoConfig points cfg at an empty Sandbox.Repos list (so
// resolveSandboxLowerDirs falls through to the workspace scan / workDir
// fallback) and restores the original cfg on cleanup.
func withEmptySandboxRepoConfig(t *testing.T) {
	t.Helper()
	originalCfg := cfg
	t.Cleanup(func() { cfg = originalCfg })
	cfg = &config.Config{Sandbox: config.SandboxConfig{Enabled: true, Repos: nil}}
}

// TestResolveSandboxLowerDirs_ConfiguredReposWins verifies cfg.Sandbox.Repos
// short-circuits resolution — workDir is never consulted, safe or not.
func TestResolveSandboxLowerDirs_ConfiguredReposWins(t *testing.T) {
	originalCfg := cfg
	defer func() { cfg = originalCfg }()
	cfg = &config.Config{Sandbox: config.SandboxConfig{Enabled: true, Repos: []string{"/configured/repo"}}}

	dirs, err := resolveSandboxLowerDirs("/does/not/matter")
	if err != nil {
		t.Fatalf("resolveSandboxLowerDirs() error = %v, want nil", err)
	}
	if len(dirs) != 1 || dirs[0] != "/configured/repo" {
		t.Errorf("resolveSandboxLowerDirs() = %v, want [/configured/repo]", dirs)
	}
}

// TestResolveSandboxLowerDirs_FailsLoudOnHomeDir is the ce-fmxv regression
// test: no configured repos, no scannable workspace, and workDir resolves to
// $HOME (the launchd-context failure mode — no --directory, no $PWD) must
// fail loud with ErrCodeNoLowerDirs instead of silently returning
// []string{$HOME}, which the apfs provider would then clone in full.
func TestResolveSandboxLowerDirs_FailsLoudOnHomeDir(t *testing.T) {
	withEmptySandboxRepoConfig(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	dirs, err := resolveSandboxLowerDirs(home)
	if err == nil {
		t.Fatalf("resolveSandboxLowerDirs(%s) = %v, nil; want an error (workDir is $HOME)", home, dirs)
	}
	var sbErr *sandbox.Error
	if !errors.As(err, &sbErr) || sbErr.Code != sandbox.ErrCodeNoLowerDirs {
		t.Errorf("resolveSandboxLowerDirs() error = %v, want *sandbox.Error{Code: ErrCodeNoLowerDirs}", err)
	}
	if dirs != nil {
		t.Errorf("resolveSandboxLowerDirs() dirs = %v, want nil on error", dirs)
	}
}

// TestResolveSandboxLowerDirs_FailsLoudOnNonRepoWorkDir covers the general
// case behind the $HOME check: any workDir that isn't itself a git
// repository is unsafe to clone as a sandbox lower dir.
func TestResolveSandboxLowerDirs_FailsLoudOnNonRepoWorkDir(t *testing.T) {
	withEmptySandboxRepoConfig(t)
	t.Setenv("HOME", t.TempDir())
	workDir := t.TempDir() // no .git inside

	_, err := resolveSandboxLowerDirs(workDir)
	if err == nil {
		t.Fatalf("resolveSandboxLowerDirs(%s) = nil error, want an error (not a git repo)", workDir)
	}
	var sbErr *sandbox.Error
	if !errors.As(err, &sbErr) || sbErr.Code != sandbox.ErrCodeNoLowerDirs {
		t.Errorf("resolveSandboxLowerDirs() error = %v, want *sandbox.Error{Code: ErrCodeNoLowerDirs}", err)
	}
}

// TestResolveSandboxLowerDirs_FallsBackToWorkDirWhenGitRepo verifies the
// legitimate fallback still works: a workDir that IS a real git repo (e.g.
// an explicit --directory pointing at a checkout) is accepted.
func TestResolveSandboxLowerDirs_FallsBackToWorkDirWhenGitRepo(t *testing.T) {
	withEmptySandboxRepoConfig(t)
	t.Setenv("HOME", t.TempDir())
	workDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workDir, ".git"), 0755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}

	dirs, err := resolveSandboxLowerDirs(workDir)
	if err != nil {
		t.Fatalf("resolveSandboxLowerDirs(%s) error = %v, want nil (valid git repo)", workDir, err)
	}
	if len(dirs) != 1 || dirs[0] != workDir {
		t.Errorf("resolveSandboxLowerDirs() = %v, want [%s]", dirs, workDir)
	}
}

// TestResolveSandboxLowerDirs_ScansWorkspaceRepos verifies the ~/src/ws/oss/repos
// scan still finds repos and takes priority over the workDir fallback, even
// when workDir itself would be unsafe.
func TestResolveSandboxLowerDirs_ScansWorkspaceRepos(t *testing.T) {
	withEmptySandboxRepoConfig(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoDir := filepath.Join(home, "src", "ws", "oss", "repos", "some-repo")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0755); err != nil {
		t.Fatalf("failed to create scanned repo: %v", err)
	}

	// workDir is deliberately unsafe (not a repo) to prove the scan wins.
	dirs, err := resolveSandboxLowerDirs(t.TempDir())
	if err != nil {
		t.Fatalf("resolveSandboxLowerDirs() error = %v, want nil (scan should find a repo)", err)
	}
	if len(dirs) != 1 || dirs[0] != repoDir {
		t.Errorf("resolveSandboxLowerDirs() = %v, want [%s]", dirs, repoDir)
	}
}
