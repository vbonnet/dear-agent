package ops

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// createMockStorage implements dolt.Storage for CreateSession tests.
type createMockStorage struct {
	created []*manifest.Manifest
}

func (s *createMockStorage) CreateSession(m *manifest.Manifest) error {
	s.created = append(s.created, m)
	return nil
}
func (s *createMockStorage) GetSession(string) (*manifest.Manifest, error) { return nil, nil }
func (s *createMockStorage) UpdateSession(*manifest.Manifest) error        { return nil }
func (s *createMockStorage) DeleteSession(string) error                    { return nil }

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
		// Storage is nil — should still create session
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

func TestBuildHarnessCommand_ClaudeCode(t *testing.T) {
	cmd := buildHarnessCommand("claude-code", "opus", "my-session", "/tmp/work")
	if cmd == "" {
		t.Fatal("empty command")
	}
	for _, want := range []string{"claude", "--model opus", "AGM_SESSION_NAME=my-session", "--enable-auto-mode"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("command %q missing %q", cmd, want)
		}
	}
}

func TestBuildHarnessCommand_GeminiCli(t *testing.T) {
	cmd := buildHarnessCommand("gemini-cli", "2.5-flash", "s", "/tmp")
	if !strings.Contains(cmd, "gemini -m 2.5-flash") {
		t.Errorf("gemini command = %q", cmd)
	}
}

func TestBuildHarnessCommand_UnknownHarness(t *testing.T) {
	cmd := buildHarnessCommand("unknown", "m", "s", "/tmp")
	if !strings.Contains(cmd, "Unknown harness") {
		t.Errorf("unknown harness command = %q", cmd)
	}
}
