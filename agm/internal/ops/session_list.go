package ops

import (
	"sort"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// ListSessionsRequest defines the input for listing sessions.
type ListSessionsRequest struct {
	// Status filters by lifecycle: "active" (default), "archived", "all".
	Status string `json:"status,omitempty"`

	// Harness filters by agent type: "claude-code", "gemini-cli", etc.
	Harness string `json:"harness,omitempty"`

	// Tags filters by context tags (all must match).
	Tags []string `json:"tags,omitempty"`

	// ExcludeStopped hides sessions without an active tmux session from
	// results. When true, only sessions with running tmux are shown.
	ExcludeStopped bool `json:"exclude_stopped,omitempty"`

	// Limit caps the number of results (default: 100, max: 1000).
	Limit int `json:"limit,omitempty"`

	// Offset for pagination.
	Offset int `json:"offset,omitempty"`

	// StableOrder uses an immutable creation key so offset pagination is not
	// perturbed when an existing session's updated_at changes.
	StableOrder bool `json:"stable_order,omitempty"`
}

// SessionSummary is the per-session output for list operations.
// Designed for token efficiency — only includes fields agents typically need.
//
// NOTE: State field was removed — it produced false positives (PERMISSION_PROMPT,
// DONE, OFFLINE) that caused cascading bad decisions. State detection will be
// reimplemented with capture-pane-based ground truth. See cross_check.go.
type SessionSummary struct {
	// ID is omitempty so agent-mode can drop the raw UUID by default (name is
	// the stable handle). In human/JSON mode ID is always a non-empty UUID, so
	// omitempty never changes that output.
	ID            string   `json:"id,omitempty"`
	Name          string   `json:"name"`
	Status        string   `json:"status"`
	Outcome       string   `json:"outcome,omitempty"` // How the session ended (set for archived sessions)
	Attached      bool     `json:"attached"`
	Harness       string   `json:"harness"`
	Workspace     string   `json:"workspace,omitempty"`
	Project       string   `json:"project"`
	Tags          []string `json:"tags,omitempty"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
	EstimatedCost float64  `json:"estimated_cost,omitempty"`
}

// ListSessionsResult is the output of ListSessions.
type ListSessionsResult struct {
	Operation          string           `json:"operation"`
	Sessions           []SessionSummary `json:"sessions"`
	Total              int              `json:"total"`
	Limit              int              `json:"limit"`
	Offset             int              `json:"offset"`
	OrphanTmuxSessions []string         `json:"orphan_tmux_sessions,omitempty"`
}

// ListSessions returns sessions matching the given filters.
func ListSessions(ctx *OpContext, req *ListSessionsRequest) (*ListSessionsResult, error) {
	if req == nil {
		req = &ListSessionsRequest{}
	}

	// Apply defaults
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		return nil, ErrInvalidInput("limit", "Limit must be between 1 and 1000.")
	}

	// Build storage filter
	filter := &dolt.SessionFilter{
		Harness:     req.Harness,
		Tags:        req.Tags,
		Limit:       limit,
		Offset:      req.Offset,
		StableOrder: req.StableOrder,
	}

	switch req.Status {
	case "", "active":
		filter.ExcludeArchived = true
	case "archived":
		filter.Lifecycle = "archived"
	case "all":
		// No filter
	default:
		return nil, ErrInvalidInput("status", "Status must be one of: active, archived, all.")
	}

	manifests, err := ctx.Storage.ListSessions(filter)
	if err != nil {
		return nil, ErrStorageError("list_sessions", err)
	}

	// Fetch tmux session info once (single tmux call) and reuse for
	// both status computation and orphan detection.
	var tmuxSessions []session.SessionInfo
	tmuxObserved := false
	if ctx.Tmux != nil {
		// Prefer the strict lister. The plain ListSessionsWithInfo maps every
		// tmux ExitError to an empty list with a nil error, so a missing or
		// permission-denied socket is indistinguishable from "no sessions
		// running" — which would set tmuxObserved and report every live
		// manifest as stopped, recreating the false-dead result this guard
		// exists to prevent (ce-0zng9).
		if strict, ok := ctx.Tmux.(tmuxStrictInfoProvider); ok {
			if infos, err := strict.ListSessionsWithInfoStrict(); err == nil {
				tmuxSessions = infos
				tmuxObserved = true
			}
		} else if ti, ok := ctx.Tmux.(tmuxInfoProvider); ok {
			if infos, err := ti.ListSessionsWithInfo(); err == nil {
				tmuxSessions = infos
				tmuxObserved = true
			}
		}
	}

	// Compute statuses and attachment from cached tmux data
	statuses, attached := computeStatusesFromInfo(manifests, tmuxSessions)
	if !tmuxObserved {
		// No tmux observation happened, so "not in the tmux set" proves
		// nothing. Reporting "stopped" here made every live session look dead
		// to tmux-less consumers such as the MCP read tools (ce-0zng9).
		for name := range statuses {
			statuses[name] = "unknown"
		}
	}
	// Session existence alone is a false-green liveness signal (ce-axsr):
	// demote "active" to "zombie" where the harness process is provably dead.
	refineActiveStatusesWithLiveness(manifests, statuses, ctx.Tmux)

	// Transform to summaries
	sessions := make([]SessionSummary, 0, len(manifests))
	for _, m := range manifests {
		s := toSessionSummary(m, statuses, attached)
		if req.ExcludeStopped && s.Status == "stopped" {
			continue
		}
		sessions = append(sessions, s)
	}

	// Detect orphan tmux sessions (tmux sessions with no AGM counterpart)
	orphans := findOrphanTmuxSessions(manifests, tmuxSessions)

	return &ListSessionsResult{
		Operation:          "list_sessions",
		Sessions:           sessions,
		Total:              len(sessions),
		Limit:              limit,
		Offset:             req.Offset,
		OrphanTmuxSessions: orphans,
	}, nil
}

func toSessionSummary(m *manifest.Manifest, statuses map[string]string, attached map[string]bool) SessionSummary {
	status := "active"
	if m.Lifecycle == manifest.LifecycleArchived {
		status = "archived"
	} else if m.Lifecycle == manifest.LifecycleReaping {
		status = "stopped" // reaping sessions display as stopped
	} else if s, ok := statuses[m.Name]; ok {
		status = s
	}

	estCost := m.LastKnownCost
	if estCost == 0 && m.CostTracking != nil {
		inCost := float64(m.CostTracking.TokensIn) / 1_000_000 * 15.0   // Opus input
		outCost := float64(m.CostTracking.TokensOut) / 1_000_000 * 75.0 // Opus output
		estCost = inCost + outCost
	}

	return SessionSummary{
		ID:            m.SessionID,
		Name:          m.Name,
		Status:        status,
		Outcome:       string(m.Outcome),
		Attached:      attached[m.Name],
		Harness:       m.Harness,
		Workspace:     m.Workspace,
		Project:       m.Context.Project,
		Tags:          m.Context.Tags,
		CreatedAt:     m.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:     m.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		EstimatedCost: estCost,
	}
}

// tmuxStrictInfoProvider is the optional capability that reports a tmux
// observation failure as an error instead of an empty session list.
type tmuxStrictInfoProvider interface {
	ListSessionsWithInfoStrict() ([]session.SessionInfo, error)
}

// tmuxInfoProvider is the interface needed for attachment-aware status computation.
type tmuxInfoProvider interface {
	ListSessions() ([]string, error)
	ListSessionsWithInfo() ([]session.SessionInfo, error)
}

// computeStatuses wraps computeStatusesWithAttachment for callers that only need status.
func computeStatuses(manifests []*manifest.Manifest, tmux interface{}) map[string]string {
	statuses, _ := computeStatusesWithAttachment(manifests, tmux)
	return statuses
}

// refineActiveStatusesWithLiveness demotes "active" entries to "zombie" when
// the tmux backend can prove no harness process is running in the session's
// pane tree (ce-axsr). `tmux has-session` alone is false-green: the session
// keeps existing after the harness exits and the pane falls back to a bare
// shell. Only sessions currently marked "active" are scanned. The batch
// capability is preferred — it costs a constant number of subprocesses for
// the whole list (one `tmux list-panes -a`, one `ps`) instead of 3 per
// session. Best-effort: backends without either liveness capability, and
// scan errors, leave the status as "active" (a failed scan proves nothing).
func refineActiveStatusesWithLiveness(manifests []*manifest.Manifest, statuses map[string]string, tmux any) {
	// Collect the tmux names of "active" sessions, keyed back to manifest name.
	manifestByTmuxName := make(map[string]string)
	var tmuxNames []string
	for _, m := range manifests {
		if statuses[m.Name] != "active" {
			continue
		}
		tmuxName := session.TmuxSessionName(m)
		manifestByTmuxName[tmuxName] = m.Name
		tmuxNames = append(tmuxNames, tmuxName)
	}
	if len(tmuxNames) == 0 {
		return
	}

	if batcher, ok := tmux.(session.HarnessLivenessBatchChecker); ok {
		infos, err := batcher.HarnessLivenessBatch(tmuxNames)
		if err != nil {
			return // a failed scan proves nothing
		}
		for tmuxName, info := range infos {
			if info.SessionExists && !info.HarnessAlive {
				statuses[manifestByTmuxName[tmuxName]] = "zombie"
			}
		}
		return
	}

	checker, ok := tmux.(session.HarnessLivenessChecker)
	if !ok {
		return
	}
	for _, tmuxName := range tmuxNames {
		info, err := checker.HarnessLiveness(tmuxName)
		if err == nil && info.SessionExists && !info.HarnessAlive {
			statuses[manifestByTmuxName[tmuxName]] = "zombie"
		}
	}
}

// unobservedStatuses reports every session as "unknown" — the answer for a tmux
// backend that could not be observed at all. "Not in the observed set" only
// means "stopped" when there was a successful observation to be absent from;
// without one, "stopped" is a false-dead verdict on live workers (ce-0zng9).
func unobservedStatuses(manifests []*manifest.Manifest) map[string]string {
	statuses := make(map[string]string, len(manifests))
	for _, m := range manifests {
		statuses[m.Name] = "unknown"
	}
	return statuses
}

// computeStatusesWithAttachment computes both status and attachment info in a
// single tmux call.
//
// It prefers the strict lister for the same reason ListSessions does: the plain
// ListSessionsWithInfo maps every tmux ExitError to an empty list with a nil
// error, so an unreachable or permission-denied socket is indistinguishable
// from "no sessions running". LastSession, SessionHealth and the batch worker
// view all read through this helper, so leaving the failure-collapsing lister
// here kept reporting live sessions as stopped on those surfaces even after the
// list path was fixed (ce-0zng9).
func computeStatusesWithAttachment(manifests []*manifest.Manifest, tmux any) (map[string]string, map[string]bool) {
	attached := make(map[string]bool, len(manifests))

	// Preferred: strict listing, which reports an observation failure as an
	// error instead of an empty successful observation.
	if strict, ok := tmux.(tmuxStrictInfoProvider); ok {
		sessions, err := strict.ListSessionsWithInfoStrict()
		if err != nil {
			return unobservedStatuses(manifests), attached
		}
		return statusesFromObservation(manifests, sessions, tmux)
	}

	// Backends without the strict capability (test fakes): their answer is used
	// as-is, the same accommodation ListSessions makes.
	if ti, ok := tmux.(tmuxInfoProvider); ok {
		sessions, err := ti.ListSessionsWithInfo()
		if err != nil {
			return unobservedStatuses(manifests), attached
		}
		return statusesFromObservation(manifests, sessions, tmux)
	}

	// Fallback: basic interface without attachment info
	ti, ok := tmux.(interface {
		ListSessions() ([]string, error)
	})
	if !ok {
		return unobservedStatuses(manifests), attached
	}

	tmuxSessions, err := ti.ListSessions()
	if err != nil {
		return unobservedStatuses(manifests), attached
	}

	statuses := make(map[string]string, len(manifests))
	tmuxSet := make(map[string]bool, len(tmuxSessions))
	for _, s := range tmuxSessions {
		tmuxSet[s] = true
	}

	for _, m := range manifests {
		tmuxName := session.TmuxSessionName(m)
		if tmuxSet[tmuxName] {
			statuses[m.Name] = "active"
		} else {
			statuses[m.Name] = "stopped"
		}
	}
	return statuses, attached
}

// statusesFromObservation folds a successful tmux observation into statuses and
// attachment, then applies the liveness refinement that demotes false-green
// "active" to "zombie".
func statusesFromObservation(manifests []*manifest.Manifest, sessions []session.SessionInfo, tmux any) (map[string]string, map[string]bool) {
	statuses, attached := computeStatusesFromInfo(manifests, sessions)
	refineActiveStatusesWithLiveness(manifests, statuses, tmux)
	return statuses, attached
}

// computeStatusesFromInfo computes status and attachment from pre-fetched tmux session info.
func computeStatusesFromInfo(manifests []*manifest.Manifest, tmuxSessions []session.SessionInfo) (map[string]string, map[string]bool) {
	statuses := make(map[string]string, len(manifests))
	attached := make(map[string]bool, len(manifests))

	sessionMap := make(map[string]session.SessionInfo, len(tmuxSessions))
	for _, s := range tmuxSessions {
		sessionMap[s.Name] = s
	}

	for _, m := range manifests {
		tmuxName := session.TmuxSessionName(m)
		if info, exists := sessionMap[tmuxName]; exists {
			statuses[m.Name] = "active"
			attached[m.Name] = info.AttachedClients > 0
		} else {
			statuses[m.Name] = "stopped"
		}
	}
	return statuses, attached
}

// findOrphanTmuxSessions returns tmux session names that have no corresponding
// AGM session. These are tmux sessions on the AGM socket that were never
// registered or whose AGM records were deleted.
func findOrphanTmuxSessions(manifests []*manifest.Manifest, tmuxSessions []session.SessionInfo) []string {
	if len(tmuxSessions) == 0 {
		return nil
	}

	// Build set of known AGM tmux session names
	agmNames := make(map[string]bool, len(manifests)*2)
	for _, m := range manifests {
		tmuxName := session.TmuxSessionName(m)
		agmNames[tmuxName] = true
		agmNames[m.Name] = true
	}

	// Find tmux sessions not in AGM
	var orphans []string
	for _, s := range tmuxSessions {
		if !agmNames[s.Name] {
			orphans = append(orphans, s.Name)
		}
	}

	sort.Strings(orphans)
	return orphans
}
