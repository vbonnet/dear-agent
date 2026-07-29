package ops

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/launchparity"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

type resumeTestTmux struct {
	exists            bool
	hasSessionErr     error
	created           int
	killed            bool
	identityStillLive bool
	createErr         error
	killErr           error
	identityCheckErr  error
	commands          []string
	sendHook          func()
	attached          []string
	readinessCalls    int
	readiness         session.ResumeReadiness
	readinessErr      error
	readinessHook     func()
	liveness          session.LivenessInfo
	livenessErr       error
	promptSubmission  session.PromptSubmission
	promptErr         error
	promptHook        func()
	promptSession     string
	promptHarness     string
	promptBody        string
	literalKeys       []string
	inputReadiness    session.InputReadiness
	inputErr          error
}

func (f *resumeTestTmux) HasSession(string) (bool, error) {
	return f.exists, f.hasSessionErr
}

func (f *resumeTestTmux) HasSessionStrict(context.Context, string) (bool, error) {
	return f.exists, f.hasSessionErr
}

func (f *resumeTestTmux) ListSessions() ([]string, error) {
	return nil, nil
}

func (f *resumeTestTmux) ListSessionsWithInfo() ([]session.SessionInfo, error) {
	return nil, nil
}

func (f *resumeTestTmux) ListClients(string) ([]session.ClientInfo, error) {
	return nil, nil
}

func (f *resumeTestTmux) CreateSession(string, string) error {
	return errors.New("resume must use exact identity creation")
}

func (f *resumeTestTmux) AttachSession(name string) error {
	f.attached = append(f.attached, name)
	return nil
}

func (f *resumeTestTmux) SendKeys(_ string, keys string) error {
	if f.sendHook != nil {
		f.sendHook()
	}
	f.commands = append(f.commands, keys)
	return nil
}

func (f *resumeTestTmux) CreateSessionWithIdentity(string, string) (tmux.SessionIdentity, error) {
	f.created++
	f.exists = true
	f.identityStillLive = true
	return resumeTestIdentity(), f.createErr
}

func (f *resumeTestTmux) KillSessionIdentityChecked(identity tmux.SessionIdentity) error {
	if identity != resumeTestIdentity() {
		return errors.New("unexpected resume identity")
	}
	if f.killErr != nil {
		return f.killErr
	}
	f.killed = true
	f.exists = false
	f.identityStillLive = false
	return nil
}

func (f *resumeTestTmux) HasSessionIdentityStrict(identity tmux.SessionIdentity) (bool, error) {
	if identity != resumeTestIdentity() {
		return false, errors.New("unexpected resume identity")
	}
	return f.identityStillLive, f.identityCheckErr
}

func (f *resumeTestTmux) ExpectedHarnessLiveness(context.Context, string, string) (session.LivenessInfo, error) {
	return f.liveness, f.livenessErr
}

func (f *resumeTestTmux) WaitForResumeReady(context.Context, string, string, string, time.Duration) (session.ResumeReadiness, error) {
	f.readinessCalls++
	if f.readinessHook != nil {
		f.readinessHook()
	}
	return f.readiness, f.readinessErr
}

func (f *resumeTestTmux) SendPrompt(_ context.Context, sessionName, harness, prompt string) (session.PromptSubmission, error) {
	f.promptSession = sessionName
	f.promptHarness = harness
	f.promptBody = prompt
	if f.promptHook != nil {
		f.promptHook()
	}
	return f.promptSubmission, f.promptErr
}

func (f *resumeTestTmux) CheckInputReadiness(context.Context, string, string) (session.InputReadiness, error) {
	return f.inputReadiness, f.inputErr
}

func (f *resumeTestTmux) SendLiteralKeys(_ string, keys string) error {
	f.literalKeys = append(f.literalKeys, keys)
	return nil
}

func resumeTestIdentity() tmux.SessionIdentity {
	return tmux.SessionIdentity{ID: "$42", Token: "0123456789abcdef0123456789abcdef"}
}

type countingResumeStore struct {
	*dolt.Adapter
	mu   sync.Mutex
	gets int
}

type failingBeginResumeStore struct {
	*dolt.Adapter
	err error
}

func (s *failingBeginResumeStore) BeginTmuxSessionNameChange(context.Context, string, string) (*dolt.TmuxSessionNameChange, error) {
	return nil, s.err
}

type rejectingRestoreResumeStore struct {
	*dolt.Adapter
}

type ambiguousBeginResumeStore struct {
	*dolt.Adapter
	err error
}

func (s *ambiguousBeginResumeStore) BeginTmuxSessionNameChange(ctx context.Context, sessionID, name string) (*dolt.TmuxSessionNameChange, error) {
	change, err := s.Adapter.BeginTmuxSessionNameChange(ctx, sessionID, name)
	if err != nil {
		return change, err
	}
	return change, s.err
}

type failingReloadResumeStore struct {
	*dolt.Adapter
	err   error
	reads int
}

func (s *failingReloadResumeStore) GetSession(sessionID string) (*manifest.Manifest, error) {
	s.reads++
	if s.reads > 1 {
		return nil, s.err
	}
	return s.Adapter.GetSession(sessionID)
}

type completingResumeStore struct {
	*dolt.Adapter
	completed bool
	err       error
}

func (s *completingResumeStore) CompleteTmuxSessionNameChange(context.Context, dolt.TmuxSessionNameChange) (bool, error) {
	return s.completed, s.err
}

func (s *rejectingRestoreResumeStore) RestoreTmuxSessionNameChange(context.Context, dolt.TmuxSessionNameChange) (bool, error) {
	return false, nil
}

func (s *countingResumeStore) GetSession(sessionID string) (*manifest.Manifest, error) {
	s.mu.Lock()
	s.gets++
	s.mu.Unlock()
	return s.Adapter.GetSession(sessionID)
}

func (s *countingResumeStore) getCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gets
}

func setupResumeOperation(t *testing.T, harness string, tmuxExists bool) (*dolt.Adapter, *manifest.Manifest, *resumeTestTmux) {
	t.Helper()
	adapter, err := dolt.NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() {
		if err := adapter.Close(); err != nil {
			t.Errorf("close adapter: %v", err)
		}
	})
	now := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "resume-operation-id",
		Name:          "resume operation",
		Harness:       harness,
		CreatedAt:     now,
		UpdatedAt:     now,
		Context:       manifest.Context{Project: t.TempDir()},
	}
	if tmuxExists {
		m.Tmux.SessionName = "resume-operation"
	}
	if harness == "codex-cli" {
		m.Codex = &manifest.Codex{SessionID: "codex-native-id"}
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	fakeTmux := &resumeTestTmux{
		exists:         tmuxExists,
		inputReadiness: session.InputReadiness{Ready: true, State: "YES"},
	}
	return adapter, m, fakeTmux
}

func TestResumeSessionValidatesPublicContract(t *testing.T) {
	tests := []struct {
		name  string
		ctx   *OpContext
		req   *ResumeSessionRequest
		field string
	}{
		{name: "nil request", ctx: &OpContext{}, field: "request"},
		{name: "missing session ID", ctx: &OpContext{}, req: &ResumeSessionRequest{}, field: "session_id"},
		{name: "missing storage", ctx: &OpContext{}, req: &ResumeSessionRequest{SessionID: "id"}, field: "storage"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ResumeSession(tt.ctx, tt.req)
			var opErr *OpError
			if !errors.As(err, &opErr) || opErr.Parameters["field"] != tt.field {
				t.Fatalf("ResumeSession() error = %v, want invalid %s", err, tt.field)
			}
		})
	}
}

func TestResumeSessionReturnsHealthBeforeMutation(t *testing.T) {
	adapter, m, fakeTmux := setupResumeOperation(t, "opencode-cli", false)
	m.Context.Project = filepath.Join(t.TempDir(), "missing")
	if err := adapter.UpdateSession(m); err != nil {
		t.Fatalf("UpdateSession() error: %v", err)
	}

	result, err := ResumeSession(&OpContext{Storage: adapter, Tmux: fakeTmux}, &ResumeSessionRequest{SessionID: m.SessionID})
	var opErr *OpError
	if !errors.As(err, &opErr) || opErr.Code != ErrCodeSessionNotReady {
		t.Fatalf("ResumeSession() error = %v, want %s", err, ErrCodeSessionNotReady)
	}
	if result == nil || result.Health.CanResume || len(result.Health.Issues) == 0 {
		t.Fatalf("ResumeSession() result = %#v, want unresumable health facts", result)
	}
	if fakeTmux.created != 0 || len(fakeTmux.commands) != 0 {
		t.Fatalf("unhealthy resume mutated tmux: created=%d commands=%v", fakeTmux.created, fakeTmux.commands)
	}
}

func TestResumeSessionFailsClosedWhenTmuxHealthIsUnknown(t *testing.T) {
	adapter, m, fakeTmux := setupResumeOperation(t, "opencode-cli", false)
	fakeTmux.hasSessionErr = errors.New("tmux socket unavailable")

	result, err := ResumeSession(&OpContext{Storage: adapter, Tmux: fakeTmux}, &ResumeSessionRequest{SessionID: m.SessionID})
	var opErr *OpError
	if !errors.As(err, &opErr) || opErr.Code != ErrCodeSessionNotReady {
		t.Fatalf("ResumeSession() error = %v, want fail-closed health result", err)
	}
	if result == nil || result.Health.CanResume || fakeTmux.created != 0 {
		t.Fatalf("unknown tmux health mutated runtime: result=%#v created=%d", result, fakeTmux.created)
	}
}

func TestResumeSessionColdStartCommitsPublicOutcome(t *testing.T) {
	adapter, m, fakeTmux := setupResumeOperation(t, "opencode-cli", false)
	var events []string
	result, err := ResumeSession(
		&OpContext{Context: t.Context(), Storage: adapter, Tmux: fakeTmux},
		&ResumeSessionRequest{
			SessionID: m.SessionID,
			OnEvent: func(event ResumeSessionEvent) {
				events = append(events, event.Kind)
			},
		},
	)
	if err != nil {
		t.Fatalf("ResumeSession() error: %v", err)
	}
	if !result.CreatedTmux || !result.StartedHarness || result.TmuxSessionName != "resume-operation" {
		t.Fatalf("ResumeSession() result = %#v", result)
	}
	if fakeTmux.created != 1 || fakeTmux.killed || fakeTmux.readinessCalls != 1 {
		t.Fatalf("tmux outcome = created %d killed %v readiness %d", fakeTmux.created, fakeTmux.killed, fakeTmux.readinessCalls)
	}
	if len(fakeTmux.commands) != 1 || !strings.Contains(fakeTmux.commands[0], "opencode attach") {
		t.Fatalf("resume commands = %v, want canonical OpenCode attach", fakeTmux.commands)
	}
	stored, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() error: %v", err)
	}
	if stored.Tmux.SessionName != result.TmuxSessionName {
		t.Fatalf("stored tmux name = %q, want %q", stored.Tmux.SessionName, result.TmuxSessionName)
	}
	for _, want := range []string{ResumeEventHealthClassified, ResumeEventTmuxCreated, ResumeEventHarnessReady} {
		if !containsString(events, want) {
			t.Fatalf("events = %v, missing %s", events, want)
		}
	}
}

func TestResumeSessionReadinessFailureRemovesExactColdRuntime(t *testing.T) {
	adapter, m, fakeTmux := setupResumeOperation(t, "opencode-cli", false)
	wantErr := errors.New("readiness failed")
	fakeTmux.readinessErr = wantErr

	result, err := ResumeSession(&OpContext{Storage: adapter, Tmux: fakeTmux}, &ResumeSessionRequest{SessionID: m.SessionID})
	if !errors.Is(err, wantErr) {
		t.Fatalf("ResumeSession() error = %v, want %v", err, wantErr)
	}
	if result == nil || !result.CreatedTmux || !fakeTmux.killed || fakeTmux.identityStillLive {
		t.Fatalf("rollback outcome = result %#v killed=%v live=%v", result, fakeTmux.killed, fakeTmux.identityStillLive)
	}
	stored, loadErr := adapter.GetSession(m.SessionID)
	if loadErr != nil {
		t.Fatalf("GetSession() error: %v", loadErr)
	}
	if stored.Tmux.SessionName != "" {
		t.Fatalf("stored tmux name = %q, want original empty name", stored.Tmux.SessionName)
	}
}

func TestResumeSessionCodexHookReviewFailsBeforeActivityUpdate(t *testing.T) {
	adapter, m, fakeTmux := setupResumeOperation(t, "codex-cli", false)
	before, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() before resume error: %v", err)
	}
	fakeTmux.readinessErr = tmux.CodexHookReviewError()

	result, err := ResumeSession(
		&OpContext{Storage: adapter, Tmux: fakeTmux},
		&ResumeSessionRequest{SessionID: m.SessionID},
	)
	if !errors.Is(err, tmux.ErrCodexHookReviewRequired) {
		t.Fatalf("ResumeSession() error = %v, want ErrCodexHookReviewRequired", err)
	}
	if result == nil || !fakeTmux.killed {
		t.Fatalf("hook-review rollback = result %#v killed=%v", result, fakeTmux.killed)
	}
	after, loadErr := adapter.GetSession(m.SessionID)
	if loadErr != nil {
		t.Fatalf("GetSession() after resume error: %v", loadErr)
	}
	if !after.UpdatedAt.Equal(before.UpdatedAt) || after.Tmux.SessionName != before.Tmux.SessionName {
		t.Fatalf("hook review committed resume effects: before=%#v after=%#v", before, after)
	}
}

func TestPrepareResumeLaunchDefaultsModelLessCodexSession(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	m := &manifest.Manifest{
		SessionID: "legacy-codex-session",
		Harness:   "codex-cli",
		Codex:     &manifest.Codex{SessionID: "native-codex-session"},
	}
	launch, _, _, err := prepareResumeLaunch(
		nil,
		m,
		"codex-cli",
		ResumeSessionHealth{
			TmuxSessionName: "legacy-codex",
			WorktreePath:    t.TempDir(),
		},
	)
	if err != nil {
		t.Fatalf("prepareResumeLaunch() error: %v", err)
	}
	wantModel := agent.ResolveModelFullName("codex-cli", agent.HarnessDefaults["codex-cli"])
	want := "--model " + launchparity.ShellQuote(wantModel)
	if !strings.Contains(launch.Command, want) {
		t.Fatalf("prepareResumeLaunch() command = %q, want %q", launch.Command, want)
	}
}

func TestPrepareResumeLaunchRestoresSandboxCodexPolicy(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	extraAddDir := filepath.Join(t.TempDir(), "real worktree")
	m := &manifest.Manifest{
		SessionID: "sandbox-codex-session",
		Harness:   "codex-cli",
		Codex:     &manifest.Codex{SessionID: "native-codex-session"},
		Sandbox: &manifest.SandboxConfig{
			Enabled:      true,
			ExtraAddDirs: []string{extraAddDir},
		},
	}
	launch, _, _, err := prepareResumeLaunch(
		nil,
		m,
		"codex-cli",
		ResumeSessionHealth{
			TmuxSessionName: "sandbox-codex",
			WorktreePath:    t.TempDir(),
		},
	)
	if err != nil {
		t.Fatalf("prepareResumeLaunch() error: %v", err)
	}
	want := "--add-dir " + launchparity.ShellQuote(extraAddDir)
	if !strings.Contains(launch.Command, want) {
		t.Fatalf("prepareResumeLaunch() command = %q, want %q", launch.Command, want)
	}
}

func TestPrepareResumeLaunchDoesNotRestoreCodexPolicyWithoutEnabledSandbox(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	m := &manifest.Manifest{
		SessionID: "unsandboxed-codex-session",
		Harness:   "codex-cli",
		Codex:     &manifest.Codex{SessionID: "native-codex-session"},
		Sandbox: &manifest.SandboxConfig{
			ExtraAddDirs: []string{"/tmp/untrusted"},
		},
	}
	launch, _, _, err := prepareResumeLaunch(
		nil,
		m,
		"codex-cli",
		ResumeSessionHealth{
			TmuxSessionName: "unsandboxed-codex",
			WorktreePath:    t.TempDir(),
		},
	)
	if err != nil {
		t.Fatalf("prepareResumeLaunch() error: %v", err)
	}
	if strings.Contains(launch.Command, "--add-dir") {
		t.Fatalf("prepareResumeLaunch() command = %q, unexpectedly contains --add-dir", launch.Command)
	}
}

func TestPrepareResumeLaunchAuthorizesAgyWorktree(t *testing.T) {
	worktreePath := filepath.Join(t.TempDir(), "agy worktree")
	m := &manifest.Manifest{
		SessionID:      "agy-session",
		Harness:        "agy",
		PermissionMode: "auto",
		Agy:            &manifest.Agy{ConversationID: "agy-conversation"},
	}
	launch, _, _, err := prepareResumeLaunch(
		nil,
		m,
		"agy",
		ResumeSessionHealth{
			TmuxSessionName: "agy-session",
			WorktreePath:    worktreePath,
		},
	)
	if err != nil {
		t.Fatalf("prepareResumeLaunch() error: %v", err)
	}
	want := "--add-dir " + launchparity.ShellQuote(worktreePath)
	if !strings.Contains(launch.Command, want) {
		t.Fatalf("prepareResumeLaunch() command = %q, want %q", launch.Command, want)
	}
}

func TestResumeSessionCanonicalNameFailureRemovesExactColdRuntime(t *testing.T) {
	adapter, m, fakeTmux := setupResumeOperation(t, "opencode-cli", false)
	wantErr := errors.New("canonical name write failed")
	store := &failingBeginResumeStore{Adapter: adapter, err: wantErr}

	result, err := ResumeSession(&OpContext{Storage: store, Tmux: fakeTmux}, &ResumeSessionRequest{SessionID: m.SessionID})
	if !errors.Is(err, wantErr) {
		t.Fatalf("ResumeSession() error = %v, want %v", err, wantErr)
	}
	if result == nil || !fakeTmux.killed || fakeTmux.identityStillLive {
		t.Fatalf("canonical-name rollback = result %#v killed=%v live=%v", result, fakeTmux.killed, fakeTmux.identityStillLive)
	}
}

func TestResumeSessionCreationErrorWithIdentityStillRollsBackExactRuntime(t *testing.T) {
	adapter, m, fakeTmux := setupResumeOperation(t, "opencode-cli", false)
	wantErr := errors.New("tmux reply lost after creation")
	fakeTmux.createErr = wantErr

	result, err := ResumeSession(
		&OpContext{Storage: adapter, Tmux: fakeTmux},
		&ResumeSessionRequest{SessionID: m.SessionID},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("ResumeSession() error = %v, want %v", err, wantErr)
	}
	if result == nil || !result.CreatedTmux || !fakeTmux.killed || fakeTmux.identityStillLive {
		t.Fatalf("lost creation reply rollback = result %#v killed=%v live=%v", result, fakeTmux.killed, fakeTmux.identityStillLive)
	}
}

func TestResumeSessionJoinsExactRuntimeCleanupFailure(t *testing.T) {
	adapter, m, fakeTmux := setupResumeOperation(t, "opencode-cli", false)
	primaryErr := errors.New("readiness failed")
	cleanupErr := errors.New("tmux cleanup failed")
	fakeTmux.readinessErr = primaryErr
	fakeTmux.killErr = cleanupErr

	_, err := ResumeSession(
		&OpContext{Storage: adapter, Tmux: fakeTmux},
		&ResumeSessionRequest{SessionID: m.SessionID},
	)
	if !errors.Is(err, primaryErr) || !errors.Is(err, cleanupErr) {
		t.Fatalf("ResumeSession() error = %v, want primary and cleanup failures", err)
	}
	if fakeTmux.killed || !fakeTmux.identityStillLive {
		t.Fatalf("failed cleanup reported a false removal: killed=%v live=%v", fakeTmux.killed, fakeTmux.identityStillLive)
	}
}

func TestResumeSessionAmbiguousCanonicalNameCommitRestoresBeforeCleanup(t *testing.T) {
	adapter, m, fakeTmux := setupResumeOperation(t, "opencode-cli", false)
	wantErr := errors.New("canonical-name commit acknowledgement lost")
	store := &ambiguousBeginResumeStore{Adapter: adapter, err: wantErr}

	result, err := ResumeSession(
		&OpContext{Storage: store, Tmux: fakeTmux},
		&ResumeSessionRequest{SessionID: m.SessionID},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("ResumeSession() error = %v, want %v", err, wantErr)
	}
	if result == nil || !fakeTmux.killed {
		t.Fatalf("ambiguous canonical-name rollback = result %#v killed=%v", result, fakeTmux.killed)
	}
	stored, loadErr := adapter.GetSession(m.SessionID)
	if loadErr != nil {
		t.Fatalf("GetSession() error: %v", loadErr)
	}
	if stored.Tmux.SessionName != "" {
		t.Fatalf("canonical name after rollback = %q, want previous empty name", stored.Tmux.SessionName)
	}
}

func TestResumeSessionReloadFailureCompensatesMetadataBeforeCleanup(t *testing.T) {
	adapter, m, fakeTmux := setupResumeOperation(t, "opencode-cli", false)
	wantErr := errors.New("reload failed after canonical-name write")
	store := &failingReloadResumeStore{Adapter: adapter, err: wantErr}

	result, err := ResumeSession(
		&OpContext{Storage: store, Tmux: fakeTmux},
		&ResumeSessionRequest{SessionID: m.SessionID},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("ResumeSession() error = %v, want %v", err, wantErr)
	}
	if result == nil || !fakeTmux.killed {
		t.Fatalf("reload failure rollback = result %#v killed=%v", result, fakeTmux.killed)
	}
	stored, loadErr := adapter.GetSession(m.SessionID)
	if loadErr != nil {
		t.Fatalf("GetSession() error: %v", loadErr)
	}
	if stored.Tmux.SessionName != "" {
		t.Fatalf("canonical name after reload compensation = %q, want previous empty name", stored.Tmux.SessionName)
	}
}

func TestResumeSessionReportsMetadataOwnershipFinalizationFailure(t *testing.T) {
	tests := []struct {
		name      string
		completed bool
		err       error
	}{
		{name: "superseded", completed: false},
		{name: "backend failure", err: errors.New("complete failed")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, m, fakeTmux := setupResumeOperation(t, "opencode-cli", false)
			store := &completingResumeStore{Adapter: adapter, completed: tt.completed, err: tt.err}
			result, err := ResumeSession(
				&OpContext{Storage: store, Tmux: fakeTmux},
				&ResumeSessionRequest{SessionID: m.SessionID},
			)
			if err != nil {
				t.Fatalf("ResumeSession() error: %v", err)
			}
			if result == nil || len(result.Warnings) == 0 {
				t.Fatalf("ResumeSession() result = %#v, want ownership warning", result)
			}
		})
	}
}

func TestResumeSessionPreservesExistingRuntime(t *testing.T) {
	adapter, m, fakeTmux := setupResumeOperation(t, "claude-code", true)
	result, err := ResumeSession(&OpContext{Storage: adapter, Tmux: fakeTmux}, &ResumeSessionRequest{SessionID: m.SessionID})
	if err != nil {
		t.Fatalf("ResumeSession() error: %v", err)
	}
	if result.CreatedTmux || result.StartedHarness || fakeTmux.created != 0 || len(fakeTmux.commands) != 0 || fakeTmux.readinessCalls != 0 {
		t.Fatalf("existing runtime was mutated: result=%#v tmux=%#v", result, fakeTmux)
	}
}

func TestResumeSessionCodexPromptAcknowledgementLossIsIrreversibleSuccess(t *testing.T) {
	adapter, m, fakeTmux := setupResumeOperation(t, "codex-cli", true)
	ackErr := errors.New("prompt acknowledgement lost")
	fakeTmux.promptSubmission = session.PromptSubmission{MayHaveStarted: true}
	fakeTmux.promptErr = ackErr

	result, err := ResumeSession(
		&OpContext{Storage: adapter, Tmux: fakeTmux},
		&ResumeSessionRequest{SessionID: m.SessionID, Prompt: "continue"},
	)
	if err != nil {
		t.Fatalf("ResumeSession() error: %v", err)
	}
	if !result.PromptMayHaveStarted || len(result.Warnings) == 0 {
		t.Fatalf("ResumeSession() result = %#v, want irreversible warning", result)
	}
}

func TestResumeSessionCodexPromptPositiveFailurePreservesExistingRuntime(t *testing.T) {
	adapter, m, fakeTmux := setupResumeOperation(t, "codex-cli", true)
	wantErr := errors.New("prompt rejected")
	fakeTmux.promptErr = wantErr

	result, err := ResumeSession(
		&OpContext{Storage: adapter, Tmux: fakeTmux},
		&ResumeSessionRequest{SessionID: m.SessionID, Prompt: "continue"},
	)
	if err != nil {
		t.Fatalf("ResumeSession() error = %v, want existing runtime preserved", err)
	}
	if result == nil || result.PromptMayHaveStarted || len(result.Warnings) == 0 {
		t.Fatalf("ResumeSession() result = %#v, want pre-boundary warning", result)
	}
	if result.CreatedTmux || fakeTmux.killed {
		t.Fatalf("existing runtime was treated as transaction-owned: result=%#v killed=%v", result, fakeTmux.killed)
	}
}

func TestResumeSessionCodexPromptFailureRollsBackColdRuntime(t *testing.T) {
	adapter, m, fakeTmux := setupResumeOperation(t, "codex-cli", false)
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	wantErr := errors.New("prompt rejected")
	fakeTmux.promptErr = wantErr

	result, err := ResumeSession(
		&OpContext{Storage: adapter, Tmux: fakeTmux},
		&ResumeSessionRequest{SessionID: m.SessionID, Prompt: "continue"},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("ResumeSession() error = %v, want %v", err, wantErr)
	}
	if result == nil || !result.CreatedTmux || result.PromptMayHaveStarted || !fakeTmux.killed || fakeTmux.identityStillLive {
		t.Fatalf("cold prompt rollback = result %#v killed=%v live=%v", result, fakeTmux.killed, fakeTmux.identityStillLive)
	}
	stored, loadErr := adapter.GetSession(m.SessionID)
	if loadErr != nil {
		t.Fatalf("GetSession() error: %v", loadErr)
	}
	if stored.Tmux.SessionName != "" {
		t.Fatalf("stored tmux name = %q, want pre-resume empty name", stored.Tmux.SessionName)
	}
}

func TestResumeSessionIgnoresCancellationAfterPromptStarts(t *testing.T) {
	adapter, m, fakeTmux := setupResumeOperation(t, "codex-cli", true)
	ctx, cancel := context.WithCancel(t.Context())
	fakeTmux.promptSubmission = session.PromptSubmission{MayHaveStarted: true}
	fakeTmux.promptHook = cancel

	result, err := ResumeSession(
		&OpContext{Context: ctx, Storage: adapter, Tmux: fakeTmux},
		&ResumeSessionRequest{SessionID: m.SessionID, Prompt: "continue"},
	)
	if err != nil {
		t.Fatalf("ResumeSession() after prompt cancellation error: %v", err)
	}
	if !result.PromptMayHaveStarted {
		t.Fatalf("ResumeSession() result = %#v, want prompt boundary", result)
	}
}

func TestResumeSessionCancellationBeforePromptRollsBackColdRuntime(t *testing.T) {
	adapter, m, fakeTmux := setupResumeOperation(t, "opencode-cli", false)
	ctx, cancel := context.WithCancel(t.Context())
	fakeTmux.readinessHook = cancel

	result, err := ResumeSession(
		&OpContext{Context: ctx, Storage: adapter, Tmux: fakeTmux},
		&ResumeSessionRequest{SessionID: m.SessionID},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ResumeSession() error = %v, want context.Canceled", err)
	}
	if result == nil || !fakeTmux.killed || fakeTmux.identityStillLive {
		t.Fatalf("canceled cold resume = result %#v killed=%v live=%v", result, fakeTmux.killed, fakeTmux.identityStillLive)
	}
}

func TestSubmitAndAwaitResumeCancellationDoesNotLaunchPreparedCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	fakeTmux := &resumeTestTmux{}
	cancelCalls := 0
	err := submitAndAwaitResume(
		ctx,
		fakeTmux,
		nil,
		&ResumeSessionRequest{},
		&ResumeSessionResult{},
		"pi-cli",
		ResumeSessionHealth{TmuxSessionName: "pi-session", WorktreePath: t.TempDir()},
		"",
		HarnessLaunchCommand{
			Command: "pi --session session-id",
			Cancel: func() error {
				cancelCalls++
				return nil
			},
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("submitAndAwaitResume() error = %v, want context.Canceled", err)
	}
	if len(fakeTmux.commands) != 0 {
		t.Fatalf("submitted commands after cancellation: %v", fakeTmux.commands)
	}
	if cancelCalls != 1 {
		t.Fatalf("prepared launch cancellation calls = %d, want 1", cancelCalls)
	}
}

func TestResumeSessionPreservesColdRuntimeWhenMetadataCompensationIsUnproven(t *testing.T) {
	adapter, m, fakeTmux := setupResumeOperation(t, "codex-cli", false)
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	wantErr := errors.New("prompt rejected")
	fakeTmux.promptErr = wantErr
	store := &rejectingRestoreResumeStore{Adapter: adapter}

	result, err := ResumeSession(
		&OpContext{Storage: store, Tmux: fakeTmux},
		&ResumeSessionRequest{SessionID: m.SessionID, Prompt: "continue"},
	)
	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "metadata no longer matches") {
		t.Fatalf("ResumeSession() error = %v, want prompt plus ownership failure", err)
	}
	if result == nil || fakeTmux.killed || !fakeTmux.identityStillLive {
		t.Fatalf("unproven compensation destroyed runtime: result=%#v killed=%v live=%v", result, fakeTmux.killed, fakeTmux.identityStillLive)
	}
}

func TestResumeSessionAcquiresStableLockBeforeStorageRead(t *testing.T) {
	adapter, m, fakeTmux := setupResumeOperation(t, "opencode-cli", true)
	store := &countingResumeStore{Adapter: adapter}
	locked := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- WithSessionLockContext(context.Background(), m.SessionID, func() error {
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	_, err := ResumeSession(&OpContext{Context: ctx, Storage: store, Tmux: fakeTmux}, &ResumeSessionRequest{SessionID: m.SessionID})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ResumeSession() error = %v, want lock deadline", err)
	}
	if got := store.getCount(); got != 0 {
		t.Fatalf("storage reads before lock = %d, want 0", got)
	}
	close(release)
	if lockErr := <-done; lockErr != nil {
		t.Fatalf("lock holder error: %v", lockErr)
	}
}

func TestShouldSubmitResumeCommandClassifiesExistingPiPane(t *testing.T) {
	tests := []struct {
		name     string
		liveness session.LivenessInfo
		want     bool
		wantErr  bool
	}{
		{name: "exact Pi alive", liveness: session.LivenessInfo{SessionExists: true, HarnessAlive: true}, want: false},
		{name: "restartable shell", liveness: session.LivenessInfo{SessionExists: true, RestartableShell: true}, want: true},
		{name: "unrelated process", liveness: session.LivenessInfo{SessionExists: true, Evidence: "python"}, wantErr: true},
		{name: "zombie writer", liveness: session.LivenessInfo{SessionExists: true, ZombieWriter: true, Evidence: "agm"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeTmux := &resumeTestTmux{liveness: tt.liveness}
			got, err := shouldSubmitResumeCommand(t.Context(), fakeTmux, "pi-cli", ResumeSessionHealth{
				TmuxExists: true, TmuxSessionName: "pi-session",
			})
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("shouldSubmitResumeCommand() = (%v, %v), want (%v, err=%v)", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestResumeSessionAgyWorkspaceLockCoversSubmissionAndReadiness(t *testing.T) {
	adapter, m, fakeTmux := setupResumeOperation(t, "agy", false)
	m.Agy = &manifest.Agy{ConversationID: "agy-conversation-id"}
	if err := adapter.UpdateSession(m); err != nil {
		t.Fatalf("UpdateSession() error: %v", err)
	}
	locked := false
	locker := func(_ context.Context, workDir string) (func() error, error) {
		if workDir != m.Context.Project {
			t.Fatalf("AGY lock workdir = %q, want %q", workDir, m.Context.Project)
		}
		locked = true
		return func() error {
			locked = false
			return nil
		}, nil
	}
	fakeTmux.sendHook = func() {
		if !locked {
			t.Fatal("AGY command submitted outside workspace lock")
		}
	}
	fakeTmux.readinessHook = func() {
		if !locked {
			t.Fatal("AGY readiness observed outside workspace lock")
		}
	}

	if _, err := ResumeSession(
		&OpContext{Storage: adapter, Tmux: fakeTmux, AgyWorkspaceCreateLocker: locker},
		&ResumeSessionRequest{SessionID: m.SessionID},
	); err != nil {
		t.Fatalf("ResumeSession() error: %v", err)
	}
	if locked {
		t.Fatal("AGY workspace lock remained held after operation")
	}
}

func TestResumeSessionAgyPromptUsesHarnessAwareDelivery(t *testing.T) {
	adapter, m, fakeTmux := setupResumeOperation(t, "agy", true)
	m.Agy = &manifest.Agy{ConversationID: "agy-conversation-id"}
	if err := adapter.UpdateSession(m); err != nil {
		t.Fatalf("UpdateSession() error: %v", err)
	}
	const prompt = "line one\nline two"

	result, err := ResumeSession(
		&OpContext{Storage: adapter, Tmux: fakeTmux},
		&ResumeSessionRequest{SessionID: m.SessionID, Prompt: prompt},
	)
	if err != nil {
		t.Fatalf("ResumeSession() error: %v", err)
	}
	if !result.PromptMayHaveStarted {
		t.Fatalf("ResumeSession() result = %#v, want prompt boundary", result)
	}
	if fakeTmux.promptSession != m.Tmux.SessionName || fakeTmux.promptHarness != "agy" || fakeTmux.promptBody != prompt {
		t.Fatalf("prompt delivery = session %q harness %q body %q", fakeTmux.promptSession, fakeTmux.promptHarness, fakeTmux.promptBody)
	}
}

func TestResumeSessionMigratesAmbiguousAgyModelInsideOperation(t *testing.T) {
	adapter, m, fakeTmux := setupResumeOperation(t, "agy", true)
	m.Model = "gemini-2.5-flash"
	m.Agy = &manifest.Agy{ConversationID: "agy-conversation-id"}
	if err := adapter.UpdateSession(m); err != nil {
		t.Fatalf("UpdateSession() error: %v", err)
	}

	if _, err := ResumeSession(&OpContext{Storage: adapter, Tmux: fakeTmux}, &ResumeSessionRequest{SessionID: m.SessionID}); err != nil {
		t.Fatalf("ResumeSession() error: %v", err)
	}
	stored, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() error: %v", err)
	}
	if stored.Model != "" {
		t.Fatalf("stored ambiguous AGY model = %q, want empty", stored.Model)
	}
}

func TestResumeSessionAgyUnknownModelDoesNotInventOverride(t *testing.T) {
	adapter, m, fakeTmux := setupResumeOperation(t, "agy", false)
	m.Model = ""
	m.Agy = &manifest.Agy{ConversationID: "agy-conversation-id"}
	if err := adapter.UpdateSession(m); err != nil {
		t.Fatalf("UpdateSession() error: %v", err)
	}

	if _, err := ResumeSession(
		&OpContext{Storage: adapter, Tmux: fakeTmux},
		&ResumeSessionRequest{SessionID: m.SessionID},
	); err != nil {
		t.Fatalf("ResumeSession() error: %v", err)
	}
	if len(fakeTmux.commands) != 1 {
		t.Fatalf("resume commands = %v, want one AGY command", fakeTmux.commands)
	}
	if strings.Contains(fakeTmux.commands[0], "--model") {
		t.Fatalf("unknown AGY model gained an override: %q", fakeTmux.commands[0])
	}
	if !strings.Contains(fakeTmux.commands[0], "agy --conversation 'agy-conversation-id'") {
		t.Fatalf("AGY resume command = %q, want native conversation identity", fakeTmux.commands[0])
	}
}

func TestRestoreResumePermissionUsesVerifiedLiteralKeys(t *testing.T) {
	tests := []struct {
		name    string
		harness string
		mode    string
		want    int
	}{
		{name: "Claude default", harness: "claude-code", mode: "default"},
		{name: "Claude auto", harness: "claude-code", mode: "auto", want: 1},
		{name: "Claude plan", harness: "claude-code", mode: "plan", want: 2},
		{name: "Legacy defaulted Claude auto", harness: "", mode: "auto", want: 1},
		{name: "Claude invalid", harness: "claude-code", mode: "invalid"},
		{name: "Codex plan", harness: "codex-cli", mode: "plan"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeTmux := &resumeTestTmux{inputReadiness: session.InputReadiness{Ready: true, State: "YES"}}
			result := &ResumeSessionResult{}
			resolvedHarness := agent.NormalizeHarnessName(tt.harness)
			if resolvedHarness == "" {
				resolvedHarness = "claude-code"
			}
			restoreResumePermission(
				t.Context(),
				fakeTmux,
				resolvedHarness,
				&manifest.Manifest{Harness: tt.harness, PermissionMode: tt.mode},
				ResumeSessionHealth{TmuxSessionName: "resume-session"},
				result,
				&ResumeSessionRequest{},
			)
			if len(fakeTmux.literalKeys) != tt.want {
				t.Fatalf("permission restoration keys = %v, want %d", fakeTmux.literalKeys, tt.want)
			}
			for _, key := range fakeTmux.literalKeys {
				if key != "S-Tab" {
					t.Fatalf("permission restoration key = %q, want S-Tab", key)
				}
			}
		})
	}
}

func TestResumeEventObserverCannotAbortOperation(t *testing.T) {
	adapter, m, fakeTmux := setupResumeOperation(t, "opencode-cli", true)
	result, err := ResumeSession(
		&OpContext{Storage: adapter, Tmux: fakeTmux},
		&ResumeSessionRequest{
			SessionID: m.SessionID,
			OnEvent: func(ResumeSessionEvent) {
				panic("presentation failed")
			},
		},
	)
	if err != nil || result == nil {
		t.Fatalf("ResumeSession() = (%#v, %v), observer panic altered lifecycle", result, err)
	}
}

func containsString(values []string, want string) bool {
	return slices.Contains(values, want)
}
