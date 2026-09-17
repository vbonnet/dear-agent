package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vbonnet/dear-agent/agm/internal/config"
	"github.com/vbonnet/dear-agent/agm/internal/debug"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
	"github.com/vbonnet/dear-agent/internal/sandbox"
)

// sandboxCleanup retains the exact provider instance that successfully
// provisioned one identity-checked AGM session. Provider Name values are
// display labels (for example, the "apfs" selector reports "apfs-reflink")
// and are not safe reconstruction keys for rollback.
type sandboxCleanup struct {
	provider  sandbox.Provider
	sessionID string
}

func (cleanup *sandboxCleanup) rollback(ctx context.Context) error {
	if cleanup == nil {
		return nil
	}
	return cleanup.provider.Destroy(context.WithoutCancel(ctx), cleanup.sessionID)
}

// maybeProvisionSandbox provisions a sandbox if enabled, returning the durable
// provider-spelled SandboxConfig, the physically resolved live workDir, and the
// provider-bound cleanup handle that must be retained until the complete create
// lifecycle succeeds.
func maybeProvisionSandbox(ctx context.Context, sessionID, workDir string) (*manifest.SandboxConfig, string, *sandboxCleanup, error) {
	// Guard the whole sandbox decision, not just one of its readers: the very
	// first thing this path does is consult cfg.Sandbox, so a missing snapshot
	// has to be refused here rather than deeper in path resolution.
	if cfg == nil {
		return nil, workDir, nil, errors.New("sandbox provisioning requires a loaded configuration")
	}
	if !shouldEnableSandbox(enableSandbox, noSandbox) {
		return nil, workDir, nil, nil
	}
	authority, err := cfg.RuntimeAuthority()
	if err != nil {
		return nil, workDir, nil, fmt.Errorf("resolve sandbox runtime authority: %w", err)
	}
	sandboxInfo, liveWorkDir, cleanup, err := provisionSandbox(ctx, authority, sandboxProvider, sessionID, workDir)
	if err != nil {
		ui.PrintError(err,
			"Failed to provision sandbox",
			"  • Check sandbox provider is available\n"+
				"  • Use --no-sandbox to disable sandbox isolation\n"+
				"  • Check the configured sandbox storage directory permissions")
		return nil, workDir, nil, err
	}
	workDir = liveWorkDir
	fmt.Printf("Using sandbox workspace: %s\n", workDir)
	return sandboxInfo, workDir, cleanup, nil
}

// shouldEnableSandbox determines if sandbox should be enabled based on config and flags.
// Sandbox is ON by default (config.Sandbox.Enabled=true). Use --no-sandbox to disable.
// The enable parameter is retained for backward compatibility but the --sandbox flag
// has been removed from the CLI.
func shouldEnableSandbox(enable bool, disable bool) bool {
	if disable {
		return false
	}
	if enable {
		return true
	}
	// Use config (defaults to true = sandbox-by-default)
	return cfg.Sandbox.Enabled
}

// provisionSandbox creates a sandbox environment for the session
func provisionSandbox(
	ctx context.Context,
	authority config.RuntimeAuthority,
	providerName string,
	sessionID string,
	workDir string,
) (*manifest.SandboxConfig, string, *sandboxCleanup, error) {
	debug.Phase("Provision Sandbox")
	debug.Log("Provisioning sandbox for session: %s", sessionID)
	debug.Log("Provider: %s", providerName)
	debug.Log("WorkDir: %s", workDir)

	sandboxRoot, sandboxWorkspace, homeDir, err := resolveSandboxProvisioningPaths(authority, sessionID)
	if err != nil {
		return nil, "", nil, err
	}

	// Get provider only after the complete workspace authority is valid.
	var provider sandbox.Provider
	if providerName == "auto" {
		provider, err = sandbox.NewProvider()
	} else {
		provider, err = sandbox.NewProviderForPlatform(providerName)
	}
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to create sandbox provider: %w", err)
	}

	debug.Log("Using provider: %s", provider.Name())

	lowerDirs, err := resolveSandboxLowerDirsAtHome(workDir, homeDir)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to resolve sandbox lower directories: %w", err)
	}

	// Determine the primary/target repo: the repository containing the requested
	// directory wins, then the legacy monorepo fallbacks apply.
	targetRepo := findPrimaryRepo(lowerDirs, workDir)
	if targetRepo != "" {
		debug.Log("Target repo: %s", targetRepo)
	}

	// Create sandbox
	debug.Log("Creating sandbox with workspace: %s", sandboxWorkspace)
	debug.Log("Lower dirs: %v", lowerDirs)
	sb, err := provider.Create(ctx, sandbox.SandboxRequest{
		SessionID:    sessionID,
		LowerDirs:    lowerDirs,
		WorkingDir:   workDir,
		WorkspaceDir: sandboxWorkspace,
		Secrets:      cfg.Sandbox.Secrets,
		TargetRepo:   targetRepo,
	})
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to create sandbox: %w", err)
	}
	sandboxInfo, liveWorkDir, contractErr := validateCreatedSandbox(sandboxRoot, sessionID, provider.Name(), sb)
	if contractErr != nil {
		contractErr = fmt.Errorf("sandbox provider %s violated its create contract: %w", provider.Name(), contractErr)
		if cleanupErr := provider.Destroy(context.WithoutCancel(ctx), sessionID); cleanupErr != nil {
			return nil, "", nil, errors.Join(contractErr, fmt.Errorf("cleanup failed: %w", cleanupErr))
		}
		return nil, "", nil, contractErr
	}
	// validateCreatedSandbox proved that the provider returned the stable AGM
	// identity. Only after that proof may the exact instance become a retained
	// rollback capability for later permission, authority, tmux, or launch
	// failures.
	cleanup := &sandboxCleanup{provider: provider, sessionID: sessionID}

	debug.Log("Sandbox created successfully")
	debug.Log("Merged path: %s", sb.MergedPath)
	debug.Log("Working directory: %s", sb.WorkingDir)
	ui.PrintSuccess(fmt.Sprintf("Sandbox provisioned: %s", provider.Name()))

	// Write onboarding CLAUDE.md with worktree instructions
	if cfg.Sandbox.Onboarding.Enabled {
		var content string
		var onboardErr error
		if cfg.Sandbox.Onboarding.TemplatePath != "" {
			content, onboardErr = sandbox.GenerateOnboardingContentFromFile(
				cfg.Sandbox.Onboarding.TemplatePath, sessionID, sb.MergedPath, lowerDirs,
			)
		} else {
			content, onboardErr = sandbox.GenerateOnboardingContent(sessionID, sb.MergedPath, lowerDirs)
		}
		if onboardErr != nil {
			debug.Log("Warning: failed to generate onboarding content: %v", onboardErr)
		} else if err := sandbox.WriteOnboardingClaudeMd(liveWorkDir, content); err != nil {
			debug.Log("Warning: failed to write onboarding CLAUDE.md: %v", err)
		} else {
			debug.Log("Wrote sandbox onboarding to ~/.claude/projects/ for %s", liveWorkDir)
		}
	}

	sandboxInfo.CodexHookSourceRepo = targetRepo
	return sandboxInfo, liveWorkDir, cleanup, nil
}

func resolveSandboxProvisioningPaths(authority config.RuntimeAuthority, sessionID string) (config.SandboxRoot, string, string, error) {
	sandboxRoot, err := authority.Sandboxes()
	if err != nil {
		return config.SandboxRoot{}, "", "", fmt.Errorf("resolve sandbox root: %w", err)
	}
	sandboxWorkspace, err := sandboxRoot.Workspace(sessionID)
	if err != nil {
		return config.SandboxRoot{}, "", "", fmt.Errorf("resolve sandbox workspace: %w", err)
	}
	homeRoot, err := authority.Home()
	if err != nil {
		return config.SandboxRoot{}, "", "", fmt.Errorf("resolve sandbox HOME: %w", err)
	}
	homeDir, err := homeRoot.Path()
	if err != nil {
		return config.SandboxRoot{}, "", "", fmt.Errorf("resolve sandbox HOME path: %w", err)
	}
	return sandboxRoot, sandboxWorkspace, homeDir, nil
}

// validateCreatedSandbox checks the provider adapter's result before any caller
// consumes its paths. The stable AGM session ID selects the only workspace the
// provider may return; a provider-native ID or path outside that workspace must
// never become filesystem authority for onboarding, permissions, or launch.
// It returns the physical working directory for live use without rewriting the
// provider-spelled paths needed by durable ownership validation.
func validateCreatedSandbox(
	root config.SandboxRoot,
	sessionID string,
	providerName string,
	sb *sandbox.Sandbox,
) (*manifest.SandboxConfig, string, error) {
	if sb == nil {
		return nil, "", errors.New("returned a nil sandbox")
	}
	if sb.ID != sessionID {
		return nil, "", fmt.Errorf("returned sandbox ID %q, want stable session ID %q", sb.ID, sessionID)
	}
	workspace, err := root.Workspace(sessionID)
	if err != nil {
		return nil, "", fmt.Errorf("resolve stable sandbox workspace: %w", err)
	}

	type providerPath struct {
		name     string
		value    string
		required bool
	}
	paths := []providerPath{
		{name: "merged path", value: sb.MergedPath, required: true},
		{name: "working directory", value: sb.WorkingDir, required: true},
		{name: "upper path", value: sb.UpperPath},
		{name: "work path", value: sb.WorkPath},
	}
	var normalizedWorkingDir string
	for _, path := range paths {
		if path.value == "" {
			if path.required {
				return nil, "", fmt.Errorf("returned an empty %s", path.name)
			}
			continue
		}
		resolved, err := root.ValidateExistingWorkspaceDirectory(sessionID, path.value)
		if err != nil {
			return nil, "", fmt.Errorf("returned invalid %s %q (outside authority, not materialized, or not a directory): %w", path.name, path.value, err)
		}
		if path.name == "working directory" {
			normalizedWorkingDir = resolved
		}
	}
	if filepath.Dir(sb.MergedPath) != workspace {
		return nil, "", fmt.Errorf(
			"returned merged path %q is not rooted at the exact stable workspace %q",
			sb.MergedPath, workspace,
		)
	}
	sandboxInfo := &manifest.SandboxConfig{
		Enabled:    true,
		ID:         sb.ID,
		Provider:   providerName,
		MergedPath: sb.MergedPath,
		WorkingDir: sb.WorkingDir,
		CreatedAt:  sb.CreatedAt,
	}
	if err := manifest.ValidateSandboxOwnership(sessionID, sandboxInfo); err != nil {
		return nil, "", fmt.Errorf("returned invalid durable sandbox ownership: %w", err)
	}
	return sandboxInfo, normalizedWorkingDir, nil
}

// resolveSandboxLowerDirs returns the provider lower directories for a new
// sandbox: prefer cfg.Sandbox.Repos, otherwise scan HOME/src/ws/oss/repos for
// git repos and ensure the repository containing workDir remains included.
// If nothing usable is configured or found, resolution
// fails loud (sandbox.ErrCodeNoLowerDirs) rather than silently cloning an
// arbitrary directory: an unset/misresolved workDir (e.g. launchd's default cwd of
// $HOME, with no --directory or $PWD) would otherwise trigger an unbounded
// clone of the wrong directory tree.
func resolveSandboxLowerDirs(workDir string) ([]string, error) {
	return resolveSandboxLowerDirsAtHome(workDir, os.Getenv("HOME"))
}

func resolveSandboxLowerDirsAtHome(workDir, homeDir string) ([]string, error) {
	lowerDirs := cfg.Sandbox.Repos
	if len(lowerDirs) > 0 {
		return lowerDirs, nil
	}
	wsRoot := filepath.Join(homeDir, "src", "ws", "oss", "repos")
	if entries, err := os.ReadDir(wsRoot); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			repoPath := filepath.Join(wsRoot, e.Name())
			if _, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil {
				lowerDirs = append(lowerDirs, repoPath)
			}
		}
	}
	if len(lowerDirs) > 0 {
		if _, matchErr := sandbox.MatchWorkingDir(workDir, lowerDirs); matchErr == nil {
			debug.Log("Found %d repos in workspace including requested project: %v", len(lowerDirs), lowerDirs)
			return lowerDirs, nil
		}
		requestedRoot, reason := sandboxFallbackRootAtHome(workDir, homeDir)
		if reason != "" {
			return nil, sandbox.NewNoSandboxLowerDirsError(workDir, reason)
		}
		lowerDirs = append([]string{requestedRoot}, lowerDirs...)
		debug.Log("Added requested project root to %d scanned repos: %v", len(lowerDirs)-1, lowerDirs)
		return lowerDirs, nil
	}

	fallbackRoot, reason := sandboxFallbackRootAtHome(workDir, homeDir)
	if reason != "" {
		return nil, sandbox.NewNoSandboxLowerDirsError(workDir, reason)
	}
	debug.Log("No repos found, using containing git repo as lower dir: %s", fallbackRoot)
	return []string{fallbackRoot}, nil
}

// unsafeSandboxFallbackReason returns a non-empty reason if workDir is unsafe
// to use as a sandbox lower dir fallback (empty, resolves to $HOME, or is not
// a git repository), or "" if workDir is safe to clone.
func unsafeSandboxFallbackReason(workDir string) string {
	_, reason := sandboxFallbackRoot(workDir)
	return reason
}

// sandboxFallbackRoot returns the nearest containing Git repository. This
// preserves an explicitly requested subdirectory while ensuring providers
// clone only the repository boundary, never an arbitrary ancestor tree.
func sandboxFallbackRoot(workDir string) (string, string) {
	homeDir, _ := os.UserHomeDir()
	return sandboxFallbackRootAtHome(workDir, homeDir)
}

func sandboxFallbackRootAtHome(workDir, homeDir string) (string, string) {
	if workDir == "" {
		return "", "workDir is empty"
	}
	resolvedWorkDir, err := filepath.EvalSymlinks(workDir)
	if err != nil {
		return "", fmt.Sprintf("cannot resolve workDir: %v", err)
	}
	resolvedWorkDir, err = filepath.Abs(resolvedWorkDir)
	if err != nil {
		return "", fmt.Sprintf("cannot make workDir absolute: %v", err)
	}
	resolvedHome := ""
	if homeDir != "" {
		resolvedHome, _ = filepath.EvalSymlinks(homeDir)
	}
	if resolvedHome != "" && resolvedWorkDir == resolvedHome {
		return "", "resolved to $HOME — refusing to clone the entire home directory"
	}
	for candidate := resolvedWorkDir; ; candidate = filepath.Dir(candidate) {
		if _, statErr := os.Stat(filepath.Join(candidate, ".git")); statErr == nil {
			if resolvedHome != "" && candidate == resolvedHome {
				return "", "Git root resolved to $HOME — refusing to clone the entire home directory"
			}
			return candidate, ""
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			break
		}
	}
	return "", "not inside a git repository (no .git entry in workDir or its parents)"
}

// findPrimaryRepo selects the preferred repo from a list of repo paths.
// It uses a priority cascade to avoid picking the wrong repo when multiple
// repos have go.mod (e.g. ai-conversation-logs vs ai-tools):
//  1. The repo containing the requested session directory
//  2. The repo named "ai-tools" (AGM's own monorepo — the right default)
//  3. Any repo with go.mod at root
//  4. The first repo as a last resort
//
// Returns empty string if no suitable repo is found.
func findPrimaryRepo(repoDirs []string, workDir string) string {
	// 1. The explicitly requested session directory is authoritative, even
	//    when AGM itself was invoked from another checkout.
	if match, err := sandbox.MatchWorkingDir(workDir, repoDirs); err == nil {
		debug.Log("findPrimaryRepo: requested directory %s is inside repo %s", workDir, match.LowerDir)
		return match.LowerDir
	}

	// 2. Prefer the repo named "ai-tools" — AGM lives in this monorepo,
	//    so it is almost always the correct target when invoked from AGM.
	for _, dir := range repoDirs {
		if filepath.Base(dir) == "ai-tools" {
			goMod := filepath.Join(dir, "go.mod")
			if _, err := os.Stat(goMod); err == nil {
				debug.Log("findPrimaryRepo: preferred ai-tools repo: %s", dir)
				return dir
			}
		}
	}

	// 3. Fall back to the first repo with go.mod.
	for _, dir := range repoDirs {
		goMod := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goMod); err == nil {
			return dir
		}
	}

	// 4. Last resort: return the first entry.
	if len(repoDirs) > 0 {
		return repoDirs[0]
	}
	return ""
}
