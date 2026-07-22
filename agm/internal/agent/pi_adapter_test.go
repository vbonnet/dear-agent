package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/launchparity"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

func withPiAdapterRuntime(t *testing.T) {
	t.Helper()
	t.Setenv("AGM_PI_EXTENSION_ROOT", t.TempDir())
	originalLookPath := lookPath
	originalHasSession := piHasSession
	originalNewSession := piNewSession
	originalSendShellCommand := piSendShellCommand
	originalSendCommandLiteral := piSendCommandLiteral
	originalSendPromptLiteral := piSendPromptLiteral
	originalWaitForPrompt := piWaitForPrompt
	originalKillSession := piKillSession
	originalCheckProcess := piCheckProcess
	originalCheckHarness := piCheckHarness
	originalAttachSession := piAttachSession
	originalIsIdle := piIsIdle
	t.Cleanup(func() {
		lookPath = originalLookPath
		piHasSession = originalHasSession
		piNewSession = originalNewSession
		piSendShellCommand = originalSendShellCommand
		piSendCommandLiteral = originalSendCommandLiteral
		piSendPromptLiteral = originalSendPromptLiteral
		piWaitForPrompt = originalWaitForPrompt
		piKillSession = originalKillSession
		piCheckProcess = originalCheckProcess
		piCheckHarness = originalCheckHarness
		piAttachSession = originalAttachSession
		piIsIdle = originalIsIdle
	})
	lookPath = func(file string) (string, error) { return "/test/bin/" + file, nil }
	piHasSession = func(string) (bool, error) { return false, nil }
	piNewSession = func(string, string) error { return nil }
	piSendShellCommand = func(string, string) error { return nil }
	piSendCommandLiteral = func(string, string) error { return nil }
	piSendPromptLiteral = func(string, string, bool) error { return nil }
	piWaitForPrompt = func(context.Context, string, string, time.Duration) error { return nil }
	piKillSession = func(string) error { return nil }
	piCheckProcess = func(string, string) (bool, error) { return false, nil }
	piCheckHarness = func(string, string) (tmux.PaneLiveness, error) {
		return tmux.PaneLiveness{SessionExists: true, RestartableShell: true, Evidence: "zsh"}, nil
	}
	piAttachSession = func(string) error { return nil }
	piIsIdle = func(string) (bool, error) { return true, nil }
}

func TestPiAdapterCreatePersistsNativeIdentityAndCanonicalCommand(t *testing.T) {
	withPiAdapterRuntime(t)
	t.Setenv("AGM_PI_SESSION_ROOT", t.TempDir())
	codingAgentDir := filepath.Join(t.TempDir(), "pi agent")
	if err := os.Mkdir(codingAgentDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_CODING_AGENT_DIR", codingAgentDir)
	var gotName, gotDir, gotCommand string
	piNewSession = func(name, dir string) error {
		gotName, gotDir = name, dir
		return nil
	}
	piSendShellCommand = func(name, command string) error {
		if name != "pi-worker" {
			t.Fatalf("send name = %q", name)
		}
		gotCommand = command
		return nil
	}
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	adapter, err := NewPiAdapter(store)
	if err != nil {
		t.Fatal(err)
	}
	sessionID, err := adapter.CreateSession(SessionContext{
		Name:             "pi-worker",
		WorkingDirectory: t.TempDir(),
		Model:            "sonnet",
		Environment: map[string]string{
			"AGM_PERMISSION_MODE":      "plan",
			"AGM_PI_PERMISSION_POLICY": `{"allow":["Read(/work/**)"]}`,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := store.Get(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.UUID != string(sessionID) {
		t.Fatalf("native id = %q, AGM id = %q", metadata.UUID, sessionID)
	}
	if metadata.NativeSessionDir == "" || !strings.HasPrefix(metadata.NativeSessionDir, os.Getenv("AGM_PI_SESSION_ROOT")) {
		t.Fatalf("native session dir = %q", metadata.NativeSessionDir)
	}
	if metadata.CodingAgentDir != codingAgentDir || !metadata.CodingAgentDirSet || !strings.Contains(gotCommand, "PI_CODING_AGENT_DIR="+launchparity.ShellQuote(codingAgentDir)) {
		t.Fatalf("Pi coding agent persistence/command = %q / %q", metadata.CodingAgentDir, gotCommand)
	}
	for _, token := range []string{"pi", "--session-id", string(sessionID), "PI_SESSION_ID='" + string(sessionID) + "'", "AGM_PI_PROJECT_DIR=", "--session-dir", metadata.NativeSessionDir, "--name", "pi-worker", "--model", "anthropic/claude-sonnet-4-6", "--extension", "agm-authorization.js", "AGM_PI_PERMISSION_MODE='plan'", "AGM_PI_PERMISSION_POLICY_FILE=", "policy-", "--tools", "read,grep,find,ls"} {
		if !strings.Contains(gotCommand, token) {
			t.Fatalf("command omits %q: %s", token, gotCommand)
		}
	}
	if metadata.PermissionPolicyJSON != `{"allow":["Read(/work/**)"]}` {
		t.Fatalf("permission policy = %q", metadata.PermissionPolicyJSON)
	}
	policyFiles, err := filepath.Glob(filepath.Join(os.Getenv("AGM_PI_EXTENSION_ROOT"), "policy-*.json"))
	if err != nil || len(policyFiles) != 1 {
		t.Fatalf("Pi policy files = %v, err=%v", policyFiles, err)
	}
	policy, err := os.ReadFile(policyFiles[0])
	if err != nil || string(policy) != metadata.PermissionPolicyJSON {
		t.Fatalf("Pi policy file = %q, err=%v", policy, err)
	}
	if strings.Contains(gotCommand, "Read(/work/**)") {
		t.Fatalf("Pi command inlined permission policy: %s", gotCommand)
	}
	if gotName != "pi-worker" || gotDir == "" {
		t.Fatalf("tmux create = %q, %q", gotName, gotDir)
	}
}

func TestPiAdapterCreateRollsBackReadinessFailure(t *testing.T) {
	withPiAdapterRuntime(t)
	t.Setenv("AGM_PI_SESSION_ROOT", t.TempDir())
	readyErr := errors.New("Pi fatal startup")
	piWaitForPrompt = func(context.Context, string, string, time.Duration) error { return readyErr }
	var killed string
	piKillSession = func(name string) error { killed = name; return nil }
	adapter, err := NewPiAdapter(&MockSessionStore{sessions: map[SessionID]*SessionMetadata{}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.CreateSession(SessionContext{Name: "pi-fail", WorkingDirectory: t.TempDir()})
	if !errors.Is(err, readyErr) {
		t.Fatalf("CreateSession error = %v", err)
	}
	if killed != "pi-fail" {
		t.Fatalf("killed = %q", killed)
	}
}

func TestPiAdapterResumeUsesPersistedNativeIdentityModelAndMode(t *testing.T) {
	withPiAdapterRuntime(t)
	workDir := t.TempDir()
	sessionDir := t.TempDir()
	codingAgentDir := t.TempDir()
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{
		"agm-id": {
			TmuxName: "pi-resume", WorkingDir: workDir, UUID: "native.pi-id",
			NativeSessionDir: sessionDir, Model: "gpt", PermissionMode: "auto",
			CodingAgentDir:       codingAgentDir,
			CodingAgentDirSet:    true,
			PermissionPolicyJSON: `{"allow":["Bash(git:*)"]}`,
		},
	}}
	var command, launchID string
	piSendShellCommand = func(_, value string) error { command = value; return nil }
	piWaitForPrompt = func(_ context.Context, _ string, gotLaunchID string, _ time.Duration) error {
		launchID = gotLaunchID
		return nil
	}
	adapter, err := NewPiAdapter(store)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.ResumeSession("agm-id"); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"--session-id", "native.pi-id", "--session-dir", sessionDir, "--model", "openai/gpt-5.6-terra", "--extension", "agm-authorization.js", "AGM_PI_PERMISSION_POLICY_FILE=", "policy-"} {
		if !strings.Contains(command, token) {
			t.Fatalf("resume command omits %q: %s", token, command)
		}
	}
	if !strings.Contains(command, "PI_CODING_AGENT_DIR="+launchparity.ShellQuote(codingAgentDir)) {
		t.Fatalf("Pi resume omitted persisted coding-agent directory: %s", command)
	}
	if strings.Contains(command, "Bash(git:*)") {
		t.Fatalf("Pi resume inlined permission policy: %s", command)
	}
	if launchID == "" || !strings.Contains(command, "AGM_PI_LAUNCH_ID='"+launchID+"'") {
		t.Fatalf("Pi resume command/readiness launch correlation = %q / %q", command, launchID)
	}
}

func TestPiAdapterResumePreservesPersistedNativeDefault(t *testing.T) {
	withPiAdapterRuntime(t)
	callerDir := t.TempDir()
	t.Setenv("PI_CODING_AGENT_DIR", callerDir)
	store := piResumeFixtureStore(t, "pi-native-default")
	store.sessions["agm-id"].CodingAgentDirSet = true
	var command string
	piSendShellCommand = func(_, value string) error { command = value; return nil }

	adapter, _ := NewPiAdapter(store)
	if err := adapter.ResumeSession("agm-id"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(command, "env -u CLAUDECODE -u PI_CODING_AGENT_DIR") || strings.Contains(command, "PI_CODING_AGENT_DIR=") {
		t.Fatalf("new native-default Pi adapter resume inherited %q: %s", callerDir, command)
	}
}

func TestPiAdapterResumeUsesExactProcessLivenessAndFailsSafe(t *testing.T) {
	withPiAdapterRuntime(t)
	t.Setenv("TMUX", "fixture")
	store := piResumeFixtureStore(t, "pi-live")
	piHasSession = func(string) (bool, error) { return true, nil }
	checked := false
	piCheckProcess = func(_, _ string) (bool, error) {
		checked = true
		return false, errors.New("fixture liveness unavailable")
	}
	sent := false
	piSendShellCommand = func(string, string) error { sent = true; return nil }

	adapter, _ := NewPiAdapter(store)
	err := adapter.ResumeSession("agm-id")
	if err == nil || !strings.Contains(err.Error(), "liveness unavailable") {
		t.Fatalf("ResumeSession error = %v, want liveness failure", err)
	}
	if !checked || sent {
		t.Fatalf("checked=%v sent=%v, want exact Pi fail-safe check", checked, sent)
	}
}

func TestPiAdapterResumeRejectsAnotherLiveHarnessBeforeMutation(t *testing.T) {
	withPiAdapterRuntime(t)
	t.Setenv("TMUX", "fixture")
	store := piResumeFixtureStore(t, "pi-collision")
	piHasSession = func(string) (bool, error) { return true, nil }
	piCheckProcess = func(string, string) (bool, error) { return false, nil }
	piCheckHarness = func(string, string) (tmux.PaneLiveness, error) {
		return tmux.PaneLiveness{SessionExists: true, HarnessAlive: true, Evidence: "zsh,claude"}, nil
	}
	sent := false
	piSendShellCommand = func(string, string) error { sent = true; return nil }

	adapter, _ := NewPiAdapter(store)
	err := adapter.ResumeSession("agm-id")
	if err == nil || !strings.Contains(err.Error(), "another live harness") {
		t.Fatalf("ResumeSession error = %v, want competing harness rejection", err)
	}
	if sent {
		t.Fatal("ResumeSession injected the Pi command into another live harness")
	}
}

func TestPiAdapterResumeRejectsNonShellForegroundBeforeMutation(t *testing.T) {
	withPiAdapterRuntime(t)
	t.Setenv("TMUX", "fixture")
	store := piResumeFixtureStore(t, "pi-editor")
	piHasSession = func(string) (bool, error) { return true, nil }
	piCheckProcess = func(string, string) (bool, error) { return false, nil }
	piCheckHarness = func(string, string) (tmux.PaneLiveness, error) {
		return tmux.PaneLiveness{SessionExists: true, Evidence: "zsh,vim"}, nil
	}
	sent := false
	piSendShellCommand = func(string, string) error { sent = true; return nil }

	adapter, _ := NewPiAdapter(store)
	err := adapter.ResumeSession("agm-id")
	if err == nil || !strings.Contains(err.Error(), "not a proven restartable shell") {
		t.Fatalf("ResumeSession error = %v, want non-shell rejection", err)
	}
	if sent {
		t.Fatal("ResumeSession injected the Pi command into a non-shell foreground process")
	}
}

func TestPiAdapterResumeRestartsOnlyInProvenBareShell(t *testing.T) {
	withPiAdapterRuntime(t)
	t.Setenv("TMUX", "fixture")
	store := piResumeFixtureStore(t, "pi-shell")
	piHasSession = func(string) (bool, error) { return true, nil }
	piCheckProcess = func(string, string) (bool, error) { return false, nil }
	piCheckHarness = func(string, string) (tmux.PaneLiveness, error) {
		return tmux.PaneLiveness{SessionExists: true, RestartableShell: true, Evidence: "zsh"}, nil
	}
	command := ""
	piSendShellCommand = func(_ string, value string) error { command = value; return nil }

	adapter, _ := NewPiAdapter(store)
	if err := adapter.ResumeSession("agm-id"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(command, "--session-id 'native.pi-id'") {
		t.Fatalf("bare-shell resume command = %q", command)
	}
}

func TestPiAdapterResumeLeavesLivePiUntouched(t *testing.T) {
	withPiAdapterRuntime(t)
	t.Setenv("TMUX", "fixture")
	store := piResumeFixtureStore(t, "pi-live")
	store.sessions["agm-id"].CodingAgentDir = filepath.Join(t.TempDir(), "removed-config")
	piHasSession = func(string) (bool, error) { return true, nil }
	piCheckProcess = func(string, string) (bool, error) { return true, nil }
	sent := false
	piSendShellCommand = func(string, string) error { sent = true; return nil }

	adapter, _ := NewPiAdapter(store)
	if err := adapter.ResumeSession("agm-id"); err != nil {
		t.Fatal(err)
	}
	if sent {
		t.Fatal("ResumeSession injected a command into an already-live Pi process")
	}
}

func TestPiAdapterResumeValidatesConfigBeforeCreatingRelaunchTmux(t *testing.T) {
	withPiAdapterRuntime(t)
	store := piResumeFixtureStore(t, "pi-relaunch")
	store.sessions["agm-id"].CodingAgentDir = filepath.Join(t.TempDir(), "missing-config")
	created := false
	piNewSession = func(string, string) error { created = true; return nil }

	adapter, _ := NewPiAdapter(store)
	err := adapter.ResumeSession("agm-id")
	if err == nil || !strings.Contains(err.Error(), "coding agent directory") {
		t.Fatalf("ResumeSession error = %v, want invalid coding-agent directory", err)
	}
	if created {
		t.Fatal("ResumeSession created tmux before validating relaunch configuration")
	}
}

func piResumeFixtureStore(t *testing.T, tmuxName string) *MockSessionStore {
	t.Helper()
	return &MockSessionStore{sessions: map[SessionID]*SessionMetadata{
		"agm-id": {
			TmuxName: tmuxName, WorkingDir: t.TempDir(), UUID: "native.pi-id",
			NativeSessionDir: t.TempDir(), Model: "sonnet", PermissionPolicyJSON: `{"allow":[]}`,
		},
	}}
}

func TestPiAdapterSendsMessagesLiterallyWithoutInterrupting(t *testing.T) {
	withPiAdapterRuntime(t)
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{
		"agm-id": {TmuxName: "pi-worker"},
	}}
	var gotName, gotMessage string
	var gotInterrupt bool
	piSendPromptLiteral = func(name, message string, interrupt bool) error {
		gotName, gotMessage, gotInterrupt = name, message, interrupt
		return nil
	}
	adapter, err := NewPiAdapter(store)
	if err != nil {
		t.Fatal(err)
	}
	message := "literal $(touch /tmp/nope)\nsecond line"
	if err := adapter.SendMessage("agm-id", Message{Content: message}); err != nil {
		t.Fatal(err)
	}
	if gotName != "pi-worker" || gotMessage != message || gotInterrupt {
		t.Fatalf("literal send = name %q message %q interrupt %v", gotName, gotMessage, gotInterrupt)
	}
}

func TestPiAdapterRejectsMalformedPermissionPolicyBeforeCreatingTmux(t *testing.T) {
	withPiAdapterRuntime(t)
	t.Setenv("AGM_PI_SESSION_ROOT", t.TempDir())
	created := false
	piNewSession = func(string, string) error { created = true; return nil }
	adapter, err := NewPiAdapter(&MockSessionStore{sessions: map[SessionID]*SessionMetadata{}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.CreateSession(SessionContext{
		Name: "bad-policy", WorkingDirectory: t.TempDir(),
		Environment: map[string]string{"AGM_PI_PERMISSION_POLICY": `{"allow":"everything"}`},
	})
	if err == nil || !strings.Contains(err.Error(), "Pi permission policy") {
		t.Fatalf("CreateSession error = %v", err)
	}
	if created {
		t.Fatal("tmux session created before permission policy validation")
	}
}

func TestPiAdapterHistoryReadsNativeTranscript(t *testing.T) {
	withPiAdapterRuntime(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	data := `{"type":"session","version":3,"id":"native-id","timestamp":"2026-07-21T00:00:00Z","cwd":"/tmp"}` + "\n" +
		`{"type":"message","id":"u1","parentId":null,"timestamp":"2026-07-21T00:00:01Z","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}` + "\n" +
		`{"type":"message","id":"a1","parentId":"u1","timestamp":"2026-07-21T00:00:02Z","message":{"role":"assistant","content":[{"type":"text","text":"world"}]}}` + "\n" +
		`{"type":"message","id":"t1","parentId":"a1","timestamp":"2026-07-21T00:00:03Z","message":{"role":"toolResult","content":[{"type":"text","text":"private tool output"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{
		"agm-id": {UUID: "native-id", NativeSessionDir: dir, TranscriptPath: path},
	}}
	adapter, err := NewPiAdapter(store)
	if err != nil {
		t.Fatal(err)
	}
	messages, err := adapter.GetHistory("agm-id")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Role != RoleUser || messages[1].Role != RoleAssistant {
		t.Fatalf("history = %#v", messages)
	}
}

func TestPiAdapterHistoryRejectsPersistedTranscriptMismatch(t *testing.T) {
	withPiAdapterRuntime(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"session","id":"native-id","cwd":"/tmp"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{
		"agm-id": {UUID: "native-id", NativeSessionDir: dir, TranscriptPath: filepath.Join(dir, "other.jsonl")},
	}}
	adapter, err := NewPiAdapter(store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.GetHistory("agm-id"); err == nil || !strings.Contains(err.Error(), "persisted native identity") {
		t.Fatalf("GetHistory mismatch error = %v", err)
	}
}

func TestPiAdapterImportsNativeTranscriptForColdResume(t *testing.T) {
	withPiAdapterRuntime(t)
	root := t.TempDir()
	t.Setenv("AGM_PI_SESSION_ROOT", root)
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	adapter, err := NewPiAdapter(store)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"type":"session","version":3,"id":"imported-id","timestamp":"2026-07-21T00:00:00Z","cwd":"` + t.TempDir() + `"}` + "\n" +
		`{"type":"model_change","provider":"openai","modelId":"gpt-5.6-terra"}` + "\n")
	sessionID, err := adapter.ImportConversation(data, FormatNative)
	if err != nil {
		t.Fatal(err)
	}
	if sessionID != "imported-id" {
		t.Fatalf("session id = %q", sessionID)
	}
	metadata, err := store.Get(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.UUID != "imported-id" || metadata.TranscriptPath == "" || metadata.NativeSessionDir != root {
		t.Fatalf("metadata = %#v", metadata)
	}
	if metadata.Model != "openai/gpt-5.6-terra" {
		t.Fatalf("imported model = %q", metadata.Model)
	}
}
