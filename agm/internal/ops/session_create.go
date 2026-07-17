package ops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
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
	CreateSurfaceCLI      = "cli"
	CreateSurfaceMCP      = "mcp"
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

// CreateSessionHooks are surface adapters around the one lifecycle. Hooks may
// provide interactive readiness, presentation, or storage construction, but
// tmux creation, Codex setup, manifest registration, post-create ordering, and
// rollback remain owned by CreateSessionWithContext.
type CreateSessionHooks struct {
	OpenStorage        func(context.Context) (dolt.Storage, func(), error)
	Launch             func(context.Context, HarnessLaunchSpec) (CreateSessionLaunchResult, error)
	AfterRegister      func(context.Context, *manifest.Manifest, string) error
	PostCreate         func(context.Context, *manifest.Manifest, CreateSessionLaunchResult) error
	Finalize           func(context.Context, *manifest.Manifest) error
	AfterTmuxReady     func(context.Context, string, bool) error
	CodexThreadCreator func(context.Context, string, string, string) (*manifest.Codex, error)
}

// CreateSessionLaunchResult records launch facts required by post-create
// policy. HandledLifecycle is used only by the deprecated Gemini wrapper,
// which exits after managing its own terminal lifecycle.
type CreateSessionLaunchResult struct {
	ModeAppliedAtStartup bool
	HandledLifecycle     bool
}

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
	Hooks                  *CreateSessionHooks   `json:"-"`
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

	exists, err := opCtx.Tmux.HasSession(params.name)
	if err != nil {
		return nil, ErrStorageError("tmux.HasSession", err)
	}
	if exists && !req.ReuseExistingTmux {
		return nil, sessionExistsError(params.name)
	}
	if !exists {
		if _, ok := opCtx.Tmux.(session.TmuxSessionKiller); !ok {
			return nil, ErrStorageError("tmux.rollback", fmt.Errorf("tmux backend must support KillSession before creating a rollback-owned session"))
		}
	}

	createdTmux := false
	createdManifestDir := false
	registered := false
	var store dolt.Storage
	var storageCleanup func()
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = uuid.New().String()
	}
	defer func() {
		if retErr != nil {
			rollbackCreateSession(opCtx, req, store, params.name, sessionID, createdTmux, createdManifestDir, registered)
		}
		if storageCleanup != nil {
			storageCleanup()
		}
	}()

	if !exists {
		if err := opCtx.Tmux.CreateSession(params.name, req.Cwd); err != nil {
			return nil, ErrStorageError("tmux.CreateSession", err)
		}
		createdTmux = true
	}
	if req.Hooks != nil && req.Hooks.AfterTmuxReady != nil {
		if err := req.Hooks.AfterTmuxReady(callCtx, params.name, exists); err != nil {
			return nil, err
		}
	}

	codexMeta, err := createCodexThread(callCtx, req, params)
	if err != nil {
		if os.Getenv("AGM_CODEX_REQUIRE_REMOTE_CONTROL") == "1" {
			return nil, ErrStorageError("codex.thread.start", err)
		}
		fmt.Fprintf(os.Stderr, "Warning: Codex remote-control bridge unavailable; falling back to local Codex CLI: %v\n", err)
		codexMeta = nil
	}

	launchSpec := HarnessLaunchSpec{
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
	launchResult, err := launchCreateSession(callCtx, opCtx, req, launchSpec)
	if err != nil {
		return nil, err
	}
	if launchResult.HandledLifecycle {
		return createSessionResult(req, params, sessionID), nil
	}

	registrationAllowed := true
	manifestPath := ""
	if req.ManifestDir != "" {
		manifestPath = filepath.Join(req.ManifestDir, "manifest.yaml")
		_, statErr := os.Stat(req.ManifestDir)
		manifestDirDidNotExist := os.IsNotExist(statErr)
		if err := os.MkdirAll(req.ManifestDir, 0o700); err != nil {
			if !req.ManifestDirOptional {
				return nil, ErrStorageError("manifest.mkdir", err)
			}
			registrationAllowed = false
			fmt.Fprintf(os.Stderr, "Warning: failed to create manifest directory: %v; proceeding without manifest registration\n", err)
		} else if manifestDirDidNotExist {
			createdManifestDir = true
		}
	}

	m := buildCreateSessionManifest(req, params, sessionID, codexMeta)
	if registrationAllowed {
		store, storageCleanup, err = openCreateStorage(callCtx, opCtx, req)
		if err != nil {
			return nil, ErrStorageError("storage.open", err)
		}
		if store == nil && req.RequireStorage {
			return nil, ErrStorageError("storage.open", fmt.Errorf("session storage is required"))
		}
		if store != nil {
			if err := store.CreateSession(m); err != nil {
				if req.RegistrationOptional {
					fmt.Fprintf(os.Stderr, "Warning: failed to register session: %v; the harness remains usable\n", err)
					store = nil
				} else {
					return nil, ErrStorageError("storage.CreateSession", err)
				}
			}
			if store != nil {
				registered = true
			}
		}
	}

	if req.Hooks != nil && req.Hooks.AfterRegister != nil {
		if err := req.Hooks.AfterRegister(callCtx, m, manifestPath); err != nil {
			return nil, err
		}
	}
	if req.Hooks != nil && req.Hooks.PostCreate != nil {
		if err := req.Hooks.PostCreate(callCtx, m, launchResult); err != nil {
			return nil, err
		}
	} else if req.Prompt != "" {
		if err := opCtx.Tmux.SendKeys(params.name, req.Prompt); err != nil {
			return nil, ErrStorageError("tmux.SendKeys(prompt)", err)
		}
	}
	if req.Hooks != nil && req.Hooks.Finalize != nil {
		if err := req.Hooks.Finalize(callCtx, m); err != nil {
			return nil, err
		}
	}

	return createSessionResult(req, params, sessionID), nil
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

func launchCreateSession(callCtx context.Context, opCtx *OpContext, req *CreateSessionRequest, spec HarnessLaunchSpec) (CreateSessionLaunchResult, error) {
	if req.Hooks != nil && req.Hooks.Launch != nil {
		return req.Hooks.Launch(callCtx, spec)
	}
	cmd := BuildHarnessLaunchCommand(spec)
	if err := opCtx.Tmux.SendKeys(spec.SessionName, cmd.Command); err != nil {
		return CreateSessionLaunchResult{}, ErrStorageError("tmux.SendKeys(harness)", err)
	}
	return CreateSessionLaunchResult{ModeAppliedAtStartup: cmd.ModeAppliedAtStartup}, nil
}

func openCreateStorage(callCtx context.Context, opCtx *OpContext, req *CreateSessionRequest) (dolt.Storage, func(), error) {
	if req.Hooks != nil && req.Hooks.OpenStorage != nil {
		return req.Hooks.OpenStorage(callCtx)
	}
	return opCtx.Storage, nil, nil
}

func createCodexThread(callCtx context.Context, req *CreateSessionRequest, params *createSessionParams) (*manifest.Codex, error) {
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
	if req.Hooks != nil && req.Hooks.CodexThreadCreator != nil {
		return req.Hooks.CodexThreadCreator(remoteCtx, params.name, req.Cwd, params.model)
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
	for _, existing := range values {
		if existing == value {
			return values
		}
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
		_ = store.DeleteSession(sessionID)
	}
	if createdTmux {
		if killer, ok := opCtx.Tmux.(session.TmuxSessionKiller); ok {
			_ = killer.KillSession(name)
		}
		if createdManifestDir && req.ManifestDir != "" {
			_ = os.RemoveAll(req.ManifestDir)
		}
	}
}
