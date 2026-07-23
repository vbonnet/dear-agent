package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/launchparity"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// TestResumeCommandFlags verifies that the resume command properly parses flags
func TestResumeCommandFlags(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectError bool
		description string
	}{
		{
			name:        "accepts --detached flag",
			args:        []string{"session-name", "--detached"},
			expectError: false,
			description: "Should accept --detached flag",
		},
		{
			name:        "works without --detached flag",
			args:        []string{"session-name"},
			expectError: false,
			description: "Should work without --detached flag (default behavior)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset the flag value before each test
			resumeDetached = false

			// Parse flags (this simulates cobra command parsing)
			resumeCmd.ResetFlags()
			resumeCmd.Flags().BoolVar(&resumeDetached, "detached", false, "Resume session without attaching")

			// Test that the command accepts the flags
			// Note: We can't fully test the execution without mocking tmux,
			// but we can verify the flag parsing works correctly
			err := resumeCmd.ParseFlags(tt.args)

			if tt.expectError && err == nil {
				t.Errorf("%s: expected error but got none", tt.description)
			}

			if !tt.expectError && err != nil {
				t.Errorf("%s: unexpected error: %v", tt.description, err)
			}

			// Verify the flag value is correctly set for --detached test
			if tt.name == "accepts --detached flag" && !resumeDetached {
				t.Errorf("%s: --detached flag should be true", tt.description)
			}

			// Verify the flag value is false for default test
			if tt.name == "works without --detached flag" && resumeDetached {
				t.Errorf("%s: detached flag should be false by default", tt.description)
			}
		})
	}
}

func TestWithAgyResumeWorkspaceLockCoversLifecycle(t *testing.T) {
	original := agyResumeWorkspaceLock
	t.Cleanup(func() { agyResumeWorkspaceLock = original })

	var events []string
	agyResumeWorkspaceLock = func(_ context.Context, workDir string) (func() error, error) {
		events = append(events, "lock:"+workDir)
		return func() error { events = append(events, "unlock"); return nil }, nil
	}
	if err := withAgyResumeWorkspaceLock(t.Context(), "agy", "/work", func() error {
		events = append(events, "launch-and-ready")
		return nil
	}); err != nil {
		t.Fatalf("withAgyResumeWorkspaceLock: %v", err)
	}
	want := []string{"lock:/work", "launch-and-ready", "unlock"}
	if fmt.Sprint(events) != fmt.Sprint(want) {
		t.Fatalf("resume workspace lock events = %v, want %v", events, want)
	}
}

// TestResumeDetachedHelp verifies the help text includes --detached documentation
func TestResumeDetachedHelp(t *testing.T) {
	helpText := resumeCmd.Long

	if helpText == "" {
		t.Fatal("Resume command should have Long help text")
	}

	// Verify help mentions --detached
	if !contains(helpText, "--detached") && !contains(helpText, "detached") {
		t.Error("Resume command help should mention --detached flag")
	}

	// Verify help explains detached behavior
	if !contains(helpText, "background") && !contains(helpText, "without attaching") {
		t.Error("Resume command help should explain detached mode behavior")
	}
}

// contains is a simple helper to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || contains(s[1:], substr)))
}

// TestResumePromptFlagParsing verifies --prompt and --prompt-file flags are registered
// and parsed correctly. These flags enable crash recovery by injecting a prompt
// after the session is resumed.
func TestResumePromptFlagParsing(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantPrompt  string
		wantFile    string
		expectError bool
		description string
	}{
		{
			name:        "accepts --prompt flag",
			args:        []string{"session-name", "--prompt", "continue working on X"},
			wantPrompt:  "continue working on X",
			wantFile:    "",
			expectError: false,
			description: "Should accept inline --prompt text",
		},
		{
			name:        "accepts --prompt-file flag",
			args:        []string{"session-name", "--prompt-file", "/tmp/recovery.txt"},
			wantPrompt:  "",
			wantFile:    "/tmp/recovery.txt",
			expectError: false,
			description: "Should accept --prompt-file path",
		},
		{
			name:        "works without prompt flags",
			args:        []string{"session-name"},
			wantPrompt:  "",
			wantFile:    "",
			expectError: false,
			description: "Prompt flags should be optional",
		},
		{
			name:        "accepts --detached with --prompt",
			args:        []string{"session-name", "--detached", "--prompt", "pick up where you left off"},
			wantPrompt:  "pick up where you left off",
			wantFile:    "",
			expectError: false,
			description: "Should accept --detached combined with --prompt for background crash recovery",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset vars before each test
			resumeDetached = false
			resumePrompt = ""
			resumePromptFile = ""

			// Re-register all flags (ResetFlags clears them)
			resumeCmd.ResetFlags()
			resumeCmd.Flags().BoolVar(&resumeDetached, "detached", false, "Resume session without attaching")
			resumeCmd.Flags().BoolVar(&resumeForceParent, "force-parent", false, "Resume planning session instead of execution session")
			resumeCmd.Flags().StringVar(&resumePrompt, "prompt", "", "Prompt to send after resume")
			resumeCmd.Flags().StringVar(&resumePromptFile, "prompt-file", "", "File containing prompt to send after resume")

			err := resumeCmd.ParseFlags(tt.args)

			if tt.expectError && err == nil {
				t.Errorf("%s: expected error but got none", tt.description)
			}
			if !tt.expectError && err != nil {
				t.Errorf("%s: unexpected error: %v", tt.description, err)
			}

			if resumePrompt != tt.wantPrompt {
				t.Errorf("%s: resumePrompt = %q, want %q", tt.description, resumePrompt, tt.wantPrompt)
			}
			if resumePromptFile != tt.wantFile {
				t.Errorf("%s: resumePromptFile = %q, want %q", tt.description, resumePromptFile, tt.wantFile)
			}
		})
	}
}

// TestResumeHelpMentionsPromptFlags verifies the help text documents the new flags.
func TestResumeHelpMentionsPromptFlags(t *testing.T) {
	helpText := resumeCmd.Long
	if helpText == "" {
		t.Fatal("Resume command should have Long help text")
	}
	if !contains(helpText, "--prompt") {
		t.Error("Resume command help should mention --prompt flag")
	}
	if !contains(helpText, "--prompt-file") {
		t.Error("Resume command help should mention --prompt-file flag")
	}
	if !contains(helpText, "crash recovery") && !contains(helpText, "background resume") {
		t.Error("Resume command help should explain prompt flags in context of crash recovery or background resume")
	}
}

// TestSendPostResumePrompt_FileNotFound verifies an error is returned when the
// prompt file does not exist, before any tmux operations occur.
func TestSendPostResumePrompt_FileNotFound(t *testing.T) {
	err := sendPostResumePrompt(context.Background(), "any-session", "claude-code", "", "/nonexistent/path/prompt.txt", false)
	if err == nil {
		t.Fatal("expected error for missing prompt file, got nil")
		return
	}
	if !strings.Contains(err.Error(), "failed to read prompt file") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestSendPostResumePrompt_FileTooLarge verifies the 10KB size limit is enforced
// before any tmux operations occur.
func TestSendPostResumePrompt_FileTooLarge(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "large.txt")
	// Write 11KB of data (exceeds 10KB limit)
	data := make([]byte, 11*1024)
	for i := range data {
		data[i] = 'x'
	}
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	err := sendPostResumePrompt(context.Background(), "any-session", "claude-code", "", tmp, false)
	if err == nil {
		t.Fatal("expected error for oversized prompt file, got nil")
		return
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSendPostResumePromptUsesCallerContext(t *testing.T) {
	original := sendResumePromptSafe
	t.Cleanup(func() { sendResumePromptSafe = original })
	sendResumePromptSafe = func(ctx context.Context, _, _ string, _ bool, _ string) error {
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := sendPostResumePrompt(ctx, "resume-context", "claude-code", "continue", "", false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("sendPostResumePrompt() error = %v, want context.Canceled", err)
	}
}

func TestSendPostResumePromptUsesHarnessAwareAgyDelivery(t *testing.T) {
	original := sendResumePromptSafe
	t.Cleanup(func() { sendResumePromptSafe = original })

	wantPrompt := "resume line one\nresume line two"
	called := 0
	sendResumePromptSafe = func(_ context.Context, sessionName, prompt string, interrupt bool, harness string) error {
		called++
		if sessionName != "agy-resume" || prompt != wantPrompt || interrupt || harness != "agy" {
			t.Fatalf("resume delivery = %q/%q/%t/%q", sessionName, prompt, interrupt, harness)
		}
		return nil
	}

	if err := sendPostResumePrompt(context.Background(), "agy-resume", "agy", wantPrompt, "", false); err != nil {
		t.Fatalf("sendPostResumePrompt() error = %v", err)
	}
	if called != 1 {
		t.Fatalf("harness-aware resume deliveries = %d, want 1", called)
	}
}

func TestReadResumePromptFilePreservesRejectedDisposableFile(t *testing.T) {
	file, err := os.CreateTemp("/tmp", "agm-resume-")
	if err != nil {
		t.Fatal(err)
	}
	path := file.Name()
	if _, err := file.Write(make([]byte, 11*1024)); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = readResumePromptFile(path, true)
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("readResumePromptFile() error = %v, want size error", err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("rejected prompt file was removed: %v", statErr)
	}
}

func TestReadResumePromptFileDeletesOnlyWithExplicitOptIn(t *testing.T) {
	for _, deleteFile := range []bool{false, true} {
		path := filepath.Join(t.TempDir(), fmt.Sprintf("prompt-%t.txt", deleteFile))
		if err := os.WriteFile(path, []byte("resume safely"), 0o600); err != nil {
			t.Fatal(err)
		}
		message, err := readResumePromptFile(path, deleteFile)
		if err != nil || message != "resume safely" {
			t.Fatalf("readResumePromptFile(delete=%t) = (%q, %v)", deleteFile, message, err)
		}
		_, statErr := os.Stat(path)
		if deleteFile && !os.IsNotExist(statErr) {
			t.Fatalf("opted-in prompt remains: %v", statErr)
		}
		if !deleteFile && statErr != nil {
			t.Fatalf("caller-owned prompt was removed: %v", statErr)
		}
	}
}

func TestBuildCodexResumeCommand(t *testing.T) {
	m := &manifest.Manifest{
		Model: "5.4",
	}
	health := &HealthStatus{
		TmuxSessionName: "codex-session",
		WorktreePath:    "/tmp/work",
	}

	cmd := buildCodexResumeCommand(m, health)

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
	if strings.Contains(cmd, "claude") || strings.Contains(cmd, "CLAUDE_CODE_OAUTH_TOKEN") {
		t.Errorf("codex resume command leaked Claude-specific state: %s", cmd)
	}
}

func TestBuildCodexResumeCommand_DefaultModel(t *testing.T) {
	health := &HealthStatus{
		TmuxSessionName: "codex-session",
		WorktreePath:    "/tmp/work",
	}

	cmd := buildCodexResumeCommand(&manifest.Manifest{}, health)
	if !strings.Contains(cmd, "--model 'gpt-5.5'") {
		t.Errorf("default Codex model not resolved: %s", cmd)
	}
}

func TestPrepareClaudeResumeCommandUsesCallerOnlyPrivateState(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "claude-resume-oauth-canary")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "claude-resume-otel-canary")
	m := &manifest.Manifest{
		SessionID: "agm-session-id",
		Claude:    manifest.Claude{UUID: "native-claude-id"},
	}
	health := &HealthStatus{TmuxSessionName: "claude-resume", WorktreePath: "/tmp/resume-work"}

	launch, err := prepareClaudeResumeCommand(nil, m, health)
	if err != nil {
		t.Fatalf("prepare Claude resume: %v", err)
	}
	t.Cleanup(func() { _ = launch.CancelUndelivered() })
	for _, want := range []string{
		"__exec-claude", "--handoff", "--resume-id 'native-claude-id'", "--workdir '/tmp/resume-work'",
		"--forward-telemetry",
	} {
		if !strings.Contains(launch.Command, want) {
			t.Errorf("prepared Claude resume %q missing %q", launch.Command, want)
		}
	}
	for _, secret := range []string{"claude-resume-oauth-canary", "claude-resume-otel-canary"} {
		if strings.Contains(launch.Command, secret) {
			t.Fatalf("prepared Claude resume exposed %q: %s", secret, launch.Command)
		}
	}
	if strings.Contains(launch.Command, " claude --resume ") {
		t.Fatalf("Claude resume bypassed the private executor: %s", launch.Command)
	}
}

func TestArchitectureUsesPreparedClaudeResumeBoundary(t *testing.T) {
	architecture, err := os.ReadFile("ARCHITECTURE.md")
	if err != nil {
		t.Fatalf("read AGM architecture: %v", err)
	}
	text := string(architecture)
	if strings.Contains(text, `tmux.SendKeys(sessionName, "claude --resume`) {
		t.Fatal("AGM architecture still teaches raw Claude resume through tmux")
	}
	for _, want := range []string{"prepareClaudeResumeCommand", "launch.Command", "launch.CancelUndelivered"} {
		if !strings.Contains(text, want) {
			t.Errorf("AGM architecture private resume example missing %q", want)
		}
	}
}

func TestBuildPiResumeCommandPreservesExactIdentityModelModeAndPolicy(t *testing.T) {
	t.Setenv("AGM_PI_EXTENSION_ROOT", t.TempDir())
	sessionDir := t.TempDir()
	codingAgentDir := filepath.Join(t.TempDir(), "pi agent's config")
	if err := os.Mkdir(codingAgentDir, 0o700); err != nil {
		t.Fatal(err)
	}
	m := &manifest.Manifest{
		Model: "gpt", PermissionMode: "auto",
		Pi: &manifest.Pi{
			SessionID: "native.pi-id", SessionDir: sessionDir,
			CodingAgentDir: codingAgentDir, CodingAgentDirSet: true,
		},
		PermissionPolicy: &manifest.PermissionPolicy{Allow: []string{"Bash(git:*)"}},
	}
	command, err := buildPiResumeCommand(m, &HealthStatus{TmuxSessionName: "pi-worker", WorktreePath: "/tmp/work"}, "launch-resume")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"--session-id 'native.pi-id'", "--session-dir '" + sessionDir + "'", "--name 'pi-worker'",
		"PI_SESSION_ID='native.pi-id'", "AGM_PI_PROJECT_DIR='/tmp/work'",
		"AGM_PI_LAUNCH_ID='launch-resume'",
		"PI_CODING_AGENT_DIR=" + launchparity.ShellQuote(codingAgentDir),
		"--model 'openai/gpt-5.6-terra'", "AGM_PI_PERMISSION_MODE='auto'", "AGM_PI_PERMISSION_POLICY_FILE=", "policy-", "--extension",
	} {
		if !strings.Contains(command, want) {
			t.Errorf("Pi resume %q missing %q", command, want)
		}
	}
	if strings.Contains(command, "Bash(git:*)") {
		t.Fatalf("Pi resume inlined permission policy: %s", command)
	}
}

func TestBuildPiResumeCommandUsesCurrentCodingAgentDirectoryForLegacyMetadata(t *testing.T) {
	t.Setenv("AGM_PI_EXTENSION_ROOT", t.TempDir())
	codingAgentDir := t.TempDir()
	t.Setenv("PI_CODING_AGENT_DIR", codingAgentDir)
	m := &manifest.Manifest{Pi: &manifest.Pi{SessionID: "native-id", SessionDir: t.TempDir()}}
	command, err := buildPiResumeCommand(m, &HealthStatus{TmuxSessionName: "pi-worker", WorktreePath: "/tmp/work"}, "launch-resume")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(command, "PI_CODING_AGENT_DIR="+launchparity.ShellQuote(codingAgentDir)) {
		t.Fatalf("legacy Pi resume omitted current coding-agent directory: %s", command)
	}
}

func TestBuildPiResumeCommandPreservesPersistedNativeDefault(t *testing.T) {
	t.Setenv("AGM_PI_EXTENSION_ROOT", t.TempDir())
	t.Setenv("PI_CODING_AGENT_DIR", t.TempDir())
	m := &manifest.Manifest{Pi: &manifest.Pi{
		SessionID: "native-id", SessionDir: t.TempDir(), CodingAgentDirSet: true,
	}}
	command, err := buildPiResumeCommand(m, &HealthStatus{TmuxSessionName: "pi-worker", WorktreePath: "/tmp/work"}, "launch-resume")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(command, "env -u CLAUDECODE -u PI_CODING_AGENT_DIR") || strings.Contains(command, "PI_CODING_AGENT_DIR=") {
		t.Fatalf("new native-default Pi resume inherited caller config: %s", command)
	}
}

func TestBuildPiResumeCommandRejectsMissingNativeIdentity(t *testing.T) {
	_, err := buildPiResumeCommand(&manifest.Manifest{}, &HealthStatus{TmuxSessionName: "pi-worker", WorktreePath: "/tmp/work"}, "launch-resume")
	if err == nil || !strings.Contains(err.Error(), "exact native") {
		t.Fatalf("error = %v", err)
	}
}

func TestBuildPiResumeCommandWithoutModelProvenancePreservesNativeSelection(t *testing.T) {
	t.Setenv("AGM_PI_EXTENSION_ROOT", t.TempDir())
	sessionDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sessionDir, "native.jsonl"), []byte(`{"type":"session","id":"native-id","cwd":"/tmp/work"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := &manifest.Manifest{Pi: &manifest.Pi{SessionID: "native-id", SessionDir: sessionDir}}
	command, err := buildPiResumeCommand(m, &HealthStatus{TmuxSessionName: "pi-worker", WorktreePath: "/tmp/work"}, "launch-resume")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(command, " --model ") {
		t.Fatalf("Pi resume fabricated model override: %s", command)
	}
}

func TestBuildPiResumeCommandWithoutTranscriptUsesHarnessDefault(t *testing.T) {
	t.Setenv("AGM_PI_EXTENSION_ROOT", t.TempDir())
	m := &manifest.Manifest{Pi: &manifest.Pi{SessionID: "native-id", SessionDir: t.TempDir()}}
	command, err := buildPiResumeCommand(m, &HealthStatus{TmuxSessionName: "pi-worker", WorktreePath: "/tmp/work"}, "launch-resume")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(command, "--model 'anthropic/claude-sonnet-4-6'") {
		t.Fatalf("Pi unpersisted resume omitted the harness default model: %s", command)
	}
}

func TestBuildPiResumeCommandRejectsTranscriptIdentityMismatch(t *testing.T) {
	t.Setenv("AGM_PI_EXTENSION_ROOT", t.TempDir())
	dir := t.TempDir()
	path := filepath.Join(dir, "native.jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"session","id":"native-id","cwd":"/work"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := &manifest.Manifest{Pi: &manifest.Pi{
		SessionID: "native-id", SessionDir: dir, TranscriptPath: filepath.Join(dir, "different.jsonl"),
	}}
	_, err := buildPiResumeCommand(m, &HealthStatus{TmuxSessionName: "pi-worker", WorktreePath: "/tmp/work"}, "launch-resume")
	if err == nil || !strings.Contains(err.Error(), "persisted native identity") {
		t.Fatalf("error = %v", err)
	}
}

func TestBuildAgyResumeCommand(t *testing.T) {
	m := &manifest.Manifest{
		Model: "3.1-pro-high",
		Agy: &manifest.Agy{
			ConversationID: "117ff898-a964-4a9f-b460-1be4a8a49b17",
		},
	}
	health := &HealthStatus{
		WorktreePath: "/tmp/agy-work",
	}

	cmd := buildAgyResumeCommand(m, health)

	for _, want := range []string{
		"cd '/tmp/agy-work'",
		"agy --model 'Gemini 3.1 Pro (High)' --conversation '117ff898-a964-4a9f-b460-1be4a8a49b17'",
		"--add-dir '/tmp/agy-work'",
		"&& exit",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("command %q missing %q", cmd, want)
		}
	}
}

func TestBuildAgyResumeCommand_TranslatesLegacyModels(t *testing.T) {
	health := &HealthStatus{WorktreePath: "/tmp/agy-work"}
	tests := map[string]string{
		"2.5-pro":        "Gemini 3.1 Pro (High)",
		"2.0-flash-lite": "Gemini 3.5 Flash (Low)",
	}
	for legacy, current := range tests {
		t.Run(legacy, func(t *testing.T) {
			m := &manifest.Manifest{Model: legacy, Agy: &manifest.Agy{ConversationID: "legacy-conversation"}}
			command := buildAgyResumeCommand(m, health)
			if !strings.Contains(command, "--model '"+current+"'") {
				t.Fatalf("legacy model %q command = %q, want current label %q", legacy, command, current)
			}
			if strings.Contains(command, "--model '"+legacy+"'") {
				t.Fatalf("legacy model %q leaked into resume command %q", legacy, command)
			}
		})
	}
}

func TestMigrateAmbiguousLegacyAgyModelClearsStoredOverride(t *testing.T) {
	adapter, err := dolt.NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	for index, model := range []string{"2.5-flash", "gemini-2.5-flash"} {
		m := dolt.NewTestManifest(fmt.Sprintf("legacy-agy-%d", index), fmt.Sprintf("legacy-agy-%d", index))
		m.Harness = "agy"
		m.Model = model
		m.Agy = &manifest.Agy{ConversationID: fmt.Sprintf("native-%d", index)}
		if err := adapter.CreateSession(m); err != nil {
			t.Fatalf("CreateSession(%q): %v", model, err)
		}
		if err := migrateAmbiguousLegacyAgyModel(adapter, m, "agy"); err != nil {
			t.Fatalf("migrateAmbiguousLegacyAgyModel(%q): %v", model, err)
		}
		stored, err := adapter.GetSession(m.SessionID)
		if err != nil {
			t.Fatalf("GetSession(%q): %v", model, err)
		}
		if stored.Model != "" {
			t.Fatalf("stored model = %q, want ambiguous legacy default cleared", stored.Model)
		}
		command := buildAgyResumeCommand(stored, &HealthStatus{WorktreePath: "/tmp/agy-work"})
		if strings.Contains(command, "--model") {
			t.Fatalf("migrated resume command %q must omit --model", command)
		}
	}
}

func TestGetResumeManifestStopsCanceledMigration(t *testing.T) {
	adapter, err := dolt.NewSQLiteAdapter(filepath.Join(t.TempDir(), "agm.db"))
	if err != nil {
		t.Fatalf("NewSQLiteAdapter: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	m := dolt.NewTestManifest("canceled-legacy-agy", "canceled-legacy-agy")
	m.Harness = "agy"
	m.Model = "2.5-flash"
	m.Agy = &manifest.Agy{ConversationID: "native-conversation"}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := getResumeManifest(ctx, adapter, m.SessionID, "agy"); !errors.Is(err, context.Canceled) {
		t.Fatalf("getResumeManifest() error = %v, want context.Canceled", err)
	}
	stored, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if stored.Model != "2.5-flash" {
		t.Fatalf("stored model = %q, want canceled migration to preserve provenance", stored.Model)
	}
}

func TestBuildAgyResumeCommand_PreservesImportedConversationModel(t *testing.T) {
	health := &HealthStatus{WorktreePath: "/tmp/agy-work"}
	m := &manifest.Manifest{
		Agy: &manifest.Agy{ConversationID: "imported-conversation"},
	}

	command := buildAgyResumeCommand(m, health)
	if strings.Contains(command, "--model") {
		t.Fatalf("imported AGY resume command forced an unknown model: %q", command)
	}
	if !strings.Contains(command, "agy --conversation 'imported-conversation'") {
		t.Fatalf("imported AGY resume command = %q, want native conversation resume", command)
	}
}

func TestBuildAgyResumeCommand_AutoPermissionMode(t *testing.T) {
	m := &manifest.Manifest{
		Model:          "3.5-flash",
		PermissionMode: "auto",
		Agy: &manifest.Agy{
			ConversationID: "117ff898-a964-4a9f-b460-1be4a8a49b17",
		},
	}
	health := &HealthStatus{
		WorktreePath: "/tmp/agy-work",
	}

	cmd := buildAgyResumeCommand(m, health)

	if !strings.Contains(cmd, "agy --model 'Gemini 3.5 Flash (Medium)' --dangerously-skip-permissions --conversation '117ff898-a964-4a9f-b460-1be4a8a49b17'") {
		t.Errorf("auto AGY resume should skip permissions, got %q", cmd)
	}
}

func TestBuildAgyResumeCommand_FallbacksToNewSession(t *testing.T) {
	health := &HealthStatus{
		WorktreePath: "/tmp/agy-work",
	}

	cmd := buildAgyResumeCommand(&manifest.Manifest{}, health)

	if !strings.Contains(cmd, "cd '/tmp/agy-work' && agy --model 'Gemini 3.5 Flash (Medium)' && exit") {
		t.Errorf("expected fallback AGY launch command, got %q", cmd)
	}
	if strings.Contains(cmd, "--conversation") {
		t.Errorf("fallback AGY command should not include --conversation: %q", cmd)
	}
}

func TestBuildAgyResumeCommand_FallbacksToNewSessionWithAutoPermissionMode(t *testing.T) {
	health := &HealthStatus{
		WorktreePath: "/tmp/agy-work",
	}

	cmd := buildAgyResumeCommand(&manifest.Manifest{PermissionMode: "auto"}, health)

	if !strings.Contains(cmd, "cd '/tmp/agy-work' && agy --model 'Gemini 3.5 Flash (Medium)' --dangerously-skip-permissions && exit") {
		t.Errorf("expected fallback AGY launch command with auto permissions, got %q", cmd)
	}
}

func TestBuildCodexResumeCommand_ImportedSessionUsesCodexResume(t *testing.T) {
	sessionID := "019ef2af-97e0-7443-9f07-03e40636740c"
	m := &manifest.Manifest{
		Model: "5.4",
		Codex: &manifest.Codex{
			SessionID: sessionID,
		},
	}
	health := &HealthStatus{
		TmuxSessionName: "codex-session",
		WorktreePath:    "/tmp/work",
	}

	cmd := buildCodexResumeCommand(m, health)

	for _, want := range []string{
		"agm __exec-codex",
		"--session 'codex-session'",
		"--resume-id '" + sessionID + "'",
		"--remote",
		"--model 'gpt-5.4'",
		"--workdir '/tmp/work'",
		"--sandbox 'workspace-write'",
		"&& exit",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("command %q missing %q", cmd, want)
		}
	}
	if !strings.Contains(cmd, "--resume-id") {
		t.Errorf("imported Codex session did not preserve the saved session id: %s", cmd)
	}
}

// Regression tests for session-resume fix (commit e7cacf8)
// Bug: resume sent commands to existing tmux sessions, injecting text
// into the running agent which got processed as a user prompt.

func TestShouldSendResumeCommands_NeverSendsToExistingSession(t *testing.T) {
	// The fix ensures that when a tmux session already exists,
	// we NEVER send commands to it — just attach.
	if shouldSendResumeCommands(true) {
		t.Error("shouldSendResumeCommands(tmuxExists=true) = true, want false: must never send commands to existing sessions")
	}
}

func TestShouldSendResumeCommands_SendsWhenCreatingNew(t *testing.T) {
	// When no tmux session exists, we need to create one and send
	// the resume command to start the agent.
	if !shouldSendResumeCommands(false) {
		t.Error("shouldSendResumeCommands(tmuxExists=false) = false, want true: must send commands when creating new session")
	}
}

func TestShouldSendResumeCommands_TableDriven(t *testing.T) {
	tests := []struct {
		name        string
		tmuxExists  bool
		wantSend    bool
		description string
	}{
		{
			name:        "existing session - agent running",
			tmuxExists:  true,
			wantSend:    false,
			description: "Must not inject commands into running agent",
		},
		{
			name:        "existing session - agent idle",
			tmuxExists:  true,
			wantSend:    false,
			description: "Even if agent appears idle, detection is unreliable",
		},
		{
			name:        "existing session - detection error",
			tmuxExists:  true,
			wantSend:    false,
			description: "When detection fails, safe default is no commands",
		},
		{
			name:        "no session - must create and send",
			tmuxExists:  false,
			wantSend:    true,
			description: "New session needs resume command to start agent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldSendResumeCommands(tt.tmuxExists)
			if got != tt.wantSend {
				t.Errorf("shouldSendResumeCommands(tmuxExists=%v) = %v, want %v: %s",
					tt.tmuxExists, got, tt.wantSend, tt.description)
			}
		})
	}
}
