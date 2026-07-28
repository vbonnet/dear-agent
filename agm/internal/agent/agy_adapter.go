package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/agysession"
	"github.com/vbonnet/dear-agent/agm/internal/launchparity"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// AgyAdapter implements Agent interface for the Antigravity (agy) CLI.
//
// It wraps existing AGM tmux-based session management and provides
// the Agent interface abstraction for agy sessions.
type AgyAdapter struct {
	sessionStore SessionStore
}

var (
	agyHasSession          = tmux.HasSession
	agyNewSession          = tmux.NewSession
	agySendCommand         = tmux.SendCommand
	agySendPromptLiteral   = tmux.SendPromptLiteralForHarness
	agyWaitForPrompt       = tmux.WaitForAgyPrompt
	agyWaitForResumePrompt = tmux.WaitForAgyPromptOnResume
	agyCheckProcess        = tmux.CheckProcessInPaneTree
	agyCheckHarness        = tmux.CheckPaneLiveness
	agyIsIdle              = tmux.IsAgyIdle
	agyAttachSession       = tmux.AttachSession
	agyKillSession         = tmux.KillSessionChecked
	agyIdentityTracker     = agysession.NewCreateIdentityTracker
	agyAcquireCreateLock   = func(workDir string) (func() error, error) {
		return agysession.AcquireWorkspaceCreateLock(context.Background(), workDir)
	}
)

const (
	agyResumeReadinessTimeout = 60 * time.Second
)

// NewAgyAdapter creates a new Agy adapter instance.
//
// If sessionStore is nil, creates a default JSON-backed store at ~/.agm/sessions.json.
func NewAgyAdapter(sessionStore SessionStore) (Agent, error) {
	if sessionStore == nil {
		store, err := NewJSONSessionStore("")
		if err != nil {
			return nil, fmt.Errorf("failed to create session store: %w", err)
		}
		sessionStore = store
	}

	return &AgyAdapter{
		sessionStore: sessionStore,
	}, nil
}

// Name returns the agent identifier
func (a *AgyAdapter) Name() string {
	return "agy"
}

// Version returns the model name or version
func (a *AgyAdapter) Version() string {
	return defaultAgyModel()
}

// CreateSession creates a new Agy session.
//
// Creates a tmux session with Agy CLI and stores the SessionID mapping.
func (a *AgyAdapter) CreateSession(ctx SessionContext) (SessionID, error) {
	// Generate unique SessionID
	sessionID := SessionID(uuid.New().String())

	// Use session name as tmux session name (or generate one)
	tmuxName := ctx.Name
	if tmuxName == "" {
		tmuxName = fmt.Sprintf("agy-%s", time.Now().Format("20060102-150405"))
	}
	workDir, err := canonicalAgyWorkDir(ctx.WorkingDirectory)
	if err != nil {
		return "", err
	}
	conversationID := ctx.Environment["AGY_CONVERSATION_ID"]
	if conversationID != "" {
		if err := agysession.ValidateConversationID(conversationID); err != nil {
			return "", err
		}
	}
	model := ctx.Model
	if model == "" {
		model = ctx.Environment["AGM_MODEL"]
	}
	if model == "" && conversationID == "" {
		model, _ = DefaultModelForHarness("agy")
	}
	resolvedModel := ""
	if model != "" {
		resolvedModel, err = resolveAgyAdapterModel(model)
		if err != nil {
			return "", err
		}
	}
	permissionMode := ctx.Environment["AGM_PERMISSION_MODE"]

	// AGY exposes only the latest conversation for a workspace. Serialize the
	// snapshot, launch, and discovery sequence so concurrent AGM creates cannot
	// persist one another's provider-native identity.
	releaseCreateLock, err := agyAcquireCreateLock(workDir)
	if err != nil {
		return "", err
	}
	defer func() {
		if unlockErr := releaseCreateLock(); unlockErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to release AGY workspace lock: %v\n", unlockErr)
		}
	}()

	// Check if tmux session already exists
	exists, err := agyHasSession(tmuxName)
	if err != nil {
		return "", fmt.Errorf("failed to check tmux session: %w", err)
	}
	if exists {
		return "", fmt.Errorf("refusing to create AGY session %q: tmux session already exists", tmuxName)
	}
	previousConversationID := ""
	var identityTracker agysession.CreateIdentityTracker
	if conversationID == "" {
		identityTracker = agyIdentityTracker()
		if identityTracker == nil {
			return "", fmt.Errorf("create AGY identity tracker: provider returned nil tracker")
		}
		previousConversationID, err = identityTracker.Snapshot(context.Background(), workDir)
		if err != nil {
			return "", fmt.Errorf("failed to snapshot AGY conversation before create: %w", err)
		}
	}
	if err := agyNewSession(tmuxName, workDir); err != nil {
		return "", fmt.Errorf("failed to create tmux session: %w", err)
	}
	agyCmd := launchparity.BuildAgyCommand(launchparity.AgyCommandSpec{
		WorkDir:        workDir,
		ResolvedModel:  resolvedModel,
		PermissionMode: permissionMode,
		ConversationID: conversationID,
		ExtraAddDirs:   ctx.AuthorizedDirs,
	}).Command

	// Start Agy in the tmux session
	if err := agySendCommand(tmuxName, agyCmd); err != nil {
		return "", rollbackAgyAdapterSession(tmuxName, fmt.Errorf("failed to start Agy in tmux session: %w", err))
	}

	// Wait for Agy to be ready
	if err := agyWaitForPrompt(context.Background(), tmuxName, 30*time.Second); err != nil {
		return "", rollbackAgyAdapterSession(tmuxName, fmt.Errorf("AGY did not become ready after create: %w", err))
	}
	if ctx.InitialPrompt != "" {
		if err := agySendPromptLiteral(tmuxName, ctx.InitialPrompt, false, "agy"); err != nil {
			return "", rollbackAgyAdapterSession(tmuxName, fmt.Errorf("failed to deliver AGY initial prompt before identity discovery: %w", err))
		}
	}
	if conversationID == "" {
		metadata, identityErr := identityTracker.Discover(context.Background(), workDir, previousConversationID)
		if identityErr != nil {
			return "", rollbackAgyAdapterSession(tmuxName, fmt.Errorf("failed to capture native AGY conversation after create: %w", identityErr))
		}
		if metadata == nil {
			return "", rollbackAgyAdapterSession(tmuxName, fmt.Errorf("failed to capture native AGY conversation after create: provider returned empty metadata"))
		}
		conversationID = metadata.ConversationID
		if err := agysession.ValidateConversationID(conversationID); err != nil {
			return "", rollbackAgyAdapterSession(tmuxName, fmt.Errorf("failed to capture native AGY conversation after create: %w", err))
		}
	}

	// Store session metadata
	metadata := &SessionMetadata{
		TmuxName:       tmuxName,
		Title:          ctx.Name, // Set initial title from session name
		CreatedAt:      time.Now(),
		WorkingDir:     workDir,
		Project:        ctx.Project,
		Model:          model,
		PermissionMode: permissionMode,
		AuthorizedDirs: append([]string(nil), ctx.AuthorizedDirs...),
		UUID:           conversationID,
	}

	if err := a.sessionStore.Set(sessionID, metadata); err != nil {
		return "", rollbackAgyAdapterSession(tmuxName, fmt.Errorf("failed to store session metadata: %w", err))
	}

	return sessionID, nil
}

func rollbackAgyAdapterSession(tmuxName string, primaryErr error) error {
	if cleanupErr := agyKillSession(tmuxName); cleanupErr != nil {
		return errors.Join(primaryErr, fmt.Errorf("failed to roll back AGY tmux session %q: %w", tmuxName, cleanupErr))
	}
	return primaryErr
}

func canonicalAgyWorkDir(workDir string) (string, error) {
	if workDir == "" {
		workDir = "."
	}
	canonical, err := agysession.CanonicalWorkspacePath(workDir)
	if err != nil {
		return "", fmt.Errorf("resolve AGY working directory: %w", err)
	}
	return canonical, nil
}

// ResumeSession resumes an existing Agy session.
func (a *AgyAdapter) ResumeSession(sessionID SessionID) error {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	// Process liveness and the restartable-shell proof must be evaluated while
	// holding the workspace lifecycle lock. Otherwise two cold resumes can both
	// observe a shell before either launch and the second can inject into the
	// first launch's live composer.
	if err := resumeAgyAdapterProcess(sessionID, metadata); err != nil {
		return err
	}

	// Attach to tmux session (skip if already in tmux)
	if os.Getenv("TMUX") == "" {
		if err := agyAttachSession(metadata.TmuxName); err != nil {
			return fmt.Errorf("failed to attach to tmux session: %w", err)
		}
	}

	return nil
}

func resumeAgyAdapterProcess(sessionID SessionID, metadata *SessionMetadata) error {
	workDir, resolvedModel, err := resolveAgyResumeInputs(sessionID, metadata)
	if err != nil {
		return err
	}

	releaseWorkspaceLock, err := agyAcquireCreateLock(workDir)
	if err != nil {
		return fmt.Errorf("failed to acquire AGY workspace lifecycle lock for resume: %w", err)
	}
	defer func() {
		if unlockErr := releaseWorkspaceLock(); unlockErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to release AGY workspace lock after resume: %v\n", unlockErr)
		}
	}()

	tmuxExists, running, err := lockedAgyAdapterResumeTargetState(sessionID, metadata.TmuxName)
	if err != nil {
		return err
	}
	if running {
		return nil
	}

	created := false
	if !tmuxExists {
		if err := agyNewSession(metadata.TmuxName, workDir); err != nil {
			return fmt.Errorf("failed to create tmux session: %w", err)
		}
		created = true
	}
	fullCmd := launchparity.BuildAgyCommand(launchparity.AgyCommandSpec{
		WorkDir:        workDir,
		ResolvedModel:  resolvedModel,
		PermissionMode: metadata.PermissionMode,
		ConversationID: metadata.UUID,
		ExtraAddDirs:   metadata.AuthorizedDirs,
	}).Command

	if err := agySendCommand(metadata.TmuxName, fullCmd); err != nil {
		primaryErr := fmt.Errorf("failed to send resume command: %w", err)
		if created {
			return rollbackAgyAdapterSession(metadata.TmuxName, primaryErr)
		}
		return primaryErr
	}

	if err := agyWaitForResumePrompt(context.Background(), metadata.TmuxName, agyResumeReadinessTimeout); err != nil {
		primaryErr := fmt.Errorf("AGY did not become ready after resume: %w", err)
		if created {
			return rollbackAgyAdapterSession(metadata.TmuxName, primaryErr)
		}
		return primaryErr
	}
	return nil
}

func resolveAgyResumeInputs(sessionID SessionID, metadata *SessionMetadata) (string, string, error) {
	if metadata.UUID == "" {
		return "", "", fmt.Errorf("AGY session %q has no native conversation ID; capture or reassociate it before cold resume", sessionID)
	}
	if err := agysession.ValidateConversationID(metadata.UUID); err != nil {
		return "", "", err
	}
	workDir, err := canonicalAgyWorkDir(metadata.WorkingDir)
	if err != nil {
		return "", "", err
	}
	resolvedModel := ""
	if metadata.Model != "" {
		resolvedModel, err = resolveAgyAdapterModel(metadata.Model)
		if err != nil {
			return "", "", err
		}
	}
	return workDir, resolvedModel, nil
}

func lockedAgyAdapterResumeTargetState(sessionID SessionID, tmuxName string) (bool, bool, error) {
	tmuxExists, err := agyHasSession(tmuxName)
	if err != nil {
		return false, false, fmt.Errorf("failed to check tmux session: %w", err)
	}
	if !tmuxExists {
		return false, false, nil
	}
	running, err := agyCheckProcess(tmuxName, tmux.GetSocketPath(), "agy")
	if err != nil {
		return true, false, fmt.Errorf("failed to check AGY process liveness: %w", err)
	}
	if running {
		return true, true, nil
	}
	verdict, err := agyCheckHarness(tmuxName, tmux.GetSocketPath())
	if err != nil {
		return true, false, fmt.Errorf("failed to check competing harness liveness: %w", err)
	}
	if verdict.HarnessAlive {
		return true, false, fmt.Errorf("refusing to resume AGY session %q into a tmux pane containing another live harness (pane tree: %s)", sessionID, verdict.Evidence)
	}
	if !verdict.RestartableShell {
		return true, false, fmt.Errorf("refusing to resume AGY session %q because its tmux pane is not a proven restartable shell (pane tree: %s)", sessionID, verdict.Evidence)
	}
	return true, false, nil
}

// TerminateSession terminates an Agy session.
func (a *AgyAdapter) TerminateSession(sessionID SessionID) error {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	if err := tmux.SendCommand(metadata.TmuxName, "exit\r"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to send exit to session: %v\n", err)
	}

	if err := a.sessionStore.Delete(sessionID); err != nil {
		return fmt.Errorf("failed to remove session from store: %w", err)
	}

	return nil
}

// GetSessionStatus returns the status of an Agy session.
func (a *AgyAdapter) GetSessionStatus(sessionID SessionID) (Status, error) {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return StatusTerminated, err
	}

	exists, err := agyHasSession(metadata.TmuxName)
	if err != nil {
		return StatusTerminated, fmt.Errorf("failed to check tmux session: %w", err)
	}

	if !exists {
		return StatusTerminated, nil
	}
	alive, err := agyCheckProcess(metadata.TmuxName, tmux.GetSocketPath(), "agy")
	if err != nil {
		return StatusTerminated, fmt.Errorf("failed to check AGY process liveness: %w", err)
	}
	if !alive {
		return StatusTerminated, nil
	}
	if idle, idleErr := agyIsIdle(metadata.TmuxName); idleErr == nil && idle {
		return StatusIdle, nil
	}

	return StatusActive, nil
}

// SendMessage sends a message to Agy.
func (a *AgyAdapter) SendMessage(sessionID SessionID, message Message) error {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	if err := agySendPromptLiteral(metadata.TmuxName, message.Content, false, "agy"); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

// GetHistory retrieves conversation history.
//
//nolint:dupl // adapter interface requires identical method signature on each harness type; shared logic extracted where practical
func (a *AgyAdapter) GetHistory(sessionID SessionID) ([]Message, error) {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	if metadata.UUID == "" {
		return nil, fmt.Errorf("AGY session %q has no native conversation ID; history cannot be resolved", sessionID)
	}
	if err := agysession.ValidateConversationID(metadata.UUID); err != nil {
		return nil, err
	}
	logsDir := filepath.Join(homeDir, ".gemini", "antigravity-cli", "brain", metadata.UUID, ".system_generated", "logs")
	historyPath := filepath.Join(logsDir, "transcript.jsonl")
	if _, err := os.Stat(historyPath); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("stat AGY history file: %w", err)
		}
		historyPath = filepath.Join(logsDir, "transcript_full.jsonl")
		if _, fullErr := os.Stat(historyPath); fullErr != nil {
			if os.IsNotExist(fullErr) {
				return []Message{}, nil
			}
			return nil, fmt.Errorf("stat AGY full history file: %w", fullErr)
		}
	}
	return readAgyHistory(historyPath)
}

type agyTranscriptEntry struct {
	StepIndex int    `json:"step_index"`
	Source    string `json:"source"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	Content   string `json:"content"`
}

const maxAgyHistoryLineBytes = 2 << 20

func readAgyHistory(historyPath string) ([]Message, error) {
	file, err := os.Open(historyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open AGY history file: %w", err)
	}
	defer file.Close()

	var messages []Message
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxAgyHistoryLineBytes)
	for scanner.Scan() {
		var entry agyTranscriptEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		role, include := agyTranscriptRole(entry)
		if !include {
			continue
		}
		timestamp, _ := time.Parse(time.RFC3339, entry.CreatedAt)
		messages = append(messages, Message{
			ID:        strconv.Itoa(entry.StepIndex),
			Role:      role,
			Content:   entry.Content,
			Timestamp: timestamp,
			Metadata: map[string]any{
				"source": entry.Source,
				"type":   entry.Type,
				"status": entry.Status,
			},
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read AGY history file: %w", err)
	}

	return messages, nil
}

func agyTranscriptRole(entry agyTranscriptEntry) (Role, bool) {
	switch entry.Source {
	case "USER_EXPLICIT":
		return RoleUser, true
	case "MODEL":
		return RoleAssistant, true
	case "":
		switch entry.Type {
		case "USER_INPUT":
			return RoleUser, true
		case "PLANNER_RESPONSE":
			return RoleAssistant, true
		}
	}
	return "", false
}

// ExportConversation exports conversation in specified format.
func (a *AgyAdapter) ExportConversation(sessionID SessionID, format ConversationFormat) ([]byte, error) {
	messages, err := a.GetHistory(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get history: %w", err)
	}

	switch format {
	case FormatJSONL:
		result := make([]byte, 0)
		for _, msg := range messages {
			data, err := json.Marshal(msg)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal message: %w", err)
			}
			result = append(result, data...)
			result = append(result, '\n')
		}
		return result, nil

	case FormatMarkdown:
		var sb strings.Builder
		for _, msg := range messages {
			role := "User"
			if msg.Role == RoleAssistant {
				role = "Assistant"
			}
			fmt.Fprintf(&sb, "## %s (%s)\n\n%s\n\n", role, msg.Timestamp.Format(time.RFC3339), msg.Content)
		}
		return []byte(sb.String()), nil

	case FormatHTML:
		return nil, fmt.Errorf("HTML export not yet implemented")

	case FormatNative:
		return nil, fmt.Errorf("native format export not yet implemented")

	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// ImportConversation imports conversation from serialized data.
func (a *AgyAdapter) ImportConversation(data []byte, format ConversationFormat) (SessionID, error) {
	return "", fmt.Errorf("conversation import not yet implemented")
}

// Capabilities returns Agy's feature capabilities
func (a *AgyAdapter) Capabilities() Capabilities {
	return Capabilities{
		SupportsSlashCommands: true,
		SupportsHooks:         false,
		SupportsTools:         true,
		SupportsVision:        true,
		SupportsMultimodal:    false,
		SupportsStreaming:     true,
		SupportsSystemPrompts: false,
		MaxContextWindow:      200000,
		ModelName:             defaultAgyModel(),
	}
}

func defaultAgyModel() string {
	model, err := resolveAgyAdapterModel("")
	if err != nil {
		return ""
	}
	return model
}

func resolveAgyAdapterModel(model string) (string, error) {
	if model == "" {
		model, _ = DefaultModelForHarness("agy")
	}
	if err := ValidateModel("agy", model); err != nil {
		return "", fmt.Errorf("invalid AGY model: %w", err)
	}
	resolved := ResolveModelFullName("agy", model)
	if resolved == "" {
		return "", fmt.Errorf("invalid AGY model %q: resolution returned an empty label", model)
	}
	return resolved, nil
}

// ExecuteCommand executes a generic command.
//
//nolint:dupl // adapter interface requires identical method signature on each harness type; shared logic extracted where practical
func (a *AgyAdapter) ExecuteCommand(cmd Command) error {
	sessionIDStr, err := getStringParam(cmd.Params, "session_id")
	if err != nil {
		return fmt.Errorf("invalid command: %w", err)
	}

	metadata, err := a.sessionStore.Get(SessionID(sessionIDStr))
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	switch cmd.Type {
	case CommandRename:
		newName, err := getStringParam(cmd.Params, "name")
		if err != nil {
			return fmt.Errorf("rename command: %w", err)
		}

		if err := ValidateSendKeysText("session name", newName); err != nil {
			return fmt.Errorf("rename command: %w", err)
		}

		if err := tmux.SendCommand(metadata.TmuxName, fmt.Sprintf("/rename %s\r", newName)); err != nil {
			return fmt.Errorf("failed to send rename command: %w", err)
		}

		metadata.Title = newName
		if err := a.sessionStore.Set(SessionID(sessionIDStr), metadata); err != nil {
			return fmt.Errorf("failed to update session title: %w", err)
		}

		return nil

	case CommandSetDir:
		newPath, err := getStringParam(cmd.Params, "path")
		if err != nil {
			return fmt.Errorf("setdir command: %w", err)
		}
		if err := ValidateSendDirPath(newPath); err != nil {
			return fmt.Errorf("setdir command: %w", err)
		}
		if err := sendPastedShellCommand(metadata.TmuxName, buildSetDirCommand(newPath), newPath); err != nil {
			return fmt.Errorf("failed to send cd command: %w", err)
		}
		return nil

	case CommandAuthorize:
		return fmt.Errorf("authorize command not yet implemented")

	case CommandRunHook:
		return fmt.Errorf("run_hook command not implemented for AGY")

	case CommandClearHistory:
		return fmt.Errorf("clear_history command not yet implemented")

	case CommandSetSystemPrompt:
		return fmt.Errorf("set_system_prompt command not yet implemented")

	default:
		return fmt.Errorf("unsupported command type: %s", cmd.Type)
	}
}
