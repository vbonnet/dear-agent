package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/agysession"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

type stubAgyIdentityTracker struct {
	snapshot func(context.Context, string) (string, error)
	discover func(context.Context, string, string) (*agysession.Metadata, error)
}

func (tracker *stubAgyIdentityTracker) Snapshot(ctx context.Context, workDir string) (string, error) {
	return tracker.snapshot(ctx, workDir)
}

func (tracker *stubAgyIdentityTracker) Discover(ctx context.Context, workDir, previousConversationID string) (*agysession.Metadata, error) {
	return tracker.discover(ctx, workDir, previousConversationID)
}

func useStubAgyIdentityTracker(
	snapshot func(context.Context, string) (string, error),
	discover func(context.Context, string, string) (*agysession.Metadata, error),
) {
	agyIdentityTracker = func() agysession.CreateIdentityTracker {
		return &stubAgyIdentityTracker{snapshot: snapshot, discover: discover}
	}
}

func preserveAgyAdapterSeams(t *testing.T) {
	t.Helper()
	origHasSession := agyHasSession
	origNewSession := agyNewSession
	origSendCommand := agySendCommand
	origSendPromptLiteral := agySendPromptLiteral
	origWaitForPrompt := agyWaitForPrompt
	origWaitForResumePrompt := agyWaitForResumePrompt
	origCheckProcess := agyCheckProcess
	origCheckHarness := agyCheckHarness
	origIsIdle := agyIsIdle
	origAttachSession := agyAttachSession
	origKillSession := agyKillSession
	origIdentityTracker := agyIdentityTracker
	origAcquireCreateLock := agyAcquireCreateLock
	agyAcquireCreateLock = func(string) (func() error, error) {
		return func() error { return nil }, nil
	}
	t.Cleanup(func() {
		agyHasSession = origHasSession
		agyNewSession = origNewSession
		agySendCommand = origSendCommand
		agySendPromptLiteral = origSendPromptLiteral
		agyWaitForPrompt = origWaitForPrompt
		agyWaitForResumePrompt = origWaitForResumePrompt
		agyCheckProcess = origCheckProcess
		agyCheckHarness = origCheckHarness
		agyIsIdle = origIsIdle
		agyAttachSession = origAttachSession
		agyKillSession = origKillSession
		agyIdentityTracker = origIdentityTracker
		agyAcquireCreateLock = origAcquireCreateLock
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

func TestAgyCreateSessionImportedConversationOmitsUnknownModel(t *testing.T) {
	preserveAgyAdapterSeams(t)
	agyHasSession = func(string) (bool, error) { return false, nil }
	agyNewSession = func(string, string) error { return nil }
	command := ""
	agySendCommand = func(_ string, value string) error { command = value; return nil }
	agyWaitForPrompt = func(context.Context, string, time.Duration) error { return nil }

	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	sessionID, err := (&AgyAdapter{sessionStore: store}).CreateSession(SessionContext{
		Name:             "agy-imported",
		WorkingDirectory: "/work",
		Environment:      map[string]string{"AGY_CONVERSATION_ID": "imported-native-id"},
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if strings.Contains(command, "--model") {
		t.Fatalf("imported AGY command invented model provenance: %q", command)
	}
	metadata, err := store.Get(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Model != "" || metadata.UUID != "imported-native-id" {
		t.Fatalf("stored model/native ID = %q/%q, want unknown model and imported ID", metadata.Model, metadata.UUID)
	}
}

func TestAgyCreateRejectsTerminalControlsBeforeConfiguredTmuxSender(t *testing.T) {
	preserveAgyAdapterSeams(t)
	agyHasSession = func(string) (bool, error) { return false, nil }
	agyNewSession = func(string, string) error { return nil }
	sent := false
	agySendCommand = func(string, string) error { sent = true; return nil }
	rolledBack := false
	agyKillSession = func(string) error { rolledBack = true; return nil }

	_, err := (&AgyAdapter{sessionStore: &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}}).CreateSession(SessionContext{
		Name:             "agy-terminal-control",
		WorkingDirectory: t.TempDir(),
		AuthorizedDirs:   []string{"safe\nunsafe"},
		Environment:      map[string]string{"AGY_CONVERSATION_ID": "imported-native-id"},
	})
	if err == nil || !strings.Contains(err.Error(), "pasted shell value") {
		t.Fatalf("CreateSession() error = %v, want terminal-control rejection", err)
	}
	if sent {
		t.Fatal("AGY create contacted its configured tmux sender before validation")
	}
	if !rolledBack {
		t.Fatal("AGY create did not roll back the tmux session after validation failure")
	}
}

func TestAgyColdResumeRejectsTerminalControlsBeforeConfiguredTmuxSender(t *testing.T) {
	preserveAgyAdapterSeams(t)
	t.Setenv("TMUX", "fixture")
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{
		"agy-id": {
			TmuxName:       "agy-resume",
			WorkingDir:     t.TempDir(),
			UUID:           "imported-native-id",
			AuthorizedDirs: []string{"safe\nunsafe"},
		},
	}}
	agyHasSession = func(string) (bool, error) { return false, nil }
	agyNewSession = func(string, string) error { return nil }
	sent := false
	agySendCommand = func(string, string) error { sent = true; return nil }
	rolledBack := false
	agyKillSession = func(string) error { rolledBack = true; return nil }

	err := (&AgyAdapter{sessionStore: store}).ResumeSession("agy-id")
	if err == nil || !strings.Contains(err.Error(), "pasted shell value") {
		t.Fatalf("ResumeSession() error = %v, want terminal-control rejection", err)
	}
	if sent {
		t.Fatal("AGY resume contacted its configured tmux sender before validation")
	}
	if !rolledBack {
		t.Fatal("AGY resume did not roll back the tmux session after validation failure")
	}
}

func TestAgyCreateSessionCapturesNativeConversationIdentity(t *testing.T) {
	preserveAgyAdapterSeams(t)
	agyHasSession = func(string) (bool, error) { return false, nil }
	agyNewSession = func(string, string) error { return nil }
	agySendCommand = func(string, string) error { return nil }
	agyWaitForPrompt = func(context.Context, string, time.Duration) error { return nil }
	var discoveredWorkDir string
	snapshotCalls, discoveryCalls := 0, 0
	useStubAgyIdentityTracker(
		func(_ context.Context, workDir string) (string, error) {
			snapshotCalls++
			discoveredWorkDir = workDir
			return "pre-existing-conversation-id", nil
		},
		func(_ context.Context, workDir, previousConversationID string) (*agysession.Metadata, error) {
			discoveryCalls++
			discoveredWorkDir = workDir
			if previousConversationID != "pre-existing-conversation-id" {
				t.Fatalf("previous conversation ID = %q", previousConversationID)
			}
			return &agysession.Metadata{ConversationID: "provider-conversation-id", WorkspacePath: workDir}, nil
		},
	)

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
	if discoveredWorkDir != "/work" || snapshotCalls != 1 || discoveryCalls != 1 || metadata.UUID != "provider-conversation-id" {
		t.Fatalf("discovered workdir/snapshot/discovery calls/native ID = %q/%d/%d/%q", discoveredWorkDir, snapshotCalls, discoveryCalls, metadata.UUID)
	}
}

func TestAgyCreateSessionBootstrapsLazyNativeIdentityWithInitialPrompt(t *testing.T) {
	preserveAgyAdapterSeams(t)
	agyHasSession = func(string) (bool, error) { return false, nil }
	agyNewSession = func(string, string) error { return nil }
	launches := 0
	agySendCommand = func(string, string) error { launches++; return nil }
	agyWaitForPrompt = func(context.Context, string, time.Duration) error { return nil }
	promptDeliveries := 0
	agySendPromptLiteral = func(sessionName, prompt string, interrupt bool, harness string) error {
		if sessionName != "agy-lazy-adapter" || prompt != "persist adapter\nprompt" || interrupt || harness != "agy" {
			t.Fatalf("initial prompt delivery = %q/%q/%t/%q", sessionName, prompt, interrupt, harness)
		}
		promptDeliveries++
		return nil
	}
	useStubAgyIdentityTracker(
		func(context.Context, string) (string, error) { return "old-native-id", nil },
		func(_ context.Context, workDir, previous string) (*agysession.Metadata, error) {
			if promptDeliveries != 1 || previous != "old-native-id" {
				t.Fatalf("identity discovery state = prompt deliveries:%d previous:%q", promptDeliveries, previous)
			}
			return &agysession.Metadata{ConversationID: "new-native-id", WorkspacePath: workDir}, nil
		},
	)

	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	id, err := (&AgyAdapter{sessionStore: store}).CreateSession(SessionContext{
		Name: "agy-lazy-adapter", WorkingDirectory: "/work", Model: "3.5-flash-low", InitialPrompt: "persist adapter\nprompt",
	})
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := store.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if launches != 1 || promptDeliveries != 1 || metadata.UUID != "new-native-id" {
		t.Fatalf("adapter lifecycle = launches:%d prompts:%d native ID:%q", launches, promptDeliveries, metadata.UUID)
	}
}

func TestAgyCreateSessionRollsBackWhenInitialPromptBootstrapFails(t *testing.T) {
	preserveAgyAdapterSeams(t)
	agyHasSession = func(string) (bool, error) { return false, nil }
	agyNewSession = func(string, string) error { return nil }
	agySendCommand = func(string, string) error { return nil }
	agyWaitForPrompt = func(context.Context, string, time.Duration) error { return nil }
	wantErr := errors.New("fixture initial prompt failure")
	agySendPromptLiteral = func(string, string, bool, string) error { return wantErr }
	discovered := false
	useStubAgyIdentityTracker(
		func(context.Context, string) (string, error) { return "old-native-id", nil },
		func(context.Context, string, string) (*agysession.Metadata, error) {
			discovered = true
			return nil, nil
		},
	)
	killed := ""
	agyKillSession = func(name string) error { killed = name; return nil }

	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	_, err := (&AgyAdapter{sessionStore: store}).CreateSession(SessionContext{
		Name: "agy-bootstrap-failure", WorkingDirectory: "/work", Model: "3.5-flash-low", InitialPrompt: "must fail",
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("CreateSession error = %v, want %v", err, wantErr)
	}
	if discovered || killed != "agy-bootstrap-failure" {
		t.Fatalf("failure state = discovered:%t killed:%q", discovered, killed)
	}
	if sessions, listErr := store.List(); listErr != nil || len(sessions) != 0 {
		t.Fatalf("failed create persisted sessions = %v, %v", sessions, listErr)
	}
}

func TestAgySendMessageUsesHarnessAwareMultilineDelivery(t *testing.T) {
	preserveAgyAdapterSeams(t)
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	sessionID := SessionID("agy-message-session")
	if err := store.Set(sessionID, &SessionMetadata{TmuxName: "agy-message-pane"}); err != nil {
		t.Fatal(err)
	}

	wantMessage := "[From: codex]\n\nfirst line\nsecond line"
	called := 0
	agySendPromptLiteral = func(sessionName, prompt string, interrupt bool, harness string) error {
		called++
		if sessionName != "agy-message-pane" || prompt != wantMessage || interrupt || harness != "agy" {
			t.Fatalf("message delivery = %q/%q/%t/%q", sessionName, prompt, interrupt, harness)
		}
		return nil
	}

	if err := (&AgyAdapter{sessionStore: store}).SendMessage(sessionID, Message{Content: wantMessage}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if called != 1 {
		t.Fatalf("harness-aware message deliveries = %d, want 1", called)
	}
}

func TestAgySendMessagePropagatesHarnessAwareDeliveryFailure(t *testing.T) {
	preserveAgyAdapterSeams(t)
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	sessionID := SessionID("agy-message-session")
	if err := store.Set(sessionID, &SessionMetadata{TmuxName: "agy-message-pane"}); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("fixture AGY paste failure")
	agySendPromptLiteral = func(string, string, bool, string) error { return wantErr }

	err := (&AgyAdapter{sessionStore: store}).SendMessage(sessionID, Message{Content: "line one\nline two"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("SendMessage error = %v, want %v", err, wantErr)
	}
}

func TestAgyCreateSessionNormalizesWorkingDirectoryForLaunchAndDiscovery(t *testing.T) {
	preserveAgyAdapterSeams(t)
	currentWorkDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	physicalWorkDir := t.TempDir()
	aliasWorkDir := filepath.Join(t.TempDir(), "workspace-alias")
	if err := os.Symlink(physicalWorkDir, aliasWorkDir); err != nil {
		t.Fatalf("create workspace symlink: %v", err)
	}
	relativeWorkDir, err := filepath.Rel(currentWorkDir, aliasWorkDir)
	if err != nil {
		t.Fatal(err)
	}
	wantWorkDir, err := agysession.CanonicalWorkspacePath(physicalWorkDir)
	if err != nil {
		t.Fatal(err)
	}

	agyHasSession = func(string) (bool, error) { return false, nil }
	launchedWorkDir := ""
	agyNewSession = func(_ string, workDir string) error {
		launchedWorkDir = workDir
		return nil
	}
	command := ""
	agySendCommand = func(_ string, value string) error { command = value; return nil }
	agyWaitForPrompt = func(context.Context, string, time.Duration) error { return nil }
	discoveredWorkDirs := []string{}
	useStubAgyIdentityTracker(
		func(_ context.Context, workDir string) (string, error) {
			discoveredWorkDirs = append(discoveredWorkDirs, workDir)
			return "", nil
		},
		func(_ context.Context, workDir, _ string) (*agysession.Metadata, error) {
			discoveredWorkDirs = append(discoveredWorkDirs, workDir)
			return &agysession.Metadata{ConversationID: "provider-conversation-id", WorkspacePath: workDir}, nil
		},
	)

	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	sessionID, err := (&AgyAdapter{sessionStore: store}).CreateSession(SessionContext{
		Name: "agy-relative", WorkingDirectory: relativeWorkDir, Model: "3.5-flash-low",
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	metadata, err := store.Get(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if launchedWorkDir != wantWorkDir || metadata.WorkingDir != wantWorkDir {
		t.Fatalf("launch/stored workdir = %q/%q, want %q", launchedWorkDir, metadata.WorkingDir, wantWorkDir)
	}
	if len(discoveredWorkDirs) != 2 || discoveredWorkDirs[0] != wantWorkDir || discoveredWorkDirs[1] != wantWorkDir {
		t.Fatalf("discovery workdirs = %v, want normalized %q", discoveredWorkDirs, wantWorkDir)
	}
	if !strings.Contains(command, "cd '"+wantWorkDir+"' && agy") {
		t.Fatalf("launch command %q does not use normalized workdir %q", command, wantWorkDir)
	}
}

func TestAgyCreateSessionSerializesWorkspaceIdentityDiscovery(t *testing.T) {
	preserveAgyAdapterSeams(t)
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	agyAcquireCreateLock = func(workDir string) (func() error, error) {
		return agysession.AcquireWorkspaceCreateLock(t.Context(), workDir)
	}

	var providerMu sync.Mutex
	providerConversationID := "pre-existing-conversation-id"
	useStubAgyIdentityTracker(
		func(context.Context, string) (string, error) {
			providerMu.Lock()
			defer providerMu.Unlock()
			return providerConversationID, nil
		},
		func(_ context.Context, workDir, previousConversationID string) (*agysession.Metadata, error) {
			providerMu.Lock()
			defer providerMu.Unlock()
			if providerConversationID == previousConversationID {
				return nil, fmt.Errorf("provider still reports pre-create conversation %q", previousConversationID)
			}
			return &agysession.Metadata{ConversationID: providerConversationID, WorkspacePath: workDir}, nil
		},
	)
	secondReachedLifecycle := make(chan struct{})
	var secondReachedOnce sync.Once
	agyHasSession = func(name string) (bool, error) {
		if name == "agy-second" {
			secondReachedOnce.Do(func() { close(secondReachedLifecycle) })
		}
		return false, nil
	}
	agyNewSession = func(string, string) error { return nil }
	agySendCommand = func(string, string) error { return nil }
	firstWaiting := make(chan struct{})
	allowFirst := make(chan struct{})
	agyWaitForPrompt = func(_ context.Context, name string, _ time.Duration) error {
		if name == "agy-first" {
			close(firstWaiting)
			<-allowFirst
			providerMu.Lock()
			providerConversationID = "first-conversation-id"
			providerMu.Unlock()
			return nil
		}
		providerMu.Lock()
		providerConversationID = "second-conversation-id"
		providerMu.Unlock()
		return nil
	}

	type createResult struct {
		id    SessionID
		store *MockSessionStore
		err   error
	}
	create := func(name string, result chan<- createResult) {
		store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
		id, err := (&AgyAdapter{sessionStore: store}).CreateSession(SessionContext{
			Name: name, WorkingDirectory: "/shared/work", Model: "3.5-flash-low",
		})
		result <- createResult{id: id, store: store, err: err}
	}
	firstResult := make(chan createResult, 1)
	secondResult := make(chan createResult, 1)
	go create("agy-first", firstResult)
	<-firstWaiting
	secondStarted := make(chan struct{})
	go func() {
		close(secondStarted)
		create("agy-second", secondResult)
	}()
	<-secondStarted
	select {
	case <-secondReachedLifecycle:
		t.Fatal("second create entered the workspace lifecycle while the first identity discovery was active")
	case <-time.After(100 * time.Millisecond):
	}
	close(allowFirst)

	first := <-firstResult
	second := <-secondResult
	if first.err != nil || second.err != nil {
		t.Fatalf("serialized creates returned errors: first=%v second=%v", first.err, second.err)
	}
	firstMetadata, err := first.store.Get(first.id)
	if err != nil {
		t.Fatal(err)
	}
	secondMetadata, err := second.store.Get(second.id)
	if err != nil {
		t.Fatal(err)
	}
	if firstMetadata.UUID != "first-conversation-id" || secondMetadata.UUID != "second-conversation-id" {
		t.Fatalf("serialized native IDs = %q/%q", firstMetadata.UUID, secondMetadata.UUID)
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
	discoveryCalls := 0
	useStubAgyIdentityTracker(
		func(context.Context, string) (string, error) { return "", nil },
		func(context.Context, string, string) (*agysession.Metadata, error) {
			discoveryCalls++
			return nil, wantErr
		},
	)
	killed := ""
	agyKillSession = func(name string) error { killed = name; return nil }

	sessionID, err := (&AgyAdapter{sessionStore: store}).CreateSession(SessionContext{
		Name: "agy-no-identity", WorkingDirectory: "/work", Model: "3.5-flash-low",
	})
	if !errors.Is(err, wantErr) || sessionID != "" {
		t.Fatalf("CreateSession = %q, %v; want empty ID and discovery failure", sessionID, err)
	}
	if discoveryCalls != 1 || killed != "agy-no-identity" {
		t.Fatalf("canonical tracker calls/rollback = %d/%q", discoveryCalls, killed)
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
	useStubAgyIdentityTracker(
		func(context.Context, string) (string, error) { return "stale-conversation-id", nil },
		func(context.Context, string, string) (*agysession.Metadata, error) {
			return nil, fmt.Errorf("provider still reports pre-create conversation %q", "stale-conversation-id")
		},
	)
	killed := ""
	agyKillSession = func(name string) error { killed = name; return nil }

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
	useStubAgyIdentityTracker(
		func(context.Context, string) (string, error) { return "", nil },
		func(context.Context, string, string) (*agysession.Metadata, error) {
			return &agysession.Metadata{ConversationID: "unused"}, nil
		},
	)
	wantErr := errors.New("fixture readiness failed")
	agyWaitForPrompt = func(context.Context, string, time.Duration) error { return wantErr }
	killed := ""
	agyKillSession = func(name string) error { killed = name; return nil }

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

func TestAgyCreateSessionReportsRollbackFailure(t *testing.T) {
	preserveAgyAdapterSeams(t)
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	agyHasSession = func(string) (bool, error) { return false, nil }
	agyNewSession = func(string, string) error { return nil }
	agySendCommand = func(string, string) error { return nil }
	useStubAgyIdentityTracker(
		func(context.Context, string) (string, error) { return "", nil },
		func(context.Context, string, string) (*agysession.Metadata, error) {
			return &agysession.Metadata{ConversationID: "unused"}, nil
		},
	)
	readinessErr := errors.New("fixture readiness failed")
	cleanupErr := errors.New("fixture tmux cleanup failed")
	agyWaitForPrompt = func(context.Context, string, time.Duration) error { return readinessErr }
	agyKillSession = func(string) error { return cleanupErr }

	_, err := (&AgyAdapter{sessionStore: store}).CreateSession(SessionContext{
		Name: "agy-rollback-failure", WorkingDirectory: "/work", Model: "3.5-flash-low",
	})
	if !errors.Is(err, readinessErr) || !errors.Is(err, cleanupErr) {
		t.Fatalf("CreateSession error = %v, want primary and rollback failures", err)
	}
	if !strings.Contains(err.Error(), "failed to roll back AGY tmux session") {
		t.Fatalf("CreateSession error = %v, want reported rollback context", err)
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

	_, err = adapter.CreateSession(SessionContext{
		Name: "unsafe-native-id", WorkingDirectory: "/work", Model: "3.5-flash-low",
		Environment: map[string]string{"AGY_CONVERSATION_ID": "../../escape; touch /tmp/no"},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid AGY native conversation ID") {
		t.Fatalf("unsafe native ID error = %v", err)
	}
	if created || sent {
		t.Fatalf("unsafe native ID mutated tmux: created=%v sent=%v", created, sent)
	}

	wantErr := errors.New("fixture provider metadata is corrupt")
	useStubAgyIdentityTracker(
		func(context.Context, string) (string, error) { return "", wantErr },
		func(context.Context, string, string) (*agysession.Metadata, error) {
			return nil, errors.New("unexpected discovery")
		},
	)
	_, err = adapter.CreateSession(SessionContext{Name: "snapshot-failure", WorkingDirectory: "/work", Model: "3.5-flash-low"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("snapshot failure error = %v, want provider metadata error", err)
	}
	if created || sent {
		t.Fatalf("snapshot failure mutated tmux: created=%v sent=%v", created, sent)
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
	agyWaitForResumePrompt = func(context.Context, string, time.Duration) error { return nil }

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
	agyWaitForResumePrompt = func(context.Context, string, time.Duration) error { return nil }

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

func TestAgyResumeSessionRejectsUnsafeNativeIdentityBeforeMutation(t *testing.T) {
	preserveAgyAdapterSeams(t)
	t.Setenv("TMUX", "fixture")
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	sessionID := SessionID("unsafe-native-id")
	if err := store.Set(sessionID, &SessionMetadata{
		TmuxName: "agy-unsafe", WorkingDir: "/work", UUID: "../../escape; touch /tmp/no",
	}); err != nil {
		t.Fatal(err)
	}
	agyHasSession = func(string) (bool, error) { return false, nil }
	created, sent := false, false
	agyNewSession = func(string, string) error { created = true; return nil }
	agySendCommand = func(string, string) error { sent = true; return nil }

	err := (&AgyAdapter{sessionStore: store}).ResumeSession(sessionID)
	if err == nil || !strings.Contains(err.Error(), "invalid AGY native conversation ID") {
		t.Fatalf("ResumeSession error = %v, want unsafe native ID rejection", err)
	}
	if created || sent {
		t.Fatalf("unsafe native ID mutated tmux: created=%v sent=%v", created, sent)
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

func TestAgyResumeSessionRejectsAnotherLiveHarnessBeforeMutation(t *testing.T) {
	preserveAgyAdapterSeams(t)
	t.Setenv("TMUX", "fixture")
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	sessionID := SessionID("adapter-session")
	if err := store.Set(sessionID, &SessionMetadata{
		TmuxName: "agy-collision", WorkingDir: "/work", UUID: "native-id",
	}); err != nil {
		t.Fatal(err)
	}
	agyHasSession = func(string) (bool, error) { return true, nil }
	agyCheckProcess = func(string, string, string) (bool, error) { return false, nil }
	agyCheckHarness = func(string, string) (tmux.PaneLiveness, error) {
		return tmux.PaneLiveness{SessionExists: true, HarnessAlive: true, Evidence: "zsh,claude"}, nil
	}
	sent := false
	agySendCommand = func(string, string) error { sent = true; return nil }

	err := (&AgyAdapter{sessionStore: store}).ResumeSession(sessionID)
	if err == nil || !strings.Contains(err.Error(), "another live harness") {
		t.Fatalf("ResumeSession error = %v, want competing harness rejection", err)
	}
	if sent {
		t.Fatal("ResumeSession injected the AGY command into another live harness")
	}
}

func TestAgyResumeSessionRejectsNonShellForegroundBeforeMutation(t *testing.T) {
	preserveAgyAdapterSeams(t)
	t.Setenv("TMUX", "fixture")
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	sessionID := SessionID("adapter-session")
	if err := store.Set(sessionID, &SessionMetadata{
		TmuxName: "agy-editor", WorkingDir: "/work", UUID: "native-id",
	}); err != nil {
		t.Fatal(err)
	}
	agyHasSession = func(string) (bool, error) { return true, nil }
	agyCheckProcess = func(string, string, string) (bool, error) { return false, nil }
	agyCheckHarness = func(string, string) (tmux.PaneLiveness, error) {
		return tmux.PaneLiveness{SessionExists: true, Evidence: "zsh,vim"}, nil
	}
	sent := false
	agySendCommand = func(string, string) error { sent = true; return nil }

	err := (&AgyAdapter{sessionStore: store}).ResumeSession(sessionID)
	if err == nil || !strings.Contains(err.Error(), "not a proven restartable shell") {
		t.Fatalf("ResumeSession error = %v, want non-shell foreground rejection", err)
	}
	if sent {
		t.Fatal("ResumeSession injected the AGY command into a non-shell foreground process")
	}
}

func TestAgyResumeSessionRestartsInExistingBareShell(t *testing.T) {
	preserveAgyAdapterSeams(t)
	t.Setenv("TMUX", "fixture")
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	sessionID := SessionID("adapter-session")
	if err := store.Set(sessionID, &SessionMetadata{
		TmuxName: "agy-shell", WorkingDir: "/work", UUID: "native-id",
	}); err != nil {
		t.Fatal(err)
	}
	agyHasSession = func(string) (bool, error) { return true, nil }
	agyCheckProcess = func(string, string, string) (bool, error) { return false, nil }
	agyCheckHarness = func(string, string) (tmux.PaneLiveness, error) {
		return tmux.PaneLiveness{SessionExists: true, RestartableShell: true, Evidence: "zsh"}, nil
	}
	command := ""
	agySendCommand = func(_ string, value string) error { command = value; return nil }
	agyWaitForResumePrompt = func(context.Context, string, time.Duration) error { return nil }

	if err := (&AgyAdapter{sessionStore: store}).ResumeSession(sessionID); err != nil {
		t.Fatalf("ResumeSession: %v", err)
	}
	if !strings.Contains(command, "--conversation 'native-id'") {
		t.Fatalf("bare-shell resume command = %q", command)
	}
}

func TestAgyResumeSessionHoldsWorkspaceLockThroughReadiness(t *testing.T) {
	preserveAgyAdapterSeams(t)
	t.Setenv("TMUX", "fixture")
	workDir, err := agysession.CanonicalWorkspacePath(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	aliasWorkDir := filepath.Join(t.TempDir(), "workspace-alias")
	if err := os.Symlink(workDir, aliasWorkDir); err != nil {
		t.Fatalf("create workspace symlink: %v", err)
	}
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	sessionID := SessionID("adapter-session")
	if err := store.Set(sessionID, &SessionMetadata{
		TmuxName: "agy-resume-lock", WorkingDir: aliasWorkDir, UUID: "native-id",
	}); err != nil {
		t.Fatal(err)
	}
	var events []string
	agyHasSession = func(string) (bool, error) { return false, nil }
	agyAcquireCreateLock = func(workDir string) (func() error, error) {
		events = append(events, "lock:"+workDir)
		return func() error { events = append(events, "unlock"); return nil }, nil
	}
	agyNewSession = func(_ string, gotWorkDir string) error { events = append(events, "new:"+gotWorkDir); return nil }
	agySendCommand = func(_ string, command string) error {
		if !strings.Contains(command, "cd '"+workDir+"' && agy") {
			t.Fatalf("resume command %q does not use canonical workspace %q", command, workDir)
		}
		events = append(events, "send")
		return nil
	}
	agyWaitForResumePrompt = func(context.Context, string, time.Duration) error {
		events = append(events, "ready")
		return nil
	}

	if err := (&AgyAdapter{sessionStore: store}).ResumeSession(sessionID); err != nil {
		t.Fatalf("ResumeSession: %v", err)
	}
	want := []string{"lock:" + workDir, "new:" + workDir, "send", "ready", "unlock"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("resume lifecycle events = %v, want %v", events, want)
	}
}

func TestAgyResumeSessionSerializesPaneProofWithCommandDelivery(t *testing.T) {
	preserveAgyAdapterSeams(t)
	t.Setenv("TMUX", "fixture")
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	sessionID := SessionID("adapter-session")
	if err := store.Set(sessionID, &SessionMetadata{
		TmuxName: "agy-concurrent-resume", WorkingDir: "/work", UUID: "native-id",
	}); err != nil {
		t.Fatal(err)
	}

	var lifecycle sync.Mutex
	running := false
	sendCount := 0
	agyAcquireCreateLock = func(string) (func() error, error) {
		lifecycle.Lock()
		return func() error { lifecycle.Unlock(); return nil }, nil
	}
	agyHasSession = func(string) (bool, error) { return true, nil }
	agyCheckProcess = func(string, string, string) (bool, error) { return running, nil }
	agyCheckHarness = func(string, string) (tmux.PaneLiveness, error) {
		return tmux.PaneLiveness{SessionExists: true, RestartableShell: true, Evidence: "zsh"}, nil
	}
	agySendCommand = func(string, string) error {
		sendCount++
		running = true
		return nil
	}
	agyWaitForResumePrompt = func(context.Context, string, time.Duration) error { return nil }

	start := make(chan struct{})
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			errs <- (&AgyAdapter{sessionStore: store}).ResumeSession(sessionID)
		}()
	}
	close(start)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent ResumeSession: %v", err)
		}
	}
	if sendCount != 1 {
		t.Fatalf("concurrent cold resumes delivered %d commands, want exactly one", sendCount)
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

func TestAgyResumeSessionUsesTranscriptSafeReadinessPolicy(t *testing.T) {
	preserveAgyAdapterSeams(t)
	t.Setenv("TMUX", "fixture")
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	sessionID := SessionID("adapter-session")
	if err := store.Set(sessionID, &SessionMetadata{
		TmuxName: "agy-resume", WorkingDir: "/work", UUID: "native-id",
	}); err != nil {
		t.Fatal(err)
	}
	agyHasSession = func(string) (bool, error) { return false, nil }
	agyNewSession = func(string, string) error { return nil }
	agySendCommand = func(string, string) error { return nil }
	createWaitCalls := 0
	agyWaitForPrompt = func(context.Context, string, time.Duration) error {
		createWaitCalls++
		return nil
	}
	resumeWaitCalls := 0
	agyWaitForResumePrompt = func(_ context.Context, sessionName string, timeout time.Duration) error {
		resumeWaitCalls++
		if sessionName != "agy-resume" || timeout != agyResumeReadinessTimeout {
			t.Fatalf("resume wait arguments = %q/%s, want agy-resume/%s", sessionName, timeout, agyResumeReadinessTimeout)
		}
		return nil
	}

	if err := (&AgyAdapter{sessionStore: store}).ResumeSession(sessionID); err != nil {
		t.Fatalf("ResumeSession: %v", err)
	}
	if createWaitCalls != 0 || resumeWaitCalls != 1 {
		t.Fatalf("adapter wait policy calls = create:%d resume:%d, want create:0 resume:1", createWaitCalls, resumeWaitCalls)
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
	var readinessTimeout time.Duration
	agyWaitForResumePrompt = func(_ context.Context, _ string, timeout time.Duration) error {
		readinessTimeout = timeout
		return wantErr
	}
	attached := false
	agyAttachSession = func(string) error { attached = true; return nil }
	killed := ""
	agyKillSession = func(name string) error { killed = name; return nil }

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
	if readinessTimeout != 60*time.Second {
		t.Fatalf("resume readiness timeout = %s, want 60s", readinessTimeout)
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
		`{"step_index":6,"source":"SYSTEM","type":"USER_INPUT","status":"DONE","created_at":"2026-07-20T18:23:25Z","content":"typed system"}`,
		`not-json`,
		`{"step_index":2,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","created_at":"2026-07-20T18:23:21Z","content":"hello"}`,
		`{"step_index":3,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-07-20T18:23:22Z","content":"world"}`,
		`{"step_index":4,"type":"USER_INPUT","status":"DONE","created_at":"2026-07-20T18:23:23Z","content":"legacy question"}`,
		`{"step_index":5,"type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-07-20T18:23:24Z","content":"legacy answer"}`,
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
	if len(messages) != 4 ||
		messages[0].Role != RoleUser || messages[0].Content != "hello" ||
		messages[1].Role != RoleAssistant || messages[1].Content != "world" ||
		messages[2].Role != RoleUser || messages[2].Content != "legacy question" ||
		messages[3].Role != RoleAssistant || messages[3].Content != "legacy answer" {
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

func TestAgyGetHistoryRejectsUnsafeNativeIdentity(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	sessionID := SessionID("unsafe-history")
	if err := store.Set(sessionID, &SessionMetadata{UUID: "../../escape"}); err != nil {
		t.Fatal(err)
	}
	_, err := (&AgyAdapter{sessionStore: store}).GetHistory(sessionID)
	if err == nil || !strings.Contains(err.Error(), "invalid AGY native conversation ID") {
		t.Fatalf("GetHistory error = %v, want unsafe native ID rejection", err)
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
