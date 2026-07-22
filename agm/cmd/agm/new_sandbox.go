package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vbonnet/dear-agent/agm/internal/debug"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
	"github.com/vbonnet/dear-agent/internal/sandbox"
)

// maybeProvisionSandbox provisions a sandbox if enabled, returning the new
// SandboxConfig and the (possibly rewritten) workDir.
func maybeProvisionSandbox(ctx context.Context, sessionID, workDir string) (*manifest.SandboxConfig, string, error) {
	if !shouldEnableSandbox(enableSandbox, noSandbox) {
		return nil, workDir, nil
	}
	sandboxInfo, err := provisionSandbox(ctx, sandboxProvider, sessionID, workDir)
	if err != nil {
		ui.PrintError(err,
			"Failed to provision sandbox",
			"  • Check sandbox provider is available\n"+
				"  • Use --no-sandbox to disable sandbox isolation\n"+
				"  • Check ~/.agm/sandboxes/ directory permissions")
		return nil, workDir, err
	}
	if sandboxInfo.WorkingDir == "" {
		return nil, workDir, fmt.Errorf("sandbox provider %s returned an empty working directory", sandboxInfo.Provider)
	}
	workDir = sandboxInfo.WorkingDir
	fmt.Printf("Using sandbox workspace: %s\n", workDir)
	return sandboxInfo, workDir, nil
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
func provisionSandbox(ctx context.Context, providerName string, sessionID string, workDir string) (*manifest.SandboxConfig, error) {
	debug.Phase("Provision Sandbox")
	debug.Log("Provisioning sandbox for session: %s", sessionID)
	debug.Log("Provider: %s", providerName)
	debug.Log("WorkDir: %s", workDir)

	// Get provider
	var provider sandbox.Provider
	var err error
	if providerName == "auto" {
		provider, err = sandbox.NewProvider()
	} else {
		provider, err = sandbox.NewProviderForPlatform(providerName)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create sandbox provider: %w", err)
	}

	debug.Log("Using provider: %s", provider.Name())

	// Prepare sandbox workspace directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	sandboxWorkspace := filepath.Join(homeDir, ".agm", "sandboxes", sessionID)

	lowerDirs, err := resolveSandboxLowerDirs(workDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve sandbox lower directories: %w", err)
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
		return nil, fmt.Errorf("failed to create sandbox: %w", err)
	}

	debug.Log("Sandbox created successfully")
	debug.Log("Merged path: %s", sb.MergedPath)
	debug.Log("Working directory: %s", sb.WorkingDir)
	if sb.WorkingDir == "" {
		contractErr := fmt.Errorf("sandbox provider %s returned an empty working directory", provider.Name())
		if cleanupErr := provider.Destroy(ctx, sb.ID); cleanupErr != nil {
			return nil, errors.Join(contractErr, fmt.Errorf("cleanup failed: %w", cleanupErr))
		}
		return nil, contractErr
	}
	ui.PrintSuccess(fmt.Sprintf("Sandbox provisioned: %s", provider.Name()))

	// Write onboarding CLAUDE.md with worktree instructions
	if cfg.Sandbox.Onboarding.Enabled {
		var content string
		var onboardErr error
		if cfg.Sandbox.Onboarding.TemplatePath != "" {
			content, onboardErr = sandbox.GenerateOnboardingContentFromFile(
				cfg.Sandbox.Onboarding.TemplatePath, sessionID, sb.WorkingDir, lowerDirs,
			)
		} else {
			content, onboardErr = sandbox.GenerateOnboardingContent(sessionID, sb.WorkingDir, lowerDirs)
		}
		if onboardErr != nil {
			debug.Log("Warning: failed to generate onboarding content: %v", onboardErr)
		} else if err := sandbox.WriteOnboardingClaudeMd(sb.WorkingDir, content); err != nil {
			debug.Log("Warning: failed to write onboarding CLAUDE.md: %v", err)
		} else {
			debug.Log("Wrote sandbox onboarding to ~/.claude/projects/ for %s", sb.WorkingDir)
		}
	}

	return &manifest.SandboxConfig{
		Enabled:    true,
		ID:         sb.ID,
		Provider:   provider.Name(),
		MergedPath: sb.MergedPath,
		WorkingDir: sb.WorkingDir,
		CreatedAt:  sb.CreatedAt,
	}, nil
}

// resolveSandboxLowerDirs returns the provider lower directories for a new
// sandbox: prefer cfg.Sandbox.Repos, otherwise scan ~/src/ws/oss/repos for
// git repos, otherwise fall back to workDir — but only if workDir is itself
// a git repository. If nothing usable is configured or found, resolution
// fails loud (sandbox.ErrCodeNoLowerDirs) rather than silently cloning an
// arbitrary directory: an unset/misresolved workDir (e.g. launchd's default cwd of
// $HOME, with no --directory or $PWD) would otherwise trigger an unbounded
// clone of the wrong directory tree.
func resolveSandboxLowerDirs(workDir string) ([]string, error) {
	lowerDirs := cfg.Sandbox.Repos
	if len(lowerDirs) > 0 {
		return lowerDirs, nil
	}
	wsRoot := filepath.Join(os.Getenv("HOME"), "src", "ws", "oss", "repos")
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
		debug.Log("Found %d repos in workspace: %v", len(lowerDirs), lowerDirs)
		return lowerDirs, nil
	}

	fallbackRoot, reason := sandboxFallbackRoot(workDir)
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
	if homeDir, err := os.UserHomeDir(); err == nil {
		resolvedHome, _ := filepath.EvalSymlinks(homeDir)
		if resolvedWorkDir == resolvedHome {
			return "", "resolved to $HOME — refusing to clone the entire home directory"
		}
	}
	for candidate := resolvedWorkDir; ; candidate = filepath.Dir(candidate) {
		if _, statErr := os.Stat(filepath.Join(candidate, ".git")); statErr == nil {
			if homeDir, homeErr := os.UserHomeDir(); homeErr == nil {
				resolvedHome, _ := filepath.EvalSymlinks(homeDir)
				if candidate == resolvedHome {
					return "", "Git root resolved to $HOME — refusing to clone the entire home directory"
				}
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

// cleanupSandbox destroys a sandbox on error
func cleanupSandbox(ctx context.Context, sandboxID string, providerName string) {
	if sandboxID == "" {
		return
	}

	debug.Log("Cleaning up sandbox: %s", sandboxID)

	var provider sandbox.Provider
	var err error
	if providerName == "auto" || providerName == "" {
		provider, err = sandbox.NewProvider()
	} else {
		provider, err = sandbox.NewProviderForPlatform(providerName)
	}
	if err != nil {
		debug.Log("Failed to get provider for cleanup: %v", err)
		return
	}

	if err := provider.Destroy(ctx, sandboxID); err != nil {
		debug.Log("Failed to cleanup sandbox: %v", err)
	} else {
		debug.Log("Sandbox cleaned up successfully")
	}
}
