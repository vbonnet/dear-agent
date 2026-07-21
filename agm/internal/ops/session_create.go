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
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// CodexRemoteBootTimeout is the hard outer deadline for the optional Codex
// remote-control setup. It bounds the complete
// StartRemoteControl->StartThread->SetThreadName sequence for every surface.
const CodexRemoteBootTimeout = 45 * time.Second

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
	PermissionPolicy *manifest.PermissionPolicy
	Sandbox          *manifest.SandboxConfig
	IsTest           bool
	Disposable       bool
	DisposableTTL    string
	PermissionMode   string
	OpenCodeServer   string
}

// CreateSessionLaunchResult records launch facts required by runtime
// completion. HandledLifecycle is used only by the deprecated Gemini wrapper,
// which exits after managing its own terminal lifecycle.
type CreateSessionLaunchResult struct {
	ModeAppliedAtStartup bool
	HandledLifecycle     bool
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
// reorder, or skip tmux creation, registration, or rollback.
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
	name, err := resolveSessionName(req.Title, req.Cwd, req.AllowUnsafeTitle)
	if err != nil {
		return nil, err
	}
	p.name = name
	return p, nil
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
	launchResult, err := launchCreateSession(callCtx, opCtx, buildHarnessLaunchSpec(req, params, sessionID, codexMeta))
	if err != nil {
		return nil, err
	}
	if launchResult.HandledLifecycle {
		return createSessionResult(req, params, sessionID), nil
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
	return HarnessLaunchSpec{
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
		Codex:            codexMeta,
	}
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
	if req.Prompt == "" {
		return nil
	}
	if err := opCtx.Tmux.SendKeys(name, req.Prompt); err != nil {
		return ErrStorageError("tmux.SendKeys(prompt)", err)
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
	cmd := BuildHarnessLaunchCommand(spec)
	if err := opCtx.Tmux.SendKeys(spec.SessionName, cmd.Command); err != nil {
		return CreateSessionLaunchResult{}, ErrStorageError("tmux.SendKeys(harness)", err)
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
		SchemaVersion:    manifest.SchemaVersion,
		SessionID:        sessionID,
		Name:             params.name,
		CreatedAt:        now,
		UpdatedAt:        now,
		Workspace:        req.Metadata.Workspace,
		Context:          manifest.Context{Project: req.Cwd, Tags: tags},
		Tmux:             manifest.Tmux{SessionName: params.name},
		Harness:          params.harness,
		Model:            params.model,
		ModelTier:        req.Metadata.ModelTier,
		Claude:           manifest.Claude{},
		PermissionPolicy: cloneCreatePermissionPolicy(req.Metadata.PermissionPolicy),
		Sandbox:          req.Metadata.Sandbox,
		IsTest:           req.Metadata.IsTest,
		Disposable:       req.Metadata.Disposable,
		DisposableTTL:    req.Metadata.DisposableTTL,
	}
	if codexMeta != nil {
		meta := *codexMeta
		m.Codex = &meta
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
