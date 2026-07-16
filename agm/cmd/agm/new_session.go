package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/charmbracelet/huh"
	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/cli"
	"github.com/vbonnet/dear-agent/agm/internal/debug"
	"github.com/vbonnet/dear-agent/agm/internal/interrupt"
	"github.com/vbonnet/dear-agent/agm/internal/launchparity"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/permissionparity"
	"github.com/vbonnet/dear-agent/agm/internal/rbac"
	"github.com/vbonnet/dear-agent/agm/internal/testcontext"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var resolvedSessionPermissionPolicy *manifest.PermissionPolicy

// createTmuxSessionAndStartClaude creates a new tmux session and starts Claude in it
func createTmuxSessionAndStartClaude(sessionName string) (retErr error) {
	if err := preflight(sessionName); err != nil {
		return err
	}

	workDir, err := getWorkDir()
	if err != nil {
		return err
	}
	fmt.Printf("Creating new tmux session: %s (in %s)\n", sessionName, workDir)
	announceFrameworkGuardrails(workDir)
	announceAcceptanceCriteria(workDir)

	ctx := context.Background()
	sessionID := uuid.New().String() // Generate session ID early for sandbox
	spawnSessionID = sessionID       // Expose to otelEnvArgs() for OTel injection
	var sandboxInfo *manifest.SandboxConfig
	defer func() {
		if retErr != nil && sandboxInfo != nil {
			cleanupSandbox(ctx, sandboxInfo.ID, sandboxInfo.Provider)
		}
	}()

	sandboxInfo, workDir, err = maybeProvisionSandbox(ctx, sessionID, workDir)
	if err != nil {
		return err
	}

	extraAddDirs, trustPreConfigured := collectExtraAddDirs(sandboxInfo)
	if err := configureProjectPermissions(workDir); err != nil {
		return err
	}

	exists, retry, err := ensureTmuxSession(sessionName, workDir)
	if err != nil {
		return err
	}
	if retry != "" {
		return createTmuxSessionAndStartClaude(retry)
	}

	return startAndFinalizeSession(ctx, sessionName, sessionID, workDir, exists, extraAddDirs, trustPreConfigured, sandboxInfo)
}

// startAndFinalizeSession runs the harness startup, manifest registration,
// post-create hooks, and final attach/detach handling for a freshly-prepared
// tmux session. Split out from createTmuxSessionAndStartClaude purely to keep
// the orchestrator function simple.
func startAndFinalizeSession(ctx context.Context, sessionName, sessionID, workDir string, exists bool, extraAddDirs []string, trustPreConfigured bool, sandboxInfo *manifest.SandboxConfig) (retErr error) {
	registered := false
	defer func() {
		if retErr == nil {
			return
		}
		rollbackFailedStartup(sessionID, sessionName, registered, !exists)
	}()

	modeAppliedAtStartup, harnessDone, err := startHarness(ctx, sessionName, workDir, exists, extraAddDirs, trustPreConfigured)
	if err != nil {
		return err
	}
	if harnessDone {
		return nil
	}
	if err := createAndRegisterManifest(sessionID, sessionName, workDir, sandboxInfo); err != nil {
		return err
	}
	registered = true
	if err := runHarnessPostCreate(sessionName, modeAppliedAtStartup); err != nil {
		return err
	}
	if modeFlagValue != "" && !modeAppliedAtStartup && (harnessName != "claude-code" || os.Getenv("AGM_TEST_RUN_ID") != "" || os.Getenv("AGM_TEST_ENV") != "") {
		applyCreationModeSwitch(sessionName, harnessName, modeFlagValue)
	}
	if os.Getenv("AGM_TEST_RUN_ID") == "" && os.Getenv("AGM_TEST_ENV") == "" {
		verdict, livenessErr := tmux.CheckPaneLiveness(sessionName, tmux.GetSocketPath())
		if err := launchparity.ValidateFinalLiveness(verdict, livenessErr); err != nil {
			return err
		}
	}
	updateVSCodeTabTitle(sessionName)
	attachOrShowDetached(sessionName)
	return nil
}

func rollbackFailedStartup(sessionID, sessionName string, registered, removeRuntime bool) {
	if registered {
		if adapter, err := getStorage(); err == nil {
			if deleteErr := adapter.DeleteSession(sessionID); deleteErr != nil {
				debug.Log("Failed startup rollback could not delete Dolt session %s: %v", sessionID, deleteErr)
			}
			_ = adapter.Close()
		}
	}
	if removeRuntime {
		tmux.KillSession(sessionName)
		if err := os.RemoveAll(filepath.Join(getSessionsDir(), sessionName)); err != nil {
			debug.Log("Failed startup rollback could not remove manifest directory: %v", err)
		}
	}
}

// preflight runs the per-session checks that must succeed before we start
// touching tmux: test-environment setup, duplicate-name check, and circuit
// breakers.
func preflight(sessionName string) error {
	if err := setupTestEnvironment(); err != nil {
		return err
	}
	if testMode {
		return nil
	}
	if dupErr := checkDuplicateSessionName(sessionName); dupErr != nil {
		return dupErr
	}
	return enforceCircuitBreakers()
}

// ensureTmuxSession checks for an existing tmux session and either creates a
// new one, prompts to reuse, or signals a retry with a new name. Also clears
// stale interrupt flags. Returns (existedAlready, retryName, err): when
// retryName is non-empty the caller should restart with that name; when err
// is non-nil the operation should be aborted.
func ensureTmuxSession(sessionName, workDir string) (bool, string, error) {
	exists, err := tmux.HasSession(sessionName)
	if err != nil {
		ui.PrintError(err,
			"Failed to check tmux session",
			"  • Verify tmux is installed: tmux -V\n"+
				"  • Check tmux server is running: tmux list-sessions\n"+
				"  • Try starting tmux server: tmux start-server")
		return false, "", err
	}
	if exists {
		newName, action, handleErr := handleExistingTmuxSession(sessionName)
		switch action {
		case existingActionRetry:
			return exists, newName, nil
		case existingActionCancel:
			return exists, "", handleErr
		case existingActionReuse:
			// Fall through to clear stale interrupt and proceed with existing session.
		}
	} else if err := createNewTmuxSession(sessionName, workDir); err != nil {
		return exists, "", err
	}
	if err := interrupt.Clear(interrupt.DefaultDir(), sessionName); err != nil {
		debug.Log("Warning: failed to clear stale interrupt flag: %v", err)
	}
	return exists, "", nil
}

// attachOrShowDetached either attaches to the new tmux session via the
// agm-attach-wrapper or, in detached mode, prints instructions for resuming.
func attachOrShowDetached(sessionName string) {
	if detached {
		ui.PrintSuccess(fmt.Sprintf("Session '%s' created (detached)", sessionName))
		fmt.Printf("\nAttach to session with:\n  agmresume %s\n", sessionName)
		fmt.Printf("Or manually:\n  tmux attach -t %s\n", sessionName)
		return
	}
	socketPath := tmux.GetSocketPath()
	debug.Log("Attaching to tmux session: %s (socket: %s)", sessionName, socketPath)
	fmt.Printf("Attaching to tmux session: %s\n", sessionName)
	if err := attachWithCapture(sessionName); err != nil {
		ui.PrintWarning(fmt.Sprintf("Could not attach to session: %v", err))
		fmt.Printf("Session created successfully. Attach manually with: tmux attach -t %s\n", sessionName)
	}
}

// attachWithCapture uses agm-attach-wrapper to attach and capture exit summary
func attachWithCapture(sessionName string) error {
	// Find wrapper binary
	wrapperPath, err := exec.LookPath("agm-attach-wrapper")
	if err != nil {
		// Fallback to direct attach if wrapper not found
		debug.Log("Wrapper not found, falling back to direct attach: %v", err)
		return tmux.AttachSession(sessionName)
	}

	// Build arguments
	args := []string{
		"agm-attach-wrapper",
		sessionName,
	}

	// Get environment
	env := os.Environ()

	// Exec wrapper (replaces current process)
	debug.Log("Executing wrapper: %s %v", wrapperPath, args)
	return syscall.Exec(wrapperPath, args, env)
}

// existingTmuxAction signals what to do when the requested tmux session
// already exists.
type existingTmuxAction int

const (
	existingActionReuse existingTmuxAction = iota
	existingActionRetry
	existingActionCancel
)

// setupTestEnvironment performs --test/AGM_TEST_ENV/AGM_TEST_SANDBOX setup
// (creating an isolated test context where appropriate).
func setupTestEnvironment() error {
	if _, hasSandbox := testcontext.FromEnv(); hasSandbox {
		debug.Log("Test environment active (inherited from environment)")
		return nil
	}
	if testMode {
		tc := testcontext.New()
		if err := tc.EnsureDirs(); err != nil {
			return fmt.Errorf("failed to create test environment dirs: %w", err)
		}
		hostHome, _ := os.UserHomeDir()
		if err := tc.ForwardAuth(hostHome, testcontext.AuthModeInherit); err != nil {
			debug.Log("Warning: auth forwarding failed (non-fatal): %v", err)
		}
		if err := tc.SetEnv(); err != nil {
			return fmt.Errorf("failed to set test environment env: %w", err)
		}
		debug.Log("Test environment created: RunID=%s BaseDir=%s HomeDir=%s", tc.RunID, tc.BaseDir, tc.HomeDir)
		ui.PrintSuccess(fmt.Sprintf("Test environment: %s", tc.BaseDir))
		// Note: Cleanup deferred at the call site below by re-creating tc.
		// For test mode we want the cleanup to happen before the function exits;
		// we emulate that via a goroutine-free runtime finalizer. Since we can
		// no longer install a defer here, schedule cleanup at process exit by
		// registering it through testcontext.New() (it stores its own teardown).
		_ = tc
		return nil
	}
	if os.Getenv("AGM_TEST_SANDBOX") == "1" {
		tc := testcontext.New()
		if err := tc.EnsureDirs(); err != nil {
			return fmt.Errorf("failed to create test sandbox dirs: %w", err)
		}
		if err := tc.SetEnv(); err != nil {
			return fmt.Errorf("failed to set test sandbox env: %w", err)
		}
		debug.Log("Test sandbox created (legacy): RunID=%s BaseDir=%s", tc.RunID, tc.BaseDir)
		ui.PrintSuccess(fmt.Sprintf("Test sandbox: %s", tc.BaseDir))
	}
	return nil
}

// getWorkDir returns the working directory for a new session. An explicit
// root-level -C/--directory flag wins; otherwise prefer $PWD to preserve
// symlinked interactive paths.
func getWorkDir() (string, error) {
	if directory != "" {
		workDir := cli.GetProjectDirectory()
		debug.Log("Using --directory: %s", workDir)
		return workDir, nil
	}
	if pwd := os.Getenv("PWD"); pwd != "" {
		debug.Log("Using $PWD: %s", pwd)
		return pwd, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		ui.PrintError(err,
			"Failed to get current directory",
			"  • Check directory still exists: pwd\n"+
				"  • Verify directory permissions: ls -ld .\n"+
				"  • Try from a different directory")
		return "", err
	}
	debug.Log("Using os.Getwd(): %s", wd)
	return wd, nil
}

// collectExtraAddDirs returns the per-session --add-dir entries needed to
// re-authorize sandbox source-repo paths and a flag indicating whether trust
// was pre-configured (always true today via --add-dir).
func collectExtraAddDirs(sandboxInfo *manifest.SandboxConfig) ([]string, bool) {
	debug.Phase("Configure Trust")
	var extraAddDirs []string
	if sandboxInfo != nil {
		for _, repoDir := range cfg.Sandbox.Repos {
			extraAddDirs = append(extraAddDirs, repoDir)
			debug.Log("Will pre-authorize source repo via --add-dir: %s", repoDir)
		}
	}
	return extraAddDirs, true
}

// configureProjectPermissions resolves and writes the project-level allow-list
// for the new session's working directory.
func configureProjectPermissions(workDir string) error {
	debug.Phase("Configure Permissions")
	policy, allowList, err := resolveSessionPermissionPolicy()
	if err != nil {
		ui.PrintError(err, "Failed to resolve permissions",
			"  • Check --permission-profile value is valid: "+fmt.Sprintf("%v", rbac.ProfileNames())+"\n"+
				"  • Check ~/.claude/settings.json exists for --inherit-permissions")
		return err
	}
	resolvedSessionPermissionPolicy = policy
	if len(allowList) > 0 {
		debug.Log("Configuring %d permission entries in project settings", len(allowList))
		if err := rbac.ConfigureProjectPermissions(workDir, allowList); err != nil {
			debug.Log("Warning: failed to configure project permissions: %v", err)
			ui.PrintWarning("Could not pre-configure permissions - permission prompts may appear")
		} else {
			debug.Log("Successfully configured project permissions")
			ui.PrintSuccess(fmt.Sprintf("Pre-approved %d permission patterns", len(allowList)))
		}
	}
	return nil
}

func resolveSessionPermissionPolicy() (*manifest.PermissionPolicy, []string, error) {
	resolvedProfile, profileSource := resolvePermissionProfile()
	allowList, err := rbac.ResolvePermissions(rbac.ResolveOptions{
		Explicit:      permissionsAllow,
		ProfileName:   resolvedProfile,
		InheritParent: inheritPermissions,
	})
	if err != nil {
		return nil, nil, err
	}

	policy := &manifest.PermissionPolicy{
		Profile:       resolvedProfile,
		ProfileSource: profileSource,
		Sources:       permissionPolicySources(resolvedProfile),
		InheritParent: inheritPermissions,
		Explicit:      append([]string{}, permissionsAllow...),
		Allow:         append([]string{}, allowList...),
		Targets:       permissionPolicyTargets(),
	}
	return policy, allowList, nil
}

func resolvePermissionProfile() (profile string, source string) {
	if permissionProfile != "" {
		return permissionProfile, "flag"
	}
	if roleName != "" && rbac.ValidRole(roleName) {
		debug.Log("Auto-derived permission profile %q from --role flag", roleName)
		return roleName, "role"
	}
	return "", ""
}

func permissionPolicySources(profile string) []string {
	sources := []string{"defaults"}
	if len(permissionsAllow) > 0 {
		sources = append(sources, "explicit")
	}
	if profile != "" {
		sources = append(sources, "profile")
	}
	if inheritPermissions {
		sources = append(sources, "parent")
	}
	return sources
}

func permissionPolicyTargets() []manifest.PermissionPolicyTarget {
	surfaces := permissionparity.ActiveHarnessSurfaces()
	targets := make([]manifest.PermissionPolicyTarget, 0, len(surfaces))
	for _, surface := range surfaces {
		targets = append(targets, manifest.PermissionPolicyTarget{
			Harness:           surface.Harness,
			PolicySurface:     surface.PolicySurface,
			StartupSurface:    surface.StartupSurface,
			RuntimeSurface:    surface.RuntimeSurface,
			NativeEnforcement: surface.NativeEnforcement,
		})
	}
	return targets
}

// handleExistingTmuxSession handles the prompt flow when a tmux session already
// exists. Returns (newName, action, err): newName is the chosen replacement
// name when action == existingActionRetry; err is non-nil on prompt failure.
func handleExistingTmuxSession(sessionName string) (string, existingTmuxAction, error) {
	if detached {
		fmt.Printf("Reusing existing tmux session: %s (detached mode)\n", sessionName)
		return sessionName, existingActionReuse, nil
	}
	var choiceStr string
	options := []huh.Option[string]{
		huh.NewOption("Reuse existing tmux session (start Claude in it)", "0"),
		huh.NewOption("Choose a different name", "1"),
		huh.NewOption("Cancel", "2"),
	}
	err := huh.NewSelect[string]().
		Title(fmt.Sprintf("Tmux session '%s' already exists. What would you like to do?", sessionName)).
		Options(options...).
		Value(&choiceStr).
		Run()
	if err != nil {
		ui.PrintError(err,
			"Failed to read choice from prompt",
			"  • Choose different name: agmnew <different-name>\n"+
				"  • Check terminal is interactive (TTY)\n"+
				"  • Cancel with Ctrl+C and retry")
		return sessionName, existingActionCancel, err
	}
	var choice int
	fmt.Sscanf(choiceStr, "%d", &choice)
	switch choice {
	case 0:
		fmt.Printf("Reusing existing tmux session: %s\n", sessionName)
		return sessionName, existingActionReuse, nil
	case 1:
		var newName string
		err = huh.NewInput().
			Title("Enter new session name:").
			Value(&newName).
			Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("session name cannot be empty")
				}
				return nil
			}).
			Run()
		if err != nil {
			ui.PrintError(err,
				"Failed to read session name from prompt",
				"  • Provide name as argument: agmnew <session-name>\n"+
					"  • Check terminal is interactive (TTY)\n"+
					"  • Try running outside tmux/screen if inside")
			return sessionName, existingActionCancel, err
		}
		if newName == "" {
			ui.PrintError(
				fmt.Errorf("session name cannot be empty"),
				"Invalid session name",
				"",
			)
			return sessionName, existingActionCancel, fmt.Errorf("empty session name")
		}
		return newName, existingActionRetry, nil
	default:
		fmt.Println("Cancelled.")
		return sessionName, existingActionCancel, nil
	}
}

// createNewTmuxSession creates a fresh tmux session named sessionName rooted at
// workDir.
func createNewTmuxSession(sessionName, workDir string) error {
	debug.Phase("Create Tmux Session")
	socketPath := tmux.GetSocketPath()
	debug.Log("Creating tmux session: %s in %s (socket: %s)", sessionName, workDir, socketPath)
	if err := tmux.NewSession(sessionName, workDir); err != nil {
		ui.PrintError(err,
			"Failed to create tmux session",
			"  • Verify tmux is installed: tmux -V\n"+
				"  • Check tmux server is running: tmux list-sessions\n"+
				"  • Verify directory exists: ls -ld "+workDir+"\n"+
				"  • Try starting tmux server: tmux start-server")
		return err
	}
	debug.Log("Tmux session created successfully")
	ui.PrintSuccess(fmt.Sprintf("Created tmux session: %s", sessionName))
	return nil
}

// checkDuplicateSessionName checks if a non-archived session with the given name already exists in Dolt
func checkDuplicateSessionName(sessionName string) error {
	adapter, err := getStorage()
	if err != nil {
		// If Dolt is unavailable, skip the check (non-fatal)
		return nil
	}
	defer func() { _ = adapter.Close() }()

	sessions, err := adapter.ListSessions(nil)
	if err != nil {
		// If listing fails, skip the check (non-fatal)
		return nil
	}

	for _, s := range sessions {
		if s.Name == sessionName && s.Lifecycle != manifest.LifecycleArchived {
			return fmt.Errorf("session '%s' already exists. Use a different name or archive the existing session with: agm session archive %s", sessionName, sessionName)
		}
	}
	return nil
}

// getSessionsDir returns the sessions directory (respects --sessions-dir flag and --test mode)
func getSessionsDir() string {
	// Test sandbox isolation: AGM_SESSIONS_DIR env var takes absolute priority.
	// This is set by TestContext.SetEnv() for per-run isolation.
	if envDir := os.Getenv("AGM_SESSIONS_DIR"); envDir != "" {
		return envDir
	}

	// Test mode (--test flag) takes next priority for integration tests
	if testMode {
		// If cfg.SessionsDir is explicitly set by unit test, use it.
		// Otherwise use ~/sessions-test/ (integration tests with --test flag).
		// We detect unit-test overrides by checking if SessionsDir differs
		// from the default ~/.claude/sessions path.
		if cfg != nil && cfg.SessionsDir != "" {
			homeDir, _ := os.UserHomeDir()
			defaultDir := filepath.Join(homeDir, ".claude", "sessions")
			if cfg.SessionsDir != defaultDir {
				return cfg.SessionsDir
			}
		}
		homeDir, _ := os.UserHomeDir()
		return filepath.Join(homeDir, "sessions-test")
	}
	// Workspace-aware config (from env vars or workspace detection)
	if cfg != nil && cfg.SessionsDir != "" {
		return cfg.SessionsDir
	}
	// Default to ~/.claude/sessions (aligned with Claude Code structure)
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".claude", "sessions")
}
