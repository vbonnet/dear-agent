package ops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/agysession"
	"github.com/vbonnet/dear-agent/agm/internal/codexcontrol"
	"github.com/vbonnet/dear-agent/agm/internal/codexhooks"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/launchparity"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/permissionparity/piadapter"
	"github.com/vbonnet/dear-agent/agm/internal/pisession"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// CodexRemoteBootTimeout is the hard outer deadline for the optional Codex
// remote-control setup. It bounds the complete
// StartRemoteControl->StartThread->SetThreadName sequence for every surface.
const CodexRemoteBootTimeout = 45 * time.Second

const sharedHarnessReadyTimeout = 60 * time.Second

const (
	// CreateSurfaceCLI identifies the command-line creation surface.
	CreateSurfaceCLI = "cli"
	// CreateSurfaceMCP identifies the MCP creation surface.
	CreateSurfaceMCP = "mcp"
	// CreateSurfaceInternal identifies callers within the ops layer.
	CreateSurfaceInternal = "ops"
)

// CreateSessionCaller identifies the surface that requested a session. The
// lifecycle persists this provenance instead of inferring it from which
// adapter happened to call CreateSession.
type CreateSessionCaller struct {
	Surface string `json:"surface"`
	Source  string `json:"source,omitempty"`
}

// CreateSessionMetadata carries manifest fields that are orthogonal to the
// creation sequence. Keeping these explicit lets CLI and MCP share the same
// lifecycle without reducing the richer CLI manifest to an MCP-shaped subset.
type CreateSessionMetadata struct {
	Workspace        string
	ModelTier        string
	Tags             []string
	ContextPurpose   string
	ContextNotes     string
	ParentSessionID  *string
	PermissionPolicy *manifest.PermissionPolicy
	Sandbox          *manifest.SandboxConfig
	IsTest           bool
	Disposable       bool
	DisposableTTL    string
	PermissionMode   string
	OpenCodeServer   string
	Pi               *manifest.Pi
	PiExtension      string
	PiPolicyJSON     string
	PiPolicyFile     string
}

// CreateSessionReadiness records what the launch boundary proved about the
// interactive harness. The zero value is deliberately unverified so adding a
// runtime can never silently bypass the shared readiness gate.
type CreateSessionReadiness string

const (
	// CreateSessionReadinessVerified means the runtime already observed the
	// expected harness process and its interactive composer.
	CreateSessionReadinessVerified CreateSessionReadiness = "verified"
	// CreateSessionReadinessDeferredUntilCallerExit is reserved for prompt-free
	// current-pane creation. The harness command is queued behind the foreground
	// AGM process and cannot start until creation returns.
	CreateSessionReadinessDeferredUntilCallerExit CreateSessionReadiness = "deferred-until-caller-exit"
)

// CreateSessionLaunchResult records launch facts required by shared readiness
// and runtime completion. HandledLifecycle is used only by the deprecated
// Gemini wrapper, which exits after managing its own terminal lifecycle.
type CreateSessionLaunchResult struct {
	ModeAppliedAtStartup bool
	HandledLifecycle     bool
	Readiness            CreateSessionReadiness
	// PromptDelivered records that an AGY startup prompt was delivered before
	// provider-native identity discovery. Completion must not deliver it again.
	PromptDelivered bool
}

// AgyCreateIdentityBootstrap is the prompt delivery input required by AGY
// versions that persist a new provider conversation only after first input.
type AgyCreateIdentityBootstrap struct {
	SessionName string
	Prompt      string
}

// AgyCreateIdentityBootstrapper lets a surface preserve its safe literal-input
// implementation while the shared lifecycle continues to own phase ordering.
type AgyCreateIdentityBootstrapper interface {
	BootstrapAgyCreateIdentity(context.Context, AgyCreateIdentityBootstrap) error
}

// CreateSessionCompletion is the single post-registration input presented to
// a surface runtime. The ops module decides when completion occurs and owns
// rollback if it fails.
type CreateSessionCompletion struct {
	Manifest     *manifest.Manifest
	ManifestPath string
	Prompt       string
	Launch       CreateSessionLaunchResult
}

// CreateSessionRuntime is the harness-runtime seam. Implementations adapt
// interactive startup and surface-specific completion; they cannot insert,
// reorder, or skip tmux creation, readiness, registration, or rollback. A
// runtime must explicitly report readiness it has already verified.
type CreateSessionRuntime interface {
	Launch(context.Context, HarnessLaunchSpec) (CreateSessionLaunchResult, error)
	Complete(context.Context, CreateSessionCompletion) error
}

// SessionStorageOpener lazily constructs surface-specific storage after the
// harness has launched. The returned cleanup runs after rollback.
type SessionStorageOpener func(context.Context) (dolt.Storage, func(), error)

// CodexThreadCreator adapts the external Codex remote-control dependency.
type CodexThreadCreator func(context.Context, string, string, string) (*manifest.Codex, error)

// AgyWorkspaceCreateLocker serializes the native identity window for one AGY
// workspace and returns its release operation.
type AgyWorkspaceCreateLocker func(context.Context, string) (func() error, error)

// CreateSessionRequest defines the input for creating a new AGM session.
type CreateSessionRequest struct {
	Cwd        string `json:"cwd"`
	Prompt     string `json:"prompt"`
	Title      string `json:"title,omitempty"`
	Model      string `json:"model,omitempty"`
	Harness    string `json:"harness,omitempty"`
	Persistent bool   `json:"persistent,omitempty"`

	SessionID              string                `json:"-"`
	Caller                 CreateSessionCaller   `json:"-"`
	Metadata               CreateSessionMetadata `json:"-"`
	PermissionMode         string                `json:"-"`
	DisableAutoMode        bool                  `json:"-"`
	MaxBudgetUSD           float64               `json:"-"`
	ExtraAddDirs           []string              `json:"-"`
	BypassCodexHookTrust   bool                  `json:"-"`
	ForwardTelemetry       bool                  `json:"-"`
	ForwardClaudeOAuth     bool                  `json:"-"`
	AllowEmptyPrompt       bool                  `json:"-"`
	AllowUnsafeTitle       bool                  `json:"-"`
	ReuseExistingTmux      bool                  `json:"-"`
	RequireStorage         bool                  `json:"-"`
	RegistrationOptional   bool                  `json:"-"`
	ManifestDir            string                `json:"-"`
	ManifestDirOptional    bool                  `json:"-"`
	SkipCodexRemoteControl bool                  `json:"-"`
	CodexRemoteBootTimeout time.Duration         `json:"-"`
}

// CreateSessionResult is the output of CreateSession.
type CreateSessionResult struct {
	Operation string `json:"operation"`
	SessionID string `json:"session_id"`
	Name      string `json:"name"`
	Cwd       string `json:"cwd"`
	Model     string `json:"model"`
	Harness   string `json:"harness"`
	Source    string `json:"source,omitempty"`
	Created   bool   `json:"created"`
}

type createSessionParams struct {
	name       string
	harness    string
	model      string
	persistent bool
}

type createSessionState struct {
	createdTmux        bool
	createdManifestDir bool
	registered         bool
	store              dolt.Storage
	storageCleanup     func()
}

func (state *createSessionState) finish(opCtx *OpContext, req *CreateSessionRequest, name, sessionID string, operationErr error) {
	if operationErr != nil {
		rollbackCreateSession(opCtx, req, state.store, name, sessionID, state.createdTmux, state.createdManifestDir, state.registered)
	}
	if state.storageCleanup != nil {
		state.storageCleanup()
	}
}

func isSafeNameRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
}

func resolveSessionName(title, cwd string, allowUnsafe bool) (string, error) {
	if title != "" {
		if allowUnsafe {
			return title, nil
		}
		for _, r := range title {
			if !isSafeNameRune(r) {
				return "", ErrInvalidInput("title", "Session title contains invalid characters (only alphanumeric, hyphens, and underscores are allowed).")
			}
		}
		return title, nil
	}
	base := filepath.Base(cwd)
	var safe []rune
	for _, r := range base {
		if isSafeNameRune(r) {
			safe = append(safe, r)
		} else {
			safe = append(safe, '-')
		}
	}
	return "mcp-" + string(safe), nil
}

func defaultModelForCreateSession(harness string) string {
	if model, ok := agent.DefaultModelForHarness(harness); ok {
		return model
	}
	return "sonnet"
}

func supportedCreateHarnessesMessage() string {
	return strings.Join(agent.KnownHarnesses(), ", ")
}

func validateCreateRequest(opCtx *OpContext, req *CreateSessionRequest) (*createSessionParams, error) {
	if opCtx == nil {
		return nil, ErrInvalidInput("context", "Operation context is required.")
	}
	if req == nil || req.Cwd == "" {
		return nil, ErrInvalidInput("cwd", "Working directory (cwd) is required.")
	}
	if !filepath.IsAbs(req.Cwd) {
		return nil, ErrInvalidInput("cwd", fmt.Sprintf("Working directory must be an absolute path: %s", req.Cwd))
	}
	if req.Prompt == "" && !req.AllowEmptyPrompt {
		return nil, ErrInvalidInput("prompt", "Prompt is required.")
	}
	info, err := os.Stat(req.Cwd)
	if err != nil {
		return nil, ErrInvalidInput("cwd", fmt.Sprintf("Working directory does not exist: %s", req.Cwd))
	}
	if !info.IsDir() {
		return nil, ErrInvalidInput("cwd", fmt.Sprintf("Path is not a directory: %s", req.Cwd))
	}
	if opCtx.Tmux == nil {
		return nil, ErrTmuxNotRunning()
	}

	p := &createSessionParams{harness: agent.NormalizeHarnessName(req.Harness), model: req.Model, persistent: req.Persistent}
	if p.harness == "" {
		p.harness = "claude-code"
	}
	if err := agent.ValidateHarnessName(p.harness); err != nil {
		return nil, ErrInvalidInput("harness", fmt.Sprintf("Unsupported harness: %s. Supported: %s", p.harness, supportedCreateHarnessesMessage()))
	}
	if p.model == "" {
		p.model = defaultModelForCreateSession(p.harness)
	}
	if err := agent.ValidateModel(p.harness, p.model); err != nil {
		return nil, ErrInvalidInput("model", err.Error())
	}
	if p.harness == "agy" && strings.TrimSpace(req.Prompt) == "" {
		return nil, ErrInvalidInput("prompt", "A startup prompt is required for a fresh AGY session because AGY persists its native conversation only after first input.")
	}
	name, err := resolveSessionName(req.Title, req.Cwd, req.AllowUnsafeTitle)
	if err != nil {
		return nil, err
	}
	p.name = name
	return p, nil
}

func canonicalizeAgyCreateRequest(req *CreateSessionRequest, params *createSessionParams) (*CreateSessionRequest, error) {
	canonicalWorkDir, err := agysession.CanonicalWorkspacePath(req.Cwd)
	if err != nil {
		return nil, ErrInvalidInput("cwd", fmt.Sprintf("Resolve canonical AGY workspace: %v", err))
	}
	canonicalRequest := *req
	canonicalRequest.Cwd = canonicalWorkDir
	params.name, err = resolveSessionName(canonicalRequest.Title, canonicalWorkDir, canonicalRequest.AllowUnsafeTitle)
	if err != nil {
		return nil, err
	}
	return &canonicalRequest, nil
}

// CreateSession preserves the original operation signature for non-request
// scoped callers. Request-aware surfaces should use CreateSessionWithContext.
func CreateSession(opCtx *OpContext, req *CreateSessionRequest) (*CreateSessionResult, error) {
	return CreateSessionWithContext(context.Background(), opCtx, req)
}

// CreateSessionWithContext owns the complete shared creation lifecycle.
func CreateSessionWithContext(callCtx context.Context, opCtx *OpContext, req *CreateSessionRequest) (_ *CreateSessionResult, retErr error) {
	if callCtx == nil {
		callCtx = context.Background()
	}
	params, err := validateCreateRequest(opCtx, req)
	if err != nil {
		return nil, err
	}
	if params.harness == "agy" {
		req, err = canonicalizeAgyCreateRequest(req, params)
		if err != nil {
			return nil, err
		}
	}
	// Validate every request-derived terminal value before acquiring lifecycle
	// locks or creating either a tmux session or a remote provider thread.
	// Provider-derived metadata is validated again when the final command is
	// prepared after remote setup.
	if err := validateHarnessLaunchSpec(buildHarnessLaunchSpec(req, params, req.SessionID, nil)); err != nil {
		return nil, ErrStorageError("prepare harness launch", err)
	}
	var agyIdentityTracker agysession.CreateIdentityTracker
	var previousAgyConversationID string
	var releaseAgyWorkspaceLock func() error
	if params.harness == "agy" {
		locker := opCtx.AgyWorkspaceCreateLocker
		if locker == nil {
			locker = agysession.AcquireWorkspaceCreateLock
		}
		release, lockErr := locker(callCtx, req.Cwd)
		if lockErr != nil {
			return nil, ErrStorageError("agy.workspace-lock", lockErr)
		}
		releaseAgyWorkspaceLock = release
		defer func() {
			if releaseAgyWorkspaceLock == nil {
				return
			}
			if unlockErr := releaseAgyWorkspaceLock(); unlockErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to release AGY workspace lock: %v\n", unlockErr)
			}
		}()
		agyIdentityTracker = opCtx.AgyCreateIdentityTracker
		if agyIdentityTracker == nil {
			agyIdentityTracker = agysession.NewCreateIdentityTracker()
		}
		previousAgyConversationID, err = agyIdentityTracker.Snapshot(callCtx, req.Cwd)
		if err != nil {
			return nil, ErrStorageError("agy.identity.snapshot", err)
		}
	}

	exists, err := prepareCreateTmux(opCtx, req, params.name)
	if err != nil {
		return nil, err
	}
	sessionID := createSessionID(req.SessionID)
	if params.harness == "pi-cli" {
		prepared, prepareErr := preparePiCreateRequest(req, sessionID)
		if prepareErr != nil {
			return nil, prepareErr
		}
		req = prepared
	}
	state := &createSessionState{}
	defer func() { state.finish(opCtx, req, params.name, sessionID, retErr) }()

	if err := createTmuxForSession(opCtx, req, params.name, exists); err != nil {
		return nil, err
	}
	state.createdTmux = !exists
	codexMeta, err := optionalCodexMetadata(callCtx, opCtx, req, params)
	if err != nil {
		return nil, err
	}
	if err := verifyCreateCodexHookTrust(callCtx, req, params); err != nil {
		return nil, err
	}
	launchResult, err := launchCreateSession(callCtx, opCtx, buildHarnessLaunchSpec(req, params, sessionID, codexMeta))
	if err != nil {
		return nil, err
	}
	if launchResult.HandledLifecycle {
		return createSessionResult(req, params, sessionID), nil
	}
	if err := establishCreatedHarnessReadiness(callCtx, opCtx, req, params, launchResult); err != nil {
		return nil, err
	}
	if agyIdentityTracker != nil {
		if err := bootstrapAgyCreateIdentity(callCtx, opCtx, params.name, req.Prompt); err != nil {
			return nil, err
		}
		launchResult.PromptDelivered = true
	}

	manifestPath, registrationAllowed, createdManifestDir, err := prepareCreateManifestDir(req)
	if err != nil {
		return nil, err
	}
	state.createdManifestDir = createdManifestDir
	m := buildCreateSessionManifest(req, params, sessionID, codexMeta)
	if agyIdentityTracker != nil {
		metadata, identityErr := agyIdentityTracker.Discover(callCtx, req.Cwd, previousAgyConversationID)
		if identityErr != nil {
			return nil, ErrStorageError("agy.identity.discover", identityErr)
		}
		applyAgyCreateIdentity(m, metadata)
	}
	if registrationAllowed {
		state.store, state.storageCleanup, state.registered, err = registerCreatedSession(callCtx, opCtx, req, m)
		if err != nil {
			return nil, err
		}
	}
	// Native identity is now persisted in registration. Release before runtime
	// completion because CLI completion may block for the entire interactive
	// tmux attachment; holding the workspace lock there would deadlock every
	// create or cold resume for the same workspace until the user detaches.
	if releaseAgyWorkspaceLock != nil {
		release := releaseAgyWorkspaceLock
		releaseAgyWorkspaceLock = nil
		if unlockErr := release(); unlockErr != nil {
			return nil, ErrStorageError("agy.workspace-lock.release", unlockErr)
		}
	}
	if err := callCtx.Err(); err != nil {
		return nil, err
	}
	if err := completeCreatedSession(callCtx, opCtx, req, params.name, manifestPath, m, launchResult); err != nil {
		return nil, err
	}
	return createSessionResult(req, params, sessionID), nil
}

func verifyCreateCodexHookTrust(ctx context.Context, req *CreateSessionRequest, params *createSessionParams) error {
	if params.harness != "codex-cli" || !req.BypassCodexHookTrust {
		return nil
	}
	sandbox := req.Metadata.Sandbox
	if sandbox == nil || !sandbox.Enabled {
		return ErrInvalidInput("bypass_codex_hook_trust", "An enabled sandbox with verified hook evidence is required.")
	}
	err := codexhooks.Verify(ctx, codexhooks.Attestation{
		SourceRepo:   sandbox.CodexHookSourceRepo,
		SourceCommit: sandbox.CodexHookSourceCommit,
		Digest:       sandbox.CodexHookDigest,
	}, req.Cwd)
	if err != nil {
		return ErrInvalidInput("bypass_codex_hook_trust", fmt.Sprintf("Hook evidence failed revalidation immediately before launch: %v", err))
	}
	return nil
}

func preparePiCreateRequest(req *CreateSessionRequest, sessionID string) (*CreateSessionRequest, error) {
	if err := pisession.ValidateID(sessionID); err != nil {
		return nil, ErrInvalidInput("session_id", err.Error())
	}
	codingAgentDir, err := pisession.ValidateCodingAgentDir(os.Getenv("PI_CODING_AGENT_DIR"))
	if err != nil {
		return nil, ErrInvalidInput("PI_CODING_AGENT_DIR", err.Error())
	}
	sessionRoot, err := pisession.EnsureRoot(os.Getenv("AGM_PI_SESSION_ROOT"))
	if err != nil {
		return nil, ErrStorageError("pi.session-root", err)
	}
	extensionPath, err := piadapter.EnsureExtension(os.Getenv("AGM_PI_EXTENSION_ROOT"))
	if err != nil {
		return nil, ErrStorageError("pi.authorization-extension", err)
	}
	policyJSON, err := piadapter.MarshalPolicy(req.Metadata.permissionPolicyAllow())
	if err != nil {
		return nil, ErrStorageError("pi.permission-policy", err)
	}
	prepared := *req
	prepared.Metadata = req.Metadata
	prepared.Metadata.Pi = &manifest.Pi{
		SessionID: sessionID, SessionDir: sessionRoot,
		CodingAgentDir: codingAgentDir, CodingAgentDirSet: true,
	}
	prepared.Metadata.PiExtension = extensionPath
	prepared.Metadata.PiPolicyJSON = policyJSON
	prepared.Metadata.PiPolicyFile, err = piadapter.EnsurePolicyFile(os.Getenv("AGM_PI_EXTENSION_ROOT"), sessionID, policyJSON)
	if err != nil {
		return nil, ErrStorageError("pi.permission-policy-file", err)
	}
	return &prepared, nil
}

func (metadata CreateSessionMetadata) permissionPolicyAllow() []string {
	if metadata.PermissionPolicy == nil {
		return nil
	}
	return metadata.PermissionPolicy.Allow
}

func applyAgyCreateIdentity(m *manifest.Manifest, metadata *agysession.Metadata) {
	if m == nil || metadata == nil {
		return
	}
	m.WorkingDirectory = metadata.WorkspacePath
	if m.Context.Project == "" {
		m.Context.Project = metadata.WorkspacePath
	}
	m.Agy = &manifest.Agy{
		ConversationID: metadata.ConversationID,
		WorkspacePath:  metadata.WorkspacePath,
		ConversationDB: metadata.ConversationDBPath,
		TranscriptPath: metadata.TranscriptPath,
	}
}

func waitForCreatedHarnessReady(ctx context.Context, opCtx *OpContext, sessionName, harness string) error {
	waiter, ok := opCtx.Tmux.(session.HarnessReadinessWaiter)
	if !ok {
		return ErrStorageError("tmux.WaitForHarnessReady", fmt.Errorf("tmux backend does not expose harness readiness"))
	}
	if err := waiter.WaitForHarnessReady(ctx, sessionName, harness, sharedHarnessReadyTimeout); err != nil {
		return ErrStorageError("tmux.WaitForHarnessReady", err)
	}
	return nil
}

func establishCreatedHarnessReadiness(ctx context.Context, opCtx *OpContext, req *CreateSessionRequest, params *createSessionParams, launchResult CreateSessionLaunchResult) error {
	switch launchResult.Readiness {
	case CreateSessionReadinessVerified:
		return nil
	case CreateSessionReadinessDeferredUntilCallerExit:
		if req.Caller.Surface != CreateSurfaceCLI || !supportsDeferredCurrentTmuxReadiness(params.harness) || !req.ReuseExistingTmux || req.Prompt != "" {
			return ErrStorageError("create.readiness", fmt.Errorf("deferred readiness is valid only for supported current-tmux harness creation without an initial prompt"))
		}
		return nil
	default:
		return waitForCreatedHarnessReady(ctx, opCtx, params.name, params.harness)
	}
}

func supportsDeferredCurrentTmuxReadiness(harness string) bool {
	switch harness {
	case "claude-code", "codex-cli", "opencode-cli", "pi-cli", "gemini-cli":
		return true
	default:
		return false
	}
}

func prepareCreateTmux(opCtx *OpContext, req *CreateSessionRequest, name string) (bool, error) {
	exists, err := opCtx.Tmux.HasSession(name)
	if err != nil {
		return false, ErrStorageError("tmux.HasSession", err)
	}
	if exists {
		if !req.ReuseExistingTmux {
			return false, sessionExistsError(name)
		}
		return true, nil
	}
	if _, ok := opCtx.Tmux.(session.TmuxSessionKiller); !ok {
		return false, ErrStorageError("tmux.rollback", fmt.Errorf("tmux backend must support KillSession before creating a rollback-owned session"))
	}
	return false, nil
}

func createSessionID(requested string) string {
	if requested != "" {
		return requested
	}
	return uuid.New().String()
}

func createTmuxForSession(opCtx *OpContext, req *CreateSessionRequest, name string, exists bool) error {
	if exists {
		return nil
	}
	if err := opCtx.Tmux.CreateSession(name, req.Cwd); err != nil {
		return ErrStorageError("tmux.CreateSession", err)
	}
	return nil
}

func optionalCodexMetadata(callCtx context.Context, opCtx *OpContext, req *CreateSessionRequest, params *createSessionParams) (*manifest.Codex, error) {
	meta, err := createCodexThread(callCtx, opCtx, req, params)
	if err == nil {
		return meta, nil
	}
	if os.Getenv("AGM_CODEX_REQUIRE_REMOTE_CONTROL") == "1" {
		return nil, ErrStorageError("codex.thread.start", err)
	}
	fmt.Fprintf(os.Stderr, "Warning: Codex remote-control bridge unavailable; falling back to local Codex CLI: %v\n", err)
	return nil, nil
}

func buildHarnessLaunchSpec(req *CreateSessionRequest, params *createSessionParams, sessionID string, codexMeta *manifest.Codex) HarnessLaunchSpec {
	bypassCodexHookTrust := req.BypassCodexHookTrust &&
		req.Metadata.Sandbox != nil &&
		req.Metadata.Sandbox.Enabled
	spec := HarnessLaunchSpec{
		Harness:          params.harness,
		Model:            params.model,
		SessionName:      params.name,
		SessionID:        sessionID,
		WorkDir:          req.Cwd,
		Persistent:       params.persistent,
		PermissionMode:   req.PermissionMode,
		DisableAutoMode:  req.DisableAutoMode,
		DisableOAuth:     !req.ForwardClaudeOAuth,
		MaxBudgetUSD:     req.MaxBudgetUSD,
		ExtraAddDirs:     append([]string{}, req.ExtraAddDirs...),
		ForwardTelemetry: req.ForwardTelemetry,

		BypassCodexHookTrust: bypassCodexHookTrust,
		Codex:                codexMeta,
		Pi:                   req.Metadata.Pi,
		PiExtension:          req.Metadata.PiExtension,
		PiPolicyJSON:         req.Metadata.PiPolicyJSON,
		PiPolicyFile:         req.Metadata.PiPolicyFile,
	}
	if params.harness == "pi-cli" {
		spec.PiLaunchID = launchparity.NewPiLaunchID()
	}
	return spec
}

func cloneCreateSandbox(req *CreateSessionRequest) *manifest.SandboxConfig {
	if req.Metadata.Sandbox == nil {
		return nil
	}
	sandbox := *req.Metadata.Sandbox
	sandbox.ExtraAddDirs = nil
	sandbox.BypassCodexHookTrust = false
	if sandbox.Enabled {
		sandbox.ExtraAddDirs = append([]string{}, req.ExtraAddDirs...)
		sandbox.BypassCodexHookTrust = req.BypassCodexHookTrust
	}
	return &sandbox
}

func prepareCreateManifestDir(req *CreateSessionRequest) (manifestPath string, registrationAllowed, created bool, err error) {
	if req.ManifestDir == "" {
		return "", true, false, nil
	}
	manifestPath = filepath.Join(req.ManifestDir, "manifest.yaml")
	_, statErr := os.Stat(req.ManifestDir)
	created = os.IsNotExist(statErr)
	if mkdirErr := os.MkdirAll(req.ManifestDir, 0o700); mkdirErr != nil {
		if !req.ManifestDirOptional {
			return "", false, false, ErrStorageError("manifest.mkdir", mkdirErr)
		}
		fmt.Fprintf(os.Stderr, "Warning: failed to create manifest directory: %v; proceeding without manifest registration\n", mkdirErr)
		return "", false, false, nil
	}
	return manifestPath, true, created, nil
}

func registerCreatedSession(callCtx context.Context, opCtx *OpContext, req *CreateSessionRequest, m *manifest.Manifest) (dolt.Storage, func(), bool, error) {
	store, cleanup, err := openCreateStorage(callCtx, opCtx)
	if err != nil {
		return store, cleanup, false, ErrStorageError("storage.open", err)
	}
	if store == nil {
		if req.RequireStorage {
			return nil, cleanup, false, ErrStorageError("storage.open", fmt.Errorf("session storage is required"))
		}
		return nil, cleanup, false, nil
	}
	if err := store.CreateSession(m); err != nil {
		if !req.RegistrationOptional {
			return store, cleanup, false, ErrStorageError("storage.CreateSession", err)
		}
		fmt.Fprintf(os.Stderr, "Warning: failed to register session: %v; the harness remains usable\n", err)
		return nil, cleanup, false, nil
	}
	return store, cleanup, true, nil
}

func completeCreatedSession(callCtx context.Context, opCtx *OpContext, req *CreateSessionRequest, name, manifestPath string, m *manifest.Manifest, launchResult CreateSessionLaunchResult) error {
	if opCtx.CreationRuntime != nil {
		return opCtx.CreationRuntime.Complete(callCtx, CreateSessionCompletion{
			Manifest: m, ManifestPath: manifestPath, Prompt: req.Prompt, Launch: launchResult,
		})
	}
	if req.Prompt == "" || launchResult.PromptDelivered {
		return nil
	}
	return sendCreatedInputAtomically(callCtx, opCtx, name, m.Harness, req.Prompt, "create.initial-prompt")
}

func bootstrapAgyCreateIdentity(callCtx context.Context, opCtx *OpContext, sessionName, prompt string) error {
	if err := callCtx.Err(); err != nil {
		return err
	}
	input := AgyCreateIdentityBootstrap{SessionName: sessionName, Prompt: prompt}
	if bootstrapper, ok := opCtx.CreationRuntime.(AgyCreateIdentityBootstrapper); ok {
		if err := bootstrapper.BootstrapAgyCreateIdentity(callCtx, input); err != nil {
			if ctxErr := callCtx.Err(); ctxErr != nil {
				return ctxErr
			}
			return ErrStorageError("agy.identity.bootstrap-prompt", err)
		}
		return nil
	}
	return sendCreatedInputAtomically(callCtx, opCtx, sessionName, "agy", prompt, "agy.identity.bootstrap-prompt")
}

func sendCreatedInputAtomically(callCtx context.Context, opCtx *OpContext, sessionName, harness, input, operation string) error {
	if err := callCtx.Err(); err != nil {
		return err
	}
	sender, ok := opCtx.Tmux.(session.AtomicInputSender)
	if !ok {
		return ErrStorageError(operation, fmt.Errorf("tmux backend does not expose atomic input delivery"))
	}
	readiness, err := sender.SendKeysIfInputReady(callCtx, sessionName, harness, input, session.InputDeliveryOptions{})
	if err != nil {
		if ctxErr := callCtx.Err(); ctxErr != nil {
			return ctxErr
		}
		return ErrStorageError(operation, err)
	}
	if !readiness.Ready {
		return ErrStorageError(operation, fmt.Errorf("harness input is %s", readiness.State))
	}
	if readiness.PaneID == "" {
		return ErrStorageError(operation, fmt.Errorf("harness returned no verified pane"))
	}
	return nil
}

func sessionExistsError(name string) error {
	return &OpError{
		Status: 409,
		Type:   "session/exists",
		Code:   ErrCodeSessionExists,
		Title:  "Session already exists",
		Detail: fmt.Sprintf("A tmux session named %q already exists.", name),
		Suggestions: []string{
			"Use a different title.",
			fmt.Sprintf("Archive the existing session: agm session archive %s", name),
		},
		Parameters: map[string]string{"title": name},
	}
}

func launchCreateSession(callCtx context.Context, opCtx *OpContext, spec HarnessLaunchSpec) (CreateSessionLaunchResult, error) {
	if opCtx.CreationRuntime != nil {
		return opCtx.CreationRuntime.Launch(callCtx, spec)
	}
	cmd, err := PrepareHarnessLaunchCommand(spec)
	if err != nil {
		return CreateSessionLaunchResult{}, ErrStorageError("prepare harness launch", err)
	}
	if submissionErr := opCtx.Tmux.SendKeys(spec.SessionName, cmd.Command); submissionErr != nil {
		if _, err := ResolveHarnessLaunchSubmission(cmd, submissionErr); err != nil {
			return CreateSessionLaunchResult{}, ErrStorageError("tmux.SendKeys(harness)", err)
		}
	}
	return CreateSessionLaunchResult{ModeAppliedAtStartup: cmd.ModeAppliedAtStartup}, nil
}

func openCreateStorage(callCtx context.Context, opCtx *OpContext) (dolt.Storage, func(), error) {
	if opCtx.OpenSessionStorage != nil {
		return opCtx.OpenSessionStorage(callCtx)
	}
	return opCtx.Storage, nil, nil
}

func createCodexThread(callCtx context.Context, opCtx *OpContext, req *CreateSessionRequest, params *createSessionParams) (*manifest.Codex, error) {
	if params.harness != "codex-cli" || req.SkipCodexRemoteControl || os.Getenv("AGM_CODEX_REMOTE_CONTROL") == "0" {
		return nil, nil
	}
	if err := agent.EnsureCodexWorkdirTrusted(req.Cwd); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not pre-trust Codex workdir %s: %v\n", req.Cwd, err)
	}
	timeout := req.CodexRemoteBootTimeout
	if timeout <= 0 {
		timeout = CodexRemoteBootTimeout
	}
	remoteCtx, cancel := context.WithTimeout(callCtx, timeout)
	defer cancel()
	if opCtx.CodexThreadCreator != nil {
		return opCtx.CodexThreadCreator(remoteCtx, params.name, req.Cwd, params.model)
	}
	return createCodexRemoteThread(remoteCtx, params.name, req.Cwd, params.model)
}

func createCodexRemoteThread(ctx context.Context, sessionName, workDir, model string) (*manifest.Codex, error) {
	client := codexcontrol.New()
	if err := client.StartRemoteControl(ctx); err != nil {
		return nil, err
	}
	thread, err := client.StartThread(ctx, codexcontrol.StartThreadOptions{
		CWD:   workDir,
		Model: agent.ResolveModelFullName("codex-cli", model),
	})
	if err != nil {
		return nil, err
	}
	if err := client.SetThreadName(ctx, thread.ID, sessionName); err != nil {
		return nil, err
	}
	return &manifest.Codex{SessionID: thread.ID, TranscriptPath: thread.Path}, nil
}

func buildCreateSessionManifest(req *CreateSessionRequest, params *createSessionParams, sessionID string, codexMeta *manifest.Codex) *manifest.Manifest {
	now := time.Now()
	tags := append([]string{}, req.Metadata.Tags...)
	source := createCallerSource(req)
	if source != "" {
		tags = appendUniqueString(tags, "source:"+source)
	}
	m := &manifest.Manifest{
		SchemaVersion:   manifest.SchemaVersion,
		SessionID:       sessionID,
		Name:            params.name,
		CreatedAt:       now,
		UpdatedAt:       now,
		Workspace:       req.Metadata.Workspace,
		ParentSessionID: cloneCreateString(req.Metadata.ParentSessionID),
		Context: manifest.Context{
			Project: req.Cwd,
			Purpose: req.Metadata.ContextPurpose,
			Tags:    tags,
			Notes:   req.Metadata.ContextNotes,
		},
		Tmux:             manifest.Tmux{SessionName: params.name},
		Harness:          params.harness,
		Model:            params.model,
		ModelTier:        req.Metadata.ModelTier,
		Claude:           manifest.Claude{},
		PermissionPolicy: cloneCreatePermissionPolicy(req.Metadata.PermissionPolicy),
		Sandbox:          cloneCreateSandbox(req),
		IsTest:           req.Metadata.IsTest,
		Disposable:       req.Metadata.Disposable,
		DisposableTTL:    req.Metadata.DisposableTTL,
	}
	if codexMeta != nil {
		meta := *codexMeta
		m.Codex = &meta
	}
	if req.Metadata.Pi != nil {
		meta := *req.Metadata.Pi
		m.Pi = &meta
		m.WorkingDirectory = req.Cwd
	}
	mode := req.Metadata.PermissionMode
	if mode == "" {
		mode = req.PermissionMode
	}
	if mode != "" {
		m.PermissionMode = mode
		m.PermissionModeUpdatedAt = &now
		m.PermissionModeSource = "startup"
	}
	if params.harness == "opencode-cli" {
		host := req.Metadata.OpenCodeServer
		if host == "" {
			host = "localhost"
		}
		m.OpenCode = &manifest.OpenCode{ServerPort: 4096, ServerHost: host, AttachTime: now}
	}
	return m
}

func cloneCreateString(value *string) *string {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneCreatePermissionPolicy(policy *manifest.PermissionPolicy) *manifest.PermissionPolicy {
	if policy == nil {
		return nil
	}
	clone := *policy
	clone.Sources = append([]string{}, policy.Sources...)
	clone.Explicit = append([]string{}, policy.Explicit...)
	clone.Allow = append([]string{}, policy.Allow...)
	clone.Targets = append([]manifest.PermissionPolicyTarget{}, policy.Targets...)
	return &clone
}

func appendUniqueString(values []string, value string) []string {
	if slices.Contains(values, value) {
		return values
	}
	return append(values, value)
}

func createSessionResult(req *CreateSessionRequest, params *createSessionParams, sessionID string) *CreateSessionResult {
	source := createCallerSource(req)
	return &CreateSessionResult{
		Operation: "create_session",
		SessionID: sessionID,
		Name:      params.name,
		Cwd:       req.Cwd,
		Model:     params.model,
		Harness:   params.harness,
		Source:    source,
		Created:   true,
	}
}

func createCallerSource(req *CreateSessionRequest) string {
	if req.Caller.Source != "" {
		return req.Caller.Source
	}
	if req.Caller.Surface != "" {
		return req.Caller.Surface
	}
	return CreateSurfaceInternal
}

func rollbackCreateSession(opCtx *OpContext, req *CreateSessionRequest, store dolt.Storage, name, sessionID string, createdTmux, createdManifestDir, registered bool) {
	if registered && store != nil && sessionID != "" {
		if err := store.DeleteSession(sessionID); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to delete session registration %q during create rollback: %v\n", sessionID, err)
		}
	}
	if createdTmux {
		if killer, ok := opCtx.Tmux.(session.TmuxSessionKiller); ok {
			if err := killer.KillSession(name); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to kill tmux session %q during create rollback: %v\n", name, err)
			}
		}
		if createdManifestDir && req.ManifestDir != "" {
			if err := os.RemoveAll(req.ManifestDir); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to remove manifest directory %q during create rollback: %v\n", req.ManifestDir, err)
			}
		}
	}
}
