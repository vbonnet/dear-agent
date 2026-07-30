package dolt

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// defaultIfEmpty returns dflt when s is empty, otherwise s.
func defaultIfEmpty(s, dflt string) string {
	if s == "" {
		return dflt
	}
	return s
}

// buildSessionMetadata assembles the map stored in the agm_sessions.metadata
// JSON column. It carries the engram integration fields plus the session
// outcome (stamped at archive time). Outcome is persisted here rather than in a
// dedicated column so it round-trips through every read path (list, get,
// frecency, hierarchy) without a schema migration — they all select metadata.
func buildSessionMetadata(session *manifest.Manifest) map[string]any {
	metadata := make(map[string]any)
	if session.WorkingDirectory != "" {
		metadata["working_directory"] = session.WorkingDirectory
	}
	if session.Sandbox != nil {
		metadata["sandbox"] = session.Sandbox
	}
	if session.Codex != nil {
		if session.Codex.SessionID != "" {
			metadata["codex_session_id"] = session.Codex.SessionID
		}
		if session.Codex.TranscriptPath != "" {
			metadata["codex_transcript_path"] = session.Codex.TranscriptPath
		}
	}
	addOpenAISessionMetadata(metadata, session.OpenAI)
	if session.Agy != nil {
		if session.Agy.ConversationID != "" {
			metadata["agy_conversation_id"] = session.Agy.ConversationID
		}
		if session.Agy.WorkspacePath != "" {
			metadata["agy_workspace_path"] = session.Agy.WorkspacePath
		}
		if session.Agy.ConversationDB != "" {
			metadata["agy_conversation_db_path"] = session.Agy.ConversationDB
		}
		if session.Agy.TranscriptPath != "" {
			metadata["agy_transcript_path"] = session.Agy.TranscriptPath
		}
	}
	if session.Pi != nil {
		if session.Pi.SessionID != "" {
			metadata["pi_session_id"] = session.Pi.SessionID
		}
		if session.Pi.SessionDir != "" {
			metadata["pi_session_dir"] = session.Pi.SessionDir
		}
		if session.Pi.TranscriptPath != "" {
			metadata["pi_transcript_path"] = session.Pi.TranscriptPath
		}
		if session.Pi.CodingAgentDirSet || session.Pi.CodingAgentDir != "" {
			metadata["pi_coding_agent_dir"] = session.Pi.CodingAgentDir
			metadata["pi_coding_agent_dir_set"] = true
		}
	}
	if session.EngramMetadata != nil {
		metadata["engram_enabled"] = session.EngramMetadata.Enabled
		metadata["engram_query"] = session.EngramMetadata.Query
		metadata["engram_ids"] = session.EngramMetadata.EngramIDs
		metadata["engram_loaded_at"] = session.EngramMetadata.LoadedAt
		metadata["engram_count"] = session.EngramMetadata.Count
	}
	if session.Outcome != manifest.OutcomeUnknown {
		metadata["outcome"] = string(session.Outcome)
	}
	return metadata
}

func addOpenAISessionMetadata(metadata map[string]any, openAI *manifest.OpenAI) {
	if openAI != nil {
		metadata["openai"] = openAI
	}
}

// marshalCreateSessionJSON serializes the JSON-typed fields needed by the
// agm_sessions INSERT (context tags, engram metadata, monitors).
func marshalCreateSessionJSON(session *manifest.Manifest) ([]byte, []byte, any, error) {
	contextTags, err := json.Marshal(session.Context.Tags)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to marshal context tags: %w", err)
	}
	metadataJSON, err := json.Marshal(buildSessionMetadata(session))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}
	var monitorsJSON interface{}
	if len(session.Monitors) > 0 {
		monitorsData, err := json.Marshal(session.Monitors)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to marshal monitors: %w", err)
		}
		monitorsJSON = string(monitorsData)
	}
	return contextTags, metadataJSON, monitorsJSON, nil
}

// CreateSession inserts a new session into the database
func (a *Adapter) CreateSession(session *manifest.Manifest) (retErr error) {
	if session == nil {
		return fmt.Errorf("session cannot be nil")
	}
	if session.SessionID == "" {
		return fmt.Errorf("session_id cannot be empty")
	}

	// Ensure migrations are applied
	if err := a.ApplyMigrations(); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	reservationHeld := session.Name != "" && session.Lifecycle != manifest.LifecycleArchived
	if reservationHeld {
		if err := a.ReserveSessionName(session.SessionID, session.Name); err != nil {
			return err
		}
		defer func() {
			if !reservationHeld {
				return
			}
			if err := a.ReleaseSessionNameReservation(session.SessionID); err != nil {
				retErr = errors.Join(retErr, err)
			}
		}()
	}
	if err := a.insertSessionRegistration(session, reservationHeld); err != nil {
		return err
	}
	reservationHeld = false
	return nil
}

func (a *Adapter) insertSessionRegistration(session *manifest.Manifest, reservationHeld bool) error {
	contextTags, metadataJSON, monitorsJSON, err := marshalCreateSessionJSON(session)
	if err != nil {
		return err
	}
	harness := defaultIfEmpty(session.Harness, "claude-code")
	status := lifecycleStorageStatus(session.Lifecycle)
	permissionMode := defaultIfEmpty(session.PermissionMode, "default")
	permissionModeSource := defaultIfEmpty(session.PermissionModeSource, "init")

	query := `
		INSERT INTO agm_sessions (
			id, created_at, updated_at, status, workspace, model, name, harness,
			context_project, context_purpose, context_tags, context_notes,
			claude_uuid, tmux_session_name, metadata,
			permission_mode, permission_mode_updated_at, permission_mode_source,
			parent_session_id, is_test,
			context_total_tokens, context_used_tokens, context_percentage_used,
			monitors
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	// Handle parent_session_id (nil pointer -> NULL)
	var parentSessionID interface{}
	if session.ParentSessionID != nil {
		parentSessionID = *session.ParentSessionID
	}

	// Handle context usage (nil pointer -> NULL)
	var ctxTotalTokens, ctxUsedTokens interface{}
	var ctxPercentageUsed interface{}
	if session.ContextUsage != nil {
		ctxTotalTokens = session.ContextUsage.TotalTokens
		ctxUsedTokens = session.ContextUsage.UsedTokens
		ctxPercentageUsed = session.ContextUsage.PercentageUsed
	}

	// Legacy manifests without a harness are Claude Code sessions, so retain the
	// historical Claude default for that route. Other harnesses use an empty
	// model to mean that the provider-native selection is unknown and must not be
	// replaced with an Anthropic model.
	model := session.Model
	if model == "" && harness == "claude-code" {
		model = "claude-sonnet-4-5"
	}

	tx, err := a.conn.Begin() //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		return fmt.Errorf("begin session registration: %w", err)
	}
	defer tx.Rollback()
	if reservationHeld {
		if err := reservationOwnedBy(tx, a.workspace, session.SessionID, session.Name); err != nil {
			return err
		}
	}

	_, err = tx.Exec(query, //nolint:noctx // TODO(context): plumb ctx through this layer
		session.SessionID,
		session.CreatedAt,
		session.UpdatedAt,
		status,
		a.workspace, // Auto-set workspace from adapter
		model,
		session.Name,
		harness,
		session.Context.Project,
		session.Context.Purpose,
		contextTags,
		session.Context.Notes,
		session.Claude.UUID,
		session.Tmux.SessionName,
		metadataJSON,
		permissionMode,
		session.PermissionModeUpdatedAt,
		permissionModeSource,
		parentSessionID,
		session.IsTest,
		ctxTotalTokens,
		ctxUsedTokens,
		ctxPercentageUsed,
		monitorsJSON,
	)

	if err != nil {
		return fmt.Errorf("failed to insert session: %w", err)
	}

	if reservationHeld {
		if _, err := tx.Exec( //nolint:noctx // TODO(context): plumb ctx through this layer
			`DELETE FROM agm_session_name_reservations
			 WHERE workspace = ? AND session_id = ?`,
			a.workspace,
			session.SessionID,
		); err != nil {
			return fmt.Errorf("finalize session-name reservation: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session registration: %w", err)
	}
	return nil
}

// GetSession retrieves a session by ID
func (a *Adapter) GetSession(sessionID string) (*manifest.Manifest, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id cannot be empty")
	}

	// Ensure migrations are applied
	if err := a.ApplyMigrations(); err != nil {
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}

	query := `
		SELECT id, created_at, updated_at, status, workspace, model, name, harness,
			context_project, context_purpose, context_tags, context_notes,
			claude_uuid, tmux_session_name, tmux_session_revision, metadata,
			permission_mode, permission_mode_updated_at, permission_mode_source,
			is_test,
			context_total_tokens, context_used_tokens, context_percentage_used,
			monitors
		FROM agm_sessions
		WHERE id = ? AND workspace = ?
	`

	row := a.conn.QueryRow(query, sessionID, a.workspace) //nolint:noctx // TODO(context): plumb ctx through this layer
	return a.scanSession(row)
}

// UpdateSession updates an existing session in the database
func (a *Adapter) UpdateSession(session *manifest.Manifest) error {
	if session == nil {
		return fmt.Errorf("session cannot be nil")
	}
	if session.SessionID == "" {
		return fmt.Errorf("session_id cannot be empty")
	}

	// Ensure migrations are applied
	if err := a.ApplyMigrations(); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	// Marshal context tags to JSON
	contextTags, err := json.Marshal(session.Context.Tags)
	if err != nil {
		return fmt.Errorf("failed to marshal context tags: %w", err)
	}

	// Build metadata JSON from EngramMetadata + outcome
	metadataJSON, err := json.Marshal(buildSessionMetadata(session))
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Determine harness (default to claude-code for backward compatibility)
	harness := session.Harness
	if harness == "" {
		harness = "claude-code"
	}

	// Preserve transitional lifecycle values such as "reaping" so detached
	// reapers can recover across processes in an isolated SQLite test store.
	status := lifecycleStorageStatus(session.Lifecycle)

	// Update timestamp
	session.UpdatedAt = time.Now()

	// Handle context usage (nil pointer -> NULL)
	var ctxTotalTokens, ctxUsedTokens interface{}
	var ctxPercentageUsed interface{}
	if session.ContextUsage != nil {
		ctxTotalTokens = session.ContextUsage.TotalTokens
		ctxUsedTokens = session.ContextUsage.UsedTokens
		ctxPercentageUsed = session.ContextUsage.PercentageUsed
	}

	// Marshal monitors to JSON
	var monitorsJSON interface{}
	if len(session.Monitors) > 0 {
		monitorsData, err := json.Marshal(session.Monitors)
		if err != nil {
			return fmt.Errorf("failed to marshal monitors: %w", err)
		}
		monitorsJSON = string(monitorsData)
	}

	observedTmuxRevision := nullableStringValue(sql.NullString{
		String: session.Tmux.SessionRevision,
		Valid:  session.Tmux.SessionRevision != "",
	})
	nextTmuxRevision := uuid.NewString()
	// Full-session writers may have read before a resume installed its
	// provisional tmux owner. Update their unrelated fields, but change the tmux
	// identity only when the opaque revision they observed is still current.
	// Every writer advances the revision, including a writer that loses this
	// comparison, so any other stale snapshot remains unable to reopen the CAS.
	query := `
		UPDATE agm_sessions
		SET updated_at = ?, status = ?,
			name = CASE
				WHEN ((tmux_session_revision IS NULL AND ? IS NULL) OR tmux_session_revision = ?)
				THEN ? ELSE name END,
			harness = ?, model = ?,
			context_project = ?, context_purpose = ?, context_tags = ?,
			context_notes = ?, claude_uuid = ?,
			tmux_session_name = CASE
				WHEN ((tmux_session_revision IS NULL AND ? IS NULL) OR tmux_session_revision = ?)
				THEN ? ELSE tmux_session_name END,
			tmux_session_revision = ?,
			metadata = ?,
			permission_mode = ?, permission_mode_updated_at = ?, permission_mode_source = ?,
			is_test = ?,
			context_total_tokens = ?, context_used_tokens = ?, context_percentage_used = ?,
			monitors = ?
		WHERE id = ? AND workspace = ?
	`

	result, err := a.conn.Exec(query, //nolint:noctx // TODO(context): plumb ctx through this layer
		session.UpdatedAt,
		status,
		observedTmuxRevision,
		observedTmuxRevision,
		session.Name,
		harness,
		session.Model,
		session.Context.Project,
		session.Context.Purpose,
		contextTags,
		session.Context.Notes,
		session.Claude.UUID,
		observedTmuxRevision,
		observedTmuxRevision,
		session.Tmux.SessionName,
		nextTmuxRevision,
		metadataJSON,
		session.PermissionMode,
		session.PermissionModeUpdatedAt,
		session.PermissionModeSource,
		session.IsTest,
		ctxTotalTokens,
		ctxUsedTokens,
		ctxPercentageUsed,
		monitorsJSON,
		session.SessionID,
		a.workspace,
	)

	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("session not found: %s", session.SessionID)
	}
	return nil
}

// UpdateTmuxSessionName persists only the live tmux identity for a session.
// Resume readiness can take long enough for hooks or another AGM command to
// update unrelated metadata, so writing the pre-readiness Manifest snapshot
// here would lose those concurrent changes.
func (a *Adapter) UpdateTmuxSessionName(ctx context.Context, sessionID, sessionName string) error {
	if sessionID == "" {
		return fmt.Errorf("session_id cannot be empty")
	}
	if sessionName == "" {
		return fmt.Errorf("tmux session name cannot be empty")
	}
	if err := a.ApplyMigrations(); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	result, err := a.conn.ExecContext(ctx, `
		UPDATE agm_sessions
		SET updated_at = ?, tmux_session_name = ?, tmux_session_revision = ?
		WHERE id = ? AND workspace = ?
	`, time.Now(), sessionName, uuid.NewString(), sessionID, a.workspace)
	if err != nil {
		return fmt.Errorf("failed to update tmux session name: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	return nil
}

// TouchSessionActivity updates only the session activity timestamp. It does
// not rotate the tmux identity revision: a cold-resume transaction keeps that
// ownership token provisional until every creation-finalization effect has
// succeeded, allowing an exact rollback after caller cancellation.
func (a *Adapter) TouchSessionActivity(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session_id cannot be empty")
	}
	if err := a.ApplyMigrations(); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	result, err := a.conn.ExecContext(ctx, `
		UPDATE agm_sessions
		SET updated_at = ?
		WHERE id = ? AND workspace = ?
	`, time.Now(), sessionID, a.workspace)
	if err != nil {
		return fmt.Errorf("touch session activity: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get touched session rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	return nil
}

// RenameSessionIdentityResult tells the caller whether reverting an already
// moved tmux session is safe when the storage mutation returns an error.
type RenameSessionIdentityResult struct {
	TmuxRollbackSafe bool
}

// RenameSessionIdentity atomically changes both user-visible and tmux names
// from the exact identity revision observed by the caller. Unlike a broad
// UpdateSession, an authoritative rename must report a concurrent change
// instead of silently preserving the old tmux name after the live pane moved.
func (a *Adapter) RenameSessionIdentity(ctx context.Context, sessionID, previousName, previousTmuxName, observedRevision, newName string) (RenameSessionIdentityResult, error) {
	if sessionID == "" {
		return RenameSessionIdentityResult{}, fmt.Errorf("session_id cannot be empty")
	}
	if newName == "" {
		return RenameSessionIdentityResult{}, fmt.Errorf("new session name cannot be empty")
	}
	if err := a.ApplyMigrations(); err != nil {
		return RenameSessionIdentityResult{}, fmt.Errorf("failed to apply migrations: %w", err)
	}
	observedRevisionValue := nullableStringValue(sql.NullString{
		String: observedRevision,
		Valid:  observedRevision != "",
	})
	nextRevision := uuid.NewString()
	result, err := a.conn.ExecContext(ctx, `
		UPDATE agm_sessions
		SET updated_at = ?, name = ?, tmux_session_name = ?, tmux_session_revision = ?
		WHERE id = ? AND workspace = ?
		  AND name = ? AND tmux_session_name = ?
		  AND ((tmux_session_revision IS NULL AND ? IS NULL) OR tmux_session_revision = ?)
	`, time.Now(), newName, newName, nextRevision, sessionID, a.workspace,
		previousName, previousTmuxName, observedRevisionValue, observedRevisionValue)
	if err == nil {
		rowsAffected, rowsErr := result.RowsAffected()
		if rowsErr == nil && rowsAffected == 1 {
			return RenameSessionIdentityResult{}, nil
		}
		if rowsErr != nil {
			err = fmt.Errorf("get renamed session identity rows affected: %w", rowsErr)
		} else {
			err = fmt.Errorf("session identity changed concurrently: %s", sessionID)
		}
	} else {
		err = fmt.Errorf("rename session identity: %w", err)
	}

	// ExecContext can lose its reply before the server finishes autocommit. A
	// re-read of the unchanged snapshot is not yet proof that the write will not
	// commit, so first advance the exact observed revision with a competing CAS.
	// Whichever write reaches the row first fences the other one.
	inspectCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	fenceRevision := uuid.NewString()
	if fenceErr := a.fenceSessionIdentityRename(inspectCtx, sessionID, previousName, previousTmuxName, observedRevisionValue, fenceRevision); fenceErr != nil {
		err = errors.Join(err, fenceErr)
	}
	currentName, currentTmuxName, currentRevision, inspectErr := a.inspectSessionIdentity(inspectCtx, sessionID)
	if inspectErr != nil {
		return RenameSessionIdentityResult{}, errors.Join(err, inspectErr)
	}
	return classifySessionIdentityRenameAfterError(previousName, previousTmuxName, observedRevision, newName, nextRevision, fenceRevision, currentName, currentTmuxName, currentRevision, err)
}

func (a *Adapter) fenceSessionIdentityRename(ctx context.Context, sessionID, previousName, previousTmuxName string, observedRevisionValue any, fenceRevision string) error {
	result, err := a.conn.ExecContext(ctx, `
		UPDATE agm_sessions
		SET tmux_session_revision = ?
		WHERE id = ? AND workspace = ?
		  AND name = ? AND tmux_session_name = ?
		  AND ((tmux_session_revision IS NULL AND ? IS NULL) OR tmux_session_revision = ?)
	`, fenceRevision, sessionID, a.workspace, previousName, previousTmuxName, observedRevisionValue, observedRevisionValue)
	if err != nil {
		return fmt.Errorf("fence pending session identity rename: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get session identity rename fence rows affected: %w", err)
	}
	if rowsAffected > 1 {
		return fmt.Errorf("session identity rename fence changed %d rows", rowsAffected)
	}
	return nil
}

func classifySessionIdentityRenameAfterError(previousName, previousTmuxName, observedRevision, newName, nextRevision, fenceRevision, currentName, currentTmuxName, currentRevision string, primaryErr error) (RenameSessionIdentityResult, error) {
	committedIdentity := currentName == newName && currentTmuxName == newName
	exactCommit := committedIdentity && currentRevision == nextRevision
	supersededCommit := committedIdentity && currentRevision != nextRevision
	if exactCommit || supersededCommit {
		// Exact revision proves our write committed. A different revision with
		// the same names means a later writer already superseded it while
		// preserving the intended identity; both outcomes are complete.
		return RenameSessionIdentityResult{}, nil
	}
	previousIdentity := currentName == previousName && currentTmuxName == previousTmuxName
	// The exact fence revision proves our competing CAS won. Any other revision
	// advanced away from the observed value also makes the original CAS unable
	// to commit. The unchanged observed revision remains ambiguous because the
	// original autocommit may still be in flight.
	fenced := currentRevision == fenceRevision || currentRevision != observedRevision
	rollbackSafe := previousIdentity && fenced
	return RenameSessionIdentityResult{TmuxRollbackSafe: rollbackSafe}, primaryErr
}

func (a *Adapter) inspectSessionIdentity(ctx context.Context, sessionID string) (string, string, string, error) {
	var name, tmuxName string
	var revision sql.NullString
	err := a.conn.QueryRowContext(ctx, `
		SELECT name, tmux_session_name, tmux_session_revision
		FROM agm_sessions
		WHERE id = ? AND workspace = ?
	`, sessionID, a.workspace).Scan(&name, &tmuxName, &revision)
	if err != nil {
		return "", "", "", fmt.Errorf("inspect session identity after rename error: %w", err)
	}
	return name, tmuxName, revision.String, nil
}

// TmuxSessionNameChange is the exact adapter-owned revision for a provisional
// tmux-name write. Its opaque token is compared directly by both MySQL/Dolt and
// SQLite, avoiding dialect-specific timestamp casts and precision differences.
type TmuxSessionNameChange struct {
	SessionID         string
	PreviousName      string
	PreviousRevision  sql.NullString
	PreviousUpdatedAt time.Time
	CurrentName       string
	CurrentRevision   string
}

type tmuxSessionNameChangeState uint8

const (
	tmuxSessionNameChangeUnknown tmuxSessionNameChangeState = iota
	tmuxSessionNameChangePrevious
	tmuxSessionNameChangeCurrent
	tmuxSessionNameChangeSuperseded
)

// BeginTmuxSessionNameChange persists a provisional canonical tmux name while
// preserving every unrelated column. The read and compare-and-swap run in one
// database transaction, so a concurrent session update cannot be overwritten.
// A nil change means the requested name was already current.
func (a *Adapter) BeginTmuxSessionNameChange(ctx context.Context, sessionID, newName string) (*TmuxSessionNameChange, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id cannot be empty")
	}
	if newName == "" {
		return nil, fmt.Errorf("tmux session name cannot be empty")
	}
	if err := a.ApplyMigrations(); err != nil {
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}
	tx, err := a.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tmux session name change: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var previousName string
	var previousRevision sql.NullString
	var previousUpdatedAt time.Time
	if err := tx.QueryRowContext(ctx, `
		SELECT tmux_session_name, tmux_session_revision, updated_at
		FROM agm_sessions
		WHERE id = ? AND workspace = ?
	`, sessionID, a.workspace).Scan(&previousName, &previousRevision, &previousUpdatedAt); err != nil {
		return nil, fmt.Errorf("read tmux session name revision: %w", err)
	}
	if previousName == newName {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit unchanged tmux session name: %w", err)
		}
		return nil, nil
	}
	currentRevision := uuid.NewString()
	previousRevisionValue := nullableStringValue(previousRevision)
	result, err := tx.ExecContext(ctx, `
		UPDATE agm_sessions
		SET updated_at = ?, tmux_session_name = ?, tmux_session_revision = ?
		WHERE id = ? AND workspace = ?
		  AND tmux_session_name = ?
		  AND ((tmux_session_revision IS NULL AND ? IS NULL) OR tmux_session_revision = ?)
	`, time.Now(), newName, currentRevision, sessionID, a.workspace, previousName, previousRevisionValue, previousRevisionValue)
	if err != nil {
		return nil, fmt.Errorf("write provisional tmux session name: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("get provisional tmux name rows affected: %w", err)
	}
	if rowsAffected != 1 {
		return nil, fmt.Errorf("session metadata changed concurrently")
	}
	change := &TmuxSessionNameChange{
		SessionID:         sessionID,
		PreviousName:      previousName,
		PreviousRevision:  previousRevision,
		PreviousUpdatedAt: previousUpdatedAt,
		CurrentName:       newName,
		CurrentRevision:   currentRevision,
	}
	if err := tx.Commit(); err != nil {
		// Commit can report an error after the database durably accepted the
		// write. Release any remaining transaction resources, then re-read the
		// exact ownership revision before deciding whether cleanup is safe.
		_ = tx.Rollback()
		inspectCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		state, inspectErr := a.inspectTmuxSessionNameChange(inspectCtx, *change)
		return resolveTmuxSessionNameChangeCommitError(change, err, state, inspectErr)
	}
	return change, nil
}

func (a *Adapter) inspectTmuxSessionNameChange(ctx context.Context, change TmuxSessionNameChange) (tmuxSessionNameChangeState, error) {
	var currentName string
	var currentRevision sql.NullString
	var currentUpdatedAt time.Time
	if err := a.conn.QueryRowContext(ctx, `
		SELECT tmux_session_name, tmux_session_revision, updated_at
		FROM agm_sessions
		WHERE id = ? AND workspace = ?
	`, change.SessionID, a.workspace).Scan(&currentName, &currentRevision, &currentUpdatedAt); err != nil {
		return tmuxSessionNameChangeUnknown, fmt.Errorf("re-read tmux session name after commit error: %w", err)
	}
	if currentName == change.CurrentName && currentRevision.Valid && currentRevision.String == change.CurrentRevision {
		return tmuxSessionNameChangeCurrent, nil
	}
	if currentName == change.PreviousName && currentRevision == change.PreviousRevision && currentUpdatedAt.Equal(change.PreviousUpdatedAt) {
		return tmuxSessionNameChangePrevious, nil
	}
	return tmuxSessionNameChangeSuperseded, nil
}

func resolveTmuxSessionNameChangeCommitError(change *TmuxSessionNameChange, commitErr error, state tmuxSessionNameChangeState, inspectErr error) (*TmuxSessionNameChange, error) {
	err := fmt.Errorf("commit provisional tmux session name: %w", commitErr)
	if inspectErr != nil {
		return change, errors.Join(err, inspectErr)
	}
	switch state {
	case tmuxSessionNameChangePrevious:
		// The complete previous revision proves the write did not commit.
		return nil, err
	case tmuxSessionNameChangeCurrent:
		// The owned provisional revision committed despite the error. Preserve
		// it so the caller's rollback can restore metadata before killing tmux.
		return change, err
	case tmuxSessionNameChangeUnknown, tmuxSessionNameChangeSuperseded:
		// A superseding writer (or an unrecognized state) makes ownership
		// uncertain. Preserve the change; its CAS restore will refuse to erase
		// newer metadata and therefore prevent unsafe tmux cleanup.
		return change, errors.Join(err, fmt.Errorf("tmux session metadata state after commit error cannot prove rollback safety"))
	default:
		return change, errors.Join(err, fmt.Errorf("unknown tmux session metadata state %d after commit error", state))
	}
}

func nullableStringValue(value sql.NullString) any {
	if !value.Valid {
		return nil
	}
	return value.String
}

// RestoreTmuxSessionNameChange compensates only the exact provisional revision
// returned by BeginTmuxSessionNameChange. If another operation changed the
// session afterward, it returns false and preserves that newer state.
func (a *Adapter) RestoreTmuxSessionNameChange(ctx context.Context, change TmuxSessionNameChange) (bool, error) {
	result, err := a.conn.ExecContext(ctx, `
		UPDATE agm_sessions
		SET updated_at = ?, tmux_session_name = ?, tmux_session_revision = ?
		WHERE id = ? AND workspace = ?
		  AND tmux_session_name = ? AND tmux_session_revision = ?
	`, change.PreviousUpdatedAt, change.PreviousName, nullableStringValue(change.PreviousRevision), change.SessionID, a.workspace, change.CurrentName, change.CurrentRevision)
	if err != nil {
		return false, fmt.Errorf("restore provisional tmux session name: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("get restored tmux name rows affected: %w", err)
	}
	return rowsAffected == 1, nil
}

// CompleteTmuxSessionNameChange rotates away from the provisional ownership
// token after the irreversible prompt boundary succeeds. A false result means
// another writer already superseded the provisional revision.
func (a *Adapter) CompleteTmuxSessionNameChange(ctx context.Context, change TmuxSessionNameChange) (bool, error) {
	result, err := a.conn.ExecContext(ctx, `
		UPDATE agm_sessions
		SET tmux_session_revision = ?
		WHERE id = ? AND workspace = ?
		  AND tmux_session_name = ? AND tmux_session_revision = ?
	`, uuid.NewString(), change.SessionID, a.workspace, change.CurrentName, change.CurrentRevision)
	if err != nil {
		return false, fmt.Errorf("complete provisional tmux session name: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("get completed tmux name rows affected: %w", err)
	}
	return rowsAffected == 1, nil
}

// DeleteSession deletes a session from the database
func (a *Adapter) DeleteSession(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session_id cannot be empty")
	}

	// Ensure migrations are applied
	if err := a.ApplyMigrations(); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	query := `DELETE FROM agm_sessions WHERE id = ? AND workspace = ?`

	result, err := a.conn.Exec(query, sessionID, a.workspace) //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	return nil
}

// ListActiveSessions returns the names of all non-archived sessions.
// Used for lightweight cross-reference (e.g., audit commands) without loading full manifests.
func (a *Adapter) ListActiveSessions(ctx context.Context) ([]string, error) {
	if err := a.ApplyMigrations(); err != nil {
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}
	query := `SELECT name FROM agm_sessions WHERE workspace = ? AND status != 'archived' ORDER BY updated_at DESC`
	rows, err := a.conn.QueryContext(ctx, query, a.workspace)
	if err != nil {
		return nil, fmt.Errorf("failed to list active sessions: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan session name: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// applyListSessionsFilter appends the WHERE clauses for the optional filter
// (lifecycle/test/harness/tags) and returns the updated query+args.
func applyListSessionsFilter(query string, args []any, filter *SessionFilter) (string, []any) {
	if filter == nil {
		return query, args
	}
	if filter.ExcludeArchived {
		query += " AND status != 'archived'"
	} else if filter.Lifecycle != "" {
		if filter.Lifecycle == manifest.LifecycleArchived {
			query += " AND status = 'archived'"
		} else {
			query += " AND status = 'active'"
		}
	}
	if filter.ExcludeTest {
		query += " AND (is_test = FALSE OR is_test IS NULL)"
	}
	if filter.Harness != "" {
		query += " AND harness = ?"
		args = append(args, filter.Harness)
	}
	for _, tag := range filter.Tags {
		query += " AND JSON_CONTAINS(context_tags, ?)"
		args = append(args, fmt.Sprintf("%q", tag))
	}
	return query, args
}

// applyListSessionsLimit appends LIMIT/OFFSET clauses from filter.
func applyListSessionsLimit(query string, args []any, filter *SessionFilter) (string, []any) {
	if filter == nil {
		return query, args
	}
	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}
	return query, args
}

// ListSessions returns a list of sessions matching the filter criteria
func (a *Adapter) ListSessions(filter *SessionFilter) ([]*manifest.Manifest, error) {
	// Ensure migrations are applied
	if err := a.ApplyMigrations(); err != nil {
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}

	query := `
		SELECT id, created_at, updated_at, status, workspace, model, name, harness,
			context_project, context_purpose, context_tags, context_notes,
			claude_uuid, tmux_session_name, tmux_session_revision, metadata,
			permission_mode, permission_mode_updated_at, permission_mode_source,
			is_test,
			context_total_tokens, context_used_tokens, context_percentage_used,
			monitors
		FROM agm_sessions
		WHERE workspace = ?
	`

	args := []any{a.workspace}

	query, args = applyListSessionsFilter(query, args, filter)
	if filter != nil && filter.StableOrder {
		query += " ORDER BY created_at ASC, id ASC"
	} else {
		query += " ORDER BY updated_at DESC, id ASC"
	}
	query, args = applyListSessionsLimit(query, args, filter)

	rows, err := a.conn.Query(query, args...) //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		return nil, fmt.Errorf("failed to query sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*manifest.Manifest
	for rows.Next() {
		session, err := a.scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return sessions, nil
}

// ResolveIdentifier finds a session by session ID, tmux name, or manifest name
// This replaces the filesystem-based session.ResolveIdentifier() for Dolt storage
func (a *Adapter) ResolveIdentifier(identifier string) (*manifest.Manifest, error) {
	if identifier == "" {
		return nil, fmt.Errorf("identifier cannot be empty")
	}

	// Ensure migrations are applied
	if err := a.ApplyMigrations(); err != nil {
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}

	// Try matching by session ID, tmux session name, OR manifest name
	// Exclude archived sessions (status != 'archived')
	query := `
		SELECT id, created_at, updated_at, status, workspace, model, name, harness,
			context_project, context_purpose, context_tags, context_notes,
			claude_uuid, tmux_session_name, tmux_session_revision, metadata,
			permission_mode, permission_mode_updated_at, permission_mode_source,
			is_test,
			context_total_tokens, context_used_tokens, context_percentage_used,
			monitors
		FROM agm_sessions
		WHERE workspace = ?
		  AND (id = ? OR tmux_session_name = ? OR name = ?)
		  AND status != 'archived'
		LIMIT 1
	`

	row := a.conn.QueryRow(query, a.workspace, identifier, identifier, identifier) //nolint:noctx // TODO(context): plumb ctx through this layer
	m, err := a.scanSession(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "session not found") {
			return nil, fmt.Errorf("session not found: %s", identifier)
		}
		return nil, err
	}

	return m, nil
}

// GetSessionByUUID returns the session that owns the given harness conversation
// UUID, regardless of lifecycle (active or archived), or (nil, nil) if no
// session is tracking that UUID. Claude stores this in claude_uuid; Codex and
// AGY store it in the metadata JSON blob. Conversation UUIDs identify an
// underlying transcript and are globally unique, so no workspace scoping is
// applied.
func (a *Adapter) GetSessionByUUID(conversationUUID string) (*manifest.Manifest, error) {
	if conversationUUID == "" {
		return nil, fmt.Errorf("conversation UUID cannot be empty")
	}

	if err := a.ApplyMigrations(); err != nil {
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}

	jsonValue := "JSON_UNQUOTE(JSON_EXTRACT(metadata, '$.%s'))"
	if a.IsTestStore() {
		// SQLite's json_extract already returns scalar strings unquoted and does
		// not provide MySQL's JSON_UNQUOTE function.
		jsonValue = "json_extract(metadata, '$.%s')"
	}
	query := fmt.Sprintf(`
		SELECT id, created_at, updated_at, status, workspace, model, name, harness,
			context_project, context_purpose, context_tags, context_notes,
			claude_uuid, tmux_session_name, tmux_session_revision, metadata,
			permission_mode, permission_mode_updated_at, permission_mode_source,
			is_test,
			context_total_tokens, context_used_tokens, context_percentage_used,
			monitors
		FROM agm_sessions
		WHERE claude_uuid = ?
		   OR `+jsonValue+` = ?
		   OR `+jsonValue+` = ?
		   OR `+jsonValue+` = ?
		LIMIT 1
	`, "codex_session_id", "agy_conversation_id", "pi_session_id")

	row := a.conn.QueryRow(query, conversationUUID, conversationUUID, conversationUUID, conversationUUID) //nolint:noctx // TODO(context): plumb ctx through this layer
	m, err := a.scanSession(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "session not found") {
			return nil, nil
		}
		return nil, err
	}

	return m, nil
}

// GetSessionByName returns the first non-archived session matching the given name
func (a *Adapter) GetSessionByName(name string) (*manifest.Manifest, error) {
	if name == "" {
		return nil, fmt.Errorf("name cannot be empty")
	}

	// Ensure migrations are applied
	if err := a.ApplyMigrations(); err != nil {
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}

	query := `
		SELECT id, created_at, updated_at, status, workspace, model, name, harness,
			context_project, context_purpose, context_tags, context_notes,
			claude_uuid, tmux_session_name, tmux_session_revision, metadata,
			permission_mode, permission_mode_updated_at, permission_mode_source,
			is_test,
			context_total_tokens, context_used_tokens, context_percentage_used,
			monitors
		FROM agm_sessions
		WHERE workspace = ? AND name = ? AND status != 'archived'
		LIMIT 1
	`

	row := a.conn.QueryRow(query, a.workspace, name) //nolint:noctx // TODO(context): plumb ctx through this layer
	m, err := a.scanSession(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "session not found") {
			return nil, nil
		}
		return nil, err
	}

	return m, nil
}

// scanSession is a helper that scans a row into a Manifest struct
type scanner interface {
	Scan(dest ...any) error
}

func (a *Adapter) scanSession(row scanner) (*manifest.Manifest, error) {
	var session manifest.Manifest
	var contextTagsJSON []byte
	var metadataJSON []byte
	var status string
	var workspace string
	var model string
	var permissionMode sql.NullString
	var permissionModeUpdatedAt sql.NullTime
	var permissionModeSource sql.NullString
	var isTest sql.NullBool
	var ctxTotalTokens sql.NullInt64
	var ctxUsedTokens sql.NullInt64
	var ctxPercentageUsed sql.NullFloat64
	var monitorsJSON sql.NullString
	var tmuxSessionRevision sql.NullString

	err := row.Scan(
		&session.SessionID,
		&session.CreatedAt,
		&session.UpdatedAt,
		&status,
		&workspace,
		&model,
		&session.Name,
		&session.Harness,
		&session.Context.Project,
		&session.Context.Purpose,
		&contextTagsJSON,
		&session.Context.Notes,
		&session.Claude.UUID,
		&session.Tmux.SessionName,
		&tmuxSessionRevision,
		&metadataJSON,
		&permissionMode,
		&permissionModeUpdatedAt,
		&permissionModeSource,
		&isTest,
		&ctxTotalTokens,
		&ctxUsedTokens,
		&ctxPercentageUsed,
		&monitorsJSON,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("session not found")
		}
		return nil, fmt.Errorf("failed to scan session: %w", err)
	}

	// Set lifecycle from status. Runtime active/stopped state remains outside the
	// durable lifecycle field; terminal/transitional lifecycle values round-trip.
	switch status {
	case manifest.LifecycleArchived, manifest.LifecycleReaping:
		session.Lifecycle = status
	default:
		session.Lifecycle = ""
	}

	// Set workspace and model
	session.Workspace = workspace
	session.Model = model
	if tmuxSessionRevision.Valid {
		session.Tmux.SessionRevision = tmuxSessionRevision.String
	}

	// Set schema version
	session.SchemaVersion = "2.0"

	// Note: parent_session_id temporarily removed from SELECT due to schema migration issues
	// Will be re-added once migration 007 is reliably applied across all environments

	applyNullableScanFields(&session, permissionMode, permissionModeUpdatedAt, permissionModeSource, isTest, ctxTotalTokens, ctxUsedTokens, ctxPercentageUsed)
	if err := unmarshalContextTags(&session, contextTagsJSON); err != nil {
		return nil, err
	}
	if err := unmarshalMonitors(&session, monitorsJSON); err != nil {
		return nil, err
	}
	if err := unmarshalSessionMetadata(&session, metadataJSON); err != nil {
		return nil, err
	}
	return &session, nil
}

func lifecycleStorageStatus(lifecycle string) string {
	switch lifecycle {
	case manifest.LifecycleArchived, manifest.LifecycleReaping:
		return lifecycle
	default:
		return "active"
	}
}

// applyNullableScanFields copies the nullable sql.Null* values from a Scan call
// into the corresponding manifest fields when valid.
func applyNullableScanFields(session *manifest.Manifest, permissionMode sql.NullString, permissionModeUpdatedAt sql.NullTime, permissionModeSource sql.NullString, isTest sql.NullBool, ctxTotalTokens, ctxUsedTokens sql.NullInt64, ctxPercentageUsed sql.NullFloat64) {
	if permissionMode.Valid {
		session.PermissionMode = permissionMode.String
	}
	if permissionModeUpdatedAt.Valid {
		session.PermissionModeUpdatedAt = &permissionModeUpdatedAt.Time
	}
	if permissionModeSource.Valid {
		session.PermissionModeSource = permissionModeSource.String
	}
	if isTest.Valid {
		session.IsTest = isTest.Bool
	}
	if ctxTotalTokens.Valid || ctxUsedTokens.Valid || ctxPercentageUsed.Valid {
		session.ContextUsage = &manifest.ContextUsage{}
		if ctxTotalTokens.Valid {
			session.ContextUsage.TotalTokens = int(ctxTotalTokens.Int64)
		}
		if ctxUsedTokens.Valid {
			session.ContextUsage.UsedTokens = int(ctxUsedTokens.Int64)
		}
		if ctxPercentageUsed.Valid {
			session.ContextUsage.PercentageUsed = ctxPercentageUsed.Float64
		}
	}
}

func unmarshalContextTags(session *manifest.Manifest, contextTagsJSON []byte) error {
	if len(contextTagsJSON) == 0 {
		return nil
	}
	if err := json.Unmarshal(contextTagsJSON, &session.Context.Tags); err != nil {
		return fmt.Errorf("failed to unmarshal context tags: %w", err)
	}
	return nil
}

func unmarshalMonitors(session *manifest.Manifest, monitorsJSON sql.NullString) error {
	if !monitorsJSON.Valid || monitorsJSON.String == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(monitorsJSON.String), &session.Monitors); err != nil {
		return fmt.Errorf("failed to unmarshal monitors: %w", err)
	}
	return nil
}

func unmarshalSessionMetadata(session *manifest.Manifest, metadataJSON []byte) error {
	if len(metadataJSON) == 0 {
		return nil
	}
	var metadata map[string]any
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		return fmt.Errorf("failed to unmarshal metadata: %w", err)
	}
	// Outcome lives alongside engram fields but is independent of them — read it
	// before the engram early-return so it round-trips even when engram is off.
	if outcome, ok := metadata["outcome"].(string); ok {
		session.Outcome = manifest.SessionOutcome(outcome)
	}
	if workingDirectory, ok := metadata["working_directory"].(string); ok {
		session.WorkingDirectory = workingDirectory
	}
	if err := unmarshalSandboxMetadata(session, metadata); err != nil {
		return err
	}
	applyCodexMetadata(session, metadata)
	if err := unmarshalOpenAISessionMetadata(session, metadata); err != nil {
		return err
	}
	applyAgyMetadata(session, metadata)
	applyPiMetadata(session, metadata)
	applyEngramMetadata(session, metadata)
	return nil
}

func applyCodexMetadata(session *manifest.Manifest, metadata map[string]any) {
	sessionID, _ := metadata["codex_session_id"].(string)
	transcriptPath, _ := metadata["codex_transcript_path"].(string)
	if sessionID != "" || transcriptPath != "" {
		session.Codex = &manifest.Codex{
			SessionID:      sessionID,
			TranscriptPath: transcriptPath,
		}
	}
}

func applyAgyMetadata(session *manifest.Manifest, metadata map[string]any) {
	agyConversationID, _ := metadata["agy_conversation_id"].(string)
	agyWorkspacePath, _ := metadata["agy_workspace_path"].(string)
	agyConversationDBPath, _ := metadata["agy_conversation_db_path"].(string)
	agyTranscriptPath, _ := metadata["agy_transcript_path"].(string)
	if agyConversationID != "" || agyWorkspacePath != "" || agyConversationDBPath != "" || agyTranscriptPath != "" {
		session.Agy = &manifest.Agy{
			ConversationID: agyConversationID,
			WorkspacePath:  agyWorkspacePath,
			ConversationDB: agyConversationDBPath,
			TranscriptPath: agyTranscriptPath,
		}
	}
}

func applyPiMetadata(session *manifest.Manifest, metadata map[string]any) {
	piSessionID, _ := metadata["pi_session_id"].(string)
	piSessionDir, _ := metadata["pi_session_dir"].(string)
	piTranscriptPath, _ := metadata["pi_transcript_path"].(string)
	piCodingAgentDir, _ := metadata["pi_coding_agent_dir"].(string)
	piCodingAgentDirSet, _ := metadata["pi_coding_agent_dir_set"].(bool)
	if piSessionID != "" || piSessionDir != "" || piTranscriptPath != "" || piCodingAgentDir != "" || piCodingAgentDirSet {
		session.Pi = &manifest.Pi{
			SessionID:         piSessionID,
			SessionDir:        piSessionDir,
			TranscriptPath:    piTranscriptPath,
			CodingAgentDir:    piCodingAgentDir,
			CodingAgentDirSet: piCodingAgentDirSet,
		}
	}
}

func applyEngramMetadata(session *manifest.Manifest, metadata map[string]any) {
	enabled, ok := metadata["engram_enabled"].(bool)
	if !ok || !enabled {
		return
	}
	session.EngramMetadata = &manifest.EngramMetadata{Enabled: enabled}
	if query, ok := metadata["engram_query"].(string); ok {
		session.EngramMetadata.Query = query
	}
	if count, ok := metadata["engram_count"].(float64); ok {
		session.EngramMetadata.Count = int(count)
	}
	if ids, ok := metadata["engram_ids"].([]any); ok {
		session.EngramMetadata.EngramIDs = make([]string, len(ids))
		for i, id := range ids {
			if idStr, ok := id.(string); ok {
				session.EngramMetadata.EngramIDs[i] = idStr
			}
		}
	}
	if loadedAtStr, ok := metadata["engram_loaded_at"].(string); ok {
		if loadedAt, err := time.Parse(time.RFC3339, loadedAtStr); err == nil {
			session.EngramMetadata.LoadedAt = loadedAt
		}
	}
}

func unmarshalSandboxMetadata(session *manifest.Manifest, metadata map[string]any) error {
	sandboxRaw, ok := metadata["sandbox"]
	if !ok {
		return nil
	}
	sandboxJSON, err := json.Marshal(sandboxRaw)
	if err != nil {
		return fmt.Errorf("failed to marshal sandbox metadata: %w", err)
	}
	var sandbox manifest.SandboxConfig
	if err := json.Unmarshal(sandboxJSON, &sandbox); err != nil {
		return fmt.Errorf("failed to unmarshal sandbox metadata: %w", err)
	}
	if manifest.ValidateSandboxOwnership(session.SessionID, &sandbox) != nil {
		// Partial, legacy, manually repaired, or corrupt metadata is not proof
		// of ownership. Ignore it so archive can proceed without authorizing
		// any inferred sandbox cleanup.
		return nil //nolint:nilerr // invalid ownership is deliberately ignored so cleanup fails closed
	}
	session.Sandbox = &sandbox
	return nil
}

func unmarshalOpenAISessionMetadata(session *manifest.Manifest, metadata map[string]any) error {
	openAIRaw, ok := metadata["openai"]
	if !ok {
		return nil
	}
	openAIJSON, err := json.Marshal(openAIRaw)
	if err != nil {
		return fmt.Errorf("failed to marshal OpenAI session metadata: %w", err)
	}
	var openAI manifest.OpenAI
	if err := json.Unmarshal(openAIJSON, &openAI); err != nil {
		return fmt.Errorf("failed to unmarshal OpenAI session metadata: %w", err)
	}
	session.OpenAI = &openAI
	return nil
}

// --- manifest.Store implementation (delegates to legacy methods) ---

// Create implements manifest.Store.
func (a *Adapter) Create(m *manifest.Manifest) error {
	return a.CreateSession(m)
}

// Get implements manifest.Store.
func (a *Adapter) Get(sessionID string) (*manifest.Manifest, error) {
	return a.GetSession(sessionID)
}

// Update implements manifest.Store.
func (a *Adapter) Update(m *manifest.Manifest) error {
	return a.UpdateSession(m)
}

// Delete implements manifest.Store.
func (a *Adapter) Delete(sessionID string) error {
	return a.DeleteSession(sessionID)
}

// List implements manifest.Store by converting manifest.Filter to SessionFilter.
func (a *Adapter) List(filter *manifest.Filter) ([]*manifest.Manifest, error) {
	if filter == nil {
		return a.ListSessions(nil)
	}
	sf := &SessionFilter{
		Workspace: filter.Workspace,
		Harness:   filter.Harness,
		Tags:      filter.Tags,
		Limit:     filter.Limit,
		Offset:    filter.Offset,
	}
	// Map Status to Lifecycle filter
	switch filter.Status {
	case "archived":
		sf.Lifecycle = manifest.LifecycleArchived
	case "active":
		sf.ExcludeArchived = true
	}
	return a.ListSessions(sf)
}

// RecordHarnessSwitch appends a harness-switch event to agm_harness_history.
// Call this whenever a session's harness changes to a different value.
func (a *Adapter) RecordHarnessSwitch(sessionID, fromHarness, toHarness string, switchedAt time.Time) error {
	if sessionID == "" {
		return fmt.Errorf("session_id cannot be empty")
	}
	if err := a.ApplyMigrations(); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}
	_, err := a.conn.Exec( //nolint:noctx // TODO(context): plumb ctx
		`INSERT INTO agm_harness_history (session_id, switched_at, from_harness, to_harness) VALUES (?, ?, ?, ?)`,
		sessionID, switchedAt, fromHarness, toHarness,
	)
	return err
}

// GetHarnessHistory returns all harness-switch events for a session in chronological order.
func (a *Adapter) GetHarnessHistory(sessionID string) ([]manifest.HarnessSwitch, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id cannot be empty")
	}
	if err := a.ApplyMigrations(); err != nil {
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}
	rows, err := a.conn.Query( //nolint:noctx // TODO(context): plumb ctx
		`SELECT switched_at, from_harness, to_harness FROM agm_harness_history WHERE session_id = ? ORDER BY switched_at ASC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []manifest.HarnessSwitch
	for rows.Next() {
		var sw manifest.HarnessSwitch
		if err := rows.Scan(&sw.Timestamp, &sw.FromHarness, &sw.ToHarness); err != nil {
			return nil, err
		}
		history = append(history, sw)
	}
	return history, rows.Err()
}
