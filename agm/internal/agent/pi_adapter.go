package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/launchparity"
	"github.com/vbonnet/dear-agent/agm/internal/permissionparity/piadapter"
	"github.com/vbonnet/dear-agent/agm/internal/pisession"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// PiAdapter implements Agent using Pi's interactive CLI and native JSONL.
type PiAdapter struct {
	sessionStore SessionStore
}

var (
	piHasSession         = tmux.HasSession
	piNewSession         = tmux.NewSession
	piSendShellCommand   = tmux.SendCommand
	piSendCommandLiteral = tmux.SendCommandLiteral
	piSendPromptLiteral  = tmux.SendPromptLiteral
	piWaitForPrompt      = tmux.WaitForPiLaunchPromptContext
	piKillSession        = tmux.KillSessionWithError
	piCheckProcess       = tmux.IsPiProcessInPaneTree
	piCheckHarness       = tmux.CheckPaneLiveness
	piAttachSession      = tmux.AttachSession
	piIsIdle             = tmux.IsPiIdle
)

// NewPiAdapter creates a Pi CLI adapter.
func NewPiAdapter(sessionStore SessionStore) (Agent, error) {
	if sessionStore == nil {
		store, err := NewJSONSessionStore("")
		if err != nil {
			return nil, fmt.Errorf("create Pi session store: %w", err)
		}
		sessionStore = store
	}
	return &PiAdapter{sessionStore: sessionStore}, nil
}

// Name returns Pi's canonical AGM harness identifier.
func (a *PiAdapter) Name() string { return "pi-cli" }

// Version returns the Pi executable identifier used for availability checks.
func (a *PiAdapter) Version() string { return "pi-cli" }

// CreateSession creates Pi with a caller-owned native id and fatal readiness.
//
//nolint:gocyclo // reason: lifecycle transaction keeps validation, launch, readiness, persistence, and rollback ordering explicit
func (a *PiAdapter) CreateSession(ctx SessionContext) (SessionID, error) {
	if err := ValidateHarnessAvailability("pi-cli"); err != nil {
		return "", err
	}
	workDir, err := canonicalPiWorkDir(ctx.WorkingDirectory)
	if err != nil {
		return "", err
	}
	tmuxName := ctx.Name
	if tmuxName == "" {
		tmuxName = "pi-" + time.Now().Format("20060102-150405")
	}
	exists, err := piHasSession(tmuxName)
	if err != nil {
		return "", fmt.Errorf("check Pi tmux session: %w", err)
	}
	if exists {
		return "", fmt.Errorf("refusing to create Pi session %q: tmux session already exists", tmuxName)
	}
	sessionID := SessionID(uuid.New().String())
	if err := pisession.ValidateID(string(sessionID)); err != nil {
		return "", err
	}
	codingAgentDir, err := pisession.ValidateCodingAgentDir(os.Getenv("PI_CODING_AGENT_DIR"))
	if err != nil {
		return "", err
	}
	sessionDir, err := ensurePiSessionRoot()
	if err != nil {
		return "", err
	}
	model := ctx.Model
	if model == "" {
		model = ctx.Environment["AGM_MODEL"]
	}
	if model == "" {
		model, _ = DefaultModelForHarness("pi-cli")
	}
	if err := ValidateModel("pi-cli", model); err != nil {
		return "", err
	}
	permissionMode := ctx.Environment["AGM_PERMISSION_MODE"]
	permissionPolicyJSON, err := normalizePiPermissionPolicy(ctx.Environment["AGM_PI_PERMISSION_POLICY"])
	if err != nil {
		return "", err
	}
	extensionPath, err := piadapter.EnsureExtension(os.Getenv("AGM_PI_EXTENSION_ROOT"))
	if err != nil {
		return "", fmt.Errorf("install Pi authorization extension: %w", err)
	}
	policyFile, err := piadapter.EnsurePolicyFile(os.Getenv("AGM_PI_EXTENSION_ROOT"), string(sessionID), permissionPolicyJSON)
	if err != nil {
		return "", fmt.Errorf("install Pi permission policy: %w", err)
	}
	if err := piNewSession(tmuxName, workDir); err != nil {
		return "", fmt.Errorf("create Pi tmux session: %w", err)
	}
	launchID := launchparity.NewPiLaunchID()
	command := buildPiAdapterCommand(tmuxName, string(sessionID), launchID, sessionDir, codingAgentDir, workDir, model, permissionMode, extensionPath, policyFile)
	if err := piSendShellCommand(tmuxName, command); err != nil {
		return "", rollbackPiAdapterSession(tmuxName, fmt.Errorf("start Pi in tmux: %w", err))
	}
	if err := piWaitForPrompt(context.Background(), tmuxName, launchID, 30*time.Second); err != nil {
		return "", rollbackPiAdapterSession(tmuxName, fmt.Errorf("pi did not become ready after create: %w", err))
	}
	metadata := &SessionMetadata{
		TmuxName: tmuxName, Title: ctx.Name, CreatedAt: time.Now(), WorkingDir: workDir,
		Project: ctx.Project, Model: model, PermissionMode: permissionMode,
		AuthorizedDirs: append([]string(nil), ctx.AuthorizedDirs...), UUID: string(sessionID),
		NativeSessionDir: sessionDir, PermissionPolicyJSON: permissionPolicyJSON,
		CodingAgentDir: codingAgentDir,
	}
	if transcript, findErr := pisession.FindTranscript(sessionDir, string(sessionID)); findErr == nil {
		metadata.TranscriptPath = transcript
	}
	if err := a.sessionStore.Set(sessionID, metadata); err != nil {
		return "", rollbackPiAdapterSession(tmuxName, fmt.Errorf("store Pi session metadata: %w", err))
	}
	return sessionID, nil
}

func buildPiAdapterCommand(tmuxName, nativeID, launchID, sessionDir, codingAgentDir, workDir, model, permissionMode, extensionPath, permissionPolicyFile string) string {
	return launchparity.BuildPiCommand(launchparity.PiCommandSpec{
		WorkDir: workDir, ResolvedModel: ResolveModelFullName("pi-cli", model),
		SessionName: tmuxName, SessionID: nativeID, LaunchID: launchID, SessionDir: sessionDir,
		CodingAgentDir: codingAgentDir,
		PermissionMode: permissionMode, PermissionExtension: extensionPath,
		PermissionPolicyFile: permissionPolicyFile,
	}).Command
}

func normalizePiPermissionPolicy(raw string) (string, error) {
	type policy struct {
		Allow []string `json:"allow"`
	}
	if strings.TrimSpace(raw) == "" {
		raw = `{"allow":[]}`
	}
	var parsed policy
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return "", fmt.Errorf("invalid Pi permission policy: %w", err)
	}
	normalized, err := json.Marshal(parsed)
	if err != nil {
		return "", fmt.Errorf("encode Pi permission policy: %w", err)
	}
	return string(normalized), nil
}

func canonicalPiWorkDir(workDir string) (string, error) {
	if workDir == "" {
		workDir = "."
	}
	abs, err := filepath.Abs(workDir)
	if err != nil {
		return "", fmt.Errorf("resolve Pi working directory: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("invalid Pi working directory %q", workDir)
	}
	return abs, nil
}

func ensurePiSessionRoot() (string, error) {
	return pisession.EnsureRoot(os.Getenv("AGM_PI_SESSION_ROOT"))
}

func rollbackPiAdapterSession(tmuxName string, primaryErr error) error {
	if err := piKillSession(tmuxName); err != nil {
		return errors.Join(primaryErr, fmt.Errorf("roll back Pi tmux session %q: %w", tmuxName, err))
	}
	return primaryErr
}

// ResumeSession cold-resumes the exact persisted Pi native id.
//
//nolint:gocyclo // reason: lifecycle transaction keeps identity validation, launch recovery, readiness, and attach ordering explicit
func (a *PiAdapter) ResumeSession(sessionID SessionID) error {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}
	if err := pisession.ValidateID(metadata.UUID); err != nil {
		return err
	}
	if metadata.NativeSessionDir == "" {
		return fmt.Errorf("pi session %q has no native session directory", sessionID)
	}
	if _, err := pisession.ValidateRoot(metadata.NativeSessionDir); err != nil {
		return fmt.Errorf("validate Pi session directory: %w", err)
	}
	if metadata.TranscriptPath != "" {
		if _, err := resolvePiTranscript(metadata); err != nil {
			return fmt.Errorf("validate Pi resume transcript: %w", err)
		}
	}
	exists, running, err := piResumeTargetState(sessionID, metadata.TmuxName)
	if err != nil {
		return err
	}
	created := false
	launch := !running
	codingAgentDir, extensionPath, policyFile := "", "", ""
	if launch {
		codingAgentDir = metadata.CodingAgentDir
		if codingAgentDir == "" {
			codingAgentDir = os.Getenv("PI_CODING_AGENT_DIR")
		}
		codingAgentDir, err = pisession.ValidateCodingAgentDir(codingAgentDir)
		if err != nil {
			return err
		}
		permissionPolicyJSON, policyErr := normalizePiPermissionPolicy(metadata.PermissionPolicyJSON)
		if policyErr != nil {
			return policyErr
		}
		extensionPath, err = piadapter.EnsureExtension(os.Getenv("AGM_PI_EXTENSION_ROOT"))
		if err != nil {
			return fmt.Errorf("install Pi authorization extension: %w", err)
		}
		policyFile, err = piadapter.EnsurePolicyFile(os.Getenv("AGM_PI_EXTENSION_ROOT"), metadata.UUID, permissionPolicyJSON)
		if err != nil {
			return fmt.Errorf("install Pi permission policy: %w", err)
		}
	}
	if !exists {
		if err := piNewSession(metadata.TmuxName, metadata.WorkingDir); err != nil {
			return fmt.Errorf("create Pi resume tmux session: %w", err)
		}
		created = true
	}
	if launch {
		launchID := launchparity.NewPiLaunchID()
		command := buildPiAdapterCommand(metadata.TmuxName, metadata.UUID, launchID, metadata.NativeSessionDir, codingAgentDir, metadata.WorkingDir, metadata.Model, metadata.PermissionMode, extensionPath, policyFile)
		if err := piSendShellCommand(metadata.TmuxName, command); err != nil {
			if created {
				return rollbackPiAdapterSession(metadata.TmuxName, fmt.Errorf("send Pi resume command: %w", err))
			}
			return fmt.Errorf("send Pi resume command: %w", err)
		}
		if err := piWaitForPrompt(context.Background(), metadata.TmuxName, launchID, 60*time.Second); err != nil {
			if created {
				return rollbackPiAdapterSession(metadata.TmuxName, fmt.Errorf("pi resume readiness: %w", err))
			}
			return fmt.Errorf("pi resume readiness: %w", err)
		}
	}
	if os.Getenv("TMUX") == "" {
		if err := piAttachSession(metadata.TmuxName); err != nil {
			return fmt.Errorf("attach Pi session: %w", err)
		}
	}
	return nil
}

func piResumeTargetState(sessionID SessionID, tmuxName string) (bool, bool, error) {
	exists, err := piHasSession(tmuxName)
	if err != nil {
		return false, false, fmt.Errorf("check Pi tmux session: %w", err)
	}
	if !exists {
		return false, false, nil
	}
	running, err := piCheckProcess(tmuxName, tmux.GetSocketPath())
	if err != nil {
		return true, false, fmt.Errorf("check exact Pi process liveness: %w", err)
	}
	if running {
		return true, true, nil
	}
	verdict, err := piCheckHarness(tmuxName, tmux.GetSocketPath())
	if err != nil {
		return true, false, fmt.Errorf("check competing harness liveness: %w", err)
	}
	action, err := DecidePiPaneResume(false, verdict)
	if err != nil {
		return true, false, fmt.Errorf("refusing to resume Pi session %q: %w", sessionID, err)
	}
	return true, action == PiPanePreserve, nil
}

// TerminateSession asks Pi to exit and removes the adapter's metadata record.
func (a *PiAdapter) TerminateSession(sessionID SessionID) error {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}
	if err := piSendCommandLiteral(metadata.TmuxName, "/quit"); err != nil {
		return fmt.Errorf("terminate Pi session: %w", err)
	}
	return a.sessionStore.Delete(sessionID)
}

// GetSessionStatus projects tmux and Pi managed-state signals into Agent status.
func (a *PiAdapter) GetSessionStatus(sessionID SessionID) (Status, error) {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return StatusTerminated, err
	}
	exists, err := piHasSession(metadata.TmuxName)
	if err != nil {
		return StatusTerminated, err
	}
	if !exists {
		return StatusTerminated, nil
	}
	if idle, idleErr := piIsIdle(metadata.TmuxName); idleErr == nil && idle {
		return StatusIdle, nil
	}
	return StatusActive, nil
}

// SendMessage delivers literal user text through Pi's interactive composer.
func (a *PiAdapter) SendMessage(sessionID SessionID, message Message) error {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}
	if err := piSendPromptLiteral(metadata.TmuxName, message.Content, false); err != nil {
		return fmt.Errorf("send message to Pi: %w", err)
	}
	return nil
}

// GetHistory projects user and assistant text from Pi's native transcript.
func (a *PiAdapter) GetHistory(sessionID SessionID) ([]Message, error) {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}
	path, err := resolvePiTranscript(metadata)
	if err != nil {
		return nil, err
	}
	native, err := pisession.ReadMessages(path)
	if err != nil {
		return nil, err
	}
	messages := make([]Message, 0, len(native))
	for _, item := range native {
		// The shared Agent interface intentionally has no tool role. Preserve
		// tool results only in native export instead of mislabeling them as
		// assistant speech in portable history formats.
		if item.Role == "tool" {
			continue
		}
		role := RoleAssistant
		if item.Role == "user" {
			role = RoleUser
		}
		messages = append(messages, Message{Role: role, Content: item.Content, Timestamp: item.Timestamp, Metadata: map[string]any{"pi_role": item.Role}})
	}
	return messages, nil
}

// ExportConversation exports lossless native JSONL or a portable text projection.
func (a *PiAdapter) ExportConversation(sessionID SessionID, format ConversationFormat) ([]byte, error) {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}
	loadHistory := func() ([]Message, error) {
		return a.GetHistory(sessionID)
	}
	switch format {
	case FormatNative:
		path, resolveErr := resolvePiTranscript(metadata)
		if resolveErr != nil {
			return nil, resolveErr
		}
		return pisession.ReadNative(path)
	case FormatJSONL:
		messages, historyErr := loadHistory()
		if historyErr != nil {
			return nil, historyErr
		}
		var out []byte
		for _, message := range messages {
			line, marshalErr := json.Marshal(message)
			if marshalErr != nil {
				return nil, marshalErr
			}
			out = append(out, line...)
			out = append(out, '\n')
		}
		return out, nil
	case FormatMarkdown:
		messages, historyErr := loadHistory()
		if historyErr != nil {
			return nil, historyErr
		}
		var out strings.Builder
		for _, message := range messages {
			fmt.Fprintf(&out, "## %s (%s)\n\n%s\n\n", message.Role, message.Timestamp.Format(time.RFC3339), message.Content)
		}
		return []byte(out.String()), nil
	case FormatHTML:
		messages, historyErr := loadHistory()
		if historyErr != nil {
			return nil, historyErr
		}
		var out strings.Builder
		out.WriteString("<!doctype html><meta charset=\"utf-8\"><title>Pi conversation</title>")
		for _, message := range messages {
			fmt.Fprintf(&out, "<h2>%s</h2><pre>%s</pre>", html.EscapeString(string(message.Role)), html.EscapeString(message.Content))
		}
		return []byte(out.String()), nil
	default:
		return nil, fmt.Errorf("unsupported Pi conversation format: %s", format)
	}
}

func resolvePiTranscript(metadata *SessionMetadata) (string, error) {
	path, err := pisession.FindTranscript(metadata.NativeSessionDir, metadata.UUID)
	if err != nil {
		return "", err
	}
	if metadata.TranscriptPath != "" {
		persisted, absErr := filepath.Abs(metadata.TranscriptPath)
		if absErr != nil || filepath.Clean(persisted) != filepath.Clean(path) {
			return "", fmt.Errorf("pi transcript does not match persisted native identity")
		}
	}
	return path, nil
}

// ImportConversation imports a validated native Pi JSONL transcript.
func (a *PiAdapter) ImportConversation(data []byte, format ConversationFormat) (SessionID, error) {
	if format != FormatNative {
		return "", fmt.Errorf("pi import supports native JSONL only, got %s", format)
	}
	root, err := ensurePiSessionRoot()
	if err != nil {
		return "", err
	}
	native, path, err := pisession.ImportNative(root, data)
	if err != nil {
		return "", err
	}
	sessionID := SessionID(native.ID)
	model, modelErr := pisession.ReadModel(path)
	if modelErr != nil {
		_ = pisession.RemoveTranscript(root, path)
		return "", fmt.Errorf("read imported Pi model: %w", modelErr)
	}
	codingAgentDir, configErr := pisession.ValidateCodingAgentDir(os.Getenv("PI_CODING_AGENT_DIR"))
	if configErr != nil {
		_ = pisession.RemoveTranscript(root, path)
		return "", configErr
	}
	metadata := &SessionMetadata{
		TmuxName: "pi-import-" + native.ID, Title: native.ID, CreatedAt: time.Now(),
		WorkingDir: native.CWD, Model: model, UUID: native.ID,
		NativeSessionDir: root, TranscriptPath: path, CodingAgentDir: codingAgentDir,
	}
	if err := a.sessionStore.Set(sessionID, metadata); err != nil {
		if removeErr := pisession.RemoveTranscript(root, path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return "", errors.Join(fmt.Errorf("store imported Pi session: %w", err), fmt.Errorf("roll back imported Pi transcript: %w", removeErr))
		}
		return "", fmt.Errorf("store imported Pi session: %w", err)
	}
	return sessionID, nil
}

// Capabilities reports Pi features exposed through the shared Agent interface.
func (a *PiAdapter) Capabilities() Capabilities {
	model, _ := DefaultModelForHarness("pi-cli")
	return Capabilities{
		SupportsSlashCommands: true, SupportsHooks: true, SupportsTools: true,
		SupportsVision: true, SupportsMultimodal: false, SupportsStreaming: true,
		SupportsSystemPrompts: true, MaxContextWindow: 1048576,
		ModelName: ResolveModelFullName("pi-cli", model),
	}
}

// ExecuteCommand dispatches Agent commands supported by Pi's interactive CLI.
func (a *PiAdapter) ExecuteCommand(cmd Command) error {
	sessionID, err := getStringParam(cmd.Params, "session_id")
	if err != nil {
		return fmt.Errorf("invalid Pi command: %w", err)
	}
	metadata, err := a.sessionStore.Get(SessionID(sessionID))
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}
	switch cmd.Type {
	case CommandRename:
		name, paramErr := getStringParam(cmd.Params, "name")
		if paramErr != nil {
			return paramErr
		}
		if strings.ContainsAny(name, "\r\n") {
			return fmt.Errorf("invalid Pi session name")
		}
		if err := piSendCommandLiteral(metadata.TmuxName, "/name "+name); err != nil {
			return err
		}
		metadata.Title = name
		return a.sessionStore.Set(SessionID(sessionID), metadata)
	case CommandRunHook:
		return nil
	case CommandSetDir, CommandAuthorize, CommandClearHistory, CommandSetSystemPrompt:
		return fmt.Errorf("command %s not supported by Pi", cmd.Type)
	default:
		return fmt.Errorf("command %s not supported by Pi", cmd.Type)
	}
}
