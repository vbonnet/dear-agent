package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func preserveAgyAdapterSeams(t *testing.T) {
	t.Helper()
	origHasSession := agyHasSession
	origNewSession := agyNewSession
	origSendCommand := agySendCommand
	origWaitForPrompt := agyWaitForPrompt
	origCheckProcess := agyCheckProcess
	origIsIdle := agyIsIdle
	origAttachSession := agyAttachSession
	origKillSession := agyKillSession
	origFindConversation := agyFindConversation
	origDiscoverySleep := agyDiscoverySleep
	t.Cleanup(func() {
		agyHasSession = origHasSession
		agyNewSession = origNewSession
		agySendCommand = origSendCommand
		agyWaitForPrompt = origWaitForPrompt
		agyCheckProcess = origCheckProcess
		agyIsIdle = origIsIdle
		agyAttachSession = origAttachSession
		agyKillSession = origKillSession
		agyFindConversation = origFindConversation
		agyDiscoverySleep = origDiscoverySleep
	})
}

// TestAgyAdapterImplementsAgentInterface verifies AgyAdapter implements Agent interface.
func TestAgyAdapterImplementsAgentInterface(t *testing.T) {
	// Create adapter with mock store
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	adapter, err := NewAgyAdapter(mockStore)
	if err != nil {
		t.Fatalf("NewAgyAdapter failed: %v", err)
	}

	// Verify adapter implements Agent interface (type already Agent from NewAgyAdapter)
	_ = adapter
}

// TestAgyAdapterName tests Name() method.
func TestAgyAdapterName(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	adapter, err := NewAgyAdapter(mockStore)
	if err != nil {
		t.Fatalf("NewAgyAdapter failed: %v", err)
	}

	if got := adapter.Name(); got != "agy" {
		t.Errorf("Name() = %q, want %q", got, "agy")
	}
}

// TestAgyAdapterVersion tests Version() method.
func TestAgyAdapterVersion(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	adapter, err := NewAgyAdapter(mockStore)
	if err != nil {
		t.Fatalf("NewAgyAdapter failed: %v", err)
	}

	if got := adapter.Version(); got != "Gemini 3.5 Flash (Medium)" {
		t.Errorf("Version() = %q, want current default AGY model", got)
	}
}

// TestAgyAdapterCapabilities tests Capabilities() method.
func TestAgyAdapterCapabilities(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	adapter, err := NewAgyAdapter(mockStore)
	if err != nil {
		t.Fatalf("NewAgyAdapter failed: %v", err)
	}

	caps := adapter.Capabilities()

	// Verify expected capabilities
	if !caps.SupportsSlashCommands {
		t.Error("SupportsSlashCommands should be true for Agy")
	}

	if !caps.SupportsTools {
		t.Error("SupportsTools should be true for Agy")
	}

	if caps.SupportsHooks {
		t.Error("SupportsHooks should be false while CommandRunHook has no native implementation")
	}

	if caps.MaxContextWindow != 200000 {
		t.Errorf("MaxContextWindow = %d, want 200000", caps.MaxContextWindow)
	}

	if caps.SupportsSystemPrompts {
		t.Error("SupportsSystemPrompts should be false while CommandSetSystemPrompt is unsupported")
	}

	if caps.ModelName != "Gemini 3.5 Flash (Medium)" {
		t.Errorf("ModelName = %q, want current default AGY model", caps.ModelName)
	}
}

func TestAgyCreateSessionUsesCanonicalModelAwareCommand(t *testing.T) {
	preserveAgyAdapterSeams(t)

	agyHasSession = func(string) (bool, error) { return false, nil }
	agyNewSession = func(string, string) error { return nil }

	var sent []string
	agySendCommand = func(_ string, cmd string) error {
		sent = append(sent, cmd)
		return nil
	}

	waited := false
	agyWaitForPrompt = func(_ context.Context, sessionName string, timeout time.Duration) error {
		waited = true
		if sessionName != "agy-wait-test" {
			t.Fatalf("agyWaitForPrompt session = %q, want agy-wait-test", sessionName)
		}
		if timeout != 30*time.Second {
			t.Fatalf("agyWaitForPrompt timeout = %v, want 30s", timeout)
		}
		return nil
	}

	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	adapter := &AgyAdapter{sessionStore: store}
	sessionID, err := adapter.CreateSession(SessionContext{
		Name:             "agy-wait-test",
		WorkingDirectory: "/work",
		Model:            "3.5-flash-low",
		AuthorizedDirs:   []string{"/extra dir"},
		Environment: map[string]string{
			"AGM_PERMISSION_MODE": "auto",
			"AGY_CONVERSATION_ID": "native-conversation-id",
		},
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if !waited {
		t.Fatal("CreateSession did not wait for the AGY prompt")
	}
	if len(sent) != 1 {
		t.Fatalf("CreateSession sent commands = %v, want one canonical launch", sent)
	}
	for _, want := range []string{
		"agy --model 'Gemini 3.5 Flash (Low)'",
		"--dangerously-skip-permissions",
		"--conversation 'native-conversation-id'",
		"--add-dir '/extra dir'",
		"&& exit",
	} {
		if !strings.Contains(sent[0], want) {
			t.Errorf("CreateSession command %q missing %q", sent[0], want)
		}
	}
	if strings.Contains(sent[0], "--prompt-interactive") {
		t.Errorf("CreateSession used prompt-valued flag without a prompt: %q", sent[0])
	}
	metadata, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("stored session metadata: %v", err)
	}
	if metadata.Model != "3.5-flash-low" || metadata.PermissionMode != "auto" || metadata.UUID != "native-conversation-id" {
		t.Fatalf("stored model/mode/native ID = %q/%q/%q", metadata.Model, metadata.PermissionMode, metadata.UUID)
	}
	if len(metadata.AuthorizedDirs) != 1 || metadata.AuthorizedDirs[0] != "/extra dir" {
		t.Fatalf("stored authorized dirs = %v", metadata.AuthorizedDirs)
	}
}

func TestAgyCreateSessionCapturesNativeConversationIdentity(t *testing.T) {
	preserveAgyAdapterSeams(t)
	agyHasSession = func(string) (bool, error) { return false, nil }
	agyNewSession = func(string, string) error { return nil }
	agySendCommand = func(string, string) error { return nil }
	agyWaitForPrompt = func(context.Context, string, time.Duration) error { return nil }
	var discoveredWorkDir string
	discoveryCalls := 0
	agyFindConversation = func(workDir string) (string, error) {
		discoveryCalls++
		discoveredWorkDir = workDir
		if discoveryCalls == 1 {
			return "pre-existing-conversation-id", nil
		}
		return "provider-conversation-id", nil
	}

	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	sessionID, err := (&AgyAdapter{sessionStore: store}).CreateSession(SessionContext{
		Name: "agy-fresh", WorkingDirectory: "/work", Model: "3.5-flash-low",
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	metadata, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("stored session metadata: %v", err)
	}
	if discoveredWorkDir != "/work" || discoveryCalls != 2 || metadata.UUID != "provider-conversation-id" {
		t.Fatalf("discovered workdir/calls/native ID = %q/%d/%q", discoveredWorkDir, discoveryCalls, metadata.UUID)
	}
}

func TestAgyCreateSessionRollsBackWhenNativeIdentityCannotBeCaptured(t *testing.T) {
	preserveAgyAdapterSeams(t)
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	agyHasSession = func(string) (bool, error) { return false, nil }
	agyNewSession = func(string, string) error { return nil }
	agySendCommand = func(string, string) error { return nil }
	agyWaitForPrompt = func(context.Context, string, time.Duration) error { return nil }
	wantErr := errors.New("fixture provider metadata unavailable")
	attempts := 0
	agyFindConversation = func(string) (string, error) { attempts++; return "", wantErr }
	agyDiscoverySleep = func(time.Duration) {}
	killed := ""
	agyKillSession = func(name string) { killed = name }

	sessionID, err := (&AgyAdapter{sessionStore: store}).CreateSession(SessionContext{
		Name: "agy-no-identity", WorkingDirectory: "/work", Model: "3.5-flash-low",
	})
	if !errors.Is(err, wantErr) || sessionID != "" {
		t.Fatalf("CreateSession = %q, %v; want empty ID and discovery failure", sessionID, err)
	}
	if attempts != agyConversationDiscoveryAttempts+1 || killed != "agy-no-identity" {
		t.Fatalf("discovery attempts/rollback = %d/%q", attempts, killed)
	}
	if sessions, listErr := store.List(); listErr != nil || len(sessions) != 0 {
		t.Fatalf("failed create persisted sessions = %v, %v", sessions, listErr)
	}
}

func TestAgyCreateSessionDoesNotReuseStaleNativeConversationIdentity(t *testing.T) {
	preserveAgyAdapterSeams(t)
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	agyHasSession = func(string) (bool, error) { return false, nil }
	agyNewSession = func(string, string) error { return nil }
	agySendCommand = func(string, string) error { return nil }
	agyWaitForPrompt = func(context.Context, string, time.Duration) error { return nil }
	agyFindConversation = func(string) (string, error) { return "stale-conversation-id", nil }
	agyDiscoverySleep = func(time.Duration) {}
	killed := ""
	agyKillSession = func(name string) { killed = name }

	sessionID, err := (&AgyAdapter{sessionStore: store}).CreateSession(SessionContext{
		Name: "agy-stale-identity", WorkingDirectory: "/work", Model: "3.5-flash-low",
	})
	if err == nil || !strings.Contains(err.Error(), "pre-create conversation") || sessionID != "" {
		t.Fatalf("CreateSession = %q, %v; want stale identity failure", sessionID, err)
	}
	if killed != "agy-stale-identity" {
		t.Fatalf("stale identity rollback killed %q", killed)
	}
	if sessions, listErr := store.List(); listErr != nil || len(sessions) != 0 {
		t.Fatalf("stale identity persisted sessions = %v, %v", sessions, listErr)
	}
}

func TestAgyCreateSessionPropagatesReadinessFailureAndRollsBack(t *testing.T) {
	preserveAgyAdapterSeams(t)
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	agyHasSession = func(string) (bool, error) { return false, nil }
	agyNewSession = func(string, string) error { return nil }
	agySendCommand = func(string, string) error { return nil }
	wantErr := errors.New("fixture readiness failed")
	agyWaitForPrompt = func(context.Context, string, time.Duration) error { return wantErr }
	killed := ""
	agyKillSession = func(name string) { killed = name }

	sessionID, err := (&AgyAdapter{sessionStore: store}).CreateSession(SessionContext{
		Name: "agy-create", WorkingDirectory: "/work", Model: "3.5-flash-low",
	})
	if !errors.Is(err, wantErr) || sessionID != "" {
		t.Fatalf("CreateSession = %q, %v; want empty ID and readiness failure", sessionID, err)
	}
	if killed != "agy-create" {
		t.Fatalf("readiness rollback killed %q, want agy-create", killed)
	}
	if sessions, listErr := store.List(); listErr != nil || len(sessions) != 0 {
		t.Fatalf("failed create persisted sessions = %v, %v", sessions, listErr)
	}
}

func TestAgyCreateSessionRejectsExistingTmuxAndUnsafeModelBeforeMutation(t *testing.T) {
	preserveAgyAdapterSeams(t)
	created, sent := false, false
	agyNewSession = func(string, string) error { created = true; return nil }
	agySendCommand = func(string, string) error { sent = true; return nil }
	adapter := &AgyAdapter{sessionStore: &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}}

	agyHasSession = func(string) (bool, error) { return true, nil }
	_, err := adapter.CreateSession(SessionContext{Name: "existing", WorkingDirectory: "/work"})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing tmux error = %v", err)
	}

	agyHasSession = func(string) (bool, error) { return false, nil }
	_, err = adapter.CreateSession(SessionContext{Name: "unsafe", WorkingDirectory: "/work", Model: "safe; touch /tmp/no"})
	if err == nil || !strings.Contains(err.Error(), "invalid AGY model") {
		t.Fatalf("unsafe model error = %v", err)
	}
	if created || sent {
		t.Fatalf("rejected create mutated tmux: created=%v sent=%v", created, sent)
	}
}

func TestAgyResumePolicyPersistsInJSONSessionStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	store, err := NewJSONSessionStore(path)
	if err != nil {
		t.Fatal(err)
	}
	sessionID := SessionID("agy-persisted")
	want := &SessionMetadata{
		TmuxName: "agy-persisted", WorkingDir: "/work", UUID: "native-id",
		Model: "3.5-flash-low", PermissionMode: "auto", AuthorizedDirs: []string{"/extra"},
	}
	if err := store.Set(sessionID, want); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewJSONSessionStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Get(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("persisted AGY resume metadata = %+v, want %+v", got, want)
	}
}

func TestAgyResumeSessionPreservesNativeIdentityModelAndMode(t *testing.T) {
	preserveAgyAdapterSeams(t)
	t.Setenv("TMUX", "fixture")
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	sessionID := SessionID("adapter-session")
	if err := store.Set(sessionID, &SessionMetadata{
		TmuxName: "agy-resume", WorkingDir: "/work dir", UUID: "native-conversation-id",
		Model: "claude-sonnet-4.6-thinking", PermissionMode: "auto", AuthorizedDirs: []string{"/extra dir"},
	}); err != nil {
		t.Fatal(err)
	}
	agyHasSession = func(string) (bool, error) { return false, nil }
	created := false
	agyNewSession = func(name, dir string) error {
		created = name == "agy-resume" && dir == "/work dir"
		return nil
	}
	var command string
	agySendCommand = func(_ string, value string) error { command = value; return nil }
	agyWaitForPrompt = func(context.Context, string, time.Duration) error { return nil }

	adapter := &AgyAdapter{sessionStore: store}
	if err := adapter.ResumeSession(sessionID); err != nil {
		t.Fatalf("ResumeSession: %v", err)
	}
	if !created {
		t.Fatal("ResumeSession did not recreate the missing tmux session")
	}
	for _, want := range []string{
		"agy --model 'Claude Sonnet 4.6 (Thinking)'",
		"--dangerously-skip-permissions",
		"--conversation 'native-conversation-id'",
		"--add-dir '/extra dir'",
	} {
		if !strings.Contains(command, want) {
			t.Errorf("resume command %q missing %q", command, want)
		}
	}
}

func TestAgyResumeSessionOmitsModelWhenProvenanceUnknown(t *testing.T) {
	preserveAgyAdapterSeams(t)
	t.Setenv("TMUX", "fixture")
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	sessionID := SessionID("imported-session")
	if err := store.Set(sessionID, &SessionMetadata{
		TmuxName: "agy-imported", WorkingDir: "/work", UUID: "native-conversation-id",
	}); err != nil {
		t.Fatal(err)
	}
	agyHasSession = func(string) (bool, error) { return false, nil }
	agyNewSession = func(string, string) error { return nil }
	var command string
	agySendCommand = func(_ string, value string) error { command = value; return nil }
	agyWaitForPrompt = func(context.Context, string, time.Duration) error { return nil }

	if err := (&AgyAdapter{sessionStore: store}).ResumeSession(sessionID); err != nil {
		t.Fatalf("ResumeSession: %v", err)
	}
	if strings.Contains(command, "--model") {
		t.Fatalf("unknown model resume command %q must omit --model", command)
	}
	if !strings.Contains(command, "--conversation 'native-conversation-id'") {
		t.Fatalf("resume command %q missing native conversation ID", command)
	}
}

func TestAgyResumeSessionDoesNotInventNativeIdentity(t *testing.T) {
	preserveAgyAdapterSeams(t)
	t.Setenv("TMUX", "fixture")
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	sessionID := SessionID("internal-session-id")
	if err := store.Set(sessionID, &SessionMetadata{TmuxName: "agy-missing", WorkingDir: "/work"}); err != nil {
		t.Fatal(err)
	}
	agyHasSession = func(string) (bool, error) { return false, nil }
	created, sent := false, false
	agyNewSession = func(string, string) error { created = true; return nil }
	agySendCommand = func(string, string) error { sent = true; return nil }

	err := (&AgyAdapter{sessionStore: store}).ResumeSession(sessionID)
	if err == nil || !strings.Contains(err.Error(), "no native conversation ID") {
		t.Fatalf("ResumeSession error = %v, want missing native ID", err)
	}
	if created || sent {
		t.Fatalf("missing native ID mutated tmux: created=%v sent=%v", created, sent)
	}
}

func TestAgyResumeSessionUsesExactProcessLivenessAndFailsSafe(t *testing.T) {
	preserveAgyAdapterSeams(t)
	t.Setenv("TMUX", "fixture")
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	sessionID := SessionID("adapter-session")
	if err := store.Set(sessionID, &SessionMetadata{TmuxName: "agy-live", UUID: "native-id"}); err != nil {
		t.Fatal(err)
	}
	agyHasSession = func(string) (bool, error) { return true, nil }
	var processName string
	agyCheckProcess = func(_, _, process string) (bool, error) {
		processName = process
		return false, errors.New("fixture liveness unavailable")
	}
	sent := false
	agySendCommand = func(string, string) error { sent = true; return nil }

	err := (&AgyAdapter{sessionStore: store}).ResumeSession(sessionID)
	if err == nil || !strings.Contains(err.Error(), "liveness unavailable") {
		t.Fatalf("ResumeSession error = %v, want liveness failure", err)
	}
	if processName != "agy" || sent {
		t.Fatalf("process=%q sent=%v, want exact AGY fail-safe check", processName, sent)
	}
}

func TestAgyResumeSessionLeavesLiveAgyUntouched(t *testing.T) {
	preserveAgyAdapterSeams(t)
	t.Setenv("TMUX", "fixture")
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	sessionID := SessionID("adapter-session")
	if err := store.Set(sessionID, &SessionMetadata{TmuxName: "agy-live", UUID: "native-id"}); err != nil {
		t.Fatal(err)
	}
	agyHasSession = func(string) (bool, error) { return true, nil }
	agyCheckProcess = func(_, _, process string) (bool, error) { return process == "agy", nil }
	sent := false
	agySendCommand = func(string, string) error { sent = true; return nil }

	if err := (&AgyAdapter{sessionStore: store}).ResumeSession(sessionID); err != nil {
		t.Fatalf("ResumeSession: %v", err)
	}
	if sent {
		t.Fatal("ResumeSession injected a command into an already-live AGY process")
	}
}

func TestAgyResumeSessionPropagatesReadinessFailureBeforeAttach(t *testing.T) {
	preserveAgyAdapterSeams(t)
	t.Setenv("TMUX", "")
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	sessionID := SessionID("adapter-session")
	if err := store.Set(sessionID, &SessionMetadata{
		TmuxName: "agy-resume", WorkingDir: "/work", UUID: "native-id", Model: "3.5-flash-low",
	}); err != nil {
		t.Fatal(err)
	}
	agyHasSession = func(string) (bool, error) { return false, nil }
	agyNewSession = func(string, string) error { return nil }
	agySendCommand = func(string, string) error { return nil }
	wantErr := errors.New("fixture readiness failed")
	agyWaitForPrompt = func(context.Context, string, time.Duration) error { return wantErr }
	attached := false
	agyAttachSession = func(string) error { attached = true; return nil }
	killed := ""
	agyKillSession = func(name string) { killed = name }

	err := (&AgyAdapter{sessionStore: store}).ResumeSession(sessionID)
	if !errors.Is(err, wantErr) {
		t.Fatalf("ResumeSession error = %v, want readiness failure", err)
	}
	if attached {
		t.Fatal("readiness failure continued into tmux attach")
	}
	if killed != "agy-resume" {
		t.Fatalf("readiness rollback killed %q, want agy-resume", killed)
	}
}

func TestAgyGetSessionStatusRequiresAgyProcess(t *testing.T) {
	preserveAgyAdapterSeams(t)
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	sessionID := SessionID("adapter-session")
	if err := store.Set(sessionID, &SessionMetadata{TmuxName: "agy-status"}); err != nil {
		t.Fatal(err)
	}
	agyHasSession = func(string) (bool, error) { return true, nil }
	agyCheckProcess = func(_, _, process string) (bool, error) { return process == "agy", nil }
	agyIsIdle = func(string) (bool, error) { return true, nil }

	adapter := &AgyAdapter{sessionStore: store}
	status, err := adapter.GetSessionStatus(sessionID)
	if err != nil || status != StatusIdle {
		t.Fatalf("live idle AGY status = %q, %v", status, err)
	}
	agyCheckProcess = func(string, string, string) (bool, error) { return false, nil }
	status, err = adapter.GetSessionStatus(sessionID)
	if err != nil || status != StatusTerminated {
		t.Fatalf("shell-only tmux status = %q, %v, want terminated", status, err)
	}
}

func TestAgyGetHistoryReadsNativeTranscript(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	conversationID := "native-conversation-id"
	logsDir := filepath.Join(home, ".gemini", "antigravity-cli", "brain", conversationID, ".system_generated", "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fixture := strings.Join([]string{
		`{"step_index":1,"source":"SYSTEM","type":"CHECKPOINT","status":"DONE","created_at":"2026-07-20T18:23:20Z","content":"system"}`,
		`not-json`,
		`{"step_index":2,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","created_at":"2026-07-20T18:23:21Z","content":"hello"}`,
		`{"step_index":3,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-07-20T18:23:22Z","content":"world"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(logsDir, "transcript.jsonl"), []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	sessionID := SessionID("adapter-session")
	if err := store.Set(sessionID, &SessionMetadata{UUID: conversationID}); err != nil {
		t.Fatal(err)
	}

	messages, err := (&AgyAdapter{sessionStore: store}).GetHistory(sessionID)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(messages) != 2 || messages[0].Role != RoleUser || messages[0].Content != "hello" || messages[1].Role != RoleAssistant || messages[1].Content != "world" {
		t.Fatalf("native AGY messages = %+v", messages)
	}
	if got := messages[1].Timestamp.Format(time.RFC3339); got != "2026-07-20T18:23:22Z" {
		t.Fatalf("assistant timestamp = %q", got)
	}
}

func TestAgyGetHistoryFallsBackToFullTranscript(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	conversationID := "native-full-id"
	logsDir := filepath.Join(home, ".gemini", "antigravity-cli", "brain", conversationID, ".system_generated", "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"step_index":4,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-07-20T18:23:22Z","content":"fallback"}` + "\n"
	if err := os.WriteFile(filepath.Join(logsDir, "transcript_full.jsonl"), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	sessionID := SessionID("adapter-session")
	if err := store.Set(sessionID, &SessionMetadata{UUID: conversationID}); err != nil {
		t.Fatal(err)
	}
	messages, err := (&AgyAdapter{sessionStore: store}).GetHistory(sessionID)
	if err != nil || len(messages) != 1 || messages[0].Content != "fallback" {
		t.Fatalf("full transcript fallback = %+v, %v", messages, err)
	}
}

func TestAgyGetHistoryRequiresNativeIdentity(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	sessionID := SessionID("adapter-session")
	if err := store.Set(sessionID, &SessionMetadata{}); err != nil {
		t.Fatal(err)
	}
	_, err := (&AgyAdapter{sessionStore: store}).GetHistory(sessionID)
	if err == nil || !strings.Contains(err.Error(), "no native conversation ID") {
		t.Fatalf("GetHistory error = %v, want missing native ID", err)
	}
}

func TestAgyAdapterRejectsUnsupportedRunHook(t *testing.T) {
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	sessionID := SessionID("agy-hook-session")
	if err := store.Set(sessionID, &SessionMetadata{TmuxName: "agy-hook"}); err != nil {
		t.Fatalf("store.Set: %v", err)
	}

	adapter := &AgyAdapter{sessionStore: store}
	err := adapter.ExecuteCommand(Command{
		Type: CommandRunHook,
		Params: map[string]any{
			"session_id": string(sessionID),
			"hook_name":  "SessionStart",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "not implemented for AGY") {
		t.Fatalf("ExecuteCommand(CommandRunHook) error = %v, want explicit unsupported error", err)
	}
}
