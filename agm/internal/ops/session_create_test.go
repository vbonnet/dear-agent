package ops

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/launchparity"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// createMockStorage implements dolt.Storage for CreateSession tests.
type createMockStorage struct {
	created     []*manifest.Manifest
	deleted     []string
	createErr   error
	deleteErr   error
	createOrder *[]string
	onCreate    func()
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

type createTestRuntime struct {
	launch   func(context.Context, HarnessLaunchSpec) (CreateSessionLaunchResult, error)
	complete func(context.Context, CreateSessionCompletion) error
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

func (s *createMockStorage) ListSessions(*dolt.SessionFilter) ([]*manifest.Manifest, error) {
	return nil, nil
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

	result, err := CreateSession(&OpContext{Tmux: tmuxMock, Storage: store}, &CreateSessionRequest{
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
			result, err := CreateSession(&OpContext{Tmux: session.NewMockTmux(), OutputMode: "json"}, &CreateSessionRequest{
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
	tmuxMock := session.NewMockTmux()
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
	want := []string{"launch", "storage", "register", "complete", "cleanup"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("lifecycle order = %v, want %v", order, want)
	}
}

func TestCreateSession_AgyWorkspaceLockCoversSharedLifecycle(t *testing.T) {
	dir := t.TempDir()
	var order []string
	locked := false
	store := &createMockStorage{onCreate: func() {
		if !locked {
			t.Fatal("AGY workspace lock was released before registration")
		}
	}}
	runtime := &createTestRuntime{
		launch: func(context.Context, HarnessLaunchSpec) (CreateSessionLaunchResult, error) {
			if !locked {
				t.Fatal("AGY workspace lock was not held during launch")
			}
			order = append(order, "launch")
			return CreateSessionLaunchResult{}, nil
		},
		complete: func(context.Context, CreateSessionCompletion) error {
			if !locked {
				t.Fatal("AGY workspace lock was not held during provider identity completion")
			}
			order = append(order, "complete")
			return nil
		},
	}
	opCtx := &OpContext{
		Tmux: session.NewMockTmux(), Storage: store, CreationRuntime: runtime,
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
	_, err := CreateSessionWithContext(t.Context(), opCtx, &CreateSessionRequest{
		Cwd: dir, Title: "agy-locked", Harness: "agy", Model: "3.5-flash-low",
		Prompt: "fixture", SessionID: "agy-locked-id", RequireStorage: true,
	})
	if err != nil {
		t.Fatalf("CreateSessionWithContext: %v", err)
	}
	if locked {
		t.Fatal("AGY workspace lock was not released after lifecycle completion")
	}
	want := []string{"lock", "launch", "complete", "unlock"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("AGY lock lifecycle order = %v, want %v", order, want)
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

func TestCreateSession_RollsBackEveryPostTmuxFailure(t *testing.T) {
	tests := []struct {
		name             string
		stage            string
		wantRegistration bool
	}{
		{name: "launch", stage: "launch"},
		{name: "registration", stage: "registration"},
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
		})
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
	for _, want := range []string{"claude", "--model '" + agent.ResolveModelFullName("claude-code", "opus") + "'", "AGM_SESSION_NAME='my-session'", "--enable-auto-mode", "--add-dir '/tmp/work'", "&& exit"} {
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
		"env -u CLAUDECODE",
		"AGM_SESSION_NAME='codex-session'",
		"codex -m 'gpt-5.4'",
		"-C '/tmp/work'",
		"-s workspace-write",
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
		"env -u CLAUDECODE",
		"AGM_SESSION_NAME='codex-session'",
		"codex resume --remote unix://",
		"-m 'gpt-5.4'",
		"-C '/tmp/work'",
		"-s workspace-write",
		"'thr_123'",
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
	got := launchparity.ShellQuote("a'b")
	if got != `'a'"'"'b'` {
		t.Errorf("ShellQuote = %q", got)
	}
}
