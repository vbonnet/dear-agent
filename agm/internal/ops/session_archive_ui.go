package ops

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/claudeui"
)

// ArchiveUISessions reconciles the local Claude desktop / claude.ai/code
// session store by flipping the `isArchived` flag, per ADR-026.
//
// It is a distinct verb from `agm session archive` (which archives AGM's own
// Dolt session manifests). It never deletes anything, never touches
// `~/.claude/projects/*.jsonl` transcripts, and never calls a network API. It
// is dry-run by default; mutation requires Request.Apply.
//
// Lives in the shared ops layer (ADR-016) so CLI, a future MCP tool, and a
// dear-agent skill/cron all share one tested implementation.

// ArchiveUISessionsRequest is the input to ArchiveUISessions.
type ArchiveUISessionsRequest struct {
	OlderThan time.Duration // archive sessions idle longer than this (Status=="idle")
	Status    string        // "idle" (default) | "all"
	Unarchive bool          // reverse: flip isArchived true -> false
	Apply     bool          // perform the flip; false (default) = dry-run
	Backup    bool          // back up each file before mutating (default true at CLI)
	Force     bool          // archive even when safety warnings are present
	BackupDir string        // override backup root; default ~/.agm/backups/claude-ui-sessions/<ts>
	Device    string        // optional device-dir selector ("" = autodetect single)
	Account   string        // optional account-dir selector ("" = autodetect single)

	// Injection points for tests; zero values resolve to real machine defaults.
	Now            time.Time
	HomeDir        string
	StoreRoot      string
	PIDRegistryDir string
	ProjectsDir    string // ~/.claude/projects override (transcript heuristic)

	// Safety probes; nil = real git/gh/transcript implementations.
	inspector  wtInspector
	transcript transcriptScanner
}

// UISessionOutcome is the per-session result of a plan/apply pass.
type UISessionOutcome struct {
	SessionID    string  `json:"session_id"`
	CliSessionID string  `json:"cli_session_id,omitempty"`
	Title        string  `json:"title,omitempty"`
	Cwd          string  `json:"cwd,omitempty"`
	AgeHours     float64 `json:"age_hours"`
	Live         bool    `json:"live"`
	IsArchived   bool    `json:"is_archived"`
	Action       string  `json:"action"` // would-archive|archived|would-unarchive|unarchived|skip|error
	Reason       string  `json:"reason,omitempty"`
	BackupPath   string  `json:"backup_path,omitempty"`
	Error        string  `json:"error,omitempty"`

	// Warnings is non-empty when the session is archive-eligible but Claude
	// Desktop would caution against it (uncommitted work, open PR, looks like
	// it is waiting on the user). Present even when --force archived anyway.
	Warnings []SafetyWarning `json:"warnings,omitempty"`
}

// ArchiveUISessionsResult is the aggregate result.
type ArchiveUISessionsResult struct {
	Operation string             `json:"operation"`
	Success   bool               `json:"success"`
	DryRun    bool               `json:"dry_run"`
	Direction string             `json:"direction"` // archive | unarchive
	Store     string             `json:"store"`
	BackupDir string             `json:"backup_dir,omitempty"`
	Scanned   int                `json:"scanned"`
	Changed   int                `json:"changed"`
	Skipped   int                `json:"skipped"`
	Warned    int                `json:"warned"` // sessions with >=1 safety warning
	Errors    int                `json:"errors"`
	Sessions  []UISessionOutcome `json:"sessions"`
}

// Skip reasons (stable strings for programmatic consumers).
const (
	uiSkipAlreadyArchived   = "already-archived"
	uiSkipAlreadyUnarchived = "already-unarchived"
	uiSkipLive              = "live"
	uiSkipTooRecent         = "too-recent"
	uiSkipUnknownSchema     = "unknown-schema"
	uiSkipSafetyWarnings    = "safety-warnings"
	uiActionError           = "write-failed"
)

// safetyEnv bundles the git/gh/transcript probes, defaulted to real
// implementations when the request leaves them nil.
type safetyEnv struct {
	inspector  wtInspector
	transcript transcriptScanner
}

// runSafetyGate returns the session's safety warnings and whether it must be
// skipped for them. Only the archive direction is gated; --force downgrades a
// skip to "archive anyway, warnings retained".
func runSafetyGate(s *claudeui.Session, c uiConfig, force bool, env safetyEnv) (warns []SafetyWarning, skip bool) {
	if !c.target {
		return nil, false // unarchiving cannot bury in-progress work
	}
	warns = evalSafetyWarnings(s, env.inspector, env.transcript)
	if len(warns) == 0 {
		return nil, false
	}
	return warns, !force
}

// newSafetyEnv resolves the safety probes, defaulting to the real git/gh and
// ~/.claude/projects transcript implementations when the request omits them.
func newSafetyEnv(req *ArchiveUISessionsRequest, c uiConfig) safetyEnv {
	env := safetyEnv{inspector: req.inspector, transcript: req.transcript}
	if env.inspector == nil {
		env.inspector = realInspector{}
	}
	if env.transcript == nil {
		projectsDir := req.ProjectsDir
		if projectsDir == "" {
			projectsDir = filepath.Join(c.home, ".claude", "projects")
		}
		env.transcript = realTranscriptScanner(projectsDir)
	}
	return env
}

// uiConfig is the validated, defaults-resolved view of a request.
type uiConfig struct {
	now       time.Time
	home      string
	storeRoot string
	pidDir    string
	status    string
	target    bool          // desired isArchived value
	direction string        // "archive" | "unarchive"
	olderThan time.Duration // idle threshold for --status idle
	dryRun    bool
}

// resolveUIConfig fills defaults and validates the request. Extracted from
// ArchiveUISessions to keep that function's complexity manageable.
func resolveUIConfig(opCtx *OpContext, req *ArchiveUISessionsRequest) (uiConfig, *OpError) {
	c := uiConfig{
		now:       req.Now,
		home:      req.HomeDir,
		storeRoot: req.StoreRoot,
		pidDir:    req.PIDRegistryDir,
		status:    req.Status,
		target:    !req.Unarchive,
		direction: "archive",
		olderThan: req.OlderThan,
		dryRun:    !req.Apply || opCtx.DryRun,
	}
	if c.now.IsZero() {
		c.now = time.Now()
	}
	if c.home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return c, ErrInvalidInput("home", fmt.Sprintf("cannot resolve home dir: %v", err))
		}
		c.home = h
	}
	if c.storeRoot == "" {
		c.storeRoot = claudeui.DefaultStoreRoot(c.home)
	}
	if c.pidDir == "" {
		c.pidDir = filepath.Join(c.home, ".claude", "sessions")
	}
	if c.status == "" {
		c.status = "idle"
	}
	if c.status != "idle" && c.status != "all" {
		return c, ErrInvalidInput("status", fmt.Sprintf("status must be 'idle' or 'all', got %q", c.status))
	}
	if req.Unarchive {
		c.direction = "unarchive"
	}
	return c, nil
}

// uiSkipReason returns whether a session should be skipped (and why) before any
// mutation. Encapsulating the AND-of-three idle test plus the idempotency and
// live-safety gates keeps the decision in one auditable place.
func uiSkipReason(archived, live bool, c uiConfig, ageMs int64) (skip bool, reason string) {
	// Idempotent no-op: already at the desired state.
	if archived == c.target {
		if c.target {
			return true, uiSkipAlreadyArchived
		}
		return true, uiSkipAlreadyUnarchived
	}
	// Safety gate: never touch a session with a live process, regardless of
	// --status (OPS-67).
	if live {
		return true, uiSkipLive
	}
	// Age gate applies only to the conservative default (--status idle).
	if c.status == "idle" && time.Duration(ageMs)*time.Millisecond <= c.olderThan {
		return true, uiSkipTooRecent
	}
	return false, ""
}

// ArchiveUISessions plans (and, with Apply, performs) the isArchived flip.
func ArchiveUISessions(opCtx *OpContext, req *ArchiveUISessionsRequest) (*ArchiveUISessionsResult, error) {
	c, oerr := resolveUIConfig(opCtx, req)
	if oerr != nil {
		return nil, oerr
	}

	dir, deviceID, accountID, err := claudeui.StoreDir(c.storeRoot, req.Device, req.Account)
	if err != nil {
		return nil, storeDiscoveryError(err, c.storeRoot)
	}

	sessions, loadErrs, err := claudeui.ListSessions(dir, deviceID, accountID)
	if err != nil {
		return nil, ErrInvalidInput("store", fmt.Sprintf("cannot read store dir %s: %v", dir, err))
	}

	liveIDs := readPIDRegistry(c.pidDir)
	env := newSafetyEnv(req, c)

	result := &ArchiveUISessionsResult{
		Operation: "session.archive-ui",
		DryRun:    c.dryRun,
		Direction: c.direction,
		Store:     dir,
		Scanned:   len(sessions) + len(loadErrs),
	}

	// Schema-refused files are reported and skipped, never rewritten (ADR-026).
	for _, le := range loadErrs {
		result.Sessions = append(result.Sessions, UISessionOutcome{
			SessionID: filepath.Base(le.Path),
			Action:    "skip",
			Reason:    uiSkipUnknownSchema,
			Error:     le.Err.Error(),
		})
		result.Skipped++
	}

	backupDir := resolveBackupDir(req, c, result)

	for _, s := range sessions {
		oc := newOutcome(s, c.now, isLive(s, liveIDs))

		if skip, reason := uiSkipReason(s.IsArchived, oc.Live, c, ageMsOf(s, c.now)); skip {
			oc.Action, oc.Reason = "skip", reason
			result.Skipped++
			result.Sessions = append(result.Sessions, oc)
			continue
		}

		// Safety-warning gate: surface (dry-run) or block-unless-force (apply)
		// sessions that still have work, an open PR, or look like they are
		// waiting on the user.
		warns, warnSkip := runSafetyGate(s, c, req.Force, env)
		oc.Warnings = warns
		if len(warns) > 0 {
			result.Warned++
		}
		if warnSkip {
			oc.Action, oc.Reason = "skip", uiSkipSafetyWarnings
			result.Skipped++
			result.Sessions = append(result.Sessions, oc)
			continue
		}

		if c.dryRun {
			oc.Action = "would-" + c.direction
			result.Changed++
			result.Sessions = append(result.Sessions, oc)
			continue
		}

		applyFlip(s, c, req.Backup, backupDir, &oc, result)
		result.Sessions = append(result.Sessions, oc)
	}

	// Stable, readable order: most-recently-active first.
	sort.SliceStable(result.Sessions, func(i, j int) bool {
		a, b := result.Sessions[i], result.Sessions[j]
		if a.AgeHours != b.AgeHours {
			return a.AgeHours < b.AgeHours
		}
		return a.SessionID < b.SessionID
	})

	result.Success = result.Errors == 0
	return result, nil
}

func ageMsOf(s *claudeui.Session, now time.Time) int64 {
	ageMs := now.UnixMilli() - s.LastActivityAt
	if ageMs < 0 {
		return 0 // clock skew / future timestamp: treat as just-active
	}
	return ageMs
}

func newOutcome(s *claudeui.Session, now time.Time, live bool) UISessionOutcome {
	return UISessionOutcome{
		SessionID:    s.SessionID,
		CliSessionID: s.CliSessionID,
		Title:        s.Title,
		Cwd:          s.Cwd,
		IsArchived:   s.IsArchived,
		Live:         live,
		AgeHours:     float64(ageMsOf(s, now)) / float64(time.Hour/time.Millisecond),
	}
}

// resolveBackupDir computes (and records) the per-invocation backup directory,
// only when a mutating run with backups is requested.
func resolveBackupDir(req *ArchiveUISessionsRequest, c uiConfig, result *ArchiveUISessionsResult) string {
	if !req.Apply || !req.Backup {
		return ""
	}
	dir := req.BackupDir
	if dir == "" {
		ts := c.now.UTC().Format("20060102T150405Z")
		dir = filepath.Join(c.home, ".agm", "backups", "claude-ui-sessions", ts)
	}
	result.BackupDir = dir
	return dir
}

// applyFlip performs the mutation and records the outcome/counters.
func applyFlip(s *claudeui.Session, c uiConfig, backup bool, backupDir string,
	oc *UISessionOutcome, result *ArchiveUISessionsResult) {
	changed, bp, werr := s.SetArchived(c.target, backup, backupDir)
	switch {
	case werr != nil:
		oc.Action = "error"
		oc.Reason = uiActionError
		oc.Error = werr.Error()
		result.Errors++
	case !changed:
		// Defensive: state matched after all.
		oc.Action = "skip"
		oc.Reason = uiSkipAlreadyArchived
		result.Skipped++
	default:
		oc.Action = c.direction + "d" // "archived" / "unarchived"
		oc.IsArchived = c.target
		oc.BackupPath = bp
		result.Changed++
	}
}

// isLive reports whether a stored session is owned by a running process.
//
// Liveness is keyed on session IDENTITY only — the session's cliSessionId (the
// join key to the live registry's `sessionId`) or its own sessionId. It is
// deliberately NOT keyed on cwd.
//
// cwd is not a per-session signal: many sessions share one working directory
// (every CLI run rooted at a monorepo like ~/src/dear-agent, or at $HOME), so a
// single live process there would mark every past session in that directory as
// "live" forever. On this machine that false-positive buried ~248 long-idle
// sessions (only 13 were truly live by id), which is the dominant reason the
// desktop session list never drains. Idleness is still protected by the age
// gate (--older-than) and the safety-warning gate (uncommitted/unmerged work,
// open PR, awaiting-input) downstream, so dropping the cwd match does not
// archive a session that still has live work — it only stops mislabelling dead
// sessions as live. See OPS-67 and OPS-68.
func isLive(s *claudeui.Session, liveIDs map[string]bool) bool {
	if s.CliSessionID != "" && liveIDs[s.CliSessionID] {
		return true
	}
	if s.SessionID != "" && liveIDs[s.SessionID] {
		return true
	}
	return false
}

// readPIDRegistry reads ~/.claude/sessions/<pid>.json. Each present file means a
// running process owns that CLI session; we collect the owning sessionIds. cwd
// is intentionally not collected — see isLive for why cwd is an unsound
// liveness signal. Best-effort: an unreadable entry just drops one liveness
// signal rather than aborting.
func readPIDRegistry(dir string) (ids map[string]bool) {
	ids = map[string]bool{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ids
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var rec struct {
			SessionID string `json:"sessionId"`
		}
		if json.Unmarshal(data, &rec) != nil {
			continue
		}
		if rec.SessionID != "" {
			ids[rec.SessionID] = true
		}
	}
	return ids
}

// storeDiscoveryError maps a claudeui discovery failure to an actionable OpError.
func storeDiscoveryError(err error, root string) *OpError {
	return &OpError{
		Status: 400,
		Type:   "input/invalid",
		Code:   ErrCodeInvalidInput,
		Title:  "Claude session store not usable",
		Detail: err.Error(),
		Suggestions: []string{
			fmt.Sprintf("Verify the store exists under %s", root),
			"If multiple devices/accounts exist, pass --device and/or --account.",
			"This command only operates on the local desktop session store; it never deletes anything.",
		},
		Parameters: map[string]string{"store_root": root},
	}
}
