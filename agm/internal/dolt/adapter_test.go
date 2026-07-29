package dolt

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestNew(t *testing.T) {
	// Test nil config
	_, err := New(nil)
	if err == nil {
		t.Fatal("Expected error for nil config")
	}

	// Test empty workspace
	_, err = New(&Config{Port: "3307"})
	if err == nil {
		t.Fatal("Expected error for empty workspace")
	}

	// Test empty port
	_, err = New(&Config{Workspace: "test"})
	if err == nil {
		t.Fatal("Expected error for empty port")
	}
}

func TestDefaultConfig(t *testing.T) {
	// Save originals
	originalLookupEnv := lookupEnv
	originalAgmConfigPath := agmConfigPath
	defer func() {
		lookupEnv = originalLookupEnv
		agmConfigPath = originalAgmConfigPath
	}()

	// Test missing WORKSPACE env var and no config file fallback → error
	lookupEnv = func(key string) (string, bool) {
		return "", false
	}
	agmConfigPath = "/nonexistent/path/config.yaml"

	_, err := DefaultConfig()
	if err == nil {
		t.Fatal("Expected error when WORKSPACE not set and no config file")
	}

	// Test missing WORKSPACE env var but config file provides default_workspace
	tmpCfg := t.TempDir() + "/config.yaml"
	if writeErr := os.WriteFile(tmpCfg, []byte("version: 1\ndefault_workspace: config-workspace\n"), 0o644); writeErr != nil {
		t.Fatalf("Failed to write temp config: %v", writeErr)
	}
	agmConfigPath = tmpCfg
	lookupEnv = func(key string) (string, bool) {
		switch key {
		case "ENGRAM_TEST_MODE":
			return "1", true
		case "ENGRAM_TEST_WORKSPACE":
			return "config-workspace", true
		default:
			return "", false
		}
	}

	config, err := DefaultConfig()
	if err != nil {
		t.Fatalf("Expected fallback to config file workspace, got error: %v", err)
	}
	if config.Workspace != "config-workspace" {
		t.Errorf("Expected workspace 'config-workspace' from config file, got '%s'", config.Workspace)
	}

	// Test with WORKSPACE set
	lookupEnv = func(key string) (string, bool) {
		switch key {
		case "WORKSPACE":
			return "test-workspace", true
		case "DOLT_PORT":
			return "3307", true
		case "ENGRAM_TEST_MODE":
			return "1", true
		case "ENGRAM_TEST_WORKSPACE":
			return "test-workspace", true
		default:
			return "", false
		}
	}

	config2, err2 := DefaultConfig()
	if err2 != nil {
		t.Fatalf("Failed to get default config: %v", err2)
	}

	if config2.Workspace != "test-workspace" {
		t.Errorf("Expected workspace 'test-workspace', got '%s'", config2.Workspace)
	}

	if config2.Port != "3307" {
		t.Errorf("Expected port '3307', got '%s'", config2.Port)
	}

	if config2.Host != "127.0.0.1" {
		t.Errorf("Expected default host '127.0.0.1', got '%s'", config2.Host)
	}
}

func TestConfiguredWorkspaceConfigsIncludesEveryEnabledStore(t *testing.T) {
	originalLookupEnv := lookupEnv
	originalAgmConfigPath := agmConfigPath
	t.Cleanup(func() {
		lookupEnv = originalLookupEnv
		agmConfigPath = originalAgmConfigPath
	})

	configPath := t.TempDir() + "/config.yaml"
	if err := os.WriteFile(configPath, []byte(`version: 1
default_workspace: personal
workspaces:
  - name: personal
    enabled: true
  - name: oss
    enabled: true
  - name: retired
    enabled: false
  - name: dormant
`), 0o600); err != nil {
		t.Fatal(err)
	}
	agmConfigPath = configPath
	lookupEnv = func(key string) (string, bool) {
		values := map[string]string{
			"ENGRAM_TEST_MODE":      "1",
			"ENGRAM_TEST_WORKSPACE": "personal",
			"WORKSPACE":             "personal",
			"DOLT_PORT":             "3307",
		}
		value, ok := values[key]
		return value, ok
	}

	configs, err := ConfiguredWorkspaceConfigs()
	if err != nil {
		t.Fatalf("ConfiguredWorkspaceConfigs: %v", err)
	}
	if len(configs) != 2 {
		t.Fatalf("configured stores = %d, want 2: %#v", len(configs), configs)
	}
	for index, want := range []string{"personal", "oss"} {
		if configs[index].Workspace != want || configs[index].Database != want {
			t.Errorf("config[%d] = workspace %q database %q, want %q/%q",
				index, configs[index].Workspace, configs[index].Database, want, want)
		}
	}
}

func TestConfiguredWorkspaceConfigsRejectsExplicitDatabaseAcrossWorkspaces(t *testing.T) {
	originalLookupEnv := lookupEnv
	originalAgmConfigPath := agmConfigPath
	t.Cleanup(func() {
		lookupEnv = originalLookupEnv
		agmConfigPath = originalAgmConfigPath
	})

	configPath := t.TempDir() + "/config.yaml"
	if err := os.WriteFile(configPath, []byte(`default_workspace: personal
workspaces:
  - name: personal
    enabled: true
  - name: oss
    enabled: true
`), 0o600); err != nil {
		t.Fatal(err)
	}
	agmConfigPath = configPath
	lookupEnv = func(key string) (string, bool) {
		values := map[string]string{
			"ENGRAM_TEST_MODE":      "1",
			"ENGRAM_TEST_WORKSPACE": "personal",
			"WORKSPACE":             "personal",
			"DOLT_DATABASE":         "agm_test",
		}
		value, ok := values[key]
		return value, ok
	}

	_, err := ConfiguredWorkspaceConfigs()
	if err == nil || !strings.Contains(err.Error(), "cannot prove a complete cross-workspace") {
		t.Fatalf("ConfiguredWorkspaceConfigs error = %v, want explicit-database isolation failure", err)
	}
}

func TestConfiguredWorkspaceConfigsAllowsExplicitDatabaseForOneWorkspace(t *testing.T) {
	originalLookupEnv := lookupEnv
	originalAgmConfigPath := agmConfigPath
	t.Cleanup(func() {
		lookupEnv = originalLookupEnv
		agmConfigPath = originalAgmConfigPath
	})

	configPath := t.TempDir() + "/config.yaml"
	if err := os.WriteFile(configPath, []byte(`workspaces:
  - name: personal
    enabled: true
`), 0o600); err != nil {
		t.Fatal(err)
	}
	agmConfigPath = configPath
	lookupEnv = func(key string) (string, bool) {
		values := map[string]string{
			"ENGRAM_TEST_MODE":      "1",
			"ENGRAM_TEST_WORKSPACE": "personal",
			"WORKSPACE":             "personal",
			"DOLT_DATABASE":         "agm_test",
		}
		value, ok := values[key]
		return value, ok
	}

	configs, err := ConfiguredWorkspaceConfigs()
	if err != nil {
		t.Fatalf("ConfiguredWorkspaceConfigs: %v", err)
	}
	if len(configs) != 1 || configs[0].Workspace != "personal" || configs[0].Database != "agm_test" {
		t.Fatalf("configured stores = %#v, want personal/agm_test", configs)
	}
}

func TestDefaultConfigSelectsExplicitTestWorkspace(t *testing.T) {
	originalLookupEnv := lookupEnv
	originalAgmConfigPath := agmConfigPath
	t.Cleanup(func() {
		lookupEnv = originalLookupEnv
		agmConfigPath = originalAgmConfigPath
	})
	agmConfigPath = "/nonexistent/path/config.yaml"
	lookupEnv = func(key string) (string, bool) {
		values := map[string]string{
			"WORKSPACE":             "oss",
			"ENGRAM_TEST_MODE":      "1",
			"ENGRAM_TEST_WORKSPACE": "test",
		}
		value, ok := values[key]
		return value, ok
	}

	config, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	if config.Workspace != "test" || config.Database != "test" {
		t.Fatalf("DefaultConfig target = %s/%s, want test/test", config.Workspace, config.Database)
	}
}

func TestTestExecutionRecognizesBuiltTestSubprocess(t *testing.T) {
	for _, tc := range []struct {
		name       string
		executable string
		mode       string
		want       bool
	}{
		{name: "go test binary", executable: "/tmp/dolt.test", want: true},
		{name: "built subprocess with explicit mode", executable: "/tmp/agm", mode: "1", want: true},
		{name: "ordinary binary", executable: "/tmp/agm", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := testExecution(tc.executable, tc.mode); got != tc.want {
				t.Fatalf("testExecution(%q, %q) = %t, want %t", tc.executable, tc.mode, got, tc.want)
			}
		})
	}
}

func TestValidateTestTargetUsesPositiveAllowlist(t *testing.T) {
	for _, tc := range []struct {
		name          string
		workspace     string
		database      string
		testWorkspace string
		wantError     bool
	}{
		{name: "workspace database", workspace: "test-e2e", database: "test-e2e", testWorkspace: "test-e2e"},
		{name: "shared test database", workspace: "test", database: "agm_test", testWorkspace: "test"},
		{name: "missing selection", workspace: "test", database: "test", wantError: true},
		{name: "workspace mismatch", workspace: "customer", database: "customer", testWorkspace: "test", wantError: true},
		{name: "unselected database", workspace: "test", database: "customer", testWorkspace: "test", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateTestTarget(tc.workspace, tc.database, tc.testWorkspace)
			if (err != nil) != tc.wantError {
				t.Fatalf("validateTestTarget(%q, %q, %q) error = %v, wantError=%t", tc.workspace, tc.database, tc.testWorkspace, err, tc.wantError)
			}
		})
	}
}

func TestSharedDoltTestConfigEstablishesGuardedTarget(t *testing.T) {
	originalLookupEnv := lookupEnv
	lookupEnv = os.LookupEnv
	t.Cleanup(func() { lookupEnv = originalLookupEnv })

	config := sharedDoltTestConfig(t)
	if err := validateTestExecutionTarget(config.Workspace, config.Database); err != nil {
		t.Fatalf("sharedDoltTestConfig target rejected: %v", err)
	}
}

func TestNewRejectsDirectProductionTargetInTests(t *testing.T) {
	originalLookupEnv := lookupEnv
	lookupEnv = os.LookupEnv
	t.Cleanup(func() { lookupEnv = originalLookupEnv })
	t.Setenv("ENGRAM_TEST_MODE", "1")
	t.Setenv("ENGRAM_TEST_WORKSPACE", "test")

	_, err := New(&Config{
		Workspace: "oss",
		Database:  "oss",
		Host:      "127.0.0.1",
		Port:      "3307",
		User:      "root",
	})
	if err == nil || !strings.Contains(err.Error(), "TEST POLLUTION BLOCKED") {
		t.Fatalf("New() error = %v, want direct production target rejection", err)
	}
}

func TestBuildDSN(t *testing.T) {
	config := &Config{
		User:     "testuser",
		Password: "testpass",
		Host:     "localhost",
		Port:     "3307",
		Database: "testdb",
	}

	dsn := buildDSN(config)

	expected := "testuser:testpass@tcp(localhost:3307)/testdb?parseTime=true"
	if dsn != expected {
		t.Errorf("Expected DSN '%s', got '%s'", expected, dsn)
	}
}

// Integration tests (require running Dolt server)
// Skip if DOLT_TEST_INTEGRATION is not set

func getTestAdapter(t *testing.T) *Adapter {
	if os.Getenv("DOLT_TEST_INTEGRATION") == "" {
		t.Skip("Skipping integration test (set DOLT_TEST_INTEGRATION=1 to enable)")
	}

	// Set up test environment
	t.Setenv("ENGRAM_TEST_MODE", "1")
	t.Setenv("ENGRAM_TEST_WORKSPACE", "test")
	t.Setenv("WORKSPACE", "test")
	t.Setenv("DOLT_PORT", "3307")
	os.Unsetenv("DOLT_DATABASE") // Let it default to workspace name

	// Initialize lookupEnv
	lookupEnv = os.LookupEnv

	config, err := DefaultConfig()
	if err != nil {
		t.Fatalf("Failed to get default config: %v", err)
	}

	adapter, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	// Apply migrations
	if err := adapter.ApplyMigrations(); err != nil {
		t.Fatalf("Failed to apply migrations: %v", err)
	}

	return adapter
}

func TestSessionCRUD(t *testing.T) {
	adapter := getTestAdapter(t)
	defer adapter.Close()

	// Create test session
	session := &manifest.Manifest{
		SessionID:     "test-session-" + time.Now().Format("20060102-150405"),
		Name:          "Test Session",
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Harness:       "claude-code",
		Context: manifest.Context{
			Project: "/test/project",
			Purpose: "Testing",
			Tags:    []string{"test", "integration"},
		},
		Claude: manifest.Claude{
			UUID: "test-uuid-123",
		},
		Tmux: manifest.Tmux{
			SessionName: "test-tmux",
		},
	}

	// Test Create
	if err := adapter.CreateSession(session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Test Get
	retrieved, err := adapter.GetSession(session.SessionID)
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}

	if retrieved.Name != session.Name {
		t.Errorf("Expected name '%s', got '%s'", session.Name, retrieved.Name)
	}

	if retrieved.Context.Project != session.Context.Project {
		t.Errorf("Expected project '%s', got '%s'", session.Context.Project, retrieved.Context.Project)
	}

	// Test Update
	session.Name = "Updated Test Session"
	if err := adapter.UpdateSession(session); err != nil {
		t.Fatalf("Failed to update session: %v", err)
	}

	retrieved, err = adapter.GetSession(session.SessionID)
	if err != nil {
		t.Fatalf("Failed to get updated session: %v", err)
	}

	if retrieved.Name != "Updated Test Session" {
		t.Errorf("Expected updated name, got '%s'", retrieved.Name)
	}

	// Test List
	sessions, err := adapter.ListSessions(&SessionFilter{})
	if err != nil {
		t.Fatalf("Failed to list sessions: %v", err)
	}

	if len(sessions) == 0 {
		t.Error("Expected at least one session in list")
	}

	// Test Delete
	if err := adapter.DeleteSession(session.SessionID); err != nil {
		t.Fatalf("Failed to delete session: %v", err)
	}

	// Verify deletion
	_, err = adapter.GetSession(session.SessionID)
	if err == nil {
		t.Error("Expected error when getting deleted session")
	}
}

func TestMessageCRUD(t *testing.T) {
	adapter := getTestAdapter(t)
	defer adapter.Close()

	// Create test session first
	session := &manifest.Manifest{
		SessionID:     "test-msg-session-" + time.Now().Format("20060102-150405"),
		Name:          "Test Message Session",
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Harness:       "claude-code",
		Context: manifest.Context{
			Project: "/test/project",
		},
		Claude: manifest.Claude{
			UUID: "test-uuid-456",
		},
		Tmux: manifest.Tmux{
			SessionName: "test-msg-tmux",
		},
	}

	if err := adapter.CreateSession(session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer adapter.DeleteSession(session.SessionID)

	// Create test message
	msg := &Message{
		SessionID:      session.SessionID,
		Role:           "user",
		Content:        `[{"type":"text","text":"Hello, world!"}]`,
		SequenceNumber: 0,
		Harness:        "claude-code",
		InputTokens:    10,
		OutputTokens:   0,
	}

	// Test Create
	if err := adapter.CreateMessage(msg); err != nil {
		t.Fatalf("Failed to create message: %v", err)
	}

	if msg.ID == "" {
		t.Error("Expected message ID to be generated")
	}

	if msg.Timestamp == 0 {
		t.Error("Expected timestamp to be set")
	}

	// Test Get
	retrieved, err := adapter.GetMessage(msg.ID)
	if err != nil {
		t.Fatalf("Failed to get message: %v", err)
	}

	if retrieved.Content != msg.Content {
		t.Errorf("Expected content '%s', got '%s'", msg.Content, retrieved.Content)
	}

	// Test Get Session Messages
	messages, err := adapter.GetSessionMessages(session.SessionID)
	if err != nil {
		t.Fatalf("Failed to get session messages: %v", err)
	}

	if len(messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(messages))
	}

	// Test batch create
	batchMsgs := []*Message{
		{
			SessionID:      session.SessionID,
			Role:           "assistant",
			Content:        `[{"type":"text","text":"Response 1"}]`,
			SequenceNumber: 1,
		},
		{
			SessionID:      session.SessionID,
			Role:           "user",
			Content:        `[{"type":"text","text":"Follow-up"}]`,
			SequenceNumber: 2,
		},
	}

	if err := adapter.CreateMessages(batchMsgs); err != nil {
		t.Fatalf("Failed to create batch messages: %v", err)
	}

	messages, err = adapter.GetSessionMessages(session.SessionID)
	if err != nil {
		t.Fatalf("Failed to get session messages: %v", err)
	}

	if len(messages) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(messages))
	}

	// Verify order by sequence number
	if messages[0].SequenceNumber != 0 {
		t.Errorf("Expected first message sequence 0, got %d", messages[0].SequenceNumber)
	}
}

func TestToolCallTracking(t *testing.T) {
	adapter := getTestAdapter(t)
	defer adapter.Close()

	// Create test session and message
	session := &manifest.Manifest{
		SessionID:     "test-tool-session-" + time.Now().Format("20060102-150405"),
		Name:          "Test Tool Session",
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Harness:       "claude-code",
		Context: manifest.Context{
			Project: "/test/project",
		},
		Claude: manifest.Claude{
			UUID: "test-uuid-789",
		},
		Tmux: manifest.Tmux{
			SessionName: "test-tool-tmux",
		},
	}

	if err := adapter.CreateSession(session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer adapter.DeleteSession(session.SessionID)

	msg := &Message{
		SessionID:      session.SessionID,
		Role:           "assistant",
		Content:        `[{"type":"text","text":"Using tools"}]`,
		SequenceNumber: 0,
	}

	if err := adapter.CreateMessage(msg); err != nil {
		t.Fatalf("Failed to create message: %v", err)
	}

	// Create tool call
	toolCall := &ToolCall{
		MessageID:       msg.ID,
		SessionID:       session.SessionID,
		ToolName:        "read_file",
		Arguments:       map[string]any{"path": "/test/file.txt"},
		Result:          map[string]any{"content": "file content"},
		ExecutionTimeMs: 150,
	}

	if err := adapter.CreateToolCall(toolCall); err != nil {
		t.Fatalf("Failed to create tool call: %v", err)
	}

	// Test Get
	retrieved, err := adapter.GetToolCall(toolCall.ID)
	if err != nil {
		t.Fatalf("Failed to get tool call: %v", err)
	}

	if retrieved.ToolName != "read_file" {
		t.Errorf("Expected tool name 'read_file', got '%s'", retrieved.ToolName)
	}

	// Test Get Message Tool Calls
	calls, err := adapter.GetMessageToolCalls(msg.ID)
	if err != nil {
		t.Fatalf("Failed to get message tool calls: %v", err)
	}

	if len(calls) != 1 {
		t.Errorf("Expected 1 tool call, got %d", len(calls))
	}

	// Test Get Session Tool Calls
	sessionCalls, err := adapter.GetSessionToolCalls(session.SessionID)
	if err != nil {
		t.Fatalf("Failed to get session tool calls: %v", err)
	}

	if len(sessionCalls) != 1 {
		t.Errorf("Expected 1 tool call, got %d", len(sessionCalls))
	}

	// Test Tool Call Stats
	stats, err := adapter.GetToolCallStats(session.SessionID)
	if err != nil {
		t.Fatalf("Failed to get tool call stats: %v", err)
	}

	if stats["total_calls"] != 1 {
		t.Errorf("Expected total_calls = 1, got %v", stats["total_calls"])
	}
}

func TestResolveIdentifier(t *testing.T) {
	adapter := getTestAdapter(t)
	defer adapter.Close()

	// Create test session
	session := &manifest.Manifest{
		SessionID:     "test-resolve-" + time.Now().Format("20060102-150405"),
		Name:          "resolve-test-session",
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Harness:       "claude-code",
		Context: manifest.Context{
			Project: "/test/resolve",
			Purpose: "Testing identifier resolution",
		},
		Claude: manifest.Claude{
			UUID: "resolve-uuid-123",
		},
		Tmux: manifest.Tmux{
			SessionName: "resolve-tmux-session",
		},
	}

	if err := adapter.CreateSession(session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer adapter.DeleteSession(session.SessionID)

	// Test 1: Resolve by session ID
	resolved, err := adapter.ResolveIdentifier(session.SessionID)
	if err != nil {
		t.Fatalf("Failed to resolve by session ID: %v", err)
	}
	if resolved.SessionID != session.SessionID {
		t.Errorf("Expected session ID '%s', got '%s'", session.SessionID, resolved.SessionID)
	}

	// Test 2: Resolve by tmux session name
	resolved, err = adapter.ResolveIdentifier(session.Tmux.SessionName)
	if err != nil {
		t.Fatalf("Failed to resolve by tmux name: %v", err)
	}
	if resolved.SessionID != session.SessionID {
		t.Errorf("Expected session ID '%s', got '%s'", session.SessionID, resolved.SessionID)
	}

	// Test 3: Resolve by manifest name
	resolved, err = adapter.ResolveIdentifier(session.Name)
	if err != nil {
		t.Fatalf("Failed to resolve by manifest name: %v", err)
	}
	if resolved.SessionID != session.SessionID {
		t.Errorf("Expected session ID '%s', got '%s'", session.SessionID, resolved.SessionID)
	}

	// Test 4: Resolve non-existent session
	_, err = adapter.ResolveIdentifier("non-existent-session")
	if err == nil {
		t.Error("Expected error when resolving non-existent session")
	}
	if err != nil && err.Error() != "session not found: non-existent-session" {
		t.Errorf("Expected 'session not found' error, got: %v", err)
	}

	// Test 5: Empty identifier
	_, err = adapter.ResolveIdentifier("")
	if err == nil {
		t.Error("Expected error for empty identifier")
	}
}

func TestResolveIdentifierExcludesArchived(t *testing.T) {
	adapter := getTestAdapter(t)
	defer adapter.Close()

	// Create archived session
	archivedSession := &manifest.Manifest{
		SessionID:     "test-archived-" + time.Now().Format("20060102-150405"),
		Name:          "archived-session",
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Harness:       "claude-code",
		Lifecycle:     manifest.LifecycleArchived,
		Context: manifest.Context{
			Project: "/test/archived",
		},
		Claude: manifest.Claude{
			UUID: "archived-uuid-123",
		},
		Tmux: manifest.Tmux{
			SessionName: "archived-tmux",
		},
	}

	if err := adapter.CreateSession(archivedSession); err != nil {
		t.Fatalf("Failed to create archived session: %v", err)
	}
	defer adapter.DeleteSession(archivedSession.SessionID)

	// Verify session exists in database
	retrieved, err := adapter.GetSession(archivedSession.SessionID)
	if err != nil {
		t.Fatalf("Failed to get archived session directly: %v", err)
	}
	if retrieved.Lifecycle != manifest.LifecycleArchived {
		t.Errorf("Expected archived lifecycle, got '%s'", retrieved.Lifecycle)
	}

	// Test: ResolveIdentifier should NOT find archived session
	_, err = adapter.ResolveIdentifier(archivedSession.SessionID)
	if err == nil {
		t.Error("Expected error when resolving archived session")
	}
	if err != nil && err.Error() != "session not found: "+archivedSession.SessionID {
		t.Errorf("Expected 'session not found' error, got: %v", err)
	}

	// Test: ResolveIdentifier should NOT find by tmux name
	_, err = adapter.ResolveIdentifier(archivedSession.Tmux.SessionName)
	if err == nil {
		t.Error("Expected error when resolving archived session by tmux name")
	}

	// Test: ResolveIdentifier should NOT find by manifest name
	_, err = adapter.ResolveIdentifier(archivedSession.Name)
	if err == nil {
		t.Error("Expected error when resolving archived session by manifest name")
	}
}

func TestResolveIdentifierWithDuplicateNames(t *testing.T) {
	adapter := getTestAdapter(t)
	defer adapter.Close()

	timestamp := time.Now().Format("20060102-150405")

	// Create two sessions with different IDs but potentially overlapping names
	session1 := &manifest.Manifest{
		SessionID:     "test-dup1-" + timestamp,
		Name:          "duplicate-name",
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Harness:       "claude-code",
		Context: manifest.Context{
			Project: "/test/dup1",
		},
		Claude: manifest.Claude{
			UUID: "dup1-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: "dup-tmux-1",
		},
	}

	session2 := &manifest.Manifest{
		SessionID:     "test-dup2-" + timestamp,
		Name:          "different-name",
		SchemaVersion: "2.0",
		CreatedAt:     time.Now().Add(1 * time.Second),
		UpdatedAt:     time.Now().Add(1 * time.Second),
		Harness:       "claude-code",
		Context: manifest.Context{
			Project: "/test/dup2",
		},
		Claude: manifest.Claude{
			UUID: "dup2-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: "dup-tmux-2",
		},
	}

	if err := adapter.CreateSession(session1); err != nil {
		t.Fatalf("Failed to create session1: %v", err)
	}
	defer adapter.DeleteSession(session1.SessionID)

	if err := adapter.CreateSession(session2); err != nil {
		t.Fatalf("Failed to create session2: %v", err)
	}
	defer adapter.DeleteSession(session2.SessionID)

	// Resolve by unique session IDs should work
	resolved1, err := adapter.ResolveIdentifier(session1.SessionID)
	if err != nil {
		t.Fatalf("Failed to resolve session1 by ID: %v", err)
	}
	if resolved1.SessionID != session1.SessionID {
		t.Errorf("Expected session1 ID, got '%s'", resolved1.SessionID)
	}

	resolved2, err := adapter.ResolveIdentifier(session2.SessionID)
	if err != nil {
		t.Fatalf("Failed to resolve session2 by ID: %v", err)
	}
	if resolved2.SessionID != session2.SessionID {
		t.Errorf("Expected session2 ID, got '%s'", resolved2.SessionID)
	}

	// Resolve by unique tmux names should work
	resolvedTmux1, err := adapter.ResolveIdentifier(session1.Tmux.SessionName)
	if err != nil {
		t.Fatalf("Failed to resolve session1 by tmux name: %v", err)
	}
	if resolvedTmux1.SessionID != session1.SessionID {
		t.Errorf("Expected session1 ID, got '%s'", resolvedTmux1.SessionID)
	}
}
