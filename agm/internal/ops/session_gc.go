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
	tmuxSessions := make(map[string]bool)
	if ctx.Tmux != nil {
		statuses := session.ComputeStatusBatch(allSessions, ctx.Tmux)
		for name, status := range statuses {
			if status == "active" {
				tmuxSessions[name] = true
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
		entry := GCSessionEntry{
			Name:      m.Name,
			SessionID: m.SessionID,
		}

		// Check: already archived
		if m.Lifecycle == manifest.LifecycleArchived {
			entry.Action = "skipped"
			entry.Reason = GCSkipAlreadyArchived
			result.Skipped++
			result.Sessions = append(result.Sessions, entry)
			continue
		}

		// Check: currently being reaped
		if m.Lifecycle == manifest.LifecycleReaping {
			entry.Action = "skipped"
			entry.Reason = GCSkipReaping
			result.Skipped++
			result.Sessions = append(result.Sessions, entry)
			continue
		}

		// P0 requirement: supervisor role exclusion
		// Check both configurable protected roles AND hardcoded supervisor patterns
		if matchesProtectedRole(m.Name, protectRoles) || IsSupervisorSession(m.Name) {
			entry.Action = "skipped"
			entry.Reason = GCSkipProtectedRole
			result.Skipped++
			result.Sessions = append(result.Sessions, entry)
			logGCEntry(gclog.Entry{
				Operation:   "gc_skip",
				SessionID:   m.SessionID,
				SessionName: m.Name,
				Reason:      "protected_role",
			})
			continue
		}

		// P0 requirement: active tmux session exclusion
		if tmuxSessions[m.Name] {
			entry.Action = "skipped"
			entry.Reason = GCSkipActiveTmux
			result.Skipped++
			result.Sessions = append(result.Sessions, entry)
			logGCEntry(gclog.Entry{
				Operation:   "gc_skip",
				SessionID:   m.SessionID,
				SessionName: m.Name,
				Reason:      "active_tmux_session",
			})
			continue
		}

		// Additional safety: skip sessions in active manifest states
		if activeStates[m.State] {
			entry.Action = "skipped"
			entry.Reason = GCSkipActiveState
			result.Skipped++
			result.Sessions = append(result.Sessions, entry)
			logGCEntry(gclog.Entry{
				Operation:   "gc_skip",
				SessionID:   m.SessionID,
				SessionName: m.Name,
				Reason:      fmt.Sprintf("active_state:%s", m.State),
			})
			continue
		}

		// Apply age filter
		if req.OlderThan > 0 {
			lastActivity := m.UpdatedAt
			if !m.StateUpdatedAt.IsZero() && m.StateUpdatedAt.After(lastActivity) {
				lastActivity = m.StateUpdatedAt
			}
			if now.Sub(lastActivity) < req.OlderThan {
				entry.Action = "skipped"
				entry.Reason = GCSkipTooRecent
				result.Skipped++
				result.Sessions = append(result.Sessions, entry)
				continue
			}
		}

		// Session is eligible for GC — archive it
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
			continue
		}

		// Perform actual archive via the existing ArchiveSession path
		_, archiveErr := ArchiveSession(ctx, &ArchiveSessionRequest{
			Identifier: m.SessionID,
			Force:      req.Force,
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
			continue
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

		// Best-effort: record gc_archived trust event so auditors can see which
		// sessions were cleaned up by the GC pass vs. manually archived.
		age := now.Sub(m.CreatedAt).Round(time.Second)
		detail := fmt.Sprintf("gc collected, age: %s", age)
		if err := RecordTrustEventForSession(m.Name, TrustEventGCArchived, detail); err != nil {
			slog.Warn("Failed to record gc_archived trust event", "session", m.Name, "error", err)
		}
	}

	return result, nil
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
