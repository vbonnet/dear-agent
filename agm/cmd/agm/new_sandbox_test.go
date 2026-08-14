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
	"github.com/vbonnet/dear-agent/agm/internal/ui"
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
	_, authority := loadSandboxRuntimeAuthority(t, testHome)

	const sessionID = "retired-claudecode-provider"
	got, err := provisionSandbox(context.Background(), authority, "claudecode-worktree", sessionID, testHome)
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

func TestProvisionSandboxUsesCapturedAuthority(t *testing.T) {
	for _, mode := range []string{"dotfile", "centralized"} {
		t.Run(mode, func(t *testing.T) {
			originalCfg := cfg
			t.Cleanup(func() { cfg = originalCfg })
			for _, key := range []string{"ENGRAM_TEST_MODE", "ENGRAM_TEST_WORKSPACE", "ENGRAM_WORKSPACE"} {
				t.Setenv(key, "")
			}

			root := t.TempDir()
			physicalHome := mkdirPhysical(t, filepath.Join(root, "physical-home"))
			driftHome := mkdirPhysical(t, filepath.Join(root, "drift-home"))
			driftSentinel := filepath.Join(driftHome, "sentinel")
			if err := os.WriteFile(driftSentinel, []byte("preserve"), 0o600); err != nil {
				t.Fatal(err)
			}
			logicalHome := filepath.Join(root, "logical-home")
			if err := os.Symlink(physicalHome, logicalHome); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			t.Setenv("HOME", logicalHome)
			repoRoot := mkdirPhysical(t, filepath.Join(root, "repo"))
			driftWorkspace := mkdirPhysical(t, filepath.Join(root, "drift-workspace"))

			configPath := ""
			selectedWorkspace := ""
			if mode == "centralized" {
				selectedWorkspace = mkdirPhysical(t, filepath.Join(root, "selected-workspace"))
				t.Setenv("ENGRAM_WORKSPACE", selectedWorkspace)
				configPath = filepath.Join(root, "config.yaml")
				contents := "storage:\n  mode: centralized\n  workspace: selected\n  relative_path: .agm-work\n"
				if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			loaded, err := config.Load(configPath)
			if err != nil {
				t.Fatalf("config.Load() error = %v", err)
			}
			loaded.Sandbox = config.SandboxConfig{
				Enabled:    true,
				Repos:      []string{repoRoot},
				Onboarding: config.OnboardingConfig{Enabled: false},
			}
			cfg = loaded
			authority, err := loaded.RuntimeAuthority()
			if err != nil {
				t.Fatal(err)
			}

			if err := os.Remove(logicalHome); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(driftHome, logicalHome); err != nil {
				t.Fatal(err)
			}
			t.Setenv("HOME", driftHome)
			t.Setenv("ENGRAM_WORKSPACE", driftWorkspace)
			t.Chdir(driftWorkspace)
			loaded.Storage = config.StorageConfig{
				Mode:         "centralized",
				Workspace:    driftWorkspace,
				RelativePath: ".drift",
			}

			const sessionID = "captured-authority-session"
			sandboxInfo, err := provisionSandbox(context.Background(), authority, "mock", sessionID, repoRoot)
			if err != nil {
				t.Fatalf("provisionSandbox() error = %v", err)
			}
			sandboxRoot, err := authority.Sandboxes()
			if err != nil {
				t.Fatal(err)
			}
			workspacePath, err := sandboxRoot.Workspace(sessionID)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := sandboxInfo.MergedPath, filepath.Join(workspacePath, "merged"); got != want {
				t.Fatalf("MergedPath = %q, want authority-derived %q", got, want)
			}
			if mode == "centralized" && !strings.HasPrefix(workspacePath, filepath.Join(selectedWorkspace, ".agm-work", "sandboxes")+string(filepath.Separator)) {
				t.Fatalf("centralized workspace %q is outside selected .agm-work", workspacePath)
			}
			if _, err := os.Lstat(filepath.Join(driftHome, ".agm")); !os.IsNotExist(err) {
				t.Fatalf("sandbox provisioning mutated drift HOME: %v", err)
			}
			if contents, err := os.ReadFile(driftSentinel); err != nil {
				t.Fatal(err)
			} else if string(contents) != "preserve" {
				t.Fatalf("drift sentinel = %q, want preserved", contents)
			}
		})
	}
}

func TestProvisionSandboxRejectsZeroAuthorityBeforeProviderLookup(t *testing.T) {
	originalCfg := cfg
	t.Cleanup(func() { cfg = originalCfg })
	cfg = &config.Config{Sandbox: config.SandboxConfig{Repos: []string{t.TempDir()}}}

	_, err := provisionSandbox(
		context.Background(),
		config.RuntimeAuthority{},
		"intentionally-unknown-provider",
		"session",
		t.TempDir(),
	)
	if !errors.Is(err, config.ErrRuntimeAuthorityUnavailable) {
		t.Fatalf("provisionSandbox() error = %v, want ErrRuntimeAuthorityUnavailable", err)
	}
}

func TestProvisionSandboxRejectsEscapingSessionBeforeProviderLookup(t *testing.T) {
	originalCfg := cfg
	t.Cleanup(func() { cfg = originalCfg })

	home := mkdirPhysical(t, t.TempDir())
	loaded, authority := loadSandboxRuntimeAuthority(t, home)
	loaded.Sandbox = config.SandboxConfig{
		Enabled:    true,
		Repos:      []string{t.TempDir()},
		Onboarding: config.OnboardingConfig{Enabled: false},
	}
	cfg = loaded

	sandboxRoot := filepath.Join(home, ".agm", "sandboxes")
	if err := os.MkdirAll(sandboxRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	external := mkdirPhysical(t, filepath.Join(t.TempDir(), "external"))
	const sessionID = "escaping-session"
	if err := os.Symlink(external, filepath.Join(sandboxRoot, sessionID)); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, err := provisionSandbox(
		context.Background(),
		authority,
		"intentionally-unknown-provider",
		sessionID,
		t.TempDir(),
	)
	if err == nil {
		t.Fatal("provisionSandbox() error = nil, want session authority rejection")
	}
	if strings.Contains(err.Error(), "failed to create sandbox provider") {
		t.Fatalf("provisionSandbox() looked up provider before session authority validation: %v", err)
	}
	if !strings.Contains(err.Error(), "resolves outside its retained path") {
		t.Fatalf("provisionSandbox() error = %v, want escaping workspace context", err)
	}
}

func loadSandboxRuntimeAuthority(t *testing.T, home string) (*config.Config, config.RuntimeAuthority) {
	t.Helper()
	t.Setenv("HOME", home)
	loaded, err := config.Load("")
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	authority, err := loaded.RuntimeAuthority()
	if err != nil {
		t.Fatalf("RuntimeAuthority() error = %v", err)
	}
	return loaded, authority
}

func mkdirPhysical(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	physical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return physical
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

// TestInvalidSandboxConfigCannotBroadenLowerDirs is the ce-1hu9.61 command
// boundary regression. If the loader ignores the misspelled authority field,
// the seeded empty repository list reaches resolution and scans both repos in
// the fixture. A strict load must stop before that widening can occur.
func TestInvalidSandboxConfigCannotBroadenLowerDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workspaceRepos := filepath.Join(home, "src", "ws", "oss", "repos")
	repoA := filepath.Join(workspaceRepos, "repo-a")
	repoB := filepath.Join(workspaceRepos, "repo-b")
	for _, repo := range []string{repoA, repoB} {
		if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o700); err != nil {
			t.Fatalf("create repository fixture %s: %v", repo, err)
		}
	}

	originalConfigFile, originalCfg := cfgFile, cfg
	originalSessionsDir, originalProvider := sessionsDir, sandboxProvider
	providerFlag := newCmd.Flags().Lookup("sandbox-provider")
	originalProviderChanged := providerFlag.Changed
	t.Cleanup(func() {
		cfgFile = originalConfigFile
		cfg = originalCfg
		sessionsDir = originalSessionsDir
		sandboxProvider = originalProvider
		providerFlag.Changed = originalProviderChanged
	})
	sessionsDir = filepath.Join(home, "sessions")
	sandboxProvider = "auto"
	providerFlag.Changed = false

	invalid := []struct {
		name    string
		content *string
	}{
		{name: "misspelled repositories", content: new("sandbox:\n  repoz: []\n")},
		{name: "null repository list", content: new("sandbox: {repos: null}\n")},
		{name: "null repository item", content: new("sandbox: {repos: [null]}\n")},
		{name: "second document", content: new("sandbox: {repos: [/configured]}\n---\nsandbox: {repos: []}\n")},
		{name: "missing explicit source"},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "config.yaml")
			if tt.content != nil {
				if err := os.WriteFile(configPath, []byte(*tt.content), 0o600); err != nil {
					t.Fatalf("write invalid config: %v", err)
				}
			}
			cfgFile = configPath
			loaded, err := loadConfigWithFlags()
			if err == nil {
				cfg = loaded
				broadened, resolveErr := resolveSandboxLowerDirs(repoA)
				t.Fatalf("loadConfigWithFlags() accepted invalid authority config and reached repository discovery: dirs=%v err=%v", broadened, resolveErr)
			}
			if loaded != nil {
				t.Fatalf("loadConfigWithFlags() config = %#v, want nil on invalid authority config", loaded)
			}
		})
	}

	// Prove the two-repository fixture is capable of exposing the original
	// fail-open path instead of passing vacuously because discovery is empty.
	cfg = config.Default()
	discovered, err := resolveSandboxLowerDirs(repoA)
	if err != nil {
		t.Fatalf("resolveSandboxLowerDirs() fixture error = %v", err)
	}
	seen := make(map[string]bool, len(discovered))
	for _, repo := range discovered {
		seen[repo] = true
	}
	if len(discovered) != 2 || !seen[repoA] || !seen[repoB] {
		t.Fatalf("resolveSandboxLowerDirs() fixture = %v, want both %s and %s", discovered, repoA, repoB)
	}
}

func TestLoadConfigWithFlagsProjectsSandboxProviderPrecedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, "config.yaml")
	if err := os.WriteFile(path, []byte("sandbox: {provider: overlayfs-native, repos: []}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	originalCfgFile, originalSessionsDir := cfgFile, sessionsDir
	originalProvider := sandboxProvider
	providerFlag := newCmd.Flags().Lookup("sandbox-provider")
	originalProviderChanged := providerFlag.Changed
	t.Cleanup(func() {
		cfgFile = originalCfgFile
		sessionsDir = originalSessionsDir
		sandboxProvider = originalProvider
		providerFlag.Changed = originalProviderChanged
	})
	cfgFile = path
	sessionsDir = filepath.Join(home, "sessions")

	sandboxProvider = "auto"
	providerFlag.Changed = false
	loaded, err := loadConfigWithFlags()
	if err != nil {
		t.Fatal(err)
	}
	if sandboxProvider != "overlayfs-native" || loaded.Sandbox.Provider != "overlayfs-native" {
		t.Fatalf("configured provider projection = global %q snapshot %q, want registered compatibility name", sandboxProvider, loaded.Sandbox.Provider)
	}

	sandboxProvider = "gvisor"
	providerFlag.Changed = true
	loaded, err = loadConfigWithFlags()
	if err != nil {
		t.Fatal(err)
	}
	if sandboxProvider != "gvisor" || loaded.Sandbox.Provider != "gvisor" {
		t.Fatalf("explicit provider projection = global %q snapshot %q, want gvisor", sandboxProvider, loaded.Sandbox.Provider)
	}
}

func TestProjectUIConfigUsesSelectedSnapshotWithoutReread(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, "config.yaml")
	if err := os.WriteFile(path, []byte("ui: {theme: selected, no_color: false, screen_reader: false}\nsandbox: {repos: []}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	originalCfgFile, originalSessionsDir := cfgFile, sessionsDir
	originalProvider := sandboxProvider
	providerFlag := newCmd.Flags().Lookup("sandbox-provider")
	originalProviderChanged := providerFlag.Changed
	ui.SetGlobalConfig(nil)
	t.Cleanup(func() {
		cfgFile = originalCfgFile
		sessionsDir = originalSessionsDir
		sandboxProvider = originalProvider
		providerFlag.Changed = originalProviderChanged
		ui.SetGlobalConfig(nil)
	})
	cfgFile = path
	sessionsDir = filepath.Join(home, "sessions")
	sandboxProvider = "auto"
	providerFlag.Changed = false
	loaded, err := loadConfigWithFlags()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("invalid: [yaml"), 0o600); err != nil {
		t.Fatal(err)
	}

	projectUIConfig(loaded, true, true, ModeHuman)
	if loaded.UISettings.UI.NoColor || loaded.UISettings.UI.ScreenReader {
		t.Fatalf("loaded snapshot mutated by projection: %#v", loaded.UISettings.UI)
	}
	projected := ui.GetGlobalConfig()
	if projected.UI.Theme != "selected" || !projected.UI.NoColor || !projected.UI.ScreenReader {
		t.Fatalf("projected UI = %#v, want selected cached snapshot plus overrides", projected.UI)
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

func TestResolveSandboxLowerDirsAtHomeRejectsRetainedHomeAfterAmbientDrift(t *testing.T) {
	withEmptySandboxRepoConfig(t)
	for _, withScannedRepo := range []bool{false, true} {
		name := "without-scanned-repos"
		if withScannedRepo {
			name = "with-unrelated-scanned-repo"
		}
		t.Run(name, func(t *testing.T) {
			retainedHome := mkdirPhysical(t, t.TempDir())
			if err := os.Mkdir(filepath.Join(retainedHome, ".git"), 0o700); err != nil {
				t.Fatal(err)
			}
			nested := filepath.Join(retainedHome, "src", "project")
			if err := os.MkdirAll(nested, 0o700); err != nil {
				t.Fatal(err)
			}
			if withScannedRepo {
				scannedRepo := filepath.Join(retainedHome, "src", "ws", "oss", "repos", "unrelated")
				if err := os.MkdirAll(filepath.Join(scannedRepo, ".git"), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("HOME", t.TempDir())

			for _, workDir := range []string{retainedHome, nested} {
				t.Run(filepath.Base(workDir), func(t *testing.T) {
					dirs, err := resolveSandboxLowerDirsAtHome(workDir, retainedHome)
					if err == nil {
						t.Fatalf("resolveSandboxLowerDirsAtHome(%q) = %v, nil; want retained-HOME rejection", workDir, dirs)
					}
					var sandboxErr *sandbox.Error
					if !errors.As(err, &sandboxErr) || sandboxErr.Code != sandbox.ErrCodeNoLowerDirs {
						t.Fatalf("resolveSandboxLowerDirsAtHome() error = %v, want ErrCodeNoLowerDirs", err)
					}
				})
			}
		})
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

	homeDir := mkdirPhysical(t, t.TempDir())
	t.Setenv("HOME", homeDir)
	repoRoot := t.TempDir()
	requestedDir := filepath.Join(repoRoot, ".agents", "skills")
	if err := os.MkdirAll(requestedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	templatePath := filepath.Join(homeDir, "sandbox-onboarding.tmpl")
	if err := os.WriteFile(templatePath, []byte("workspace-root={{.MergedPath}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, _ := loadSandboxRuntimeAuthority(t, homeDir)
	cfg = loaded
	cfg.Sandbox = config.SandboxConfig{
		Enabled: true,
		Repos:   []string{repoRoot},
		Onboarding: config.OnboardingConfig{
			Enabled:      true,
			TemplatePath: templatePath,
		},
	}
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
	projectDir, err := sandbox.ClaudeProjectDir(wantWorkingDir)
	if err != nil {
		t.Fatal(err)
	}
	onboarding, err := os.ReadFile(filepath.Join(projectDir, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	wantOnboarding := "workspace-root=" + sandboxInfo.MergedPath + "\n"
	if string(onboarding) != wantOnboarding {
		t.Fatalf("custom onboarding = %q, want workspace root %q", onboarding, wantOnboarding)
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
	homeDir := t.TempDir()
	loaded, authority := loadSandboxRuntimeAuthority(t, homeDir)
	repoRoot := t.TempDir()
	destroyed := false
	cleanupErr := errors.New("fixture cleanup failure")
	sandbox.RegisterProvider("empty-working-dir-cleanup-failure-test", func() sandbox.Provider {
		return &emptyWorkingDirProvider{destroyed: &destroyed, destroyErr: cleanupErr}
	})
	cfg = loaded
	cfg.Sandbox = config.SandboxConfig{Enabled: true, Repos: []string{repoRoot}}

	_, err := provisionSandbox(context.Background(), authority, "empty-working-dir-cleanup-failure-test", "contract-session", repoRoot)
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
	loaded, authority := loadSandboxRuntimeAuthority(t, homeDir)
	repoRoot := t.TempDir()
	destroyed := false
	sandbox.RegisterProvider("empty-working-dir-test", func() sandbox.Provider {
		return &emptyWorkingDirProvider{destroyed: &destroyed}
	})
	cfg = loaded
	cfg.Sandbox = config.SandboxConfig{Enabled: true, Repos: []string{repoRoot}}

	_, err := provisionSandbox(context.Background(), authority, "empty-working-dir-test", "contract-session", repoRoot)
	if err == nil {
		t.Fatal("provisionSandbox() error = nil, want provider contract failure")
	}
	if !destroyed {
		t.Fatal("provisionSandbox() did not clean up workspace after provider contract failure")
	}
}

// TestResolveSandboxLowerDirs_ScansWorkspaceRepos verifies the ~/src/ws/oss/repos
// scan finds the requested repository without adding a duplicate fallback.
func TestResolveSandboxLowerDirs_ScansWorkspaceRepos(t *testing.T) {
	withEmptySandboxRepoConfig(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoDir := filepath.Join(home, "src", "ws", "oss", "repos", "some-repo")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0755); err != nil {
		t.Fatalf("failed to create scanned repo: %v", err)
	}

	workDir := filepath.Join(repoDir, "agm", "cmd", "agm")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dirs, err := resolveSandboxLowerDirs(workDir)
	if err != nil {
		t.Fatalf("resolveSandboxLowerDirs() error = %v, want nil", err)
	}
	if len(dirs) != 1 || dirs[0] != repoDir {
		t.Errorf("resolveSandboxLowerDirs() = %v, want [%s]", dirs, repoDir)
	}
}

func TestResolveSandboxLowerDirs_IncludesRequestedRepoAlongsideWorkspaceScan(t *testing.T) {
	withEmptySandboxRepoConfig(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	scannedRepo := filepath.Join(home, "src", "ws", "oss", "repos", "scanned-repo")
	if err := os.MkdirAll(filepath.Join(scannedRepo, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create scanned repo: %v", err)
	}
	requestedRepo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(requestedRepo, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create requested repo: %v", err)
	}
	workDir := filepath.Join(requestedRepo, "agm", "cmd", "agm")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}

	dirs, err := resolveSandboxLowerDirs(workDir)
	if err != nil {
		t.Fatalf("resolveSandboxLowerDirs() error = %v, want nil", err)
	}
	resolvedRequestedRepo, err := filepath.EvalSymlinks(requestedRepo)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{resolvedRequestedRepo, scannedRepo}
	if len(dirs) != len(want) || dirs[0] != want[0] || dirs[1] != want[1] {
		t.Fatalf("resolveSandboxLowerDirs() = %v, want %v", dirs, want)
	}
}
