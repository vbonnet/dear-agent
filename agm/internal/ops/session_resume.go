package ops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/agysession"
	"github.com/vbonnet/dear-agent/agm/internal/claude"
	"github.com/vbonnet/dear-agent/agm/internal/codexhooks"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	gitmanifest "github.com/vbonnet/dear-agent/agm/internal/git"
	"github.com/vbonnet/dear-agent/agm/internal/harnessexec"
	"github.com/vbonnet/dear-agent/agm/internal/launchparity"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/permissionparity/piadapter"
	"github.com/vbonnet/dear-agent/agm/internal/pisession"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	uuidpkg "github.com/vbonnet/dear-agent/agm/internal/uuid"
	"github.com/vbonnet/dear-agent/pkg/override"
)

const resumeReadinessTimeout = 60 * time.Second

// ResumeSessionEvent is a read-only lifecycle fact for surface presentation.
// The observer cannot authorize, replace, skip, or reorder operation phases.
type ResumeSessionEvent struct {
	Kind    string `json:"kind"`
	Message string `json:"message,omitempty"`
}

const (
	// ResumeEventHealthClassified reports that locked health facts are ready.
	ResumeEventHealthClassified = "health_classified"
	// ResumeEventTmuxExisting reports reuse of a pre-existing tmux runtime.
	ResumeEventTmuxExisting = "tmux_existing"
	// ResumeEventTmuxCreated reports an exact tmux identity owned by this attempt.
	ResumeEventTmuxCreated = "tmux_created"
	// ResumeEventHarnessReady reports native process and composer readiness.
	ResumeEventHarnessReady = "harness_ready"
	// ResumeEventPromptSubmitted reports the irreversible prompt boundary.
	ResumeEventPromptSubmitted = "prompt_submitted"
	// ResumeEventWarning reports a non-fatal lifecycle warning.
	ResumeEventWarning = "warning"
)

// ResumeSessionHealth contains the authoritative resumability facts collected
// under the stable session lock.
type ResumeSessionHealth struct {
	ManifestPath    string   `json:"manifest_path,omitempty"`
	WorktreeExists  bool     `json:"worktree_exists"`
	WorktreePath    string   `json:"worktree_path"`
	TmuxSessionName string   `json:"tmux_session_name"`
	TmuxExists      bool     `json:"tmux_exists"`
	CanResume       bool     `json:"can_resume"`
	Issues          []string `json:"issues,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
}

// ResumeSessionRequest identifies one already-resolved session. Prompt is
// already validated surface input; prompt-file IO remains outside the
// operation.
type ResumeSessionRequest struct {
	SessionID       string                   `json:"session_id"`
	ManifestPath    string                   `json:"manifest_path,omitempty"`
	Prompt          string                   `json:"prompt,omitempty"`
	CurrentAddDirs  []string                 `json:"-"`
	ExcludedAddDirs []string                 `json:"-"`
	OnEvent         func(ResumeSessionEvent) `json:"-"`
}

// ResumeSessionResult is both the lifecycle result and the post-lock
// attachment contract for interactive surfaces.
type ResumeSessionResult struct {
	Operation            string              `json:"operation"`
	SessionID            string              `json:"session_id"`
	Name                 string              `json:"name"`
	Harness              string              `json:"harness"`
	TmuxSessionName      string              `json:"tmux_session_name"`
	WorktreePath         string              `json:"worktree_path"`
	CreatedTmux          bool                `json:"created_tmux"`
	StartedHarness       bool                `json:"started_harness"`
	PromptMayHaveStarted bool                `json:"prompt_may_have_started"`
	Health               ResumeSessionHealth `json:"health"`
	Warnings             []string            `json:"warnings,omitempty"`
}

type resumeStorage interface {
	dolt.Storage
	BeginTmuxSessionNameChange(context.Context, string, string) (*dolt.TmuxSessionNameChange, error)
	RestoreTmuxSessionNameChange(context.Context, dolt.TmuxSessionNameChange) (bool, error)
	CompleteTmuxSessionNameChange(context.Context, dolt.TmuxSessionNameChange) (bool, error)
	TouchSessionActivity(context.Context, string) error
}

type createdResumeTmux struct {
	Name     string
	Identity tmux.SessionIdentity
}

func (created createdResumeTmux) owned() bool {
	return created.Name != "" && created.Identity.Cleanable()
}

type resumeTmuxNameChange struct {
	Applied bool
	Change  dolt.TmuxSessionNameChange
}

// ResumeSession owns the complete shared resume lifecycle. It returns before
// any interactive attachment so the stable session lock is never held while a
// human terminal is attached.
func ResumeSession(opCtx *OpContext, req *ResumeSessionRequest) (*ResumeSessionResult, error) {
	if req == nil {
		return nil, ErrInvalidInput("request", "Resume request is required.")
	}
	if req.SessionID == "" {
		return nil, ErrInvalidInput("session_id", "Session ID is required.")
	}
	if opCtx == nil || opCtx.Storage == nil {
		return nil, ErrInvalidInput("storage", "Session storage is required.")
	}
	if opCtx.Tmux == nil {
		return nil, ErrInvalidInput("tmux", "Tmux adapter is required.")
	}
	store, ok := opCtx.Storage.(resumeStorage)
	if !ok {
		return nil, ErrInvalidInput("storage", "Session storage does not support transactional resume.")
	}

	ctx := requestContext(opCtx)
	var result *ResumeSessionResult
	err := WithSessionLockContext(ctx, req.SessionID, func() error {
		var lockedErr error
		result, lockedErr = resumeSessionLocked(ctx, store, opCtx.Tmux, opCtx.AgyWorkspaceCreateLocker, req)
		return lockedErr
	})
	return result, err
}

func resumeSessionLocked( //nolint:gocyclo // keeping the ordered transaction and its rollback decisions together makes ownership auditable
	ctx context.Context,
	store resumeStorage,
	tmuxAdapter session.TmuxInterface,
	agyLocker AgyWorkspaceCreateLocker,
	req *ResumeSessionRequest,
) (*ResumeSessionResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m, err := store.GetSession(req.SessionID)
	if err != nil {
		return nil, ErrStorageError("session/resume.load", err)
	}
	harnessName := agent.NormalizeHarnessName(m.Harness)
	if harnessName == "" {
		harnessName = "claude-code"
	}
	result := &ResumeSessionResult{
		Operation:    "session/resume",
		SessionID:    m.SessionID,
		Name:         m.Name,
		Harness:      harnessName,
		WorktreePath: m.Context.Project,
	}
	if m.Lifecycle == manifest.LifecycleArchived {
		return result, ErrSessionArchived(m.Name)
	}
	if err := agent.ValidateHarnessAvailability(harnessName); err != nil {
		addResumeWarning(result, req, err.Error())
	}

	health := classifyResumeHealth(ctx, tmuxAdapter, m, req.ManifestPath)
	result.Health = health
	result.TmuxSessionName = health.TmuxSessionName
	emitResumeEvent(req, ResumeEventHealthClassified, "")
	for _, warning := range health.Warnings {
		addResumeWarning(result, req, warning)
	}
	if !health.CanResume {
		return result, resumeHealthError(m.Name, health.Issues)
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := migrateResumeAgyModel(store, m, harnessName); err != nil {
		return result, ErrStorageError("session/resume.migrate-agy-model", err)
	}

	created, err := ensureResumeTmux(ctx, tmuxAdapter, &health, req)
	result.Health = health
	result.TmuxSessionName = health.TmuxSessionName
	result.CreatedTmux = created.owned()
	if err != nil {
		if created.owned() {
			return result, rollbackResumeTmux(ctx, tmuxAdapter, store, m, created, resumeTmuxNameChange{}, err)
		}
		return result, err
	}

	sendCommand, err := shouldSubmitResumeCommand(ctx, tmuxAdapter, harnessName, health)
	if err != nil {
		if created.owned() {
			return result, rollbackResumeTmux(ctx, tmuxAdapter, store, m, created, resumeTmuxNameChange{}, err)
		}
		return result, err
	}
	result.StartedHarness = sendCommand

	piLaunchID := ""
	if sendCommand {
		launchManifest := resumeLaunchManifest(m, harnessName, req.CurrentAddDirs, req.ExcludedAddDirs)
		launch, launchID, warnings, prepareErr := prepareResumeLaunch(store, launchManifest, harnessName, health)
		for _, warning := range warnings {
			addResumeWarning(result, req, warning)
		}
		if prepareErr != nil {
			if created.owned() {
				return result, rollbackResumeTmux(ctx, tmuxAdapter, store, m, created, resumeTmuxNameChange{}, prepareErr)
			}
			return result, prepareErr
		}
		piLaunchID = launchID
		if err := submitAndAwaitResume(ctx, tmuxAdapter, agyLocker, req, result, harnessName, health, piLaunchID, launch); err != nil {
			if created.owned() {
				return result, rollbackResumeTmux(ctx, tmuxAdapter, store, m, created, resumeTmuxNameChange{}, err)
			}
			return result, err
		}
		if err := persistResumeSandboxPolicy(store, m, launchManifest); err != nil {
			persistErr := ErrStorageError("session/resume.persist-sandbox-policy", err)
			if created.owned() {
				return result, rollbackResumeTmux(ctx, tmuxAdapter, store, m, created, resumeTmuxNameChange{}, persistErr)
			}
			return result, persistErr
		}
	}

	var nameChange resumeTmuxNameChange
	if created.owned() {
		nameChange, err = persistResumeTmuxName(ctx, store, m, health.TmuxSessionName)
		if err != nil {
			return result, rollbackResumeTmux(ctx, tmuxAdapter, store, m, created, nameChange, err)
		}
	}

	transactionalPrompt := harnessName == "codex-cli"
	promptMayHaveStarted := false
	ownershipCompleted := false
	if transactionalPrompt && req.Prompt != "" {
		promptMayHaveStarted, err = submitResumePrompt(ctx, tmuxAdapter, req, result, health.TmuxSessionName, harnessName, true)
		if err != nil {
			if created.owned() {
				return result, rollbackResumeTmux(ctx, tmuxAdapter, store, m, created, nameChange, err)
			}
			addResumeWarning(result, req, fmt.Sprintf("Failed to send post-resume prompt: %v", err))
		}
	}

	completionCtx := ctx
	if promptMayHaveStarted {
		completionCtx = context.WithoutCancel(ctx)
		completeResumeTmuxName(completionCtx, store, nameChange, result, req)
		ownershipCompleted = true
	}
	if err := completionCtx.Err(); err != nil {
		if created.owned() {
			return result, rollbackResumeTmux(ctx, tmuxAdapter, store, m, created, nameChange, err)
		}
		return result, err
	}
	if sendCommand {
		restoreResumePermission(completionCtx, tmuxAdapter, harnessName, m, health, result, req)
	}
	if err := completionCtx.Err(); err != nil {
		if created.owned() {
			return result, rollbackResumeTmux(ctx, tmuxAdapter, store, m, created, nameChange, err)
		}
		return result, err
	}
	if err := updateResumeActivity(completionCtx, store, m, req.ManifestPath); err != nil {
		addResumeWarning(result, req, fmt.Sprintf("Failed to update manifest activity: %v", err))
	}
	if err := completionCtx.Err(); err != nil {
		if created.owned() {
			return result, rollbackResumeTmux(ctx, tmuxAdapter, store, m, created, nameChange, err)
		}
		return result, err
	}

	if !transactionalPrompt && req.Prompt != "" {
		var promptErr error
		promptMayHaveStarted, promptErr = submitResumePrompt(completionCtx, tmuxAdapter, req, result, health.TmuxSessionName, harnessName, false)
		if promptMayHaveStarted {
			completionCtx = context.WithoutCancel(ctx)
		}
		if promptErr != nil {
			if ctx.Err() != nil {
				if created.owned() {
					return result, rollbackResumeTmux(ctx, tmuxAdapter, store, m, created, nameChange, ctx.Err())
				}
				return result, ctx.Err()
			}
			addResumeWarning(result, req, fmt.Sprintf("Failed to send post-resume prompt: %v", promptErr))
		}
	}

	if !ownershipCompleted {
		completeResumeTmuxName(completionCtx, store, nameChange, result, req)
	}
	result.PromptMayHaveStarted = promptMayHaveStarted
	return result, nil
}

// resumeLaunchManifest returns a launch copy whose Codex grants are the stable
// union of the session's persisted policy and the current trusted host handoff.
// The caller persists that union only after the harness is confirmed ready, so
// failed resumes cannot partially rewrite policy while repaired sessions remain
// durable across later cold resumes.
func resumeLaunchManifest(m *manifest.Manifest, harnessName string, currentAddDirs, excludedAddDirs []string) *manifest.Manifest {
	if m == nil || harnessName != "codex-cli" || m.Sandbox == nil || !m.Sandbox.Enabled || (len(currentAddDirs) == 0 && len(excludedAddDirs) == 0) {
		return m
	}
	launchManifest := *m
	sandbox := *m.Sandbox
	sandbox.ExtraAddDirs = nil
	for _, dir := range m.Sandbox.ExtraAddDirs {
		if !pathWithinAny(dir, excludedAddDirs) {
			sandbox.ExtraAddDirs = append(sandbox.ExtraAddDirs, dir)
		}
	}
	seen := make(map[string]struct{}, len(sandbox.ExtraAddDirs)+len(currentAddDirs))
	for _, dir := range sandbox.ExtraAddDirs {
		seen[dir] = struct{}{}
	}
	for _, dir := range currentAddDirs {
		if _, ok := seen[dir]; ok {
			continue
		}
		sandbox.ExtraAddDirs = append(sandbox.ExtraAddDirs, dir)
		seen[dir] = struct{}{}
	}
	launchManifest.Sandbox = &sandbox
	return &launchManifest
}

func pathWithinAny(path string, roots []string) bool {
	clean := filepath.Clean(path)
	for _, root := range roots {
		rel, err := filepath.Rel(filepath.Clean(root), clean)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func persistResumeSandboxPolicy(store dolt.Storage, persisted, launch *manifest.Manifest) error {
	if store == nil || persisted == nil || launch == nil || launch == persisted || launch.Sandbox == nil {
		return nil
	}
	original := persisted.Sandbox
	sandbox := *launch.Sandbox
	sandbox.ExtraAddDirs = append([]string{}, launch.Sandbox.ExtraAddDirs...)
	persisted.Sandbox = &sandbox
	if err := store.UpdateSession(persisted); err != nil {
		persisted.Sandbox = original
		return err
	}
	return nil
}

func classifyResumeHealth(ctx context.Context, tmuxAdapter session.TmuxInterface, m *manifest.Manifest, manifestPath string) ResumeSessionHealth {
	sessionName := m.Tmux.SessionName
	if sessionName == "" {
		sessionName = tmux.SanitizeSessionName(m.Name)
		if sessionName == "" {
			sessionName = "session"
		}
	}
	health := ResumeSessionHealth{
		ManifestPath:    manifestPath,
		WorktreePath:    m.Context.Project,
		TmuxSessionName: sessionName,
		CanResume:       true,
	}
	if _, err := os.Stat(m.Context.Project); err != nil {
		if os.IsNotExist(err) {
			health.Issues = append(health.Issues, fmt.Sprintf("Working directory not found: %s", m.Context.Project))
			health.CanResume = false
		} else {
			health.Issues = append(health.Issues, fmt.Sprintf("Cannot inspect working directory %s: %v", m.Context.Project, err))
			health.CanResume = false
		}
	} else {
		health.WorktreeExists = true
	}
	checker, ok := tmuxAdapter.(session.StrictSessionExistenceChecker)
	if !ok {
		health.Issues = append(health.Issues, "Tmux adapter does not support strict session existence checks")
		health.CanResume = false
		return health
	}
	exists, err := checker.HasSessionStrict(ctx, sessionName)
	if err != nil {
		health.Issues = append(health.Issues, fmt.Sprintf("Failed to check tmux session: %v", err))
		health.CanResume = false
	}
	health.TmuxExists = exists
	return health
}

func ensureResumeTmux(ctx context.Context, tmuxAdapter session.TmuxInterface, health *ResumeSessionHealth, req *ResumeSessionRequest) (createdResumeTmux, error) {
	if err := ctx.Err(); err != nil {
		return createdResumeTmux{}, err
	}
	if health.TmuxExists {
		emitResumeEvent(req, ResumeEventTmuxExisting, health.TmuxSessionName)
		return createdResumeTmux{}, nil
	}
	manager, ok := tmuxAdapter.(session.ResumeTmuxIdentityManager)
	if !ok {
		return createdResumeTmux{}, fmt.Errorf("tmux adapter does not support exact resume identity")
	}
	name := tmux.SanitizeSessionName(health.TmuxSessionName)
	identity, err := manager.CreateSessionWithIdentity(name, health.WorktreePath)
	created := createdResumeTmux{Name: name, Identity: identity}
	if err != nil {
		return created, fmt.Errorf("create resume tmux session: %w", err)
	}
	if !identity.Valid() {
		return created, fmt.Errorf("tmux creation returned no valid creation identity")
	}
	health.TmuxSessionName = name
	emitResumeEvent(req, ResumeEventTmuxCreated, name)
	return created, nil
}

func shouldSubmitResumeCommand(ctx context.Context, tmuxAdapter session.TmuxInterface, harnessName string, health ResumeSessionHealth) (bool, error) {
	if !health.TmuxExists {
		return true, nil
	}
	if harnessName != "pi-cli" {
		return false, nil
	}
	checker, ok := tmuxAdapter.(session.ExpectedHarnessLivenessChecker)
	if !ok {
		return false, fmt.Errorf("tmux adapter does not support exact Pi liveness classification")
	}
	verdict, err := checker.ExpectedHarnessLiveness(ctx, health.TmuxSessionName, harnessName)
	if err != nil {
		return false, fmt.Errorf("check exact Pi process liveness: %w", err)
	}
	action, err := agent.DecidePiPaneResume(verdict.HarnessAlive, tmux.PaneLiveness{
		SessionExists:    verdict.SessionExists,
		HarnessAlive:     verdict.HarnessAlive,
		ZombieWriter:     verdict.ZombieWriter,
		RestartableShell: verdict.RestartableShell,
		Evidence:         verdict.Evidence,
	})
	if err != nil {
		return false, fmt.Errorf("refusing to resume Pi session %q: %w", health.TmuxSessionName, err)
	}
	return action == agent.PiPaneRelaunch, nil
}

func submitAndAwaitResume(
	ctx context.Context,
	tmuxAdapter session.TmuxInterface,
	agyLocker AgyWorkspaceCreateLocker,
	req *ResumeSessionRequest,
	result *ResumeSessionResult,
	harnessName string,
	health ResumeSessionHealth,
	piLaunchID string,
	launch HarnessLaunchCommand,
) error {
	return withResumeWorkspaceLock(ctx, harnessName, health.WorktreePath, agyLocker, result, req, func() error {
		if err := ctx.Err(); err != nil {
			if cancelErr := launch.CancelUndelivered(); cancelErr != nil {
				return errors.Join(err, fmt.Errorf("cancel undelivered %s resume launch: %w", harnessName, cancelErr))
			}
			return err
		}
		if _, err := override.CommitAll(launch.Reservations...); err != nil {
			return errors.Join(
				fmt.Errorf("commit %s resume override transaction: %w", harnessName, err),
				launch.CancelUndelivered(),
			)
		}
		uncertain, err := ResolveHarnessLaunchSubmission(launch, tmuxAdapter.SendKeys(health.TmuxSessionName, launch.Command))
		if uncertain {
			addResumeWarning(result, req, fmt.Sprintf("%s launch submission acknowledgement was lost; preserving the launch because the command may already be queued", harnessName))
		}
		if err != nil {
			return fmt.Errorf("submit %s resume command: %w", harnessName, err)
		}
		waiter, ok := tmuxAdapter.(session.ResumeReadinessWaiter)
		if !ok {
			return fmt.Errorf("tmux adapter does not support resume readiness")
		}
		readiness, err := waiter.WaitForResumeReady(ctx, health.TmuxSessionName, harnessName, piLaunchID, resumeReadinessTimeout)
		for _, warning := range readiness.Warnings {
			addResumeWarning(result, req, warning)
		}
		if err != nil {
			return fmt.Errorf("wait for %s resume readiness: %w", harnessName, err)
		}
		emitResumeEvent(req, ResumeEventHarnessReady, harnessName)
		return nil
	})
}

func withResumeWorkspaceLock(
	ctx context.Context,
	harnessName, workDir string,
	locker AgyWorkspaceCreateLocker,
	result *ResumeSessionResult,
	req *ResumeSessionRequest,
	fn func() error,
) error {
	if harnessName != "agy" {
		return fn()
	}
	if locker == nil {
		locker = agysession.AcquireWorkspaceCreateLock
	}
	release, err := locker(ctx, workDir)
	if err != nil {
		return fmt.Errorf("acquire AGY workspace lifecycle lock for resume: %w", err)
	}
	defer func() {
		if unlockErr := release(); unlockErr != nil {
			addResumeWarning(result, req, fmt.Sprintf("Failed to release AGY workspace lock after resume: %v", unlockErr))
		}
	}()
	return fn()
}

func submitResumePrompt(
	ctx context.Context,
	tmuxAdapter session.TmuxInterface,
	req *ResumeSessionRequest,
	result *ResumeSessionResult,
	sessionName, harnessName string,
	strict bool,
) (bool, error) {
	sender, ok := tmuxAdapter.(session.SafePromptSender)
	if !ok {
		return false, fmt.Errorf("tmux adapter does not support safe prompt submission")
	}
	submission, err := sender.SendPrompt(ctx, sessionName, harnessName, req.Prompt)
	if err == nil {
		emitResumeEvent(req, ResumeEventPromptSubmitted, "")
		return true, nil
	}
	if submission.MayHaveStarted {
		addResumeWarning(result, req, fmt.Sprintf("Prompt submission acknowledgement was lost; preserving the session because work may have started: %v", err))
		emitResumeEvent(req, ResumeEventPromptSubmitted, "")
		return true, nil
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if strict {
		return false, fmt.Errorf("failed to deliver transactional post-resume prompt: %w", err)
	}
	return false, err
}

func persistResumeTmuxName(ctx context.Context, store resumeStorage, m *manifest.Manifest, sessionName string) (resumeTmuxNameChange, error) {
	change, err := store.BeginTmuxSessionNameChange(ctx, m.SessionID, sessionName)
	if err != nil {
		pending := resumeTmuxNameChange{}
		if change != nil {
			pending = resumeTmuxNameChange{Applied: true, Change: *change}
		}
		return pending, fmt.Errorf("persist canonical tmux session name %q: %w", sessionName, err)
	}
	latest, err := store.GetSession(m.SessionID)
	if err != nil {
		reloadErr := fmt.Errorf("reload session after canonical tmux-name persistence: %w", err)
		if change != nil {
			pending := resumeTmuxNameChange{Applied: true, Change: *change}
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			restored, restoreErr := store.RestoreTmuxSessionNameChange(cleanupCtx, *change)
			if restoreErr != nil {
				return pending, errors.Join(reloadErr, fmt.Errorf("compensate tmux-name persistence after reload failure: %w", restoreErr))
			}
			if !restored {
				return pending, errors.Join(reloadErr, fmt.Errorf("compensate tmux-name persistence after reload failure: session metadata no longer matches this resume transaction"))
			}
		}
		return resumeTmuxNameChange{}, reloadErr
	}
	m.Tmux.SessionName = latest.Tmux.SessionName
	m.UpdatedAt = latest.UpdatedAt
	if change == nil {
		return resumeTmuxNameChange{}, nil
	}
	return resumeTmuxNameChange{Applied: true, Change: *change}, nil
}

func restoreResumeTmuxName(ctx context.Context, store resumeStorage, m *manifest.Manifest, change resumeTmuxNameChange) error {
	if !change.Applied {
		return nil
	}
	restored, err := store.RestoreTmuxSessionNameChange(ctx, change.Change)
	if err != nil {
		return err
	}
	if !restored {
		return fmt.Errorf("session metadata no longer matches this resume transaction")
	}
	m.Tmux.SessionName = change.Change.PreviousName
	m.UpdatedAt = change.Change.PreviousUpdatedAt
	return nil
}

func completeResumeTmuxName(ctx context.Context, store resumeStorage, change resumeTmuxNameChange, result *ResumeSessionResult, req *ResumeSessionRequest) {
	if !change.Applied {
		return
	}
	completed, err := store.CompleteTmuxSessionNameChange(ctx, change.Change)
	if err != nil {
		addResumeWarning(result, req, fmt.Sprintf("Failed to finalize resume metadata ownership: %v", err))
		return
	}
	if !completed {
		addResumeWarning(result, req, "Resume metadata ownership was superseded before finalization")
	}
}

func rollbackResumeTmux(
	ctx context.Context,
	tmuxAdapter session.TmuxInterface,
	store resumeStorage,
	m *manifest.Manifest,
	created createdResumeTmux,
	change resumeTmuxNameChange,
	primaryErr error,
) error {
	if change.Applied {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if restoreErr := restoreResumeTmuxName(cleanupCtx, store, m, change); restoreErr != nil {
			return errors.Join(primaryErr, fmt.Errorf("failed to compensate canonical tmux-name persistence: %w", restoreErr))
		}
	}
	manager, ok := tmuxAdapter.(session.ResumeTmuxIdentityManager)
	if !ok {
		return errors.Join(primaryErr, fmt.Errorf("tmux adapter cannot clean up exact resume identity"))
	}
	if !created.owned() {
		return errors.Join(primaryErr, fmt.Errorf("cannot clean up tmux session %q without its creation identity", created.Name))
	}
	if err := manager.KillSessionIdentityChecked(created.Identity); err != nil {
		return errors.Join(primaryErr, fmt.Errorf("failed to clean up newly created tmux session %q (%s): %w", created.Name, created.Identity.ID, err))
	}
	exists, err := manager.HasSessionIdentityStrict(created.Identity)
	if err != nil {
		return errors.Join(primaryErr, fmt.Errorf("verify tmux session identity cleanup: %w", err))
	}
	if exists {
		return errors.Join(primaryErr, fmt.Errorf("tmux session %q (%s) still exists after cleanup", created.Name, created.Identity.ID))
	}
	return primaryErr
}

func restoreResumePermission(ctx context.Context, tmuxAdapter session.TmuxInterface, harnessName string, m *manifest.Manifest, health ResumeSessionHealth, result *ResumeSessionResult, req *ResumeSessionRequest) {
	if harnessName != "claude-code" || m.PermissionMode == "" || m.PermissionMode == "default" {
		return
	}
	count := map[string]int{"auto": 1, "plan": 2}[m.PermissionMode]
	if count == 0 {
		return
	}
	checker, ok := tmuxAdapter.(session.InputReadinessChecker)
	if !ok {
		addResumeWarning(result, req, "Cannot restore permission mode: tmux adapter cannot verify input readiness")
		return
	}
	readiness, err := checker.CheckInputReadiness(ctx, health.TmuxSessionName, "claude-code")
	if err != nil {
		addResumeWarning(result, req, fmt.Sprintf("Cannot restore permission mode: %v", err))
		return
	}
	if !readiness.Ready {
		addResumeWarning(result, req, fmt.Sprintf("Cannot restore permission mode: session not at idle prompt (state: %s)", readiness.State))
		return
	}
	sender, ok := tmuxAdapter.(session.LiteralKeySender)
	if !ok {
		addResumeWarning(result, req, "Cannot restore permission mode: tmux adapter cannot send literal keys")
		return
	}
	for range count {
		if err := sender.SendLiteralKeys(health.TmuxSessionName, "S-Tab"); err != nil {
			addResumeWarning(result, req, fmt.Sprintf("Failed to restore permission mode: %v", err))
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func updateResumeActivity(ctx context.Context, store resumeStorage, m *manifest.Manifest, manifestPath string) error {
	if err := store.TouchSessionActivity(context.WithoutCancel(ctx), m.SessionID); err != nil {
		return err
	}
	_ = gitmanifest.CommitManifest(manifestPath, "resume", m.Name)
	return nil
}

func resumeHealthError(name string, issues []string) *OpError {
	return &OpError{
		Status:   409,
		Type:     "session/not_resumable",
		Code:     ErrCodeSessionNotReady,
		Title:    "Session cannot be resumed",
		Detail:   fmt.Sprintf("Session %q cannot be resumed: %s", name, strings.Join(issues, "; ")),
		Instance: "session/resume",
		Suggestions: []string{
			"Fix the reported health issues and retry.",
			"Run `agm admin doctor` for lifecycle diagnostics.",
		},
		Parameters: map[string]string{"session": name, "readiness": "HEALTH"},
	}
}

func addResumeWarning(result *ResumeSessionResult, req *ResumeSessionRequest, warning string) {
	if warning == "" {
		return
	}
	result.Warnings = append(result.Warnings, warning)
	emitResumeEvent(req, ResumeEventWarning, warning)
}

func emitResumeEvent(req *ResumeSessionRequest, kind, message string) {
	if req == nil || req.OnEvent == nil {
		return
	}
	func() {
		defer func() {
			recovered := recover()
			_ = recovered
		}()
		req.OnEvent(ResumeSessionEvent{Kind: kind, Message: message})
	}()
}

func migrateResumeAgyModel(store dolt.Storage, m *manifest.Manifest, harnessName string) error {
	if harnessName != "agy" || m.Agy == nil || m.Agy.ConversationID == "" || !isAmbiguousLegacyAgyModel(m.Model) {
		return nil
	}
	m.Model = ""
	return store.UpdateSession(m)
}

func isAmbiguousLegacyAgyModel(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "2.5-flash", "gemini-2.5-flash":
		return true
	default:
		return false
	}
}

func prepareResumeLaunch(store dolt.Storage, m *manifest.Manifest, harnessName string, health ResumeSessionHealth) (HarnessLaunchCommand, string, []string, error) {
	spec := HarnessLaunchSpec{
		Harness:        harnessName,
		Model:          m.Model,
		SessionName:    health.TmuxSessionName,
		SessionID:      m.SessionID,
		WorkDir:        health.WorktreePath,
		PermissionMode: m.PermissionMode,
		Codex:          m.Codex,
	}
	switch harnessName {
	case "claude-code":
		launch, warnings := resumeClaudeLaunch(store, m, health)
		prepared, err := harnessexec.PrepareClaudeCommand(launch, os.Environ())
		if err != nil {
			return HarnessLaunchCommand{}, "", warnings, fmt.Errorf("prepare Claude resume launch: %w", err)
		}
		return HarnessLaunchCommand{Command: prepared.Command, Cancel: prepared.Cancel}, "", warnings, nil
	case "codex-cli":
		warnings := []string{}
		if m.Sandbox != nil && m.Sandbox.Enabled {
			spec.ExtraAddDirs = append([]string{}, m.Sandbox.ExtraAddDirs...)
			// Attestation re-runs here and fails closed. Command preparation
			// reserves authorization bound to the exact source identity; resume
			// commits it at submission and seals a fresh exact ledger receipt into
			// the private handoff. A persisted launch policy therefore cannot
			// become "approve once, resume forever".
			if m.Sandbox.BypassCodexHookTrust {
				reason, reasonErr := override.ValidateReason(m.Sandbox.BypassCodexHookTrustReason)
				if reasonErr != nil {
					return HarnessLaunchCommand{}, "", warnings, fmt.Errorf("revalidate Codex hook-trust reason before resume: %w", reasonErr)
				}
				if err := codexhooks.Verify(context.Background(), codexhooks.Attestation{
					SourceRepo:   m.Sandbox.CodexHookSourceRepo,
					SourceCommit: m.Sandbox.CodexHookSourceCommit,
					Digest:       m.Sandbox.CodexHookDigest,
					HookRoot:     m.Sandbox.CodexHookRoot,
				}, health.WorktreePath); err != nil {
					return HarnessLaunchCommand{}, "", warnings, fmt.Errorf("revalidate Codex hook trust before resume: %w", err)
				}
				spec.BypassCodexHookTrust = true
				spec.CodexHookRoot = m.Sandbox.CodexHookRoot
				spec.CodexHookTrustReason = reason
				spec.CodexHookTrustActor = OverrideActor()
				spec.CodexHookSourceRepo = m.Sandbox.CodexHookSourceRepo
				spec.CodexHookSourceCommit = m.Sandbox.CodexHookSourceCommit
				spec.CodexHookDigest = m.Sandbox.CodexHookDigest
			}
		}
		if err := agent.EnsureCodexWorkdirTrusted(health.WorktreePath); err != nil {
			warnings = append(warnings, fmt.Sprintf("Could not pre-trust Codex workdir %s: %v", health.WorktreePath, err))
		}
		if spec.Model == "" {
			spec.Model = agent.HarnessDefaults["codex-cli"]
		}
		launch, err := PrepareHarnessLaunchCommand(spec)
		if err != nil {
			return HarnessLaunchCommand{}, "", warnings, fmt.Errorf("prepare Codex resume launch: %w", err)
		}
		return launch, "", warnings, nil
	case "agy":
		// AGY cold resume enters the workspace through its native
		// conversation route, so retain the workspace as an explicitly
		// authorized add-dir just as session creation does.
		spec.ExtraAddDirs = []string{health.WorktreePath}
		if m.Agy != nil && m.Agy.ConversationID != "" {
			if isAmbiguousLegacyAgyModel(spec.Model) {
				spec.Model = ""
			}
			launch, err := PrepareAgyResumeCommand(spec, m.Agy.ConversationID)
			return launch, "", nil, err
		}
		if spec.Model == "" {
			spec.Model = agent.HarnessDefaults["agy"]
		}
		launch, err := PrepareHarnessLaunchCommand(spec)
		return launch, "", []string{"No AGY conversation ID found; starting a new AGY session"}, err
	case "pi-cli":
		launchID := launchparity.NewPiLaunchID()
		launch, err := buildPiResumeLaunch(m, health, launchID)
		return launch, launchID, nil, err
	case "opencode-cli":
		launch, err := PrepareHarnessLaunchCommand(spec)
		return launch, "", nil, err
	default:
		launch, err := PrepareFallbackResumeCommand(health.WorktreePath)
		return launch, "", []string{fmt.Sprintf("Harness %q does not support resume; starting in its working directory", harnessName)}, err
	}
}

func resumeClaudeLaunch(store dolt.Storage, m *manifest.Manifest, health ResumeSessionHealth) (harnessexec.ClaudeLaunch, []string) {
	resumeUUID := m.Claude.UUID
	warnings := []string{}
	if resumeUUID == "" {
		findByName := func(name string) (*manifest.Manifest, error) {
			manifests, err := store.ListSessions(&dolt.SessionFilter{})
			if err != nil {
				return nil, err
			}
			for _, candidate := range manifests {
				if candidate.Tmux.SessionName == name || candidate.Name == name {
					return candidate, nil
				}
			}
			return nil, fmt.Errorf("no session found for: %s", name)
		}
		if discovered, err := uuidpkg.Discover(health.TmuxSessionName, findByName, false); err == nil && discovered != "" {
			resumeUUID = discovered
			warnings = append(warnings, fmt.Sprintf("Discovered Claude UUID via fallback: %s", discovered[:8]))
		}
	}
	resumeDir := health.WorktreePath
	if resumeUUID != "" {
		resumeDir, warnings = resolveClaudeResumeDir(resumeUUID, health.WorktreePath, warnings)
	} else {
		warnings = append(warnings, "No Claude UUID found; starting a new Claude session")
	}
	return harnessexec.ClaudeLaunch{
		SessionName:      health.TmuxSessionName,
		SessionID:        m.SessionID,
		ResumeID:         resumeUUID,
		WorkDir:          resumeDir,
		ForwardTelemetry: true,
	}, warnings
}

func resolveClaudeResumeDir(resumeUUID, worktreePath string, warnings []string) (string, []string) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return worktreePath, warnings
	}
	cwd, err := claude.FindTranscriptCwd(homeDir, resumeUUID)
	if err != nil || cwd == "" {
		return worktreePath, warnings
	}
	if cwd != worktreePath {
		warnings = append(warnings, fmt.Sprintf("Conversation lives in %s, not the worktree %s; resuming from there", cwd, worktreePath))
	}
	return cwd, warnings
}

func buildPiResumeLaunch(m *manifest.Manifest, health ResumeSessionHealth, launchID string) (HarnessLaunchCommand, error) { //nolint:gocyclo // linear fail-closed validation keeps the native identity checks visible
	if m.Pi == nil || m.Pi.SessionID == "" || m.Pi.SessionDir == "" {
		return HarnessLaunchCommand{}, fmt.Errorf("pi session metadata is incomplete; exact native session_id and session_dir are required for resume")
	}
	if err := pisession.ValidateID(m.Pi.SessionID); err != nil {
		return HarnessLaunchCommand{}, err
	}
	if _, err := pisession.ValidateRoot(m.Pi.SessionDir); err != nil {
		return HarnessLaunchCommand{}, fmt.Errorf("pi session directory is unavailable: %w", err)
	}
	transcriptPath, findErr := pisession.FindTranscript(m.Pi.SessionDir, m.Pi.SessionID)
	hasTranscript := findErr == nil
	if findErr != nil && !errors.Is(findErr, pisession.ErrTranscriptNotFound) {
		return HarnessLaunchCommand{}, fmt.Errorf("resolve Pi resume transcript: %w", findErr)
	}
	if m.Pi.TranscriptPath != "" {
		persisted, absErr := filepath.Abs(m.Pi.TranscriptPath)
		if !hasTranscript || absErr != nil || filepath.Clean(transcriptPath) != filepath.Clean(persisted) {
			return HarnessLaunchCommand{}, fmt.Errorf("pi resume transcript does not match the persisted native identity")
		}
	}
	extensionPath, err := piadapter.EnsureExtension(os.Getenv("AGM_PI_EXTENSION_ROOT"))
	if err != nil {
		return HarnessLaunchCommand{}, fmt.Errorf("install Pi authorization extension: %w", err)
	}
	allow := []string(nil)
	if m.PermissionPolicy != nil {
		allow = m.PermissionPolicy.Allow
	}
	policyJSON, err := piadapter.MarshalPolicy(allow)
	if err != nil {
		return HarnessLaunchCommand{}, err
	}
	policyFile, err := piadapter.EnsurePolicyFile(os.Getenv("AGM_PI_EXTENSION_ROOT"), m.Pi.SessionID, policyJSON)
	if err != nil {
		return HarnessLaunchCommand{}, fmt.Errorf("install Pi permission policy: %w", err)
	}
	model := m.Model
	if model == "" && !hasTranscript {
		model = agent.HarnessDefaults["pi-cli"]
	}
	codingAgentDir := pisession.ResolveCodingAgentDir(m.Pi.CodingAgentDir, m.Pi.CodingAgentDirSet, os.Getenv("PI_CODING_AGENT_DIR"))
	codingAgentDir, err = pisession.ValidateCodingAgentDir(codingAgentDir)
	if err != nil {
		return HarnessLaunchCommand{}, err
	}
	piMeta := *m.Pi
	piMeta.CodingAgentDir = codingAgentDir
	piMeta.CodingAgentDirSet = true
	return PrepareHarnessLaunchCommand(HarnessLaunchSpec{
		Harness:        "pi-cli",
		Model:          model,
		SessionName:    health.TmuxSessionName,
		SessionID:      m.Pi.SessionID,
		WorkDir:        health.WorktreePath,
		PermissionMode: m.PermissionMode,
		Pi:             &piMeta,
		PiLaunchID:     launchID,
		PiExtension:    extensionPath,
		PiPolicyJSON:   policyJSON,
		PiPolicyFile:   policyFile,
	})
}
