// Package importer provides importer functionality.
package importer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/agysession"
	"github.com/vbonnet/dear-agent/agm/internal/codexsession"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/history"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/pisession"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// SessionMetadata holds extracted metadata from history.jsonl
type SessionMetadata struct {
	UUID         string
	ProjectPath  string
	LastModified time.Time
}

// ImportOptions configures orphaned-session import behavior.
type ImportOptions struct {
	Harness string
}

// ImportOrphanedSession imports an orphaned conversation by creating an AGM manifest
//
// Parameters:
//   - conversationUUID: Claude conversation UUID to import
//   - sessionName: Name for the AGM session (sanitized for tmux)
//   - workspace: Workspace name (e.g., "oss", "acme")
//   - adapter: Dolt database adapter
//   - sessionsDir: Directory where manifests are stored (for YAML backward compat)
//
// Returns:
//   - sessionID: Generated AGM session ID
//   - error: Error if import fails
func ImportOrphanedSession(conversationUUID, sessionName, workspace string, adapter *dolt.Adapter, sessionsDir string) (string, error) {
	return ImportOrphanedSessionWithOptions(conversationUUID, sessionName, workspace, adapter, sessionsDir, ImportOptions{
		Harness: "claude-code",
	})
}

// ImportOrphanedSessionWithOptions imports an orphaned harness conversation by
// creating an AGM manifest in Dolt.
func ImportOrphanedSessionWithOptions(conversationUUID, sessionName, workspace string, adapter *dolt.Adapter, sessionsDir string, opts ImportOptions) (string, error) {
	harness := agent.NormalizeHarnessName(opts.Harness)
	if harness == "" {
		harness = "claude-code"
	}
	switch harness {
	case "claude-code":
		return importClaudeOrphanedSession(conversationUUID, sessionName, workspace, adapter)
	case "codex-cli":
		return importCodexOrphanedSession(conversationUUID, sessionName, workspace, adapter)
	case "agy":
		return importAgyOrphanedSession(conversationUUID, sessionName, workspace, adapter)
	case "pi-cli":
		return importPiOrphanedSession(conversationUUID, sessionName, workspace, adapter)
	default:
		return "", fmt.Errorf("unsupported import harness %q", harness)
	}
}

func importPiOrphanedSession(nativeID, sessionName, workspace string, adapter *dolt.Adapter) (string, error) {
	if err := pisession.ValidateID(nativeID); err != nil {
		return "", err
	}
	if sessionName == "" {
		return "", fmt.Errorf("session name cannot be empty")
	}
	if adapter == nil {
		return "", fmt.Errorf("Dolt adapter not available") //nolint:staticcheck // proper noun
	}
	if err := ValidateNotDuplicate(nativeID, adapter); err != nil {
		return "", err
	}
	codingAgentDir, err := pisession.ValidateCodingAgentDir(os.Getenv("PI_CODING_AGENT_DIR"))
	if err != nil {
		return "", err
	}
	privateRoot, err := pisession.EnsureRoot("")
	if err != nil {
		return "", err
	}
	native, transcript, createdCopy, err := resolvePiImport(nativeID, privateRoot, codingAgentDir)
	if err != nil {
		return "", err
	}
	modifiedAt, err := pisession.TranscriptModTime(privateRoot, transcript)
	if err != nil {
		return "", rollbackPiImport(privateRoot, transcript, createdCopy, err)
	}
	model, modelErr := pisession.ReadModel(transcript)
	if modelErr != nil {
		return "", rollbackPiImport(privateRoot, transcript, createdCopy, fmt.Errorf("read Pi native model: %w", modelErr))
	}
	sessionID := uuid.New().String()
	m := buildPiImportedManifest(native, transcript, privateRoot, codingAgentDir, model, sessionName, workspace, sessionID, modifiedAt, time.Now())
	if err := adapter.CreateSession(m); err != nil {
		return "", rollbackPiImport(privateRoot, transcript, createdCopy, fmt.Errorf("failed to create session in Dolt: %w", err))
	}
	return sessionID, nil
}

func resolvePiImport(nativeID, privateRoot, codingAgentDir string) (pisession.Metadata, string, bool, error) {
	transcript, findErr := pisession.FindTranscript(privateRoot, nativeID)
	switch {
	case findErr == nil:
		native, err := pisession.ReadMetadata(transcript)
		return native, transcript, false, err
	case !errors.Is(findErr, pisession.ErrTranscriptNotFound):
		return pisession.Metadata{}, "", false, findErr
	}

	agentDir := codingAgentDir
	if agentDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return pisession.Metadata{}, "", false, fmt.Errorf("failed to determine Pi agent directory: %w", err)
		}
		agentDir = filepath.Join(home, ".pi", "agent")
	}
	source, err := pisession.FindTranscriptTree(filepath.Join(agentDir, "sessions"), nativeID)
	if err != nil {
		return pisession.Metadata{}, "", false, fmt.Errorf("failed to find Pi saved session: %w", err)
	}
	native, path, err := pisession.ImportNativeFile(privateRoot, source)
	return native, path, err == nil, err
}

func rollbackPiImport(privateRoot, transcript string, created bool, primary error) error {
	if !created {
		return primary
	}
	if err := pisession.RemoveTranscript(privateRoot, transcript); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Join(primary, fmt.Errorf("roll back imported Pi transcript: %w", err))
	}
	return primary
}

func buildPiImportedManifest(native pisession.Metadata, transcript, sessionDir, codingAgentDir, model, sessionName, workspace, sessionID string, createdAt, now time.Time) *manifest.Manifest {
	if createdAt.IsZero() {
		createdAt = now
	}
	return &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion, SessionID: sessionID, Name: sessionName,
		CreatedAt: createdAt, UpdatedAt: now, Workspace: workspace, Harness: "pi-cli",
		Model: model, WorkingDirectory: native.CWD,
		Context: manifest.Context{Project: native.CWD},
		Pi: &manifest.Pi{
			SessionID: native.ID, SessionDir: sessionDir, TranscriptPath: transcript,
			CodingAgentDir: codingAgentDir, CodingAgentDirSet: true,
		},
		Tmux: manifest.Tmux{SessionName: tmux.SanitizeSessionName(sessionName)},
	}
}

func importClaudeOrphanedSession(conversationUUID, sessionName, workspace string, adapter *dolt.Adapter) (string, error) {
	// 1. Validate inputs
	if conversationUUID == "" {
		return "", fmt.Errorf("conversation UUID cannot be empty")
	}
	if sessionName == "" {
		return "", fmt.Errorf("session name cannot be empty")
	}
	if adapter == nil {
		return "", fmt.Errorf("Dolt adapter not available") //nolint:staticcheck // proper noun
	}

	// 2. Check for duplicate (UUID already has manifest)
	if err := ValidateNotDuplicate(conversationUUID, adapter); err != nil {
		return "", err
	}

	// 3. Infer project path from conversation file location
	projectPath, err := InferProjectPath(conversationUUID)
	if err != nil {
		return "", fmt.Errorf("failed to infer project path: %w", err)
	}

	// 4. Extract metadata from history.jsonl
	metadata, err := ExtractMetadataFromHistory(conversationUUID)
	if err != nil {
		// Non-fatal: we can still import without history metadata
		metadata = &SessionMetadata{
			UUID:         conversationUUID,
			ProjectPath:  projectPath,
			LastModified: time.Now(),
		}
	} else if metadata.ProjectPath != "" {
		// Override project path if history has more accurate info
		projectPath = metadata.ProjectPath
	}

	// 5. Generate new session ID
	sessionID := uuid.New().String()

	// 6. Sanitize tmux session name
	tmuxName := tmux.SanitizeSessionName(sessionName)

	// 7. Create manifest
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     sessionID,
		Name:          sessionName,
		CreatedAt:     metadata.LastModified,
		UpdatedAt:     time.Now(),
		Lifecycle:     "", // Active by default
		Workspace:     workspace,
		Context: manifest.Context{
			Project: projectPath,
		},
		Claude: manifest.Claude{
			UUID: conversationUUID,
		},
		Tmux: manifest.Tmux{
			SessionName: tmuxName,
		},
	}

	// Write to Dolt database
	if err := adapter.CreateSession(m); err != nil {
		return "", fmt.Errorf("failed to create session in Dolt: %w", err)
	}

	return sessionID, nil
}

func importCodexOrphanedSession(conversationUUID, sessionName, workspace string, adapter *dolt.Adapter) (string, error) {
	if conversationUUID == "" {
		return "", fmt.Errorf("conversation UUID cannot be empty")
	}
	if sessionName == "" {
		return "", fmt.Errorf("session name cannot be empty")
	}
	if adapter == nil {
		return "", fmt.Errorf("Dolt adapter not available") //nolint:staticcheck // proper noun
	}

	if err := ValidateNotDuplicate(conversationUUID, adapter); err != nil {
		return "", err
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine home directory: %w", err)
	}
	codexMeta, err := codexsession.FindByID(homeDir, conversationUUID)
	if err != nil {
		return "", fmt.Errorf("failed to find Codex saved session: %w", err)
	}
	if codexMeta.Archived {
		return "", fmt.Errorf("codex saved session %s is already archived", conversationUUID)
	}

	createdAt := codexMeta.ModTime
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	sessionID := uuid.New().String()
	tmuxName := tmux.SanitizeSessionName(sessionName)
	m := &manifest.Manifest{
		SchemaVersion:    manifest.SchemaVersion,
		SessionID:        sessionID,
		Name:             sessionName,
		CreatedAt:        createdAt,
		UpdatedAt:        time.Now(),
		Lifecycle:        "",
		Workspace:        workspace,
		Harness:          "codex-cli",
		Model:            agent.HarnessDefaults["codex-cli"],
		WorkingDirectory: codexMeta.CWD,
		Context: manifest.Context{
			Project: codexMeta.CWD,
		},
		Codex: &manifest.Codex{
			SessionID:      codexMeta.SessionID,
			TranscriptPath: codexMeta.Path,
		},
		Tmux: manifest.Tmux{
			SessionName: tmuxName,
		},
	}
	if err := adapter.CreateSession(m); err != nil {
		return "", fmt.Errorf("failed to create session in Dolt: %w", err)
	}
	return sessionID, nil
}

func importAgyOrphanedSession(conversationUUID, sessionName, workspace string, adapter *dolt.Adapter) (string, error) {
	if conversationUUID == "" {
		return "", fmt.Errorf("conversation UUID cannot be empty")
	}
	if sessionName == "" {
		return "", fmt.Errorf("session name cannot be empty")
	}
	if adapter == nil {
		return "", fmt.Errorf("Dolt adapter not available") //nolint:staticcheck // proper noun
	}

	if err := ValidateNotDuplicate(conversationUUID, adapter); err != nil {
		return "", err
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine home directory: %w", err)
	}
	agyMeta, err := agysession.FindByID(homeDir, conversationUUID)
	if err != nil {
		return "", fmt.Errorf("failed to find AGY saved conversation: %w", err)
	}
	if agyMeta.WorkspacePath == "" {
		return "", fmt.Errorf("failed to determine AGY workspace path for conversation %s", conversationUUID)
	}

	sessionID := uuid.New().String()
	m := buildAgyImportedManifest(agyMeta, sessionName, workspace, sessionID, time.Now())
	if err := adapter.CreateSession(m); err != nil {
		return "", fmt.Errorf("failed to create session in Dolt: %w", err)
	}
	return sessionID, nil
}

func buildAgyImportedManifest(agyMeta *agysession.Metadata, sessionName, workspace, sessionID string, now time.Time) *manifest.Manifest {
	createdAt := agyMeta.ModTime
	if createdAt.IsZero() {
		createdAt = now
	}
	return &manifest.Manifest{
		SchemaVersion:    manifest.SchemaVersion,
		SessionID:        sessionID,
		Name:             sessionName,
		CreatedAt:        createdAt,
		UpdatedAt:        now,
		Lifecycle:        "",
		Workspace:        workspace,
		Harness:          "agy",
		WorkingDirectory: agyMeta.WorkspacePath,
		Context: manifest.Context{
			Project: agyMeta.WorkspacePath,
		},
		Agy: &manifest.Agy{
			ConversationID: agyMeta.ConversationID,
			WorkspacePath:  agyMeta.WorkspacePath,
			ConversationDB: agyMeta.ConversationDBPath,
			TranscriptPath: agyMeta.TranscriptPath,
		},
		Tmux: manifest.Tmux{
			SessionName: tmux.SanitizeSessionName(sessionName),
		},
	}
}

// RegisterResult describes the outcome of a RegisterSession call.
type RegisterResult struct {
	SessionID      string // AGM session ID (existing one if AlreadyTracked)
	Workspace      string // Workspace label that was used/stored
	Project        string // Inferred project path
	Name           string // Session name (existing one if AlreadyTracked)
	AlreadyTracked bool   // true if the UUID already had a manifest (idempotent no-op)
}

// RegisterSession idempotently ensures an AGM manifest exists for a Claude
// conversation UUID.
//
// Unlike ImportOrphanedSession, RegisterSession is safe to call repeatedly and
// non-interactively (it never prompts), which is what makes it callable from a
// SessionStart hook:
//   - If the UUID is already tracked, it returns the existing session unchanged
//     (AlreadyTracked=true) instead of erroring or minting a duplicate ID.
//   - When workspace is empty, it is inferred from the conversation's project
//     directory (InferWorkspaceFromProject) rather than blindly defaulting to
//     the active config workspace, which mis-attributes every imported session
//     to a single workspace (see the 2026-05-21 unified-session-management retro).
//   - When sessionName is empty, it is derived from the project basename.
//   - projectDir, when non-empty, is used as the project directory directly,
//     skipping the InferProjectPath scan. This is the fix for the SessionStart
//     timing window: the hook payload includes cwd but the conversation
//     .jsonl file has not been written yet when SessionStart fires.
//
// Parameters mirror ImportOrphanedSession. fallbackWorkspace is used only when
// neither an explicit workspace nor an inferable one is available (typically the
// caller's cfg.Workspace).
func RegisterSession(conversationUUID, sessionName, workspace, fallbackWorkspace, projectDir string, adapter *dolt.Adapter) (*RegisterResult, error) {
	if conversationUUID == "" {
		return nil, fmt.Errorf("conversation UUID cannot be empty")
	}
	if adapter == nil {
		return nil, fmt.Errorf("Dolt adapter not available") //nolint:staticcheck // proper noun
	}

	// 1. Idempotency: if already tracked, return the existing session untouched.
	existing, err := FindByUUID(conversationUUID, adapter)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return &RegisterResult{
			SessionID:      existing.SessionID,
			Workspace:      existing.Workspace,
			Project:        existing.Context.Project,
			Name:           existing.Name,
			AlreadyTracked: true,
		}, nil
	}

	// 2. Resolve the conversation's real project path.
	//
	// projectDir wins when provided (SessionStart hook knows cwd before the
	// .jsonl is written). When absent, fall back to the conversation file scan.
	var projectPath string
	if projectDir != "" {
		projectPath = projectDir
	} else {
		projectPath, err = InferProjectPath(conversationUUID)
		if err != nil {
			return nil, fmt.Errorf("failed to infer project path: %w", err)
		}
	}
	metadata, err := ExtractMetadataFromHistory(conversationUUID)
	if err != nil {
		metadata = &SessionMetadata{
			UUID:         conversationUUID,
			ProjectPath:  projectPath,
			LastModified: time.Now(),
		}
	} else if metadata.ProjectPath != "" {
		// History is more authoritative when available; prefer it over cwd hint.
		projectPath = metadata.ProjectPath
	}

	// 3. Resolve workspace label: explicit > inferred-from-project > fallback.
	if workspace == "" {
		workspace = InferWorkspaceFromProject(projectPath)
	}
	if workspace == "" {
		workspace = fallbackWorkspace
	}
	if workspace == "" {
		return nil, fmt.Errorf("workspace could not be determined for %s\n\n"+
			"Solutions:\n"+
			"  • Provide workspace explicitly: --workspace=oss\n"+
			"  • Ensure the conversation's project directory is under a git/WORKSPACE.yaml root\n"+
			"  • Set a default workspace in ~/.agm/config.yaml", conversationUUID)
	}

	// 4. Derive a name when none was given (keeps register non-interactive).
	if sessionName == "" {
		sessionName = deriveSessionName(projectPath, conversationUUID)
	}

	// 5. Create the manifest.
	sessionID := uuid.New().String()
	m := &manifest.Manifest{
		SchemaVersion:    manifest.SchemaVersion,
		SessionID:        sessionID,
		Name:             sessionName,
		CreatedAt:        metadata.LastModified,
		UpdatedAt:        time.Now(),
		Lifecycle:        "",
		Workspace:        workspace,
		WorkingDirectory: projectPath,
		Context: manifest.Context{
			Project: projectPath,
		},
		Claude: manifest.Claude{
			UUID: conversationUUID,
		},
		Tmux: manifest.Tmux{
			SessionName: tmux.SanitizeSessionName(sessionName),
		},
	}
	if err := adapter.CreateSession(m); err != nil {
		return nil, fmt.Errorf("failed to create session in Dolt: %w", err)
	}

	return &RegisterResult{
		SessionID:      sessionID,
		Workspace:      workspace,
		Project:        projectPath,
		Name:           sessionName,
		AlreadyTracked: false,
	}, nil
}

// FindByUUID returns the manifest for a tracked conversation UUID, or nil if
// no manifest exists for it. Unlike ValidateNotDuplicate it does not treat an
// existing manifest as an error — callers decide what to do with it. Backed by
// an indexed lookup on agm_sessions.claude_uuid (see Adapter.GetSessionByUUID),
// so it stays O(1) instead of scanning the full session list.
func FindByUUID(conversationUUID string, adapter *dolt.Adapter) (*manifest.Manifest, error) {
	if adapter == nil {
		return nil, fmt.Errorf("Dolt adapter not available") //nolint:staticcheck // proper noun
	}
	return adapter.GetSessionByUUID(conversationUUID)
}

// InferWorkspaceFromProject derives a workspace label from a project directory
// by walking upward to the nearest workspace root (a directory containing a
// WORKSPACE.yaml or a .git entry) and returning that root's basename. It returns
// "" when no root is found or the path is not a usable absolute filesystem path
// (e.g. a ~/.claude/projects/<encoded> directory when history was unavailable).
func InferWorkspaceFromProject(projectPath string) string {
	if projectPath == "" {
		return ""
	}
	// Expand a leading ~ so history-supplied paths resolve on disk.
	if strings.HasPrefix(projectPath, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			projectPath = filepath.Join(home, strings.TrimPrefix(projectPath, "~"))
		}
	}
	if !filepath.IsAbs(projectPath) {
		return ""
	}
	dir := projectPath
	for {
		if _, err := os.Stat(filepath.Join(dir, "WORKSPACE.yaml")); err == nil {
			return filepath.Base(dir)
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return filepath.Base(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// deriveSessionName builds a stable, non-empty session name from the project
// path, falling back to a short UUID prefix so register never needs to prompt.
func deriveSessionName(projectPath, conversationUUID string) string {
	base := filepath.Base(projectPath) // filepath.Base trims trailing separators.
	if base != "" && base != "." && base != "/" {
		short := conversationUUID
		if len(short) > 8 {
			short = short[:8]
		}
		return fmt.Sprintf("%s-%s", base, short)
	}
	if len(conversationUUID) > 8 {
		return "session-" + conversationUUID[:8]
	}
	return "session-" + conversationUUID
}

// InferProjectPath finds the project directory for a conversation UUID
// by scanning ~/.claude/projects/*/<uuid>.jsonl
func InferProjectPath(conversationUUID string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	projectsDir := filepath.Join(home, ".claude", "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return "", fmt.Errorf("failed to read projects directory: %w", err)
	}

	// Search for <uuid>.jsonl in each project directory
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		conversationFile := filepath.Join(projectsDir, entry.Name(), conversationUUID+".jsonl")
		if _, err := os.Stat(conversationFile); err == nil {
			// Found it! Return the project directory path
			return filepath.Join(projectsDir, entry.Name()), nil
		}
	}

	return "", fmt.Errorf("no conversation file found for UUID: %s", conversationUUID)
}

// ExtractMetadataFromHistory extracts metadata from history.jsonl for a UUID
func ExtractMetadataFromHistory(conversationUUID string) (*SessionMetadata, error) {
	parser := history.NewParser("")
	sessions, err := parser.ReadConversations(10000) // Read last 10k entries
	if err != nil {
		return nil, fmt.Errorf("failed to read history: %w", err)
	}

	// Find the session matching our UUID
	for _, session := range sessions {
		if session.SessionID == conversationUUID {
			// Get last modified time from most recent entry
			var lastModified time.Time
			if len(session.Entries) > 0 {
				// Timestamp is in milliseconds
				lastModified = time.Unix(0, session.Entries[0].Timestamp*int64(time.Millisecond))
			} else {
				lastModified = time.Now()
			}

			return &SessionMetadata{
				UUID:         conversationUUID,
				ProjectPath:  session.Project,
				LastModified: lastModified,
			}, nil
		}
	}

	return nil, fmt.Errorf("no history entries found for UUID: %s", conversationUUID)
}

// ValidateNotDuplicate checks if a UUID already has a manifest
func ValidateNotDuplicate(conversationUUID string, adapter *dolt.Adapter) error {
	if adapter == nil {
		return fmt.Errorf("dolt adapter not available")
	}
	existing, err := adapter.GetSessionByUUID(conversationUUID)
	if err != nil {
		return fmt.Errorf("failed to check existing sessions in Dolt: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("conversation UUID %s already has manifest (session: %s)", conversationUUID, existing.Name)
	}
	return nil
}

// ValidateNotDuplicateWithAdapter checks if a UUID already has a session in Dolt
// Test helper function for Phase 5 migration
func ValidateNotDuplicateWithAdapter(conversationUUID string, adapter dolt.Storage) error {
	// List all sessions from database
	manifests, err := adapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	// Check if any session has this UUID
	for _, m := range manifests {
		if m.Claude.UUID == conversationUUID ||
			(m.Codex != nil && m.Codex.SessionID == conversationUUID) ||
			(m.Agy != nil && m.Agy.ConversationID == conversationUUID) ||
			(m.Pi != nil && m.Pi.SessionID == conversationUUID) {
			return fmt.Errorf("conversation UUID %s already has manifest (session: %s)", conversationUUID, m.Name)
		}
	}

	return nil
}

// ImportOrphanedSessionWithAdapter imports an orphaned session using Dolt adapter
// Test helper function for Phase 5 migration
func ImportOrphanedSessionWithAdapter(conversationUUID, sessionName, workspace string, adapter *dolt.Adapter) (string, error) {
	// 1. Validate inputs
	if conversationUUID == "" {
		return "", fmt.Errorf("conversation UUID cannot be empty")
	}
	if sessionName == "" {
		return "", fmt.Errorf("session name cannot be empty")
	}

	// 2. Check for duplicate (UUID already in database)
	if err := ValidateNotDuplicateWithAdapter(conversationUUID, adapter); err != nil {
		return "", err
	}

	// 3. Infer project path from conversation file location
	projectPath, err := InferProjectPath(conversationUUID)
	if err != nil {
		return "", fmt.Errorf("failed to infer project path: %w", err)
	}

	// 4. Extract metadata from history.jsonl
	metadata, err := ExtractMetadataFromHistory(conversationUUID)
	if err != nil {
		// Non-fatal: we can still import without history metadata
		metadata = &SessionMetadata{
			UUID:         conversationUUID,
			ProjectPath:  projectPath,
			LastModified: time.Now(),
		}
	} else if metadata.ProjectPath != "" {
		// Override project path if history has more accurate info
		projectPath = metadata.ProjectPath
	}

	// 5. Generate new session ID
	sessionID := uuid.New().String()

	// 6. Sanitize tmux session name
	tmuxName := tmux.SanitizeSessionName(sessionName)

	// 7. Create manifest
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     sessionID,
		Name:          sessionName,
		CreatedAt:     metadata.LastModified,
		UpdatedAt:     time.Now(),
		Lifecycle:     "", // Active by default
		Workspace:     workspace,
		Context: manifest.Context{
			Project: projectPath,
		},
		Claude: manifest.Claude{
			UUID: conversationUUID,
		},
		Tmux: manifest.Tmux{
			SessionName: tmuxName,
		},
	}

	// 8. Save to database
	if err := adapter.CreateSession(m); err != nil {
		return "", fmt.Errorf("failed to save session to database: %w", err)
	}

	return sessionID, nil
}
