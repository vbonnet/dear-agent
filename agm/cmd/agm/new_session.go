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
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/git"
	"github.com/vbonnet/dear-agent/agm/internal/interrupt"
	"github.com/vbonnet/dear-agent/agm/internal/launchparity"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/modelrouter"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/permissionparity"
	"github.com/vbonnet/dear-agent/agm/internal/rbac"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/testcontext"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
	"github.com/vbonnet/dear-agent/internal/telemetry"
)

var resolvedSessionPermissionPolicy *manifest.PermissionPolicy

type cliCreateSessionRuntime struct {
	launch               func(context.Context, ops.HarnessLaunchSpec) (ops.CreateSessionLaunchResult, error)
	bootstrapAgyIdentity func(context.Context, ops.AgyCreateIdentityBootstrap) error
	complete             func(context.Context, ops.CreateSessionCompletion) error
}

type cliCreateFinalizationRuntime struct {
	checkLiveness func(context.Context, string, string) (tmux.PaneLiveness, error)
	updateTitle   func(string)
	attach        func(string)
}

func (r *cliCreateSessionRuntime) Launch(ctx context.Context, spec ops.HarnessLaunchSpec) (ops.CreateSessionLaunchResult, error) {
	return r.launch(ctx, spec)
}

func (r *cliCreateSessionRuntime) Complete(ctx context.Context, completion ops.CreateSessionCompletion) error {
	return r.complete(ctx, completion)
}

func (r *cliCreateSessionRuntime) BootstrapAgyCreateIdentity(ctx context.Context, input ops.AgyCreateIdentityBootstrap) error {
	if r.bootstrapAgyIdentity == nil {
		return fmt.Errorf("CLI runtime does not support AGY identity bootstrap")
	}
	return r.bootstrapAgyIdentity(ctx, input)
}

func newCLICreateSessionRuntime(sessionName string, existed, trustPreConfigured bool) *cliCreateSessionRuntime {
	return &cliCreateSessionRuntime{
		launch: func(ctx context.Context, spec ops.HarnessLaunchSpec) (ops.CreateSessionLaunchResult, error) {
			return launchCLICreateSession(ctx, spec, existed, trustPreConfigured)
		},
		bootstrapAgyIdentity: func(ctx context.Context, input ops.AgyCreateIdentityBootstrap) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			readiness, err := tmux.CheckExpectedHarnessInputAndSend(ctx, input.SessionName, "agy", input.Prompt, tmux.InputDeliveryOptions{})
			if err != nil {
				return fmt.Errorf("revalidate CLI AGY identity bootstrap prompt: %w", err)
			}
			if !readiness.Ready {
				return fmt.Errorf("revalidate CLI AGY identity bootstrap prompt: harness input is %s", readiness.State)
			}
			if readiness.TargetPane == "" {
				return fmt.Errorf("revalidate CLI AGY identity bootstrap prompt: harness returned no verified pane")
			}
			return nil
		},
		complete: func(ctx context.Context, completion ops.CreateSessionCompletion) error {
			return completeCLICreateSession(ctx, sessionName, completion)
		},
	}
}

func launchCLICreateSession(ctx context.Context, spec ops.HarnessLaunchSpec, existed, trustPreConfigured bool) (ops.CreateSessionLaunchResult, error) {
	if !existed {
		ui.PrintSuccess(fmt.Sprintf("Created tmux session: %s", spec.SessionName))
	}
	if err := interrupt.Clear(interrupt.DefaultDir(), spec.SessionName); err != nil {
		debug.Log("Warning: failed to clear stale interrupt flag: %v", err)
	}
	modeApplied, handled, err := startHarness(ctx, spec, trustPreConfigured)
	return ops.CreateSessionLaunchResult{ModeAppliedAtStartup: modeApplied, HandledLifecycle: handled}, err
}

func completeCLICreateSession(ctx context.Context, sessionName string, completion ops.CreateSessionCompletion) error {
	m := completion.Manifest
	if completion.ManifestPath != "" {
		if err := git.CommitManifest(completion.ManifestPath, "create", sessionName); err != nil {
			debug.Log("manifest commit skipped: %v", err)
		}
	}
	if m.ModelTier != "" {
		d := &modelrouter.Decision{Tier: modelrouter.Tier(m.ModelTier), Model: m.Model, Reason: "recorded at manifest creation", ExplicitTier: modelTierFlag != ""}
		modelrouter.RecordRoutingDecision(ctx, m.Harness, d)
	}
	telemetry.SessionStarted(ctx, m.SessionID, m.Model, m.Harness, m.State, roleName)
	if err := runHarnessPostCreate(ctx, sessionName, completion.Launch.ModeAppliedAtStartup, completion.Launch.PromptDelivered); err != nil {
		return err
	}
	if modeFlagValue != "" && !completion.Launch.ModeAppliedAtStartup && (harnessName != "claude-code" || os.Getenv("AGM_TEST_RUN_ID") != "" || os.Getenv("AGM_TEST_ENV") != "") {
		applyCreationModeSwitchContext(ctx, sessionName, harnessName, modeFlagValue)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return finalizeCLICreateSession(ctx, sessionName, cliCreateFinalizationRuntime{
		checkLiveness: tmux.CheckPaneLivenessContext,
		updateTitle:   updateVSCodeTabTitle,
		attach:        attachOrShowDetached,
	})
}

func finalizeCLICreateSession(ctx context.Context, sessionName string, runtime cliCreateFinalizationRuntime) error {
	if os.Getenv("AGM_TEST_RUN_ID") == "" && os.Getenv("AGM_TEST_ENV") == "" {
		verdict, livenessErr := runtime.checkLiveness(ctx, sessionName, tmux.GetSocketPath())
		if err := launchparity.ValidateFinalLiveness(verdict, livenessErr); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	runtime.updateTitle(sessionName)
	runtime.attach(sessionName)
	return nil
}

// createTmuxSessionAndStartClaude creates a new tmux session and starts Claude in it
func createTmuxSessionAndStartClaude(ctx context.Context, sessionName string) (retErr error) {
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

	sessionID := uuid.New().String() // Generate session ID early for sandbox
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

	exists, retry, err := resolveTmuxSession(sessionName)
	if err != nil {
		return err
	}
	if retry != "" {
		return createTmuxSessionAndStartClaude(ctx, retry)
	}

	return runCreateSessionLifecycle(ctx, sessionName, sessionID, workDir, exists, extraAddDirs, trustPreConfigured, sandboxInfo)
}

// runCreateSessionLifecycle adapts CLI presentation and readiness behavior to
// the shared ops lifecycle. Business ordering and rollback stay in ops.
func runCreateSessionLifecycle(ctx context.Context, sessionName, sessionID, workDir string, exists bool, extraAddDirs []string, trustPreConfigured bool, sandboxInfo *manifest.SandboxConfig) error {
	if harnessName == "codex-cli" {
		if err := validateCodexCredentials(); err != nil {
			return err
		}
	}
	manifestDir := filepath.Join(getSessionsDir(), sessionName)
	createPrompt, err := resolveCreateLifecyclePrompt(harnessName, prompt, promptFile)
	if err != nil {
		return err
	}
	runtime := newCLICreateSessionRuntime(sessionName, exists, trustPreConfigured)
	opCtx := &ops.OpContext{
		Tmux:            session.NewRealTmux(),
		CreationRuntime: runtime,
		OpenSessionStorage: func(context.Context) (dolt.Storage, func(), error) {
			adapter, err := getStorage()
			if err != nil {
				return nil, nil, err
			}
			return adapter, func() { _ = adapter.Close() }, nil
		},
	}
	_, err = ops.CreateSessionWithContext(ctx, opCtx, &ops.CreateSessionRequest{
		Cwd:                 workDir,
		Prompt:              createPrompt,
		Title:               sessionName,
		Model:               modelName,
		Harness:             harnessName,
		Persistent:          persistent,
		SessionID:           sessionID,
		Caller:              ops.CreateSessionCaller{Surface: ops.CreateSurfaceCLI},
		PermissionMode:      modeFlagValue,
		DisableAutoMode:     noAutoMode,
		MaxBudgetUSD:        maxBudgetUsd,
		ExtraAddDirs:        extraAddDirs,
		ForwardTelemetry:    true,
		ForwardClaudeOAuth:  true,
		AllowEmptyPrompt:    true,
		AllowUnsafeTitle:    true,
		ReuseExistingTmux:   exists,
		RequireStorage:      true,
		ManifestDir:         manifestDir,
		ManifestDirOptional: true,
		Metadata: ops.CreateSessionMetadata{
			Workspace:        cfg.Workspace,
			ModelTier:        modelTierFlag,
			Tags:             buildSessionTags(roleName, sessionTags),
			PermissionPolicy: resolvedSessionPermissionPolicy,
			Sandbox:          sandboxInfo,
			IsTest:           testMode,
			Disposable:       disposable,
			DisposableTTL:    disposableTTL,
			PermissionMode:   modeFlagValue,
			OpenCodeServer:   os.Getenv("OPENCODE_SERVER_URL"),
		},
	})
	return err
}

func resolveCreateLifecyclePrompt(harness, promptText, promptPath string) (string, error) {
	if harness != "agy" || promptText != "" || promptPath == "" {
		return promptText, nil
	}
	content, err := os.ReadFile(promptPath)
	if err != nil {
		return "", fmt.Errorf("read AGY startup prompt file %s: %w", promptPath, err)
	}
	const maxPromptFileSize = 10 * 1024
	if len(content) > maxPromptFileSize {
		return "", fmt.Errorf("prompt file too large: %d bytes (max 10KB)", len(content))
	}
	return string(content), nil
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

// resolveTmuxSession checks for an existing tmux session and either prompts to
// reuse it or signals a retry with a new name. The shared ops lifecycle owns
// creation. Returns (existedAlready, retryName, err): when
// retryName is non-empty the caller should restart with that name; when err
// is non-nil the operation should be aborted.
func resolveTmuxSession(sessionName string) (bool, string, error) {
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

// configureProjectPermissions resolves the shared session policy, persists it
// through the manifest path, and maintains the Claude project compatibility
// surface. Other harnesses consume the same resolved policy through their
// launch adapters.
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
