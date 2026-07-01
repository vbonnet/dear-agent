package ops

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
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

func TestCreateSession_DefaultsModelPerHarness(t *testing.T) {
	tests := []struct {
		harness string
		want    string
	}{
		{"codex-cli", "5.6"},
		{"agy", "2.5-flash"},
		{"opencode-cli", "glm-5.2"},
	}
	for _, tt := range tests {
		t.Run(tt.harness, func(t *testing.T) {
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

func TestBuildHarnessCommand_ClaudeCode(t *testing.T) {
	cmd := buildHarnessCommand("claude-code", "opus", "my-session", "/tmp/work", false)
	if cmd == "" {
		t.Fatal("empty command")
	}
	for _, want := range []string{"claude", "--model 'opus'", "AGM_SESSION_NAME='my-session'", "--enable-auto-mode", "--add-dir '/tmp/work'", "&& exit"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("command %q missing %q", cmd, want)
		}
	}
}

func TestBuildHarnessCommand_GeminiCli(t *testing.T) {
	cmd := buildHarnessCommand("gemini-cli", "2.5-flash", "s", "/tmp", false)
	if !strings.Contains(cmd, "gemini -m '2.5-flash'") {
		t.Errorf("gemini command = %q", cmd)
	}
	if !strings.Contains(cmd, "&& exit") {
		t.Errorf("gemini command missing exit suffix: %q", cmd)
	}
}

func TestBuildHarnessCommand_CodexCli(t *testing.T) {
	cmd := buildHarnessCommand("codex-cli", "5.4", "codex-session", "/tmp/work", false)
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

func TestBuildHarnessCommand_OpenCodeCli(t *testing.T) {
	cmd := buildHarnessCommand("opencode-cli", "sonnet", "open-session", "/tmp/work", false)
	for _, want := range []string{
		"cd '/tmp/work'",
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
		cmd := buildHarnessCommand(harness, "sonnet", "session", "/tmp/work", false)
		if strings.Contains(cmd, "Unknown harness") {
			t.Errorf("active harness %q produced unknown-harness command: %s", harness, cmd)
		}
	}
}

func TestBuildHarnessCommand_UnknownHarness(t *testing.T) {
	cmd := buildHarnessCommand("unknown", "m", "s", "/tmp", false)
	if !strings.Contains(cmd, "Unknown harness") {
		t.Errorf("unknown harness command = %q", cmd)
	}
}

func TestBuildHarnessCommand_EscapesSingleQuotes(t *testing.T) {
	cmd := buildHarnessCommand("claude-code", "opus", "sess", "/tmp/it's a dir", false)
	if strings.Contains(cmd, "it's a") {
		t.Errorf("unescaped single quote in command: %s", cmd)
	}
	if !strings.Contains(cmd, "it'\\''s a dir") {
		t.Errorf("expected escaped single quote, got: %s", cmd)
	}
}

func TestBuildHarnessCommand_BracketedModelQuoted(t *testing.T) {
	cmd := buildHarnessCommand("claude-code", "claude-sonnet-4-6[1m]", "sess", "/tmp/work", false)
	if !strings.Contains(cmd, "--model 'claude-sonnet-4-6[1m]'") {
		t.Errorf("bracketed model not quoted; zsh would glob-expand [1m]: %s", cmd)
	}
}

// TestBuildHarnessCommand_Persistent verifies that persistent=true omits
// "&&  exit" from the command for all harnesses that normally include it,
// so supervisor sessions survive their Claude turn/loop ending (ce-pzca).
func TestBuildHarnessCommand_Persistent(t *testing.T) {
	for _, harness := range append(agent.ActiveHarnesses(), agent.DeprecatedHarnesses()...) {
		cmd := buildHarnessCommand(harness, "opus", "sup-session", "/tmp/work", true)
		if strings.Contains(cmd, "&& exit") {
			t.Errorf("persistent=true: harness %q command still has '&& exit': %s", harness, cmd)
		}
	}
}

// TestBuildHarnessCommand_NonPersistentHasExit verifies that persistent=false
// keeps the "&&  exit" suffix for clean one-shot worker teardown.
func TestBuildHarnessCommand_NonPersistentHasExit(t *testing.T) {
	for _, harness := range append(agent.ActiveHarnesses(), agent.DeprecatedHarnesses()...) {
		cmd := buildHarnessCommand(harness, "opus", "worker-session", "/tmp/work", false)
		if !strings.Contains(cmd, "&& exit") {
			t.Errorf("persistent=false: harness %q command missing '&& exit': %s", harness, cmd)
		}
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"simple", "simple"},
		{"it's", "it'\\''s"},
		{"a'b'c", "a'\\''b'\\''c"},
		{"no-quotes", "no-quotes"},
	}
	for _, tt := range tests {
		got := shellQuote(tt.in)
		if got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
