package ops

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/contracts"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/gclog"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// DefaultProtectedRoles returns session name substrings that are always
// excluded from GC unless explicitly overridden.
func DefaultProtectedRoles() []string {
	return contracts.Load().SessionLifecycle.GCProtectedRoles
}

// activeStates lists manifest states that indicate a session is actively
// doing work and must not be garbage-collected.
var activeStates = map[string]bool{
	manifest.StateWorking:          true,
	manifest.StatePermissionPrompt: true,
	manifest.StateCompacting:       true,
	manifest.StateWaitingAgent:     true,
	manifest.StateLooping:          true,
	manifest.StateBackgroundTasks:  true,
	manifest.StateUserPrompt:       true,
	manifest.StateReady:            true,
}

// GCRequest defines the input for session garbage collection.
type GCRequest struct {
	// OlderThan filters to sessions inactive for at least this duration.
	// Zero means no age filter.
	OlderThan time.Duration `json:"older_than,omitempty"`

	// ProtectRoles is a list of role substrings to protect from GC.
	// If nil, DefaultProtectedRoles is used.
	// Set to an empty slice to disable role protection.
	ProtectRoles []string `json:"protect_roles,omitempty"`

	// Force skips pre-archive verification checks on each session.
	Force bool `json:"force,omitempty"`
}

// GCSkipReason describes why a session was skipped during GC.
type GCSkipReason string

// GCSkipReason values explaining why a session was skipped during GC.
const (
	GCSkipAlreadyArchived GCSkipReason = "already_archived"
	GCSkipReaping         GCSkipReason = "lifecycle_reaping"
	GCSkipActiveTmux      GCSkipReason = "active_tmux_session"
	GCSkipActiveState     GCSkipReason = "active_state"
	GCSkipProtectedRole   GCSkipReason = "protected_role"
	GCSkipTooRecent       GCSkipReason = "too_recent"
)

// GCSessionEntry describes the outcome for a single session in a GC pass.
type GCSessionEntry struct {
	Name      string       `json:"name"`
	SessionID string       `json:"session_id"`
	Action    string       `json:"action"` // "archived", "skipped", "error"
	Reason    GCSkipReason `json:"reason,omitempty"`
	Error     string       `json:"error,omitempty"`
}

// GCResult is the output of GC.
type GCResult struct {
	Operation string            `json:"operation"`
	DryRun    bool              `json:"dry_run,omitempty"`
	Scanned   int               `json:"scanned"`
	Archived  int               `json:"archived"`
	Skipped   int               `json:"skipped"`
	Errors    int               `json:"errors"`
	Sessions  []GCSessionEntry  `json:"sessions,omitempty"`
}

// GC performs safe garbage collection of sessions.
//
// Safety guarantees (from postmortem P0 requirements):
//  1. Pre-GC health check: aborts if storage is unreachable
//  2. Active session exclusion: skips sessions with active tmux sessions
//  3. Active state exclusion: skips sessions in WORKING/PERMISSION_PROMPT/etc
//  4. Supervisor role exclusion: skips sessions matching protected role names
//  5. All actions logged to gc.jsonl
func GC(ctx *OpContext, req *GCRequest) (*GCResult, error) {
	if req == nil {
		req = &GCRequest{}
	}

	protectRoles := req.ProtectRoles
	if protectRoles == nil {
		protectRoles = DefaultProtectedRoles()
	}

	// P0 requirement: pre-GC health check — abort if storage unreachable.
	allSessions, err := ctx.Storage.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		return nil, &OpError{
			Status: 503,
			Type:   "gc/health_check_failed",
			Code:   ErrCodeStorageError,
			Title:  "Pre-GC health check failed",
			Detail: fmt.Sprintf("Cannot reach session storage: %v. Aborting GC pass.", err),
			Suggestions: []string{
				"Run `agm admin doctor` to diagnose storage issues.",
				"Verify Dolt server is running.",
			},
		}
	}

	// Get active tmux sessions in a single batch call for efficiency.
	// Key by SessionID (UUID), NOT Name: two manifests can share a Name (e.g. an
	// archived shadow and a live worker), and a Name-keyed map lets the archived
	// entry overwrite the live one ("stopped" shadows "active"), making GC archive
	// a LIVE session. UUID is unique per session, so no shadowing is possible.
	activeTmuxByID := make(map[string]bool)
	if ctx.Tmux != nil {
		statuses := session.ComputeStatusBatchByID(allSessions, ctx.Tmux)
		for id, status := range statuses {
			if status == "active" {
				activeTmuxByID[id] = true
			}
		}
	}

	result := &GCResult{
		Operation: "session_gc",
		DryRun:    ctx.DryRun,
	}

	now := time.Now()

	for _, m := range allSessions {
		result.Scanned++
		processGCSession(ctx, m, req, protectRoles, activeTmuxByID, now, result)
	}

	return result, nil
}

// processGCSession evaluates a single session for GC eligibility and either
// skips, archives (or records dry-run intent), or records an error result.
func processGCSession(ctx *OpContext, m *manifest.Manifest, req *GCRequest, protectRoles []string, activeTmuxByID map[string]bool, now time.Time, result *GCResult) {
	entry := GCSessionEntry{
		Name:      m.Name,
		SessionID: m.SessionID,
	}
	if reason, ok := gcSkipReason(m, protectRoles, activeTmuxByID, req.OlderThan, now); ok {
		entry.Action = "skipped"
		entry.Reason = reason
		result.Skipped++
		result.Sessions = append(result.Sessions, entry)
		logGCSkipIfMonitored(reason, m)
		return
	}
	if ctx.DryRun {
		entry.Action = "archived"
		result.Archived++
		result.Sessions = append(result.Sessions, entry)
		logGCEntry(gclog.Entry{
			Operation:   "gc_archive",
			SessionID:   m.SessionID,
			SessionName: m.Name,
			Reason:      "eligible",
			DryRun:      true,
		})
		return
	}
	_, archiveErr := ArchiveSession(ctx, &ArchiveSessionRequest{
		Identifier: m.SessionID,
		Force:      req.Force,
		// GC archives stale/abandoned sessions — stamp the outcome so the
		// archive pile distinguishes gc-reaped records from clean completions.
		Outcome: manifest.OutcomeGCStale,
		// gcSkipReason already confirmed any protected-role record reaching
		// here is STOPPED with no live tmux pane, so the supervisor-protection
		// guard in ArchiveSession must not re-block it. checkActiveTmuxBlock
		// still independently re-verifies tmux liveness, so a truly live
		// supervisor can never be archived by this path.
		AllowSupervisorReap: true,
	})
	if archiveErr != nil {
		entry.Action = "error"
		entry.Error = archiveErr.Error()
		result.Errors++
		result.Sessions = append(result.Sessions, entry)
		slog.Warn("GC archive failed", "session", m.Name, "error", archiveErr)
		logGCEntry(gclog.Entry{
			Operation:   "gc_archive_error",
			SessionID:   m.SessionID,
			SessionName: m.Name,
			Reason:      "archive_failed",
			Error:       archiveErr.Error(),
		})
		return
	}
	entry.Action = "archived"
	result.Archived++
	result.Sessions = append(result.Sessions, entry)
	logGCEntry(gclog.Entry{
		Operation:   "gc_archive",
		SessionID:   m.SessionID,
		SessionName: m.Name,
		Reason:      "eligible",
	})
	age := now.Sub(m.CreatedAt).Round(time.Second)
	detail := fmt.Sprintf("gc collected, age: %s", age)
	if err := RecordTrustEventForSession(m.Name, TrustEventGCArchived, detail); err != nil {
		slog.Warn("Failed to record gc_archived trust event", "session", m.Name, "error", err)
	}
}

// gcSkipReason returns the (skip-reason, true) pair when m should not be
// considered for archive. Order matches the historical GC checks.
func gcSkipReason(m *manifest.Manifest, protectRoles []string, activeTmuxByID map[string]bool, olderThan time.Duration, now time.Time) (GCSkipReason, bool) {
	if m.Lifecycle == manifest.LifecycleArchived {
		return GCSkipAlreadyArchived, true
	}
	if m.Lifecycle == manifest.LifecycleReaping {
		return GCSkipReaping, true
	}
	if matchesProtectedRole(m.Name, protectRoles) || IsSupervisorSession(m.Name) {
		// Protected roles are only protected while ACTIVE (a live tmux pane
		// exists). STOPPED protected-role records with no live tmux are dead
		// duplicates/orphans and ARE gc-eligible — otherwise they accumulate
		// after crash/kill cycles and permanently block reuse of the canonical
		// name. Fall through to the normal stopped-session gc logic below.
		if activeTmuxByID[m.SessionID] {
			return GCSkipProtectedRole, true
		}
	}
	if activeTmuxByID[m.SessionID] {
		return GCSkipActiveTmux, true
	}
	if activeStates[m.State] {
		return GCSkipActiveState, true
	}
	if olderThan > 0 {
		lastActivity := m.UpdatedAt
		if !m.StateUpdatedAt.IsZero() && m.StateUpdatedAt.After(lastActivity) {
			lastActivity = m.StateUpdatedAt
		}
		if now.Sub(lastActivity) < olderThan {
			return GCSkipTooRecent, true
		}
	}
	return "", false
}

// logGCSkipIfMonitored emits a gc_skip log entry for the skip reasons that
// historically had logged entries.
func logGCSkipIfMonitored(reason GCSkipReason, m *manifest.Manifest) {
	switch reason {
	case GCSkipProtectedRole:
		logGCEntry(gclog.Entry{
			Operation:   "gc_skip",
			SessionID:   m.SessionID,
			SessionName: m.Name,
			Reason:      "protected_role",
		})
	case GCSkipActiveTmux:
		logGCEntry(gclog.Entry{
			Operation:   "gc_skip",
			SessionID:   m.SessionID,
			SessionName: m.Name,
			Reason:      "active_tmux_session",
		})
	case GCSkipActiveState:
		logGCEntry(gclog.Entry{
			Operation:   "gc_skip",
			SessionID:   m.SessionID,
			SessionName: m.Name,
			Reason:      fmt.Sprintf("active_state:%s", m.State),
		})
	case GCSkipAlreadyArchived, GCSkipReaping, GCSkipTooRecent:
		// Reasons that historically had no logged gc_skip entry.
	}
}

// SupervisorPatterns returns the list of name substrings that identify
// supervisor sessions. These sessions are protected from archive and GC
// operations unless explicitly overridden.
func SupervisorPatterns() []string {
	return []string{"orchestrator", "overseer", "meta-"}
}

// IsSupervisorSession returns true if the session name matches any
// supervisor pattern (case-insensitive substring match).
func IsSupervisorSession(name string) bool {
	return matchesProtectedRole(name, SupervisorPatterns())
}

// matchesProtectedRole returns true if the session name contains any of the
// protected role substrings (case-insensitive).
func matchesProtectedRole(name string, roles []string) bool {
	lower := strings.ToLower(name)
	for _, role := range roles {
		if strings.Contains(lower, strings.ToLower(role)) {
			return true
		}
	}
	return false
}
