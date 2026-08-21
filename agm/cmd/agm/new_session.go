package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"syscall"

	"github.com/charmbracelet/huh"
	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/cli"
	"github.com/vbonnet/dear-agent/agm/internal/codexhooks"
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
	"github.com/vbonnet/dear-agent/pkg/override"
)

var resolvedSessionPermissionPolicy *manifest.PermissionPolicy
var checkExpectedHarnessInputAndSend = tmux.CheckExpectedHarnessInputAndSend

type cliCreateSessionRuntime struct {
	prepare              func(context.Context, ops.CreateSessionPreparation) (ops.CreateSessionPreparation, error)
	launch               func(context.Context, ops.HarnessLaunchSpec) (ops.CreateSessionLaunchResult, error)
	bootstrapAgyIdentity func(context.Context, ops.AgyCreateIdentityBootstrap) error
	complete             func(context.Context, ops.CreateSessionCompletion) error
}

func (r *cliCreateSessionRuntime) Prepare(ctx context.Context, input ops.CreateSessionPreparation) (ops.CreateSessionPreparation, error) {
	if r.prepare == nil {
		return input, nil
	}
	return r.prepare(ctx, input)
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

func newCLICreateSessionRuntime(
	sessionName string,
	existed, trustPreConfigured bool,
	admission *circuitBreakerAdmission,
) *cliCreateSessionRuntime {
	return &cliCreateSessionRuntime{
		launch: func(ctx context.Context, spec ops.HarnessLaunchSpec) (ops.CreateSessionLaunchResult, error) {
			if admission != nil {
				spec.BeforeSpawn = admission.beforeSpawn
				spec.AfterAuthorization = admission.afterAuthorization
				spec.OnAbort = func() {
					if admission.onAbort != nil {
						admission.onAbort()
					}
				}
			}
			return launchCLICreateSession(ctx, spec, existed, trustPreConfigured)
		},
		bootstrapAgyIdentity: func(ctx context.Context, input ops.AgyCreateIdentityBootstrap) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			readiness, err := checkExpectedHarnessInputAndSend(ctx, input.SessionName, "agy", input.Prompt, tmux.InputDeliveryOptions{})
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
	admission, err := preflight(sessionName)
	if err != nil {
		return err
	}

	workDir, err := getWorkDir()
	if err != nil {
		return err
	}
	fmt.Printf("Creating new tmux session: %s (in %s)\n", sessionName, workDir)
	announceFrameworkGuardrails(workDir)
	announceAcceptanceCriteria(workDir)

	sessionID := uuid.New().String()
	exists, retry, err := resolveTmuxSession(sessionName)
	if err != nil {
		return err
	}
	if retry != "" {
		return createTmuxSessionAndStartClaude(ctx, retry)
	}

	return runCreateSessionLifecycle(ctx, sessionName, sessionID, workDir, exists, admission)
}

// runCreateSessionLifecycle adapts CLI presentation and readiness behavior to
// the shared ops lifecycle. Business ordering and rollback stay in ops.
func runCreateSessionLifecycle(
	ctx context.Context,
	sessionName, sessionID, workDir string,
	exists bool,
	admission *circuitBreakerAdmission,
) (retErr error) {
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
	var sandboxInfo *manifest.SandboxConfig
	var extraAddDirs []string
	var trustPreConfigured bool
	var bypassCodexHookTrust bool
	runtime := newCLICreateSessionRuntime(sessionName, exists, false, admission)
	runtime.launch = func(launchCtx context.Context, spec ops.HarnessLaunchSpec) (ops.CreateSessionLaunchResult, error) {
		return launchCLICreateSession(launchCtx, spec, exists, trustPreConfigured)
	}
	runtime.prepare = func(prepareCtx context.Context, input ops.CreateSessionPreparation) (ops.CreateSessionPreparation, error) {
		trustedAddDirs, guardPath, prepareErr := trustedAddDirsForSession(sessionName, roleName)
		if prepareErr != nil {
			return ops.CreateSessionPreparation{}, prepareErr
		}
		preparedSandbox, preparedWorkDir, prepareErr := maybeProvisionSandbox(prepareCtx, input.SessionID, input.Cwd)
		sandboxInfo = preparedSandbox
		if prepareErr != nil {
			return ops.CreateSessionPreparation{}, prepareErr
		}
		extraAddDirs, trustPreConfigured = collectExtraAddDirs(preparedSandbox, trustedAddDirs)
		if prepareErr := configureWorkerWriteBoundary(harnessName, roleName, guardPath, extraAddDirs); prepareErr != nil {
			return ops.CreateSessionPreparation{}, prepareErr
		}
		if prepareErr := configureProjectPermissions(preparedWorkDir); prepareErr != nil {
			return ops.CreateSessionPreparation{}, prepareErr
		}
		bypassCodexHookTrust, prepareErr = prepareCodexHookTrustBypass(prepareCtx, preparedSandbox)
		if prepareErr != nil {
			return ops.CreateSessionPreparation{}, prepareErr
		}
		return ops.CreateSessionPreparation{
			SessionID: input.SessionID, SessionName: input.SessionName,
			Cwd: preparedWorkDir, ExtraAddDirs: extraAddDirs,
			PermissionPolicy: resolvedSessionPermissionPolicy, Sandbox: preparedSandbox,
			BypassCodexHookTrust: bypassCodexHookTrust,
		}, nil
	}
	defer func() {
		if retErr != nil && sandboxInfo != nil {
			cleanupSandbox(ctx, sandboxInfo.ID, sandboxInfo.Provider)
		}
	}()
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
		Cwd:                  workDir,
		Prompt:               createPrompt,
		Title:                sessionName,
		Model:                modelName,
		Harness:              harnessName,
		Persistent:           persistent,
		SessionID:            sessionID,
		Caller:               ops.CreateSessionCaller{Surface: ops.CreateSurfaceCLI},
		PermissionMode:       modeFlagValue,
		DisableAutoMode:      noAutoMode,
		MaxBudgetUSD:         maxBudgetUsd,
		ExtraAddDirs:         extraAddDirs,
		BypassCodexHookTrust: bypassCodexHookTrust,
		ForwardTelemetry:     true,
		ForwardClaudeOAuth:   true,
		AllowEmptyPrompt:     true,
		AllowUnsafeTitle:     true,
		ReuseExistingTmux:    exists,
		RequireStorage:       true,
		ManifestDir:          manifestDir,
		ManifestDirOptional:  true,
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

// prepareCodexHookTrustBypass validates and attests a requested hook-trust
// override. Command preparation reserves authorization for this exact source
// identity; the launch callback seals it atomically with any admission-brake
// claim into the private handoff, and the executor revalidates and commits the
// complete transaction only after every other fallible launch check.
func prepareCodexHookTrustBypass(ctx context.Context, sandboxInfo *manifest.SandboxConfig) (bool, error) {
	reason := codexHookTrustBypassReason
	if reason == "" {
		reason = cfg.Sandbox.BypassCodexHookTrustReason
	}
	if harnessName != "codex-cli" || reason == "" {
		return false, nil
	}
	normalizedReason, err := override.ValidateReason(reason)
	if err != nil {
		return false, fmt.Errorf("refusing Codex hook-trust bypass: %w", err)
	}
	reason = normalizedReason
	if sandboxInfo == nil || !sandboxInfo.Enabled {
		// Outside a sandbox the hooks sit at their reviewed golden path, so the
		// override buys nothing and would only widen what runs unreviewed.
		return false, nil
	}
	if len(cfg.Sandbox.Repos) == 0 || sandboxInfo.CodexHookSourceRepo == "" {
		return false, fmt.Errorf("the Codex hook-trust override requires an explicit sandbox.repos source")
	}
	storeBase, err := codexhooks.DefaultStoreBase()
	if err != nil {
		return false, fmt.Errorf("refusing Codex hook-trust bypass: %w", err)
	}
	writableRoots := append([]string{sandboxInfo.WorkingDir, sandboxInfo.MergedPath}, cfg.Sandbox.WritableDirs...)
	attestation, err := codexhooks.Attest(
		ctx, sandboxInfo.CodexHookSourceRepo, sandboxInfo.WorkingDir, storeBase, writableRoots,
	)
	if err != nil {
		return false, fmt.Errorf("refusing Codex hook-trust bypass: %w", err)
	}
	sandboxInfo.CodexHookSourceRepo = attestation.SourceRepo
	sandboxInfo.CodexHookSourceCommit = attestation.SourceCommit
	sandboxInfo.CodexHookDigest = attestation.Digest
	sandboxInfo.CodexHookRoot = attestation.HookRoot
	sandboxInfo.BypassCodexHookTrustReason = reason
	ui.PrintSuccess("Codex hook-trust override attested; exact-source authorization will be recorded at launch")
	return true, nil
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
func preflight(sessionName string) (*circuitBreakerAdmission, error) {
	hostHome := os.Getenv("HOME")
	if err := setupTestEnvironment(); err != nil {
		return nil, err
	}
	if err := rebindAuthorityAfterTestEnvironment(hostHome); err != nil {
		return nil, err
	}
	if testMode {
		return nil, nil
	}
	if dupErr := checkDuplicateSessionName(sessionName); dupErr != nil {
		return nil, dupErr
	}
	admission, err := enforceCircuitBreakers(sessionName, harnessName, modelName)
	if err != nil {
		return nil, err
	}
	return admission, nil
}

// rebindAuthorityAfterTestEnvironment recaptures the runtime authority when
// test-environment activation moved HOME. Configuration — and with it the
// authority that binds sandbox creation — is loaded in PersistentPreRunE,
// before --test/AGM_TEST_SANDBOX relocates HOME. Leaving the host roots in
// place would provision the isolated run's sandbox inside production storage
// and scan the host HOME for lower dirs.
func rebindAuthorityAfterTestEnvironment(hostHome string) error {
	isolatedHome := os.Getenv("HOME")
	if isolatedHome == "" || isolatedHome == hostHome {
		return nil
	}
	if err := cfg.RebindRuntimeAuthorityToIsolatedHome(isolatedHome); err != nil {
		return fmt.Errorf("rebind runtime authority to isolated HOME %q: %w", isolatedHome, err)
	}
	debug.Log("Rebound runtime authority to isolated HOME: %s", isolatedHome)
	return nil
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

// collectExtraAddDirs returns the explicitly configured host paths that Codex
// may modify outside its sandbox workspace. Source repositories are lower
// layers inside the sandbox and must never be forwarded as writable add-dirs.
// The second return value reports that trust was pre-configured.
// codexHookTrustBypassReason is the CLI-supplied justification. The flag takes
// a reason rather than being a bool: the caller must say why, and that text is
// what the recurring override audit reads.
var codexHookTrustBypassReason string

func collectExtraAddDirs(sandboxInfo *manifest.SandboxConfig, requested []string) ([]string, bool) {
	return collectExtraAddDirsForHarness(sandboxInfo, harnessName, roleName, requested), true
}

// collectExtraAddDirsForHarness resolves the current configured writable roots
// for one sandboxed harness. Keeping this independent of the create command's
// globals lets cold resume reconcile sessions created under an older config.
func collectExtraAddDirsForHarness(sandboxInfo *manifest.SandboxConfig, harness, role string, requested []string) []string {
	debug.Phase("Configure Trust")
	var extraAddDirs []string
	isCodexWorker := agent.NormalizeHarnessName(harness) == "codex-cli" && role == "worker"
	if sandboxInfo != nil {
		if !isCodexWorker {
			for _, repoDir := range cfg.Sandbox.Repos {
				extraAddDirs = appendUnique(extraAddDirs, repoDir)
				debug.Log("Will pre-authorize source repo via --add-dir: %s", repoDir)
			}
		}
		// Sandboxed sessions are otherwise confined to their workspace, so any
		// real worktree or shared task database is read-only to them. Without
		// these a worker can do the work but cannot land it.
		if isCodexWorker {
			for _, dir := range cfg.Sandbox.WritableDirs {
				extraAddDirs = appendUnique(extraAddDirs, dir)
				debug.Log("Will pre-authorize writable dir via --add-dir: %s", dir)
			}
		}
	}
	for _, dir := range requested {
		extraAddDirs = appendUnique(extraAddDirs, dir)
		debug.Log("Will pre-authorize session dir via --add-dir: %s", dir)
	}
	return extraAddDirs
}

func appendUnique(values []string, value string) []string {
	if slices.Contains(values, value) {
		return values
	}
	return append(values, value)
}

const (
	trustedAddDirsEnv        = "AGM_TRUSTED_ADD_DIRS_JSON"
	trustedAddDirsSessionEnv = "AGM_TRUSTED_ADD_DIRS_SESSION"
	trustedGuardPathEnv      = "AGM_TRUSTED_GUARD_PATH"
	workerWriteRootsEnv      = "AGM_WORKER_WRITE_ROOTS_JSON"
	defaultWorkerGuardPath   = "/etc/codex/hooks/pretool-worker-write-boundary"
)

// trustedAddDirsForSession consumes the host dispatcher's one-launch handoff.
// The variables are removed before the harness starts, so a worker cannot copy
// the trusted parent's authority into later child sessions. A nested AGM call
// from inside an existing sandbox also cannot loosen its parent OS sandbox.
func trustedAddDirsForSession(sessionName, role string) ([]string, string, error) {
	raw := os.Getenv(trustedAddDirsEnv)
	boundSession := os.Getenv(trustedAddDirsSessionEnv)
	guardPath := os.Getenv(trustedGuardPathEnv)
	_ = os.Unsetenv(trustedAddDirsEnv)
	_ = os.Unsetenv(trustedAddDirsSessionEnv)
	_ = os.Unsetenv(trustedGuardPathEnv)
	if raw == "" && boundSession == "" && guardPath == "" {
		return nil, "", nil
	}
	if raw == "" || boundSession != sessionName {
		return nil, "", fmt.Errorf("trusted worker handoff is incomplete or bound to another session")
	}
	if role != "worker" {
		return nil, "", fmt.Errorf("trusted worker handoff requires worker role")
	}
	out, err := validateTrustedAddDirs(raw)
	if err != nil {
		return nil, "", err
	}
	if err := validateTrustedWorkerGuard(guardPath); err != nil {
		return nil, "", err
	}
	return out, guardPath, nil
}

func validateTrustedAddDirs(raw string) ([]string, error) {
	var dirs []string
	if err := json.Unmarshal([]byte(raw), &dirs); err != nil {
		return nil, fmt.Errorf("decode trusted add-dir handoff: %w", err)
	}
	var out []string
	for _, dir := range dirs {
		if !filepath.IsAbs(dir) {
			return nil, fmt.Errorf("trusted add-dir must be absolute: %q", dir)
		}
		info, err := os.Stat(dir)
		if err != nil {
			return nil, fmt.Errorf("inspect trusted add-dir %s: %w", dir, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("trusted add-dir is not a directory: %s", dir)
		}
		out = appendUnique(out, dir)
	}
	return out, nil
}

func validateTrustedWorkerGuard(guardPath string) error {
	if guardPath == "" {
		return nil
	}
	if !filepath.IsAbs(guardPath) {
		return fmt.Errorf("trusted worker guard path must be absolute: %q", guardPath)
	}
	info, err := os.Stat(guardPath) // #nosec G703 -- path is an authenticated absolute host handoff, not worker input.
	if err != nil {
		return fmt.Errorf("inspect trusted worker guard %s: %w", guardPath, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return fmt.Errorf("trusted worker guard is not an executable file: %s", guardPath)
	}
	return nil
}

func configureWorkerWriteBoundary(harness, role, guardPath string, dirs []string) error {
	_ = os.Unsetenv(workerWriteRootsEnv)
	if harness != "codex-cli" || role != "worker" {
		return nil
	}
	if guardPath == "" {
		return fmt.Errorf("codex worker launch requires the managed write-boundary guard at %s", defaultWorkerGuardPath)
	}
	if len(dirs) == 0 {
		return fmt.Errorf("codex worker launch requires at least one host-authorized write root")
	}
	info, err := os.Stat(guardPath) // #nosec G703 -- path was validated from the authenticated host handoff.
	if err != nil {
		return fmt.Errorf("inspect Codex worker write-boundary guard: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return fmt.Errorf("codex worker write-boundary guard is not an executable file: %s", guardPath)
	}
	payload, err := json.Marshal(dirs)
	if err != nil {
		return fmt.Errorf("encode Codex worker write roots: %w", err)
	}
	if err := os.Setenv(workerWriteRootsEnv, string(payload)); err != nil {
		return fmt.Errorf("set Codex worker write roots: %w", err)
	}
	return nil
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
