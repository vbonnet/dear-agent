package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	t.Log("4. --sandbox-provider selects a materializing provider (auto, bubblewrap, overlayfs, gvisor, apfs, mock)")
	t.Log("")
	t.Log("FLAGS:")
	t.Log("--no-sandbox        Disable sandbox isolation (sandbox is ON by default)")
	t.Log("--sandbox-provider  Specify provider (auto, bubblewrap, overlayfs, gvisor, apfs, mock)")
	t.Log("")
	t.Log("BEHAVIOR:")
	t.Log("- Default: Sandbox enabled (config.Sandbox.Enabled=true)")
	t.Log("- If --no-sandbox: Sandbox disabled")
	t.Log("- If sandbox enabled: workDir changed to the provider-mapped project directory")
	t.Log("- If error during creation: Sandbox cleaned up automatically")

	flag := newCmd.Flags().Lookup("sandbox-provider")
	if flag == nil {
		t.Fatal("new command is missing --sandbox-provider")
	}
	for _, name := range []string{"auto", "bubblewrap", "overlayfs", "gvisor", "apfs", "mock"} {
		if !strings.Contains(flag.Usage, name) {
			t.Errorf("--sandbox-provider usage %q omits %q", flag.Usage, name)
		}
	}
	if strings.Contains(flag.Usage, "claudecode-worktree") {
		t.Errorf("--sandbox-provider usage still advertises retired provider: %q", flag.Usage)
	}
}

func TestProvisionSandboxRejectsRetiredClaudeCodeProviderBeforeWorkspaceCreation(t *testing.T) {
	testHome := t.TempDir()
	t.Setenv("HOME", testHome)

	const sessionID = "retired-claudecode-provider"
	got, err := provisionSandbox(context.Background(), "claudecode-worktree", sessionID, testHome)
	if err == nil {
		t.Fatal("provisionSandbox() error = nil, want retired provider rejection")
	}
	if got != nil {
		t.Fatalf("provisionSandbox() sandbox = %#v, want nil", got)
	}
	var sandboxErr *sandbox.Error
	if !errors.As(err, &sandboxErr) || sandboxErr.Code != sandbox.ErrCodeUnsupportedPlatform {
		t.Fatalf("provisionSandbox() error = %v, want unsupported-platform sandbox error", err)
	}
	if _, statErr := os.Stat(filepath.Join(testHome, ".agm", "sandboxes", sessionID)); !os.IsNotExist(statErr) {
		t.Fatalf("retired provider materialized a workspace: %v", statErr)
	}
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

// TestUnsafeSandboxFallbackReason_EmptyWorkDir is the Gemini-review
// regression test: an empty workDir must never fall through to the
// os.Stat(filepath.Join("", ".git")) check, which resolves to ".git" in the
// process's current working directory — if that happens to be a git repo,
// the empty-workDir case would be misclassified as safe.
func TestUnsafeSandboxFallbackReason_EmptyWorkDir(t *testing.T) {
	reason := unsafeSandboxFallbackReason("")
	if reason == "" {
		t.Fatal("unsafeSandboxFallbackReason(\"\") = \"\", want a non-empty reason (empty workDir must never be treated as safe)")
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
	wantRoot, err := filepath.EvalSymlinks(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 || dirs[0] != wantRoot {
		t.Errorf("resolveSandboxLowerDirs() = %v, want [%s]", dirs, wantRoot)
	}
}

func TestResolveSandboxLowerDirs_FallsBackToContainingGitRepoForSubdirectory(t *testing.T) {
	withEmptySandboxRepoConfig(t)
	t.Setenv("HOME", t.TempDir())
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	workDir := filepath.Join(repoRoot, "agm", "cmd", "agm")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}

	dirs, err := resolveSandboxLowerDirs(workDir)
	if err != nil {
		t.Fatalf("resolveSandboxLowerDirs(%s) error = %v", workDir, err)
	}
	wantRoot, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 || dirs[0] != wantRoot {
		t.Fatalf("resolveSandboxLowerDirs() = %v, want containing repo [%s]", dirs, wantRoot)
	}
}

func TestFindPrimaryRepoUsesRequestedDirectoryInsteadOfProcessCWD(t *testing.T) {
	firstRepo := t.TempDir()
	targetRepo := t.TempDir()
	targetWorkDir := filepath.Join(targetRepo, "wayfinder")
	if err := os.MkdirAll(targetWorkDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if got := findPrimaryRepo([]string{firstRepo, targetRepo}, targetWorkDir); got != targetRepo {
		t.Fatalf("findPrimaryRepo() = %q, want requested repo %q", got, targetRepo)
	}
}

func TestMaybeProvisionSandboxReturnsProviderMappedWorkingDirectory(t *testing.T) {
	originalCfg := cfg
	originalEnableSandbox := enableSandbox
	originalNoSandbox := noSandbox
	originalProvider := sandboxProvider
	t.Cleanup(func() {
		cfg = originalCfg
		enableSandbox = originalEnableSandbox
		noSandbox = originalNoSandbox
		sandboxProvider = originalProvider
	})

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	repoRoot := t.TempDir()
	requestedDir := filepath.Join(repoRoot, ".agents", "skills")
	if err := os.MkdirAll(requestedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg = &config.Config{Sandbox: config.SandboxConfig{Enabled: true, Repos: []string{repoRoot}}}
	enableSandbox = false
	noSandbox = false
	sandboxProvider = "mock"

	sandboxInfo, workingDir, err := maybeProvisionSandbox(context.Background(), "mapped-session", requestedDir)
	if err != nil {
		t.Fatalf("maybeProvisionSandbox() error = %v", err)
	}
	wantWorkingDir := filepath.Join(homeDir, ".agm", "sandboxes", "mapped-session", "merged", ".agents", "skills")
	if workingDir != wantWorkingDir {
		t.Fatalf("workingDir = %q, want %q", workingDir, wantWorkingDir)
	}
	if sandboxInfo == nil || sandboxInfo.WorkingDir != wantWorkingDir {
		t.Fatalf("SandboxConfig = %+v, want persisted mapped working directory %q", sandboxInfo, wantWorkingDir)
	}
	if sandboxInfo.MergedPath == sandboxInfo.WorkingDir {
		t.Fatalf("merged root %q must remain distinct from nested working directory", sandboxInfo.MergedPath)
	}
}

type emptyWorkingDirProvider struct {
	destroyed  *bool
	destroyErr error
}

func (p *emptyWorkingDirProvider) Create(_ context.Context, req sandbox.SandboxRequest) (*sandbox.Sandbox, error) {
	return &sandbox.Sandbox{
		ID:         req.SessionID,
		MergedPath: filepath.Join(req.WorkspaceDir, "merged"),
		CreatedAt:  time.Now(),
	}, nil
}

func (p *emptyWorkingDirProvider) Destroy(_ context.Context, _ string) error {
	*p.destroyed = true
	return p.destroyErr
}

func TestProvisionSandboxPreservesContractAndCleanupFailures(t *testing.T) {
	originalCfg := cfg
	t.Cleanup(func() { cfg = originalCfg })
	t.Setenv("HOME", t.TempDir())
	repoRoot := t.TempDir()
	destroyed := false
	cleanupErr := errors.New("fixture cleanup failure")
	sandbox.RegisterProvider("empty-working-dir-cleanup-failure-test", func() sandbox.Provider {
		return &emptyWorkingDirProvider{destroyed: &destroyed, destroyErr: cleanupErr}
	})
	cfg = &config.Config{Sandbox: config.SandboxConfig{Enabled: true, Repos: []string{repoRoot}}}

	_, err := provisionSandbox(context.Background(), "empty-working-dir-cleanup-failure-test", "contract-session", repoRoot)
	if err == nil || !errors.Is(err, cleanupErr) {
		t.Fatalf("provisionSandbox() error = %v, want joined cleanup failure", err)
	}
	if !destroyed {
		t.Fatal("provisionSandbox() did not attempt cleanup after provider contract failure")
	}
}

func (*emptyWorkingDirProvider) Validate(context.Context, string) error { return nil }
func (*emptyWorkingDirProvider) Name() string                           { return "empty-working-dir-test" }

func TestProvisionSandboxCleansUpProviderThatViolatesWorkingDirectoryContract(t *testing.T) {
	originalCfg := cfg
	t.Cleanup(func() { cfg = originalCfg })
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	repoRoot := t.TempDir()
	destroyed := false
	sandbox.RegisterProvider("empty-working-dir-test", func() sandbox.Provider {
		return &emptyWorkingDirProvider{destroyed: &destroyed}
	})
	cfg = &config.Config{Sandbox: config.SandboxConfig{Enabled: true, Repos: []string{repoRoot}}}

	_, err := provisionSandbox(context.Background(), "empty-working-dir-test", "contract-session", repoRoot)
	if err == nil {
		t.Fatal("provisionSandbox() error = nil, want provider contract failure")
	}
	if !destroyed {
		t.Fatal("provisionSandbox() did not clean up workspace after provider contract failure")
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
