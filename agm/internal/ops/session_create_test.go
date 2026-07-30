package ops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/agysession"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/shellquote"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// createMockStorage implements dolt.Storage for CreateSession tests.
type createMockStorage struct {
	created     []*manifest.Manifest
	deleted     []string
	sessions    []*manifest.Manifest
	listFilter  *dolt.SessionFilter
	listErr     error
	createErr   error
	reserveErr  error
	releaseErr  error
	deleteErr   error
	createOrder *[]string
	onCreate    func()
	onReserve   func()
	reserved    []string
	released    []string
}

type createOnlyTmux struct {
	session.TmuxInterface
}

type createFailingKillTmux struct {
	session.TmuxInterface
	err error
}

func (t *createFailingKillTmux) KillSession(string) error {
	return t.err
}

type createNoReadinessTmux struct {
	session.TmuxInterface
	kill func(string) error
}

func (t *createNoReadinessTmux) KillSession(name string) error {
	return t.kill(name)
}

type createReadinessTmux struct {
	*session.MockTmux
	order   *[]string
	waitErr error
	waitCtx context.Context
	atomic  func()
}

func (t *createReadinessTmux) SendKeys(sessionName, keys string) error {
	phase := "prompt"
	if len(t.SentCommands) == 0 {
		phase = "launch"
	}
	*t.order = append(*t.order, phase)
	return t.MockTmux.SendKeys(sessionName, keys)
}

func (t *createReadinessTmux) WaitForHarnessReady(ctx context.Context, sessionName, harness string, timeout time.Duration) error {
	t.waitCtx = ctx
	*t.order = append(*t.order, "ready")
	t.WaitedHarnesses = append(t.WaitedHarnesses, sessionName+":"+harness)
	if timeout != sharedHarnessReadyTimeout {
		return fmt.Errorf("readiness timeout = %v, want %v", timeout, sharedHarnessReadyTimeout)
	}
	return t.waitErr
}

func (t *createReadinessTmux) SendKeysIfInputReady(ctx context.Context, sessionName, harness, keys string, options session.InputDeliveryOptions) (session.InputReadiness, error) {
	if t.atomic != nil {
		t.atomic()
	}
	sentBefore := len(t.SentCommands)
	readiness, err := t.MockTmux.SendKeysIfInputReady(ctx, sessionName, harness, keys, options)
	if len(t.SentCommands) > sentBefore {
		*t.order = append(*t.order, "prompt")
	}
	return readiness, err
}

func TestWaitForCreatedHarnessReadyPropagatesRequestContext(t *testing.T) {
	type contextKey struct{}
	wantCtx := context.WithValue(context.Background(), contextKey{}, "request")
	var order []string
	tmuxMock := &createReadinessTmux{MockTmux: session.NewMockTmux(), order: &order}

	if err := waitForCreatedHarnessReady(wantCtx, &OpContext{Tmux: tmuxMock}, "worker", "codex-cli"); err != nil {
		t.Fatalf("waitForCreatedHarnessReady() error = %v", err)
	}
	if tmuxMock.waitCtx != wantCtx {
		t.Fatal("startup readiness did not receive the request context")
	}
}

type createTestRuntime struct {
	launch   func(context.Context, HarnessLaunchSpec) (CreateSessionLaunchResult, error)
	complete func(context.Context, CreateSessionCompletion) error
}

type createTestAgyBootstrapRuntime struct {
	*createTestRuntime
	bootstrap func(context.Context, AgyCreateIdentityBootstrap) error
}

func (r *createTestAgyBootstrapRuntime) BootstrapAgyCreateIdentity(ctx context.Context, input AgyCreateIdentityBootstrap) error {
	return r.bootstrap(ctx, input)
}

type createTestAgyIdentityTracker struct {
	snapshot func(context.Context, string) (string, error)
	discover func(context.Context, string, string) (*agysession.Metadata, error)
}

func (tracker *createTestAgyIdentityTracker) Snapshot(ctx context.Context, workDir string) (string, error) {
	return tracker.snapshot(ctx, workDir)
}

func (tracker *createTestAgyIdentityTracker) Discover(ctx context.Context, workDir, previousConversationID string) (*agysession.Metadata, error) {
	return tracker.discover(ctx, workDir, previousConversationID)
}

func successfulCreateTestAgyIdentityTracker() *createTestAgyIdentityTracker {
	return &createTestAgyIdentityTracker{
		snapshot: func(context.Context, string) (string, error) { return "previous-native-id", nil },
		discover: func(_ context.Context, workDir, _ string) (*agysession.Metadata, error) {
			return &agysession.Metadata{ConversationID: "new-native-id", WorkspacePath: workDir}, nil
		},
	}
}

func (r *createTestRuntime) Launch(ctx context.Context, spec HarnessLaunchSpec) (CreateSessionLaunchResult, error) {
	if r.launch == nil {
		return CreateSessionLaunchResult{}, nil
	}
	return r.launch(ctx, spec)
}

func (r *createTestRuntime) Complete(ctx context.Context, completion CreateSessionCompletion) error {
	if r.complete == nil {
		return nil
	}
	return r.complete(ctx, completion)
}

func (s *createMockStorage) CreateSession(m *manifest.Manifest) error {
	if s.createOrder != nil {
		*s.createOrder = append(*s.createOrder, "register")
	}
	s.created = append(s.created, m)
	if s.onCreate != nil {
		s.onCreate()
	}
	return s.createErr
}
func (s *createMockStorage) GetSession(string) (*manifest.Manifest, error) { return nil, nil }
func (s *createMockStorage) UpdateSession(*manifest.Manifest) error        { return nil }
func (s *createMockStorage) DeleteSession(id string) error {
	s.deleted = append(s.deleted, id)
	return s.deleteErr
}

func (s *createMockStorage) ListSessions(filter *dolt.SessionFilter) ([]*manifest.Manifest, error) {
	s.listFilter = filter
	if s.listErr != nil {
		return nil, s.listErr
	}
	results := make([]*manifest.Manifest, 0, len(s.sessions))
	for _, m := range s.sessions {
		if filter != nil && filter.ExcludeArchived && m.Lifecycle == manifest.LifecycleArchived {
			continue
		}
		results = append(results, m)
	}
	return results, nil
}

func (s *createMockStorage) GetSessionByUUID(string) (*manifest.Manifest, error) {
	return nil, nil
}

func (s *createMockStorage) RecordHarnessSwitch(string, string, string, time.Time) error {
	return nil
}

func (s *createMockStorage) GetHarnessHistory(string) ([]manifest.HarnessSwitch, error) {
	return nil, nil
}

func (s *createMockStorage) Create(*manifest.Manifest) error                     { return nil }
func (s *createMockStorage) Get(string) (*manifest.Manifest, error)              { return nil, nil }
func (s *createMockStorage) Update(*manifest.Manifest) error                     { return nil }
func (s *createMockStorage) Delete(string) error                                 { return nil }
func (s *createMockStorage) List(*manifest.Filter) ([]*manifest.Manifest, error) { return nil, nil }
func (s *createMockStorage) Close() error                                        { return nil }
func (s *createMockStorage) ApplyMigrations() error                              { return nil }

func (s *createMockStorage) ReserveSessionName(sessionID, name string) error {
	if s.createOrder != nil {
		*s.createOrder = append(*s.createOrder, "reserve")
	}
	s.reserved = append(s.reserved, sessionID+":"+name)
	if s.onReserve != nil {
		s.onReserve()
	}
	return s.reserveErr
}

func (s *createMockStorage) ReleaseSessionNameReservation(sessionID string) error {
	s.released = append(s.released, sessionID)
	return s.releaseErr
}

func testHarnessCommand(harness, model, sessionName, workDir string, persistent bool) string {
	return BuildHarnessLaunchCommand(HarnessLaunchSpec{
		Harness: harness, Model: model, SessionName: sessionName,
		WorkDir: workDir, Persistent: persistent, DisableOAuth: true,
	}).Command
}

func testHarnessCommandWithCodex(harness, model, sessionName, workDir string, persistent bool, codex *manifest.Codex) string {
	return BuildHarnessLaunchCommand(HarnessLaunchSpec{
		Harness: harness, Model: model, SessionName: sessionName,
		WorkDir: workDir, Persistent: persistent, Codex: codex, DisableOAuth: true,
	}).Command
}

func TestCreateSession_HappyPath(t *testing.T) {
	dir := t.TempDir()
	tmuxMock := session.NewMockTmux()
	store := &createMockStorage{}

	ctx := &OpContext{
		Tmux:       tmuxMock,
		Storage:    store,
		OutputMode: "json",
	}

	result, err := CreateSession(ctx, &CreateSessionRequest{
		Cwd:    dir,
		Prompt: "hello world",
		Title:  "test-session",
		Model:  "opus",
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if result.Operation != "create_session" {
		t.Errorf("operation = %q, want create_session", result.Operation)
	}
	if result.Name != "test-session" {
		t.Errorf("name = %q, want test-session", result.Name)
	}
	if result.SessionID == "" {
		t.Error("session_id is empty")
	}
	if result.Cwd != dir {
		t.Errorf("cwd = %q, want %q", result.Cwd, dir)
	}
	if result.Model != "opus" {
		t.Errorf("model = %q, want opus", result.Model)
	}
	if result.Harness != "claude-code" {
		t.Errorf("harness = %q, want claude-code", result.Harness)
	}
	if !result.Created {
		t.Error("created = false, want true")
	}

	// Verify tmux was called correctly
	if len(tmuxMock.CreatedSessions) != 1 || tmuxMock.CreatedSessions[0] != "test-session" {
		t.Errorf("tmux sessions created: %v", tmuxMock.CreatedSessions)
	}
	// 2 send-keys calls: harness command + prompt
	if len(tmuxMock.SentCommands) != 2 {
		t.Fatalf("tmux commands sent: %d, want 2 (harness + prompt)", len(tmuxMock.SentCommands))
	}
	if !strings.Contains(tmuxMock.SentCommands[0], "claude") {
		t.Errorf("first command should be claude harness, got: %s", tmuxMock.SentCommands[0])
	}
	if tmuxMock.SentCommands[1] != "hello world" {
		t.Errorf("second command should be prompt, got: %s", tmuxMock.SentCommands[1])
	}

	// Verify storage was called
	if len(store.created) != 1 {
		t.Fatalf("dolt sessions created: %d, want 1", len(store.created))
	}
	if store.created[0].Model != "opus" {
		t.Errorf("stored model = %q, want opus", store.created[0].Model)
	}
	if store.created[0].Harness != "claude-code" {
		t.Errorf("stored harness = %q, want claude-code", store.created[0].Harness)
	}
}

func TestCreateSession_AgyDetachedPromptUsesCanonicalCommand(t *testing.T) {
	dir := t.TempDir()
	tmuxMock := session.NewMockTmux()
	store := &createMockStorage{}
	tracker := successfulCreateTestAgyIdentityTracker()
	tracker.discover = func(_ context.Context, workDir, _ string) (*agysession.Metadata, error) {
		if len(tmuxMock.SentCommands) != 2 || tmuxMock.SentCommands[1] != "detached AGY prompt" {
			t.Fatalf("commands before identity discovery = %v, want launch then startup prompt", tmuxMock.SentCommands)
		}
		return &agysession.Metadata{ConversationID: "new-native-id", WorkspacePath: workDir}, nil
	}

	result, err := CreateSession(&OpContext{
		Tmux: tmuxMock, Storage: store, AgyCreateIdentityTracker: tracker,
	}, &CreateSessionRequest{
		Cwd: dir, Prompt: "detached AGY prompt", Title: "agy-detached",
		Harness: "agy", Model: "3.5-flash-low", PermissionMode: "auto",
		ExtraAddDirs: []string{"/tmp/extra dir"},
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if result.Harness != "agy" || result.Model != "3.5-flash-low" {
		t.Fatalf("result harness/model = %q/%q", result.Harness, result.Model)
	}
	if len(tmuxMock.SentCommands) != 2 {
		t.Fatalf("tmux commands = %v, want launch then detached prompt", tmuxMock.SentCommands)
	}
	launch := tmuxMock.SentCommands[0]
	for _, want := range []string{
		"agy --model 'Gemini 3.5 Flash (Low)'",
		"--dangerously-skip-permissions",
		"--add-dir '/tmp/extra dir'",
	} {
		if !strings.Contains(launch, want) {
			t.Errorf("AGY launch %q missing %q", launch, want)
		}
	}
	if strings.Contains(launch, "--prompt-interactive") {
		t.Errorf("AGY launch used prompt flag without a prompt: %q", launch)
	}
	if tmuxMock.SentCommands[1] != "detached AGY prompt" {
		t.Fatalf("startup prompt = %q", tmuxMock.SentCommands[1])
	}
	if len(store.created) != 1 || store.created[0].Harness != "agy" || store.created[0].Model != "3.5-flash-low" {
		t.Fatalf("stored AGY manifest = %+v", store.created)
	}
}

func TestIntegration_CreateSession_AgyBootstrapsLazyIdentityBeforeRegistrationExactlyOnce(t *testing.T) {
	dir := t.TempDir()
	tmuxMock := session.NewMockTmux()
	store := &createMockStorage{}
	locked := false
	promptDeliveries := 0
	order := []string{}
	runtime := &createTestAgyBootstrapRuntime{createTestRuntime: &createTestRuntime{}}
	runtime.launch = func(ctx context.Context, spec HarnessLaunchSpec) (CreateSessionLaunchResult, error) {
		if ctx != t.Context() || !locked || spec.SessionName != "agy-lazy-identity" {
			t.Fatalf("launch input = caller:%t locked:%t session:%q", ctx == t.Context(), locked, spec.SessionName)
		}
		order = append(order, "launch")
		return CreateSessionLaunchResult{}, nil
	}
	runtime.bootstrap = func(ctx context.Context, input AgyCreateIdentityBootstrap) error {
		if ctx != t.Context() || !locked || input.SessionName != "agy-lazy-identity" || input.Prompt != "persist me once" {
			t.Fatalf("bootstrap input = caller:%t locked:%t %+v", ctx == t.Context(), locked, input)
		}
		promptDeliveries++
		order = append(order, "prompt")
		return nil
	}
	runtime.complete = func(ctx context.Context, completion CreateSessionCompletion) error {
		if ctx != t.Context() || locked {
			t.Fatalf("completion input = caller:%t locked:%t", ctx == t.Context(), locked)
		}
		if !completion.Launch.PromptDelivered || completion.Prompt != "persist me once" {
			t.Fatalf("completion prompt state = delivered:%t prompt:%q", completion.Launch.PromptDelivered, completion.Prompt)
		}
		order = append(order, "complete")
		return nil
	}
	tracker := &createTestAgyIdentityTracker{
		snapshot: func(context.Context, string) (string, error) {
			order = append(order, "snapshot")
			return "old-native-id", nil
		},
		discover: func(_ context.Context, workDir, previous string) (*agysession.Metadata, error) {
			if !locked || promptDeliveries != 1 || previous != "old-native-id" {
				t.Fatalf("discovery state = locked:%t prompt deliveries:%d previous:%q", locked, promptDeliveries, previous)
			}
			order = append(order, "discover")
			return &agysession.Metadata{ConversationID: "new-native-id", WorkspacePath: workDir}, nil
		},
	}
	store.onReserve = func() {
		if !locked || promptDeliveries != 0 {
			t.Fatalf("reservation state = locked:%t prompt deliveries:%d", locked, promptDeliveries)
		}
		order = append(order, "reserve")
	}
	store.onCreate = func() {
		if !locked || promptDeliveries != 1 || store.created[0].Agy == nil || store.created[0].Agy.ConversationID != "new-native-id" {
			t.Fatalf("registration state = locked:%t prompt deliveries:%d manifest:%+v", locked, promptDeliveries, store.created[0])
		}
		order = append(order, "register")
	}

	result, err := CreateSessionWithContext(t.Context(), &OpContext{
		Tmux: tmuxMock, Storage: store, CreationRuntime: runtime,
		AgyCreateIdentityTracker: tracker,
		AgyWorkspaceCreateLocker: func(context.Context, string) (func() error, error) {
			locked = true
			order = append(order, "lock")
			return func() error {
				locked = false
				order = append(order, "unlock")
				return nil
			}, nil
		},
	}, &CreateSessionRequest{
		Cwd: dir, Prompt: "persist me once", Title: "agy-lazy-identity", Harness: "agy",
		Model: "3.5-flash-low", SessionID: "agy-lazy-id", RequireStorage: true,
	})
	if err != nil {
		t.Fatalf("CreateSessionWithContext: %v", err)
	}
	if result.SessionID != "agy-lazy-id" || promptDeliveries != 1 {
		t.Fatalf("result/prompt deliveries = %+v/%d", result, promptDeliveries)
	}
	wantOrder := []string{"lock", "snapshot", "reserve", "launch", "prompt", "discover", "register", "unlock", "complete"}
	if !reflect.DeepEqual(order, wantOrder) {
		t.Fatalf("AGY lazy identity lifecycle = %v, want %v", order, wantOrder)
	}
}

func TestCreateSession_AgyRejectsMissingIdentityBootstrapPromptBeforeMutation(t *testing.T) {
	for _, harness := range []string{"agy", "agy-cli", "antigravity"} {
		t.Run(harness, func(t *testing.T) {
			tmuxMock := session.NewMockTmux()
			_, err := CreateSessionWithContext(t.Context(), &OpContext{Tmux: tmuxMock}, &CreateSessionRequest{
				Cwd: t.TempDir(), Prompt: " \n\t", Title: "agy-no-prompt", Harness: harness, Model: "3.5-flash-low", AllowEmptyPrompt: true,
			})
			if err == nil || !strings.Contains(err.Error(), "startup prompt is required") {
				t.Fatalf("CreateSessionWithContext error = %v, want actionable AGY prompt requirement", err)
			}
			if len(tmuxMock.CreatedSessions) != 0 || len(tmuxMock.SentCommands) != 0 {
				t.Fatalf("missing-prompt create mutated tmux: created=%v sent=%v", tmuxMock.CreatedSessions, tmuxMock.SentCommands)
			}
		})
	}
}

func TestCreateSession_AgyBootstrapFailureRollsBackBeforeDiscoveryAndRegistration(t *testing.T) {
	dir := t.TempDir()
	tmuxMock := session.NewMockTmux()
	store := &createMockStorage{}
	discovered, completed := false, false
	wantErr := errors.New("fixture prompt delivery failed")
	runtime := &createTestAgyBootstrapRuntime{
		createTestRuntime: &createTestRuntime{complete: func(context.Context, CreateSessionCompletion) error {
			completed = true
			return nil
		}},
		bootstrap: func(context.Context, AgyCreateIdentityBootstrap) error { return wantErr },
	}
	tracker := successfulCreateTestAgyIdentityTracker()
	tracker.discover = func(context.Context, string, string) (*agysession.Metadata, error) {
		discovered = true
		return nil, nil
	}

	_, err := CreateSessionWithContext(t.Context(), &OpContext{
		Tmux: tmuxMock, Storage: store, CreationRuntime: runtime, AgyCreateIdentityTracker: tracker,
	}, &CreateSessionRequest{
		Cwd: dir, Prompt: "must roll back", Title: "agy-bootstrap-failure", Harness: "agy", Model: "3.5-flash-low",
	})
	if err == nil || !strings.Contains(err.Error(), wantErr.Error()) {
		t.Fatalf("CreateSessionWithContext error = %v, want %v", err, wantErr)
	}
	if discovered || completed || len(store.created) != 0 || len(store.released) != 1 || tmuxMock.Sessions["agy-bootstrap-failure"] {
		t.Fatalf("post-failure state = discovered:%t completed:%t registrations:%d released:%v tmux:%t", discovered, completed, len(store.created), store.released, tmuxMock.Sessions["agy-bootstrap-failure"])
	}
}

func TestCreateSession_AgyCancellationDuringIdentityBootstrapRollsBackWithCallerError(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	tmuxMock := session.NewMockTmux()
	store := &createMockStorage{}
	discovered, completed := false, false
	runtime := &createTestAgyBootstrapRuntime{
		createTestRuntime: &createTestRuntime{complete: func(context.Context, CreateSessionCompletion) error {
			completed = true
			return nil
		}},
		bootstrap: func(context.Context, AgyCreateIdentityBootstrap) error {
			cancel()
			return context.Canceled
		},
	}
	tracker := successfulCreateTestAgyIdentityTracker()
	tracker.discover = func(context.Context, string, string) (*agysession.Metadata, error) {
		discovered = true
		return nil, nil
	}

	_, err := CreateSessionWithContext(ctx, &OpContext{
		Tmux: tmuxMock, Storage: store, CreationRuntime: runtime, AgyCreateIdentityTracker: tracker,
	}, &CreateSessionRequest{
		Cwd: dir, Prompt: "cancel while sending", Title: "agy-bootstrap-cancel", Harness: "agy", Model: "3.5-flash-low",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CreateSessionWithContext error = %v, want context.Canceled", err)
	}
	if discovered || completed || len(store.created) != 0 || len(store.released) != 1 || tmuxMock.Sessions["agy-bootstrap-cancel"] {
		t.Fatalf("post-cancel state = discovered:%t completed:%t registrations:%d released:%v tmux:%t", discovered, completed, len(store.created), store.released, tmuxMock.Sessions["agy-bootstrap-cancel"])
	}
}

func TestCreateSessionPiPreparesExactNativeIdentityPolicyAndManifest(t *testing.T) {
	root := t.TempDir()
	extensionRoot := t.TempDir()
	t.Setenv("AGM_PI_SESSION_ROOT", root)
	t.Setenv("AGM_PI_EXTENSION_ROOT", extensionRoot)
	workDir := t.TempDir()
	codingAgentDir := filepath.Join(t.TempDir(), "pi agent")
	if err := os.Mkdir(codingAgentDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_CODING_AGENT_DIR", codingAgentDir)
	store := &createMockStorage{}
	tmuxMock := session.NewMockTmux()
	var launched HarnessLaunchSpec
	var completed *manifest.Manifest
	runtime := &createTestRuntime{
		launch: func(_ context.Context, spec HarnessLaunchSpec) (CreateSessionLaunchResult, error) {
			launched = spec
			return CreateSessionLaunchResult{ModeAppliedAtStartup: true}, nil
		},
		complete: func(_ context.Context, completion CreateSessionCompletion) error {
			completed = completion.Manifest
			return nil
		},
	}
	result, err := CreateSessionWithContext(t.Context(), &OpContext{
		Tmux: tmuxMock, Storage: store, CreationRuntime: runtime,
	}, &CreateSessionRequest{
		Cwd: workDir, Prompt: "hello", Title: "pi-worker", Harness: "pi", Model: "sonnet",
		SessionID: "pi-native-id", PermissionMode: "plan",
		Metadata: CreateSessionMetadata{PermissionPolicy: &manifest.PermissionPolicy{Allow: []string{"Read(/work/**)"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Harness != "pi-cli" || launched.Pi == nil {
		t.Fatalf("result/spec = %#v / %#v", result, launched)
	}
	if launched.Pi.SessionID != "pi-native-id" || launched.Pi.SessionDir != root {
		t.Fatalf("Pi identity = %#v", launched.Pi)
	}
	if launched.Pi.CodingAgentDir != codingAgentDir || !launched.Pi.CodingAgentDirSet {
		t.Fatalf("Pi coding agent directory = %q, want %q", launched.Pi.CodingAgentDir, codingAgentDir)
	}
	if launched.PiLaunchID == "" {
		t.Fatal("Pi creation omitted process launch identity")
	}
	if launched.PiPolicyJSON != `{"allow":["Read(/work/**)"]}` {
		t.Fatalf("Pi policy JSON = %q", launched.PiPolicyJSON)
	}
	policyData, readErr := os.ReadFile(launched.PiPolicyFile)
	if readErr != nil || string(policyData) != launched.PiPolicyJSON {
		t.Fatalf("Pi policy file = %q, read=%v data=%q", launched.PiPolicyFile, readErr, policyData)
	}
	if info, statErr := os.Stat(launched.PiExtension); statErr != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("Pi extension = %q, stat=%v info=%v", launched.PiExtension, statErr, info)
	}
	if completed == nil || completed.Pi == nil || completed.Pi.SessionID != "pi-native-id" || completed.Pi.CodingAgentDir != codingAgentDir || !completed.Pi.CodingAgentDirSet || completed.WorkingDirectory != workDir {
		t.Fatalf("Pi manifest = %#v", completed)
	}
}

func TestCreateSessionPiRejectsInvalidCodingAgentDirectoryBeforeTmux(t *testing.T) {
	t.Setenv("AGM_PI_SESSION_ROOT", t.TempDir())
	t.Setenv("AGM_PI_EXTENSION_ROOT", t.TempDir())
	t.Setenv("PI_CODING_AGENT_DIR", filepath.Join(t.TempDir(), "missing"))
	tmuxMock := session.NewMockTmux()
	_, err := CreateSessionWithContext(t.Context(), &OpContext{Tmux: tmuxMock}, &CreateSessionRequest{
		Cwd: t.TempDir(), Prompt: "fixture", Title: "pi-invalid-config", Harness: "pi", SessionID: "pi-invalid-config",
	})
	if err == nil || !strings.Contains(err.Error(), "coding agent directory") {
		t.Fatalf("CreateSessionWithContext error = %v", err)
	}
	if tmuxMock.Sessions["pi-invalid-config"] {
		t.Fatal("Pi tmux session was created before coding-agent directory validation")
	}
}

func TestBuildAgyResumeCommandPreservesModelConversationAndMode(t *testing.T) {
	command := BuildAgyResumeCommand(HarnessLaunchSpec{
		Harness: "agy", Model: "claude-sonnet-4.6-thinking", WorkDir: "/tmp/agy resume",
		PermissionMode: "auto", ExtraAddDirs: []string{"/tmp/agy resume"},
	}, "117ff898-a964-4a9f-b460-1be4a8a49b17").Command
	for _, want := range []string{
		"cd '/tmp/agy resume' && agy --model 'Claude Sonnet 4.6 (Thinking)'",
		"--dangerously-skip-permissions",
		"--conversation '117ff898-a964-4a9f-b460-1be4a8a49b17'",
		"--add-dir '/tmp/agy resume'",
		"&& exit",
	} {
		if !strings.Contains(command, want) {
			t.Errorf("resume command %q missing %q", command, want)
		}
	}
}

func TestCreateSession_DefaultsModelAndHarness(t *testing.T) {
	dir := t.TempDir()
	tmuxMock := session.NewMockTmux()

	ctx := &OpContext{
		Tmux:       tmuxMock,
		OutputMode: "json",
	}

	result, err := CreateSession(ctx, &CreateSessionRequest{
		Cwd:    dir,
		Prompt: "test",
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if result.Model != "sonnet" {
		t.Errorf("default model = %q, want sonnet", result.Model)
	}
	if result.Harness != "claude-code" {
		t.Errorf("default harness = %q, want claude-code", result.Harness)
	}
}

func TestCreateSession_DefaultsModelPerHarness(t *testing.T) {
	tests := []struct {
		harness string
		want    string
	}{
		{"codex-cli", "5.5"},
		{"agy", "3.5-flash"},
		{"opencode-cli", "glm-5.2"},
	}
	for _, tt := range tests {
		t.Run(tt.harness, func(t *testing.T) {
			// Isolate the codex trust pre-write from the developer's real ~/.codex.
			t.Setenv("CODEX_HOME", t.TempDir())
			opCtx := &OpContext{Tmux: session.NewMockTmux(), OutputMode: "json"}
			if tt.harness == "agy" {
				opCtx.AgyCreateIdentityTracker = successfulCreateTestAgyIdentityTracker()
			}
			result, err := CreateSession(opCtx, &CreateSessionRequest{
				Cwd:     t.TempDir(),
				Prompt:  "test",
				Title:   "session-" + strings.ReplaceAll(tt.harness, "-", "_"),
				Harness: tt.harness,
			})
			if err != nil {
				t.Fatalf("CreateSession: %v", err)
			}
			if result.Model != tt.want {
				t.Errorf("default model = %q, want %q", result.Model, tt.want)
			}
		})
	}
}

func TestCreateSession_DerivesNameFromCwd(t *testing.T) {
	dir := t.TempDir()
	dirName := filepath.Base(dir)
	tmuxMock := session.NewMockTmux()

	ctx := &OpContext{Tmux: tmuxMock, OutputMode: "json"}

	result, err := CreateSession(ctx, &CreateSessionRequest{
		Cwd:    dir,
		Prompt: "test",
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	want := "mcp-" + dirName
	if result.Name != want {
		t.Errorf("derived name = %q, want %q", result.Name, want)
	}
}

func TestCreateSession_RejectsEmptyCwd(t *testing.T) {
	ctx := &OpContext{Tmux: session.NewMockTmux(), OutputMode: "json"}
	_, err := CreateSession(ctx, &CreateSessionRequest{Prompt: "test"})
	if err == nil {
		t.Fatal("expected error for empty cwd")
	}
	var opErr *OpError
	if !errors.As(err, &opErr) {
		t.Fatalf("expected *OpError, got %T", err)
	}
	if opErr.Code != ErrCodeInvalidInput {
		t.Errorf("code = %q, want %q", opErr.Code, ErrCodeInvalidInput)
	}
}

func TestCreateSession_RejectsRelativeCwd(t *testing.T) {
	ctx := &OpContext{Tmux: session.NewMockTmux(), OutputMode: "json"}
	_, err := CreateSession(ctx, &CreateSessionRequest{
		Cwd:    "relative/path",
		Prompt: "test",
	})
	if err == nil {
		t.Fatal("expected error for relative cwd")
	}
	var opErr *OpError
	if !errors.As(err, &opErr) {
		t.Fatalf("expected *OpError, got %T", err)
	}
	if !strings.Contains(opErr.Detail, "absolute path") {
		t.Errorf("error should mention absolute path, got: %s", opErr.Detail)
	}
}

func TestCreateSession_GeminiAllowsControlWorkdirWhenNoRepairIsNeeded(t *testing.T) {
	workdir := filepath.Join(t.TempDir(), "valid\tpath")
	if err := os.Mkdir(workdir, 0o700); err != nil {
		t.Fatal(err)
	}
	tmuxMock := session.NewMockTmux()

	result, err := CreateSession(&OpContext{Tmux: tmuxMock}, &CreateSessionRequest{
		Cwd:     workdir,
		Prompt:  "test",
		Title:   "safe-title",
		Harness: "gemini-cli",
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v, want healthy non-repair creation", err)
	}
	if result == nil || len(tmuxMock.CreatedSessions) != 1 {
		t.Fatalf("CreateSession() result = %#v, sessions = %v", result, tmuxMock.CreatedSessions)
	}
}

func TestCreateSession_RejectsEmptyPrompt(t *testing.T) {
	dir := t.TempDir()
	ctx := &OpContext{Tmux: session.NewMockTmux(), OutputMode: "json"}
	_, err := CreateSession(ctx, &CreateSessionRequest{Cwd: dir})
	if err == nil {
		t.Fatal("expected error for empty prompt")
	}
}

func TestCreateSession_RejectsNonexistentCwd(t *testing.T) {
	ctx := &OpContext{Tmux: session.NewMockTmux(), OutputMode: "json"}
	_, err := CreateSession(ctx, &CreateSessionRequest{
		Cwd:    "/nonexistent/path/xyz",
		Prompt: "test",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent cwd")
	}
}

func TestCreateSession_RejectsFileCwd(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	ctx := &OpContext{Tmux: session.NewMockTmux(), OutputMode: "json"}
	_, err = CreateSession(ctx, &CreateSessionRequest{
		Cwd:    f.Name(),
		Prompt: "test",
	})
	if err == nil {
		t.Fatal("expected error for file (not dir) cwd")
	}
}

func TestCreateSession_RejectsInvalidHarness(t *testing.T) {
	dir := t.TempDir()
	ctx := &OpContext{Tmux: session.NewMockTmux(), OutputMode: "json"}
	_, err := CreateSession(ctx, &CreateSessionRequest{
		Cwd:     dir,
		Prompt:  "test",
		Harness: "unknown-harness",
	})
	if err == nil {
		t.Fatal("expected error for unsupported harness")
	}
	var opErr *OpError
	if !errors.As(err, &opErr) {
		t.Fatalf("expected *OpError, got %T", err)
	}
	if !strings.Contains(opErr.Detail, "Unsupported harness") {
		t.Errorf("error should mention unsupported harness, got: %s", opErr.Detail)
	}
}

func TestCreateSession_RejectsInvalidModelChars(t *testing.T) {
	dir := t.TempDir()
	ctx := &OpContext{Tmux: session.NewMockTmux(), OutputMode: "json"}
	_, err := CreateSession(ctx, &CreateSessionRequest{
		Cwd:    dir,
		Prompt: "test",
		Model:  "opus; rm -rf /",
	})
	if err == nil {
		t.Fatal("expected error for model with invalid characters")
	}
	var opErr *OpError
	if !errors.As(err, &opErr) {
		t.Fatalf("expected *OpError, got %T", err)
	}
	if !strings.Contains(opErr.Detail, "disallowed character") {
		t.Errorf("error should mention disallowed character, got: %s", opErr.Detail)
	}
}

func TestCreateSession_RejectsInvalidTitleChars(t *testing.T) {
	dir := t.TempDir()
	ctx := &OpContext{Tmux: session.NewMockTmux(), OutputMode: "json"}
	_, err := CreateSession(ctx, &CreateSessionRequest{
		Cwd:    dir,
		Prompt: "test",
		Title:  "bad title with spaces",
	})
	if err == nil {
		t.Fatal("expected error for title with invalid characters")
	}
	var opErr *OpError
	if !errors.As(err, &opErr) {
		t.Fatalf("expected *OpError, got %T", err)
	}
	if !strings.Contains(opErr.Detail, "invalid characters") {
		t.Errorf("error should mention invalid characters, got: %s", opErr.Detail)
	}
}

func TestCreateSession_AcceptsValidModel(t *testing.T) {
	dir := t.TempDir()
	tmuxMock := session.NewMockTmux()
	ctx := &OpContext{Tmux: tmuxMock, OutputMode: "json"}

	result, err := CreateSession(ctx, &CreateSessionRequest{
		Cwd:    dir,
		Prompt: "test",
		Model:  "claude-3.5-sonnet",
	})
	if err != nil {
		t.Fatalf("CreateSession with dotted model: %v", err)
	}
	if result.Model != "claude-3.5-sonnet" {
		t.Errorf("model = %q, want claude-3.5-sonnet", result.Model)
	}
}

func TestCreateSession_AcceptsRegistryModelIdentifier(t *testing.T) {
	dir := t.TempDir()
	tmuxMock := session.NewMockTmux()
	ctx := &OpContext{Tmux: tmuxMock, OutputMode: "json"}

	result, err := CreateSession(ctx, &CreateSessionRequest{
		Cwd:     dir,
		Prompt:  "test",
		Harness: "opencode-cli",
		Model:   "z-ai/glm-5.2",
	})
	if err != nil {
		t.Fatalf("CreateSession with slash model: %v", err)
	}
	if result.Model != "z-ai/glm-5.2" {
		t.Errorf("model = %q, want z-ai/glm-5.2", result.Model)
	}
}

func TestCreateSession_RejectsDuplicateTmuxSession(t *testing.T) {
	dir := t.TempDir()
	tmuxMock := session.NewMockTmux()
	tmuxMock.Sessions["existing"] = true

	ctx := &OpContext{Tmux: tmuxMock, OutputMode: "json"}
	_, err := CreateSession(ctx, &CreateSessionRequest{
		Cwd:    dir,
		Prompt: "test",
		Title:  "existing",
	})
	if err == nil {
		t.Fatal("expected error for duplicate tmux session")
	}
	var opErr *OpError
	if !errors.As(err, &opErr) {
		t.Fatalf("expected *OpError, got %T", err)
	}
	if opErr.Code != ErrCodeSessionExists {
		t.Errorf("code = %q, want %q", opErr.Code, ErrCodeSessionExists)
	}
}

func TestCreateSession_RejectsDuplicateStoredSessionNameBeforeTmuxCreate(t *testing.T) {
	dir := t.TempDir()
	tmuxMock := session.NewMockTmux()
	store := &createMockStorage{
		sessions: []*manifest.Manifest{
			newManifest("old-id", "existing", dir),
		},
	}

	ctx := &OpContext{Tmux: tmuxMock, Storage: store, OutputMode: "json"}
	_, err := CreateSession(ctx, &CreateSessionRequest{
		Cwd:    dir,
		Prompt: "test",
		Title:  "existing",
	})
	if err == nil {
		t.Fatal("expected error for duplicate stored session name")
	}
	var opErr *OpError
	if !errors.As(err, &opErr) {
		t.Fatalf("expected *OpError, got %T", err)
	}
	if opErr.Code != ErrCodeSessionExists {
		t.Errorf("code = %q, want %q", opErr.Code, ErrCodeSessionExists)
	}
	if opErr.Detail != sessionNameExistsMessage("existing") {
		t.Errorf("detail = %q, want %q", opErr.Detail, sessionNameExistsMessage("existing"))
	}
	if store.listFilter == nil || !store.listFilter.ExcludeArchived {
		t.Fatalf("ListSessions filter = %#v, want ExcludeArchived", store.listFilter)
	}
	if len(tmuxMock.CreatedSessions) != 0 {
		t.Fatalf("tmux sessions created before duplicate rejection: %v", tmuxMock.CreatedSessions)
	}
	if len(store.created) != 0 {
		t.Fatalf("stored sessions created after duplicate rejection: %d", len(store.created))
	}
}

func TestCreateSession_AtomicNameConflictPreventsQueuedCurrentTmuxLaunch(t *testing.T) {
	tmuxMock := session.NewMockTmux()
	tmuxMock.Sessions["concurrent-name"] = true
	store := &createMockStorage{
		reserveErr: &dolt.SessionNameConflictError{Name: "concurrent-name"},
	}
	launched := false
	runtime := &createTestRuntime{
		launch: func(context.Context, HarnessLaunchSpec) (CreateSessionLaunchResult, error) {
			launched = true
			return CreateSessionLaunchResult{Readiness: CreateSessionReadinessDeferredUntilCallerExit}, nil
		},
	}

	_, err := CreateSessionWithContext(t.Context(), &OpContext{
		Tmux: tmuxMock, Storage: store, CreationRuntime: runtime,
	}, &CreateSessionRequest{
		Cwd: t.TempDir(), Title: "concurrent-name", Harness: "claude-code", Model: "sonnet",
		Caller: CreateSessionCaller{Surface: CreateSurfaceCLI}, AllowEmptyPrompt: true,
		ReuseExistingTmux: true, RequireStorage: true,
	})
	var opErr *OpError
	if !errors.As(err, &opErr) || opErr.Code != ErrCodeSessionExists {
		t.Fatalf("CreateSessionWithContext() error = %v, want %s", err, ErrCodeSessionExists)
	}
	if launched || len(tmuxMock.SentCommands) != 0 {
		t.Fatalf("losing creator launched harness: runtime=%t commands=%v", launched, tmuxMock.SentCommands)
	}
	if len(store.created) != 0 {
		t.Fatalf("losing creator reached registration: %d writes", len(store.created))
	}
	if !tmuxMock.Sessions["concurrent-name"] {
		t.Fatal("name-conflict rollback removed the reused tmux session")
	}
}

func TestCreateSession_ConcurrentCurrentTmuxCreatorsLaunchExactlyOnce(t *testing.T) {
	tmuxMock := session.NewMockTmux()
	tmuxMock.Sessions["concurrent-name"] = true
	store := dolt.NewMockAdapter()
	workDir := t.TempDir()
	var launches atomic.Int32
	runtime := &createTestRuntime{
		launch: func(context.Context, HarnessLaunchSpec) (CreateSessionLaunchResult, error) {
			launches.Add(1)
			return CreateSessionLaunchResult{Readiness: CreateSessionReadinessDeferredUntilCallerExit}, nil
		},
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	for _, sessionID := range []string{"creator-a", "creator-b"} {
		go func(id string) {
			<-start
			_, err := CreateSessionWithContext(t.Context(), &OpContext{
				Tmux: tmuxMock, Storage: store, CreationRuntime: runtime,
			}, &CreateSessionRequest{
				Cwd: workDir, Title: "concurrent-name", Harness: "claude-code", Model: "sonnet",
				SessionID: id, Caller: CreateSessionCaller{Surface: CreateSurfaceCLI},
				AllowEmptyPrompt: true, ReuseExistingTmux: true, RequireStorage: true,
			})
			results <- err
		}(sessionID)
	}
	close(start)

	var successes, conflicts int
	for range 2 {
		err := <-results
		if err == nil {
			successes++
			continue
		}
		var opErr *OpError
		if errors.As(err, &opErr) && opErr.Code == ErrCodeSessionExists {
			conflicts++
			continue
		}
		t.Fatalf("concurrent creator error = %v", err)
	}
	if successes != 1 || conflicts != 1 || launches.Load() != 1 {
		t.Fatalf(
			"concurrent creators = %d success, %d conflict, %d launch; want 1, 1, 1",
			successes,
			conflicts,
			launches.Load(),
		)
	}
}

func TestCreateSession_FailsWithoutTmux(t *testing.T) {
	dir := t.TempDir()
	ctx := &OpContext{OutputMode: "json"}
	_, err := CreateSession(ctx, &CreateSessionRequest{
		Cwd:    dir,
		Prompt: "test",
	})
	if err == nil {
		t.Fatal("expected error when Tmux is nil")
	}
}

func TestCreateSession_NilRequest(t *testing.T) {
	ctx := &OpContext{Tmux: session.NewMockTmux(), OutputMode: "json"}
	_, err := CreateSession(ctx, nil)
	if err == nil {
		t.Fatal("expected error for nil request")
	}
}

func TestCreateSession_WorksWithoutStorage(t *testing.T) {
	dir := t.TempDir()
	tmuxMock := session.NewMockTmux()

	ctx := &OpContext{
		Tmux:       tmuxMock,
		OutputMode: "json",
	}

	result, err := CreateSession(ctx, &CreateSessionRequest{
		Cwd:    dir,
		Prompt: "test",
		Title:  "no-storage",
	})
	if err != nil {
		t.Fatalf("CreateSession without storage: %v", err)
	}
	if !result.Created {
		t.Error("created = false, want true")
	}
}

func TestCreateSession_RequiresRollbackCapableTmuxBeforeCreate(t *testing.T) {
	tmuxMock := session.NewMockTmux()
	_, err := CreateSession(&OpContext{Tmux: &createOnlyTmux{TmuxInterface: tmuxMock}}, &CreateSessionRequest{
		Cwd: t.TempDir(), Prompt: "test", Title: "no-rollback",
	})
	if err == nil || !strings.Contains(err.Error(), "KillSession") {
		t.Fatalf("error = %v, want rollback capability failure", err)
	}
	if len(tmuxMock.CreatedSessions) != 0 {
		t.Fatalf("tmux was mutated before rollback capability validation: %v", tmuxMock.CreatedSessions)
	}
}

func TestCreateSession_LifecycleOrder(t *testing.T) {
	dir := t.TempDir()
	manifestDir := filepath.Join(t.TempDir(), "ordered")
	var order []string
	store := &createMockStorage{createOrder: &order}
	tmuxMock := &createReadinessTmux{MockTmux: session.NewMockTmux(), order: &order}
	runtime := &createTestRuntime{
		launch: func(context.Context, HarnessLaunchSpec) (CreateSessionLaunchResult, error) {
			if !tmuxMock.Sessions["ordered"] {
				t.Fatal("runtime launch ran before tmux creation")
			}
			order = append(order, "launch")
			return CreateSessionLaunchResult{}, nil
		},
		complete: func(context.Context, CreateSessionCompletion) error {
			order = append(order, "complete")
			return nil
		},
	}

	opCtx := &OpContext{
		Tmux:            tmuxMock,
		CreationRuntime: runtime,
		OpenSessionStorage: func(context.Context) (dolt.Storage, func(), error) {
			order = append(order, "storage")
			return store, func() { order = append(order, "cleanup") }, nil
		},
	}
	_, err := CreateSessionWithContext(context.Background(), opCtx, &CreateSessionRequest{
		Cwd: dir, Title: "ordered", Model: "sonnet", Harness: "claude-code",
		AllowEmptyPrompt: true, RequireStorage: true, ManifestDir: manifestDir,
	})
	if err != nil {
		t.Fatalf("CreateSessionWithContext: %v", err)
	}
	want := []string{"storage", "reserve", "launch", "ready", "register", "complete", "cleanup"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("lifecycle order = %v, want %v", order, want)
	}
}

func TestCreateSession_NoRuntimeInitialPromptRevalidatesAfterRegistration(t *testing.T) {
	t.Parallel()

	callerCtx := t.Context()
	var order []string
	registered := false
	tmuxMock := &createReadinessTmux{MockTmux: session.NewMockTmux(), order: &order}
	tmuxMock.InputReadiness = session.InputReadiness{State: "PERMISSION", PaneID: "%42"}
	tmuxMock.atomic = func() {
		if !registered {
			t.Fatal("atomic initial-prompt readiness ran before registration")
		}
	}
	store := &createMockStorage{createOrder: &order, onCreate: func() { registered = true }}

	_, err := CreateSessionWithContext(callerCtx, &OpContext{Tmux: tmuxMock, Storage: store}, &CreateSessionRequest{
		Cwd: t.TempDir(), Title: "atomic-prompt", SessionID: "atomic-prompt-id",
		Harness: "claude-code", Model: "sonnet", Prompt: "do not inject",
	})
	if err == nil || !strings.Contains(err.Error(), "harness input is PERMISSION") {
		t.Fatalf("CreateSessionWithContext() error = %v, want post-registration permission rejection", err)
	}
	if !reflect.DeepEqual(tmuxMock.AtomicInputChecks, []string{"atomic-prompt:claude-code"}) {
		t.Fatalf("atomic input checks = %v", tmuxMock.AtomicInputChecks)
	}
	if tmuxMock.InputContext != callerCtx {
		t.Fatal("atomic initial-prompt readiness did not receive the caller context")
	}
	if len(tmuxMock.SentCommands) != 1 || len(tmuxMock.ExactPaneDeliveries) != 0 {
		t.Fatalf("tmux deliveries = commands %v panes %v, want launch only", tmuxMock.SentCommands, tmuxMock.ExactPaneDeliveries)
	}
	if !reflect.DeepEqual(store.deleted, []string{"atomic-prompt-id"}) || tmuxMock.Sessions["atomic-prompt"] {
		t.Fatalf("failed prompt rollback = deleted %v sessionExists %v", store.deleted, tmuxMock.Sessions["atomic-prompt"])
	}
}

func TestCreateSession_NoRuntimeInitialPromptUsesAtomicExactPaneDelivery(t *testing.T) {
	t.Parallel()

	callerCtx := t.Context()
	var order []string
	registered := false
	tmuxMock := &createReadinessTmux{MockTmux: session.NewMockTmux(), order: &order}
	tmuxMock.InputReadiness = session.InputReadiness{Ready: true, State: "YES", PaneID: "%42"}
	tmuxMock.atomic = func() {
		if !registered {
			t.Fatal("atomic initial-prompt readiness ran before registration")
		}
	}
	store := &createMockStorage{createOrder: &order, onCreate: func() { registered = true }}

	_, err := CreateSessionWithContext(callerCtx, &OpContext{Tmux: tmuxMock, Storage: store}, &CreateSessionRequest{
		Cwd: t.TempDir(), Title: "atomic-prompt", SessionID: "atomic-prompt-id",
		Harness: "claude-code", Model: "sonnet", Prompt: "deliver exactly once",
	})
	if err != nil {
		t.Fatalf("CreateSessionWithContext() error = %v", err)
	}
	if !reflect.DeepEqual(tmuxMock.AtomicInputChecks, []string{"atomic-prompt:claude-code"}) ||
		!reflect.DeepEqual(tmuxMock.ExactPaneDeliveries, []string{"%42"}) {
		t.Fatalf("atomic input = checks %v panes %v", tmuxMock.AtomicInputChecks, tmuxMock.ExactPaneDeliveries)
	}
	if tmuxMock.InputContext != callerCtx || tmuxMock.PaneSendContext != callerCtx {
		t.Fatal("atomic initial-prompt delivery did not retain the caller context")
	}
	if len(tmuxMock.SentCommands) != 2 || tmuxMock.SentCommands[1] != "deliver exactly once" {
		t.Fatalf("tmux commands = %v, want launch then exact initial prompt", tmuxMock.SentCommands)
	}
}

func TestEstablishCreatedHarnessReadinessAllowsOnlyQueuedCurrentTmuxDeferral(t *testing.T) {
	t.Parallel()

	validRequest := &CreateSessionRequest{
		Caller: CreateSessionCaller{Surface: CreateSurfaceCLI}, ReuseExistingTmux: true,
	}
	validParams := &createSessionParams{name: "current", harness: "codex-cli"}
	launch := CreateSessionLaunchResult{Readiness: CreateSessionReadinessDeferredUntilCallerExit}
	for _, harness := range []string{"claude-code", "codex-cli", "opencode-cli", "pi-cli", "gemini-cli"} {
		params := &createSessionParams{name: "current", harness: harness}
		if err := establishCreatedHarnessReadiness(t.Context(), &OpContext{Tmux: session.NewMockTmux()}, validRequest, params, launch); err != nil {
			t.Fatalf("valid current-tmux %s deferral: %v", harness, err)
		}
	}

	tests := []struct {
		name    string
		request CreateSessionRequest
		params  createSessionParams
	}{
		{name: "MCP surface", request: CreateSessionRequest{Caller: CreateSessionCaller{Surface: CreateSurfaceMCP}, ReuseExistingTmux: true}, params: *validParams},
		{name: "unsupported harness", request: *validRequest, params: createSessionParams{name: "current", harness: "agy"}},
		{name: "new tmux", request: CreateSessionRequest{Caller: CreateSessionCaller{Surface: CreateSurfaceCLI}}, params: *validParams},
		{name: "initial prompt", request: CreateSessionRequest{Caller: CreateSessionCaller{Surface: CreateSurfaceCLI}, ReuseExistingTmux: true, Prompt: "must wait"}, params: *validParams},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := establishCreatedHarnessReadiness(t.Context(), &OpContext{Tmux: session.NewMockTmux()}, &tt.request, &tt.params, launch)
			if err == nil || !strings.Contains(err.Error(), "deferred readiness") {
				t.Fatalf("establishCreatedHarnessReadiness() error = %v, want invalid deferral", err)
			}
		})
	}
}

func TestCreateSession_AgyWorkspaceLockReleasesBeforeSurfaceCompletion(t *testing.T) {
	dir, err := agysession.CanonicalWorkspacePath(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	aliasDir := filepath.Join(t.TempDir(), "workspace-alias")
	if err := os.Symlink(dir, aliasDir); err != nil {
		t.Fatalf("create workspace symlink: %v", err)
	}
	var order []string
	locked := false
	store := &createMockStorage{}
	store.onReserve = func() {
		if !locked {
			t.Fatal("AGY workspace lock was released before reservation")
		}
		order = append(order, "reserve")
	}
	store.onCreate = func() {
		if !locked {
			t.Fatal("AGY workspace lock was released before registration")
		}
		created := store.created[len(store.created)-1]
		if created.Agy == nil || created.Agy.ConversationID != "new-native-id" {
			t.Fatalf("registered AGY identity = %+v, want new-native-id", created.Agy)
		}
		if created.WorkingDirectory != dir || created.Agy.WorkspacePath != dir {
			t.Fatalf("registered canonical workspace = manifest %q AGY %q, want %q", created.WorkingDirectory, created.Agy.WorkspacePath, dir)
		}
	}
	runtime := &createTestRuntime{
		launch: func(_ context.Context, spec HarnessLaunchSpec) (CreateSessionLaunchResult, error) {
			if !locked {
				t.Fatal("AGY workspace lock was not held during launch")
			}
			if spec.WorkDir != dir {
				t.Fatalf("AGY launch workspace = %q, want canonical %q", spec.WorkDir, dir)
			}
			order = append(order, "launch")
			return CreateSessionLaunchResult{}, nil
		},
		complete: func(context.Context, CreateSessionCompletion) error {
			if locked {
				t.Fatal("AGY workspace lock remained held during surface completion")
			}
			order = append(order, "complete")
			return nil
		},
	}
	opCtx := &OpContext{
		Tmux: session.NewMockTmux(), Storage: store, CreationRuntime: runtime,
		AgyCreateIdentityTracker: &createTestAgyIdentityTracker{
			snapshot: func(ctx context.Context, workDir string) (string, error) {
				if !locked || ctx != t.Context() || workDir != dir {
					t.Fatalf("AGY snapshot input = locked:%t %v/%q, want locked caller context/%q", locked, ctx, workDir, dir)
				}
				order = append(order, "snapshot")
				return "previous-native-id", nil
			},
			discover: func(ctx context.Context, workDir, previousConversationID string) (*agysession.Metadata, error) {
				if !locked || ctx != t.Context() || workDir != dir || previousConversationID != "previous-native-id" {
					t.Fatalf("AGY discovery input = locked:%t %v/%q/%q", locked, ctx, workDir, previousConversationID)
				}
				order = append(order, "discover")
				return &agysession.Metadata{
					ConversationID:     "new-native-id",
					WorkspacePath:      dir,
					ConversationDBPath: "/provider/new-native-id.db",
					TranscriptPath:     "/provider/new-native-id/transcript.jsonl",
				}, nil
			},
		},
		AgyWorkspaceCreateLocker: func(ctx context.Context, workDir string) (func() error, error) {
			if ctx != t.Context() || workDir != dir {
				t.Fatalf("AGY lock input = %v/%q, want caller context/%q", ctx, workDir, dir)
			}
			locked = true
			order = append(order, "lock")
			return func() error {
				locked = false
				order = append(order, "unlock")
				return nil
			}, nil
		},
	}
	result, err := CreateSessionWithContext(t.Context(), opCtx, &CreateSessionRequest{
		Cwd: aliasDir, Title: "agy-locked", Harness: "agy", Model: "3.5-flash-low",
		Prompt: "fixture", SessionID: "agy-locked-id", RequireStorage: true,
	})
	if err != nil {
		t.Fatalf("CreateSessionWithContext: %v", err)
	}
	if locked {
		t.Fatal("AGY workspace lock was not released after lifecycle completion")
	}
	if result.Cwd != dir {
		t.Fatalf("result cwd = %q, want canonical workspace %q", result.Cwd, dir)
	}
	want := []string{"lock", "snapshot", "reserve", "launch", "discover", "unlock", "complete"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("AGY lock lifecycle order = %v, want %v", order, want)
	}
}

func TestCreateSession_AgyIdentitySnapshotFailsBeforeTmuxMutation(t *testing.T) {
	dir := t.TempDir()
	tmuxMock := session.NewMockTmux()
	wantErr := errors.New("corrupt provider snapshot")
	tracker := successfulCreateTestAgyIdentityTracker()
	tracker.snapshot = func(context.Context, string) (string, error) { return "", wantErr }

	_, err := CreateSessionWithContext(t.Context(), &OpContext{
		Tmux: tmuxMock, CreationRuntime: &createTestRuntime{}, AgyCreateIdentityTracker: tracker,
	}, &CreateSessionRequest{
		Cwd: dir, Title: "agy-snapshot-failure", Harness: "agy", Model: "3.5-flash-low",
		Prompt: "fixture",
	})
	if err == nil || !strings.Contains(err.Error(), wantErr.Error()) {
		t.Fatalf("CreateSessionWithContext error = %v, want %v", err, wantErr)
	}
	if len(tmuxMock.CreatedSessions) != 0 {
		t.Fatalf("tmux mutated after failed identity snapshot: %v", tmuxMock.CreatedSessions)
	}
}

func TestCreateSession_AgyIdentityDiscoveryFailureRollsBackBeforeRegistration(t *testing.T) {
	dir := t.TempDir()
	tmuxMock := session.NewMockTmux()
	store := &createMockStorage{}
	wantErr := errors.New("provider still reports stale identity")
	tracker := successfulCreateTestAgyIdentityTracker()
	tracker.discover = func(context.Context, string, string) (*agysession.Metadata, error) {
		return nil, wantErr
	}

	_, err := CreateSessionWithContext(t.Context(), &OpContext{
		Tmux: tmuxMock, Storage: store, CreationRuntime: &createTestRuntime{}, AgyCreateIdentityTracker: tracker,
	}, &CreateSessionRequest{
		Cwd: dir, Title: "agy-discovery-failure", Harness: "agy", Model: "3.5-flash-low",
		Prompt: "fixture", SessionID: "agy-discovery-failure-id", RequireStorage: true,
	})
	if err == nil || !strings.Contains(err.Error(), wantErr.Error()) {
		t.Fatalf("CreateSessionWithContext error = %v, want %v", err, wantErr)
	}
	if tmuxMock.Sessions["agy-discovery-failure"] {
		t.Fatal("tmux survived failed identity discovery")
	}
	if len(store.created) != 0 || !slices.Contains(store.released, "agy-discovery-failure-id") {
		t.Fatalf("reservation rollback after failed identity discovery = created:%d released:%v", len(store.created), store.released)
	}
}

func TestCreateSession_CancellationAfterRegistrationRollsBackBeforeCompletion(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	store := &createMockStorage{onCreate: cancel}
	tmuxMock := session.NewMockTmux()
	completed := false
	runtime := &createTestRuntime{
		complete: func(context.Context, CreateSessionCompletion) error {
			completed = true
			return nil
		},
	}

	_, err := CreateSessionWithContext(ctx, &OpContext{
		Tmux: tmuxMock, Storage: store, CreationRuntime: runtime,
		AgyCreateIdentityTracker: successfulCreateTestAgyIdentityTracker(),
	}, &CreateSessionRequest{
		Cwd: dir, Title: "cancel-after-register", Harness: "agy", Model: "3.5-flash-low",
		Prompt: "must not run", SessionID: "cancel-after-register-id", RequireStorage: true,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CreateSessionWithContext error = %v, want context.Canceled", err)
	}
	if completed {
		t.Fatal("runtime completion ran after registration canceled the request context")
	}
	if tmuxMock.Sessions["cancel-after-register"] {
		t.Fatal("new tmux session survived cancellation rollback")
	}
	if !slices.Contains(store.deleted, "cancel-after-register-id") {
		t.Fatalf("deleted registrations = %v, want canceled session ID", store.deleted)
	}
}

func TestCreateSession_NoRuntimeWaitsBeforeRegistrationAndPrompt(t *testing.T) {
	var order []string
	tmuxMock := &createReadinessTmux{MockTmux: session.NewMockTmux(), order: &order}
	store := &createMockStorage{createOrder: &order}

	result, err := CreateSession(&OpContext{Tmux: tmuxMock, Storage: store}, &CreateSessionRequest{
		Cwd: t.TempDir(), Title: "readiness-order", Model: "sonnet", Harness: "claude-code", Prompt: "start work",
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if !result.Created {
		t.Fatal("CreateSession() did not report creation")
	}
	want := []string{"reserve", "launch", "ready", "register", "prompt"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("creation order = %v, want %v", order, want)
	}
	if got := tmuxMock.WaitedHarnesses; len(got) != 1 || got[0] != "readiness-order:claude-code" {
		t.Fatalf("readiness checks = %v, want [readiness-order:claude-code]", got)
	}
}

func TestCreateSession_ReadinessFailureRollsBackBeforeRegistrationOrPrompt(t *testing.T) {
	var order []string
	wantErr := errors.New("composer missing")
	tmuxMock := &createReadinessTmux{MockTmux: session.NewMockTmux(), order: &order, waitErr: wantErr}
	store := &createMockStorage{createOrder: &order}

	_, err := CreateSession(&OpContext{Tmux: tmuxMock, Storage: store}, &CreateSessionRequest{
		Cwd: t.TempDir(), Title: "readiness-failure", Model: "5.5", Harness: "codex-cli", Prompt: "must not send",
		SkipCodexRemoteControl: true,
	})
	if err == nil || !strings.Contains(err.Error(), wantErr.Error()) {
		t.Fatalf("CreateSession() error = %v, want readiness failure", err)
	}
	if tmuxMock.Sessions["readiness-failure"] {
		t.Fatal("new tmux session survived readiness failure")
	}
	if len(store.created) != 0 || len(store.released) != 1 {
		t.Fatalf("readiness failure reservation rollback = created:%d released:%v", len(store.created), store.released)
	}
	if len(tmuxMock.SentCommands) != 1 {
		t.Fatalf("commands = %v, want harness launch only", tmuxMock.SentCommands)
	}
	if want := []string{"reserve", "launch", "ready"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("creation order = %v, want %v", order, want)
	}
}

func TestCreateSession_CodexHookReviewPropagatesBeforeRegistrationOrPrompt(t *testing.T) {
	var order []string
	tmuxMock := &createReadinessTmux{
		MockTmux: session.NewMockTmux(),
		order:    &order,
		waitErr:  tmux.CodexHookReviewError(),
	}
	store := &createMockStorage{createOrder: &order}

	_, err := CreateSession(&OpContext{Tmux: tmuxMock, Storage: store}, &CreateSessionRequest{
		Cwd: t.TempDir(), Title: "hook-review", Model: "5.6", Harness: "codex-cli", Prompt: "must not send",
		SkipCodexRemoteControl: true,
	})
	if !errors.Is(err, tmux.ErrCodexHookReviewRequired) {
		t.Fatalf("CreateSession() error = %v, want ErrCodexHookReviewRequired", err)
	}
	if tmuxMock.Sessions["hook-review"] {
		t.Fatal("new tmux session survived Codex hook review failure")
	}
	if len(store.created) != 0 || len(store.released) != 1 {
		t.Fatalf("Codex hook review reservation rollback = created:%d released:%v", len(store.created), store.released)
	}
	if len(tmuxMock.SentCommands) != 1 {
		t.Fatalf("commands = %v, want harness launch only", tmuxMock.SentCommands)
	}
	if want := []string{"reserve", "launch", "ready"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("creation order = %v, want %v", order, want)
	}
}

func TestCreateSession_ReadinessFailurePreservesReusedTmux(t *testing.T) {
	var order []string
	tmuxMock := &createReadinessTmux{
		MockTmux: session.NewMockTmux(), order: &order, waitErr: errors.New("onboarding still visible"),
	}
	tmuxMock.Sessions["reused-readiness"] = true

	_, err := CreateSession(&OpContext{Tmux: tmuxMock}, &CreateSessionRequest{
		Cwd: t.TempDir(), Title: "reused-readiness", Model: "sonnet", Harness: "claude-code", Prompt: "must not send",
		ReuseExistingTmux: true,
	})
	if err == nil {
		t.Fatal("CreateSession() returned success without readiness")
	}
	if !tmuxMock.Sessions["reused-readiness"] {
		t.Fatal("readiness rollback killed a pre-existing tmux session")
	}
	if len(tmuxMock.SentCommands) != 1 {
		t.Fatalf("commands = %v, want harness launch only", tmuxMock.SentCommands)
	}
}

func TestCreateSession_NoRuntimeRequiresReadinessCapability(t *testing.T) {
	base := session.NewMockTmux()
	tmuxWithoutReadiness := &createNoReadinessTmux{
		TmuxInterface: base,
		kill:          base.KillSession,
	}

	_, err := CreateSession(&OpContext{Tmux: tmuxWithoutReadiness}, &CreateSessionRequest{
		Cwd: t.TempDir(), Title: "no-readiness", Model: "sonnet", Harness: "claude-code", Prompt: "must not send",
	})
	if err == nil || !strings.Contains(err.Error(), "does not expose harness readiness") {
		t.Fatalf("CreateSession() error = %v, want readiness capability failure", err)
	}
	if base.Sessions["no-readiness"] {
		t.Fatal("new tmux session survived missing-readiness capability")
	}
}

func TestCreateSession_RollsBackEveryPostTmuxFailure(t *testing.T) {
	tests := []struct {
		name             string
		stage            string
		wantRegistration bool
		wantRelease      bool
	}{
		{name: "launch", stage: "launch", wantRelease: true},
		{name: "registration", stage: "registration", wantRelease: true},
		{name: "completion", stage: "completion", wantRegistration: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			manifestDir := filepath.Join(t.TempDir(), "rollback")
			tmuxMock := session.NewMockTmux()
			store := &createMockStorage{}
			if tt.stage == "registration" {
				store.createErr = errors.New("registration failed")
			}
			stageErr := errors.New(tt.stage + " failed")
			runtime := &createTestRuntime{
				launch: func(context.Context, HarnessLaunchSpec) (CreateSessionLaunchResult, error) {
					if tt.stage == "launch" {
						return CreateSessionLaunchResult{}, stageErr
					}
					return CreateSessionLaunchResult{}, nil
				},
				complete: func(context.Context, CreateSessionCompletion) error {
					if tt.stage == "completion" {
						return stageErr
					}
					return nil
				},
			}
			_, err := CreateSessionWithContext(context.Background(), &OpContext{Tmux: tmuxMock, Storage: store, CreationRuntime: runtime}, &CreateSessionRequest{
				Cwd: dir, Title: "rollback", Model: "sonnet", Harness: "claude-code", SessionID: "rollback-id",
				AllowEmptyPrompt: true, RequireStorage: true, ManifestDir: manifestDir,
			})
			if err == nil {
				t.Fatal("expected lifecycle failure")
			}
			if tmuxMock.Sessions["rollback"] {
				t.Fatal("new tmux session survived failed creation")
			}
			if _, statErr := os.Stat(manifestDir); !os.IsNotExist(statErr) {
				t.Fatalf("manifest directory survived rollback: %v", statErr)
			}
			if got := len(store.deleted); got != boolInt(tt.wantRegistration) {
				t.Fatalf("storage deletes = %d, want %d", got, boolInt(tt.wantRegistration))
			}
			if got := len(store.released); got != boolInt(tt.wantRelease) {
				t.Fatalf("reservation releases = %d, want %d", got, boolInt(tt.wantRelease))
			}
		})
	}
}

func TestCreateSession_ReportsReservationReleaseFailure(t *testing.T) {
	launchErr := errors.New("launch failed")
	releaseErr := errors.New("reservation release failed")
	store := &createMockStorage{releaseErr: releaseErr}

	_, err := CreateSessionWithContext(context.Background(), &OpContext{
		Tmux:    session.NewMockTmux(),
		Storage: store,
		CreationRuntime: &createTestRuntime{launch: func(context.Context, HarnessLaunchSpec) (CreateSessionLaunchResult, error) {
			return CreateSessionLaunchResult{}, launchErr
		}},
	}, &CreateSessionRequest{
		Cwd: t.TempDir(), Title: "release-failure", Model: "sonnet", Harness: "claude-code",
		SessionID: "release-failure-id", AllowEmptyPrompt: true, RequireStorage: true,
	})
	if !errors.Is(err, launchErr) || !errors.Is(err, releaseErr) {
		t.Fatalf("CreateSessionWithContext() error = %v, want joined launch and reservation-release failures", err)
	}
}

func TestPrepareCreateManifestDirOptionalFailureReturnsNoPath(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	manifestPath, registrationAllowed, created, err := prepareCreateManifestDir(&CreateSessionRequest{
		ManifestDir:         filepath.Join(blocker, "session"),
		ManifestDirOptional: true,
	})
	if err != nil {
		t.Fatalf("prepareCreateManifestDir: %v", err)
	}
	if manifestPath != "" {
		t.Fatalf("manifest path = %q, want empty path after optional mkdir failure", manifestPath)
	}
	if registrationAllowed || created {
		t.Fatalf("registrationAllowed = %v, created = %v; want both false", registrationAllowed, created)
	}
}

func TestRollbackCreateSessionReportsCleanupFailures(t *testing.T) {
	store := &createMockStorage{deleteErr: errors.New("delete failed")}
	tmuxMock := &createFailingKillTmux{
		TmuxInterface: session.NewMockTmux(),
		err:           errors.New("kill failed"),
	}

	stderrPath := filepath.Join(t.TempDir(), "stderr")
	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		t.Fatal(err)
	}
	oldStderr := os.Stderr
	os.Stderr = stderrFile
	t.Cleanup(func() { os.Stderr = oldStderr })

	rollbackCreateSession(&OpContext{Tmux: tmuxMock}, &CreateSessionRequest{}, store, "rollback", "rollback-id", true, false, true)
	os.Stderr = oldStderr
	if err := stderrFile.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := os.ReadFile(stderrPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"delete session registration", "delete failed", "kill tmux session", "kill failed"} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("rollback stderr = %q, want %q", output, want)
		}
	}
}

func TestCreateSession_FailedReusePreservesExistingArtifacts(t *testing.T) {
	dir := t.TempDir()
	manifestDir := filepath.Join(t.TempDir(), "existing")
	if err := os.MkdirAll(manifestDir, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(manifestDir, "keep")
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	tmuxMock := session.NewMockTmux()
	tmuxMock.Sessions["existing"] = true
	_, err := CreateSessionWithContext(context.Background(), &OpContext{
		Tmux: tmuxMock,
		CreationRuntime: &createTestRuntime{launch: func(context.Context, HarnessLaunchSpec) (CreateSessionLaunchResult, error) {
			return CreateSessionLaunchResult{}, errors.New("launch failed")
		}},
	}, &CreateSessionRequest{
		Cwd: dir, Title: "existing", Model: "sonnet", Harness: "claude-code",
		AllowEmptyPrompt: true, ReuseExistingTmux: true, ManifestDir: manifestDir,
	})
	if err == nil {
		t.Fatal("expected launch failure")
	}
	if !tmuxMock.Sessions["existing"] {
		t.Fatal("rollback killed a reused tmux session")
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Fatalf("rollback removed pre-existing manifest data: %v", statErr)
	}
}

func TestCreateSession_CodexRemoteBootIsBounded(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	t.Setenv("AGM_CODEX_REMOTE_CONTROL", "1")
	t.Setenv("AGM_CODEX_REQUIRE_REMOTE_CONTROL", "1")
	tmuxMock := session.NewMockTmux()
	started := time.Now()
	_, err := CreateSessionWithContext(context.Background(), &OpContext{
		Tmux: tmuxMock,
		CodexThreadCreator: func(ctx context.Context, _, _, _ string) (*manifest.Codex, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}, &CreateSessionRequest{
		Cwd: t.TempDir(), Title: "bounded", Model: "5.4", Harness: "codex-cli",
		AllowEmptyPrompt: true, CodexRemoteBootTimeout: 20 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected bounded remote-control failure")
	}
	if !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("remote-control deadline was not bounded: %v", elapsed)
	}
	if tmuxMock.Sessions["bounded"] {
		t.Fatal("timed-out Codex creation left a tmux session behind")
	}
}

func TestCreateSession_CodexRejectsUnsafeLaunchInputBeforeRemoteOrTmuxMutation(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	t.Setenv("AGM_CODEX_REMOTE_CONTROL", "1")
	t.Setenv("AGM_CODEX_REQUIRE_REMOTE_CONTROL", "1")
	tmuxMock := session.NewMockTmux()
	remoteCalls := 0

	_, err := CreateSessionWithContext(context.Background(), &OpContext{
		Tmux: tmuxMock,
		CodexThreadCreator: func(context.Context, string, string, string) (*manifest.Codex, error) {
			remoteCalls++
			return &manifest.Codex{SessionID: "must-not-exist"}, nil
		},
	}, &CreateSessionRequest{
		Cwd: t.TempDir(), Title: "prevalidate", Model: "5.4", Harness: "codex-cli",
		AllowEmptyPrompt: true, ExtraAddDirs: []string{"/tmp/unsafe\x1bdir"},
	})
	if err == nil || !strings.Contains(err.Error(), "validate harness launch") {
		t.Fatalf("CreateSessionWithContext error = %v, want terminal-control rejection", err)
	}
	if remoteCalls != 0 {
		t.Fatalf("Codex remote thread creations = %d, want 0", remoteCalls)
	}
	if tmuxMock.Sessions["prevalidate"] {
		t.Fatal("unsafe Codex request created a tmux session")
	}
}

func TestCreateSession_CLIAndMCPShareCoreContract(t *testing.T) {
	sharedDir := t.TempDir()
	type surfaceResult struct {
		result   *CreateSessionResult
		manifest *manifest.Manifest
		launch   HarnessLaunchSpec
	}
	run := func(t *testing.T, surface string) surfaceResult {
		t.Helper()
		store := &createMockStorage{}
		var launch HarnessLaunchSpec
		runtime := &createTestRuntime{launch: func(_ context.Context, spec HarnessLaunchSpec) (CreateSessionLaunchResult, error) {
			launch = spec
			return CreateSessionLaunchResult{ModeAppliedAtStartup: true}, nil
		}}
		result, err := CreateSessionWithContext(context.Background(), &OpContext{Tmux: session.NewMockTmux(), Storage: store, CreationRuntime: runtime}, &CreateSessionRequest{
			Cwd: sharedDir, Prompt: "same prompt", Title: "parity", Model: "sonnet", Harness: "claude-code",
			SessionID: "shared-id", Caller: CreateSessionCaller{Surface: surface}, PermissionMode: "plan",
			Metadata: CreateSessionMetadata{Workspace: "shared", ModelTier: "high", Tags: []string{"role:worker"}},
		})
		if err != nil {
			t.Fatalf("CreateSessionWithContext(%s): %v", surface, err)
		}
		if len(store.created) != 1 {
			t.Fatalf("created manifests = %d, want 1", len(store.created))
		}
		return surfaceResult{result: result, manifest: store.created[0], launch: launch}
	}

	cli := run(t, CreateSurfaceCLI)
	mcp := run(t, CreateSurfaceMCP)
	if !reflect.DeepEqual(cli.launch, mcp.launch) {
		t.Fatalf("launch specs diverged:\nCLI: %#v\nMCP: %#v", cli.launch, mcp.launch)
	}
	for label, got := range map[string]*manifest.Manifest{"cli": cli.manifest, "mcp": mcp.manifest} {
		if got.Workspace != "shared" || got.ModelTier != "high" || got.PermissionMode != "plan" {
			t.Fatalf("%s manifest lost shared metadata: %#v", label, got)
		}
	}
	if !slicesContain(cli.manifest.Context.Tags, "source:cli") || !slicesContain(mcp.manifest.Context.Tags, "source:mcp") {
		t.Fatalf("source provenance missing: cli=%v mcp=%v", cli.manifest.Context.Tags, mcp.manifest.Context.Tags)
	}
	if cli.result.Source != CreateSurfaceCLI || mcp.result.Source != CreateSurfaceMCP {
		t.Fatalf("result provenance mismatch: cli=%q mcp=%q", cli.result.Source, mcp.result.Source)
	}
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func slicesContain(values []string, want string) bool {
	return slices.Contains(values, want)
}

func TestBuildHarnessCommand_ClaudeCode(t *testing.T) {
	cmd := testHarnessCommand("claude-code", "opus", "my-session", "/tmp/work", false)
	if cmd == "" {
		t.Fatal("empty command")
	}
	for _, want := range []string{"agm __exec-claude", "--model '" + agent.ResolveModelFullName("claude-code", "opus") + "'", "--session 'my-session'", "--auto-mode", "--add-dir '/tmp/work'", "&& exit"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("command %q missing %q", cmd, want)
		}
	}
}

func TestBuildHarnessCommand_GeminiCli(t *testing.T) {
	cmd := testHarnessCommand("gemini-cli", "2.5-flash", "s", "/tmp", false)
	if !strings.Contains(cmd, "gemini -m '2.5-flash'") {
		t.Errorf("gemini command = %q", cmd)
	}
	if !strings.Contains(cmd, "&& exit") {
		t.Errorf("gemini command missing exit suffix: %q", cmd)
	}
}

func TestBuildHarnessCommand_CodexCli(t *testing.T) {
	cmd := testHarnessCommand("codex-cli", "5.4", "codex-session", "/tmp/work", false)
	for _, want := range []string{
		"agm __exec-codex",
		"--session 'codex-session'",
		"--model 'gpt-5.4'",
		"--workdir '/tmp/work'",
		"--sandbox 'workspace-write'",
		"&& exit",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("command %q missing %q", cmd, want)
		}
	}
	if strings.Contains(cmd, "CLAUDE_CODE_OAUTH_TOKEN") || strings.Contains(cmd, "ANTHROPIC_") {
		t.Errorf("codex command leaked Claude/Anthropic env: %s", cmd)
	}
}

func TestBuildHarnessCommand_CodexCliRemoteThread(t *testing.T) {
	cmd := testHarnessCommandWithCodex("codex-cli", "5.4", "codex-session", "/tmp/work", false, &manifest.Codex{SessionID: "thr_123"})
	for _, want := range []string{
		"agm __exec-codex",
		"--session 'codex-session'",
		"--model 'gpt-5.4'",
		"--workdir '/tmp/work'",
		"--sandbox 'workspace-write'",
		"--resume-id 'thr_123'",
		"--remote",
		"&& exit",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("remote Codex command %q missing %q", cmd, want)
		}
	}
}

func TestBuildHarnessCommand_OpenCodeCli(t *testing.T) {
	cmd := testHarnessCommand("opencode-cli", "sonnet", "open-session", "/tmp/work", false)
	for _, want := range []string{
		"opencode attach",
		"&& exit",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("command %q missing %q", cmd, want)
		}
	}
	if strings.Contains(cmd, "--model") || strings.Contains(cmd, "-m ") {
		t.Errorf("opencode command should not pass a model flag: %s", cmd)
	}
}

func TestBuildHarnessCommand_ActiveHarnessesSupported(t *testing.T) {
	for _, harness := range agent.ActiveHarnesses() {
		cmd := testHarnessCommand(harness, "sonnet", "session", "/tmp/work", false)
		if strings.Contains(cmd, "Unknown harness") {
			t.Errorf("active harness %q produced unknown-harness command: %s", harness, cmd)
		}
	}
}

func TestBuildHarnessCommand_UnknownHarness(t *testing.T) {
	cmd := testHarnessCommand("unknown", "m", "s", "/tmp", false)
	if !strings.Contains(cmd, "Unknown harness") {
		t.Errorf("unknown harness command = %q", cmd)
	}
}

func TestBuildHarnessCommand_EscapesSingleQuotes(t *testing.T) {
	cmd := testHarnessCommand("claude-code", "opus", "sess", "/tmp/it's a dir", false)
	if strings.Contains(cmd, "it's a") {
		t.Errorf("unescaped single quote in command: %s", cmd)
	}
	if !strings.Contains(cmd, `it'"'"'s a dir`) {
		t.Errorf("expected escaped single quote, got: %s", cmd)
	}
}

func TestBuildHarnessCommand_BracketedModelQuoted(t *testing.T) {
	model := agent.ResolveModelFullName("claude-code", "sonnet")
	cmd := testHarnessCommand("claude-code", "sonnet", "sess", "/tmp/work", false)
	if !strings.Contains(cmd, "--model '"+model+"'") {
		t.Errorf("bracketed model not quoted; zsh would glob-expand [1m]: %s", cmd)
	}
}

// TestBuildHarnessCommand_Persistent verifies that persistent=true omits
// "&&  exit" from the command for all harnesses that normally include it,
// so supervisor sessions survive their Claude turn/loop ending (ce-pzca).
func TestBuildHarnessCommand_Persistent(t *testing.T) {
	for _, harness := range append(agent.ActiveHarnesses(), agent.DeprecatedHarnesses()...) {
		cmd := testHarnessCommand(harness, "opus", "sup-session", "/tmp/work", true)
		if strings.Contains(cmd, "&& exit") {
			t.Errorf("persistent=true: harness %q command still has '&& exit': %s", harness, cmd)
		}
	}
}

// TestBuildHarnessCommand_NonPersistentHasExit verifies that persistent=false
// keeps the "&&  exit" suffix for clean one-shot worker teardown.
func TestBuildHarnessCommand_NonPersistentHasExit(t *testing.T) {
	for _, harness := range append(agent.ActiveHarnesses(), agent.DeprecatedHarnesses()...) {
		cmd := testHarnessCommand(harness, "opus", "worker-session", "/tmp/work", false)
		if !strings.Contains(cmd, "&& exit") {
			t.Errorf("persistent=false: harness %q command missing '&& exit': %s", harness, cmd)
		}
	}
}

func TestSharedShellQuote(t *testing.T) {
	got := shellquote.Quote("a'b")
	if got != `'a'"'"'b'` {
		t.Errorf("ShellQuote = %q", got)
	}
}

func TestBuildCreateSessionManifestPreservesRelationshipMetadata(t *testing.T) {
	parentID := "parent-session-id"
	req := &CreateSessionRequest{
		Cwd:    "/tmp/work",
		Caller: CreateSessionCaller{Surface: CreateSurfaceCLI, Source: "session.create-child"},
		Metadata: CreateSessionMetadata{
			Tags:            []string{"inherited"},
			ContextPurpose:  "Inherited purpose",
			ContextNotes:    "Inherited notes",
			ParentSessionID: &parentID,
		},
	}
	params := &createSessionParams{name: "child", harness: "codex-cli", model: "gpt-5.3-codex"}

	got := buildCreateSessionManifest(req, params, "child-id", nil)

	if got.ParentSessionID == nil || *got.ParentSessionID != parentID {
		t.Fatalf("ParentSessionID = %v, want %q", got.ParentSessionID, parentID)
	}
	if got.ParentSessionID == req.Metadata.ParentSessionID {
		t.Fatal("ParentSessionID aliases request metadata")
	}
	if got.Context.Purpose != req.Metadata.ContextPurpose || got.Context.Notes != req.Metadata.ContextNotes {
		t.Fatalf("Context = %#v, want purpose and notes from metadata", got.Context)
	}
	for _, want := range []string{"inherited", "source:session.create-child"} {
		if !slices.Contains(got.Context.Tags, want) {
			t.Fatalf("Context.Tags = %v, missing %q", got.Context.Tags, want)
		}
	}
}

func TestBuildCreateSessionManifestPersistsSandboxLaunchPolicyWithoutAliasing(t *testing.T) {
	sandbox := &manifest.SandboxConfig{
		Enabled: true,
		ID:      "sandbox-session",
	}
	req := &CreateSessionRequest{
		Cwd:          "/tmp/sandbox",
		ExtraAddDirs: []string{"/tmp/worktree", "/tmp/beads"},
		Metadata:     CreateSessionMetadata{Sandbox: sandbox},
	}
	params := &createSessionParams{name: "sandbox", harness: "codex-cli", model: "gpt-5.5"}

	got := buildCreateSessionManifest(req, params, "sandbox-session", nil)

	if got.Sandbox == sandbox {
		t.Fatal("Sandbox aliases request metadata")
	}
	if !slices.Equal(got.Sandbox.ExtraAddDirs, req.ExtraAddDirs) {
		t.Fatalf("ExtraAddDirs = %v, want %v", got.Sandbox.ExtraAddDirs, req.ExtraAddDirs)
	}
	req.ExtraAddDirs[0] = "/tmp/mutated"
	if got.Sandbox.ExtraAddDirs[0] != "/tmp/worktree" {
		t.Fatalf("persisted ExtraAddDirs aliases request: %v", got.Sandbox.ExtraAddDirs)
	}
}
