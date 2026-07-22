package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/harnessexec"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// CodexCLIAdapter implements Agent for the interactive OpenAI Codex CLI.
//
// It is intentionally tmux-backed. The OpenAI API adapter remains available as
// a legacy API implementation, but it is not the codex-cli harness.
type CodexCLIAdapter struct {
	sessionStore SessionStore
}

var (
	codexHasSession       = tmux.HasSession
	codexNewSession       = tmux.NewSession
	codexSendCommand      = tmux.SendCommand
	codexWaitForPrompt    = tmux.WaitForCodexPrompt
	codexAttachSession    = tmux.AttachSession
	codexIsIdle           = tmux.IsCodexIdle
	codexIsProcessRunning = tmux.IsProcessRunning
	ensureCodexTrusted    = EnsureCodexWorkdirTrusted
)

// NewCodexCLIAdapter creates a Codex CLI adapter.
func NewCodexCLIAdapter(sessionStore SessionStore) (Agent, error) {
	if sessionStore == nil {
		store, err := NewJSONSessionStore("")
		if err != nil {
			return nil, fmt.Errorf("failed to create session store: %w", err)
		}
		sessionStore = store
	}
	return &CodexCLIAdapter{sessionStore: sessionStore}, nil
}

// Name returns the canonical harness name.
func (a *CodexCLIAdapter) Name() string { return "codex-cli" }

// Version returns the adapter version string.
func (a *CodexCLIAdapter) Version() string { return "codex-cli" }

// CreateSession starts a Codex CLI session in tmux and waits for the composer.
func (a *CodexCLIAdapter) CreateSession(ctx SessionContext) (SessionID, error) {
	if err := ValidateHarnessAvailability("codex-cli"); err != nil {
		return "", err
	}

	sessionID := SessionID(uuid.New().String())
	tmuxName := ctx.Name
	if tmuxName == "" {
		tmuxName = fmt.Sprintf("codex-%s", time.Now().Format("20060102-150405"))
	}
	workDir := ctx.WorkingDirectory
	if workDir == "" {
		workDir = "."
	}

	exists, err := codexHasSession(tmuxName)
	if err != nil {
		return "", fmt.Errorf("failed to check tmux session: %w", err)
	}
	if !exists {
		if err := codexNewSession(tmuxName, workDir); err != nil {
			return "", fmt.Errorf("failed to create tmux session: %w", err)
		}
	}

	model := ctx.Environment["AGM_MODEL"]
	if model == "" {
		model, _ = DefaultModelForHarness("codex-cli")
	}
	resolvedModel := ResolveModelFullName("codex-cli", model)
	// Pre-trust the workdir so Codex does not block on its interactive trust
	// prompt in fresh non-git sandbox dirs (ce-cmsq). Best-effort.
	if err := ensureCodexTrusted(workDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not pre-trust Codex workdir %s: %v\n", workDir, err)
	}
	prepared, err := harnessexec.PrepareCodexCommand(harnessexec.CodexLaunch{
		SessionName: tmuxName,
		Model:       resolvedModel,
		WorkDir:     workDir,
		Sandbox:     "workspace-write",
	}, os.Environ())
	if err != nil {
		if !exists {
			cleanupCodexCreatedSession(tmuxName)
		}
		return "", fmt.Errorf("prepare Codex CLI launch: %w", err)
	}
	if err := codexSendCommand(tmuxName, prepared.Command); err != nil {
		_ = prepared.Cancel()
		if !exists {
			cleanupCodexCreatedSession(tmuxName)
		}
		return "", fmt.Errorf("failed to start Codex CLI in tmux session: %w", err)
	}
	if err := codexWaitForPrompt(tmuxName, 30*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Codex prompt not detected (still initializing): %v\n", err)
	}

	metadata := &SessionMetadata{
		TmuxName:   tmuxName,
		Title:      ctx.Name,
		CreatedAt:  time.Now(),
		WorkingDir: workDir,
		Project:    ctx.Project,
		UUID:       ctx.Environment["CODEX_SESSION_ID"],
	}
	if err := a.sessionStore.Set(sessionID, metadata); err != nil {
		if cleanupErr := codexSendCommand(tmuxName, "exit\r"); cleanupErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to clean up Codex tmux session: %v\n", cleanupErr)
		}
		return "", fmt.Errorf("failed to store session metadata: %w", err)
	}

	return sessionID, nil
}

func cleanupCodexCreatedSession(tmuxName string) {
	if cleanupErr := codexSendCommand(tmuxName, "exit\r"); cleanupErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to clean up Codex tmux session: %v\n", cleanupErr)
	}
}

// ResumeSession resumes a stored Codex CLI session.
func (a *CodexCLIAdapter) ResumeSession(sessionID SessionID) error {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	exists, err := codexHasSession(metadata.TmuxName)
	if err != nil {
		return fmt.Errorf("failed to check tmux session: %w", err)
	}
	sendCommands := false
	created := false
	if !exists {
		if err := codexNewSession(metadata.TmuxName, metadata.WorkingDir); err != nil {
			return fmt.Errorf("failed to create tmux session: %w", err)
		}
		sendCommands = true
		created = true
	} else {
		running, err := codexIsProcessRunning(metadata.TmuxName, "codex")
		if err != nil || !running {
			sendCommands = true
		}
	}
	if sendCommands {
		model, _ := DefaultModelForHarness("codex-cli")
		resolvedModel := ResolveModelFullName("codex-cli", model)
		// Pre-trust the workdir so the Codex relaunch does not block on its
		// interactive trust prompt in non-git sandbox dirs (ce-cmsq).
		if err := ensureCodexTrusted(metadata.WorkingDir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not pre-trust Codex workdir %s: %v\n", metadata.WorkingDir, err)
		}
		prepared, err := harnessexec.PrepareCodexCommand(harnessexec.CodexLaunch{
			SessionName: metadata.TmuxName,
			Model:       resolvedModel,
			WorkDir:     metadata.WorkingDir,
			Sandbox:     "workspace-write",
			ResumeID:    metadata.UUID,
		}, os.Environ())
		if err != nil {
			if created {
				cleanupCodexCreatedSession(metadata.TmuxName)
			}
			return fmt.Errorf("prepare Codex CLI resume: %w", err)
		}
		if err := codexSendCommand(metadata.TmuxName, prepared.Command); err != nil {
			_ = prepared.Cancel()
			if created {
				cleanupCodexCreatedSession(metadata.TmuxName)
			}
			return fmt.Errorf("failed to send Codex resume command: %w", err)
		}
		if err := codexWaitForPrompt(metadata.TmuxName, 5*time.Second); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Codex prompt not detected on resume: %v\n", err)
		}
	}

	if os.Getenv("TMUX") == "" {
		if err := codexAttachSession(metadata.TmuxName); err != nil {
			return fmt.Errorf("failed to attach to tmux session: %w", err)
		}
	}
	return nil
}

// TerminateSession exits the tmux-backed Codex CLI session and removes metadata.
func (a *CodexCLIAdapter) TerminateSession(sessionID SessionID) error {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}
	if err := codexSendCommand(metadata.TmuxName, "exit\r"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to send exit to Codex session: %v\n", err)
	}
	if err := a.sessionStore.Delete(sessionID); err != nil {
		return fmt.Errorf("failed to remove session from store: %w", err)
	}
	return nil
}

// GetSessionStatus returns the tmux-backed Codex CLI session status.
func (a *CodexCLIAdapter) GetSessionStatus(sessionID SessionID) (Status, error) {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return StatusTerminated, err
	}
	exists, err := codexHasSession(metadata.TmuxName)
	if err != nil {
		return StatusTerminated, fmt.Errorf("failed to check tmux session: %w", err)
	}
	if !exists {
		return StatusTerminated, nil
	}
	if idle, err := codexIsIdle(metadata.TmuxName); err == nil && idle {
		return StatusIdle, nil
	}
	return StatusActive, nil
}

// SendMessage sends a message into the Codex CLI session.
func (a *CodexCLIAdapter) SendMessage(sessionID SessionID, message Message) error {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}
	if err := codexSendCommand(metadata.TmuxName, message.Content); err != nil {
		return fmt.Errorf("failed to send message to Codex: %w", err)
	}
	return nil
}

// GetHistory retrieves Codex CLI conversation history.
func (a *CodexCLIAdapter) GetHistory(sessionID SessionID) ([]Message, error) {
	if _, err := a.sessionStore.Get(sessionID); err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}
	// TODO: Implement actual history retrieval from ~/.codex/sessions/.
	return []Message{}, nil
}

// ExportConversation exports Codex CLI conversation history.
func (a *CodexCLIAdapter) ExportConversation(sessionID SessionID, format ConversationFormat) ([]byte, error) {
	messages, err := a.GetHistory(sessionID)
	if err != nil {
		return nil, err
	}
	switch format {
	case FormatJSONL:
		var out []byte
		for _, msg := range messages {
			data, err := json.Marshal(msg)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal message: %w", err)
			}
			out = append(out, data...)
			out = append(out, '\n')
		}
		return out, nil
	case FormatMarkdown:
		var sb strings.Builder
		for _, msg := range messages {
			fmt.Fprintf(&sb, "## %s (%s)\n\n%s\n\n", msg.Role, msg.Timestamp.Format(time.RFC3339), msg.Content)
		}
		return []byte(sb.String()), nil
	case FormatHTML, FormatNative:
		return nil, fmt.Errorf("unsupported format for Codex CLI: %s", format)
	default:
		return nil, fmt.Errorf("unsupported format for Codex CLI: %s", format)
	}
}

// ImportConversation imports conversation data into a Codex CLI session.
func (a *CodexCLIAdapter) ImportConversation(data []byte, format ConversationFormat) (SessionID, error) {
	return "", fmt.Errorf("conversation import not implemented in CodexCLIAdapter; use AGM session import/register paths")
}

// Capabilities returns Codex CLI harness capabilities.
func (a *CodexCLIAdapter) Capabilities() Capabilities {
	model, _ := DefaultModelForHarness("codex-cli")
	return Capabilities{
		SupportsSlashCommands: false,
		SupportsHooks:         true,
		SupportsTools:         true,
		SupportsVision:        true,
		SupportsMultimodal:    false,
		SupportsStreaming:     true,
		SupportsSystemPrompts: true,
		MaxContextWindow:      200000,
		ModelName:             ResolveModelFullName("codex-cli", model),
	}
}

// ExecuteCommand runs a harness command against a Codex CLI session.
func (a *CodexCLIAdapter) ExecuteCommand(cmd Command) error {
	sessionIDStr, err := getStringParam(cmd.Params, "session_id")
	if err != nil {
		return fmt.Errorf("invalid command: %w", err)
	}
	metadata, err := a.sessionStore.Get(SessionID(sessionIDStr))
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	switch cmd.Type {
	case CommandSetDir:
		path, err := getStringParam(cmd.Params, "path")
		if err != nil {
			return fmt.Errorf("invalid set_directory command: %w", err)
		}
		if err := ValidateSendDirPath(path); err != nil {
			return fmt.Errorf("invalid set_directory path: %w", err)
		}
		return codexSendCommand(metadata.TmuxName, fmt.Sprintf("cd %s\r", shellQuote(path)))
	case CommandRunHook:
		return nil
	case CommandRename, CommandAuthorize, CommandClearHistory, CommandSetSystemPrompt:
		return fmt.Errorf("command %s not supported by Codex CLI", cmd.Type)
	default:
		return fmt.Errorf("command %s not supported by Codex CLI", cmd.Type)
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
