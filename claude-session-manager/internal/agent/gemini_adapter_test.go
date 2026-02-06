package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestMockStore creates a new mock session store for testing.
func newTestMockStore() *MockSessionStore {
	return &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}
}

func TestGeminiAdapter_NewGeminiAdapter(t *testing.T) {
	tests := []struct {
		name        string
		config      *GeminiConfig
		setupEnv    func()
		cleanupEnv  func()
		wantErr     bool
		errContains string
	}{
		{
			name:   "default config with env var",
			config: nil,
			setupEnv: func() {
				os.Setenv("GEMINI_API_KEY", "test-api-key")
			},
			cleanupEnv: func() {
				os.Unsetenv("GEMINI_API_KEY")
			},
			wantErr: false,
		},
		{
			name: "explicit api key",
			config: &GeminiConfig{
				APIKey: "explicit-key",
			},
			setupEnv:   func() {},
			cleanupEnv: func() {},
			wantErr:    false,
		},
		{
			name:   "missing api key",
			config: nil,
			setupEnv: func() {
				os.Unsetenv("GEMINI_API_KEY")
			},
			cleanupEnv:  func() {},
			wantErr:     true,
			errContains: "GEMINI_API_KEY",
		},
		{
			name: "custom model name",
			config: &GeminiConfig{
				ModelName: "gemini-1.5-pro",
				APIKey:    "test-key",
			},
			setupEnv:   func() {},
			cleanupEnv: func() {},
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupEnv()
			defer tt.cleanupEnv()

			if tt.config != nil && tt.config.SessionStore == nil {
				tt.config.SessionStore = newTestMockStore()
			}

			adapter, err := NewGeminiAdapter(tt.config)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, adapter)

			geminiAdapter, ok := adapter.(*GeminiAdapter)
			require.True(t, ok)

			// Verify default model
			if tt.config == nil || tt.config.ModelName == "" {
				assert.Equal(t, "gemini-2.0-flash-exp", geminiAdapter.modelName)
			} else {
				assert.Equal(t, tt.config.ModelName, geminiAdapter.modelName)
			}
		})
	}
}

func TestGeminiAdapter_Name(t *testing.T) {
	adapter, err := NewGeminiAdapter(&GeminiConfig{
		APIKey:       "test-key",
		SessionStore: newTestMockStore(),
	})
	require.NoError(t, err)

	assert.Equal(t, "gemini", adapter.Name())
}

func TestGeminiAdapter_Version(t *testing.T) {
	tests := []struct {
		name      string
		modelName string
		want      string
	}{
		{
			name:      "default model",
			modelName: "",
			want:      "gemini-2.0-flash-exp",
		},
		{
			name:      "custom model",
			modelName: "gemini-1.5-pro",
			want:      "gemini-1.5-pro",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := NewGeminiAdapter(&GeminiConfig{
				ModelName:    tt.modelName,
				APIKey:       "test-key",
				SessionStore: newTestMockStore(),
			})
			require.NoError(t, err)

			assert.Equal(t, tt.want, adapter.Version())
		})
	}
}

func TestGeminiAdapter_CreateSession(t *testing.T) {
	// Create temp directory for session storage
	tmpDir := t.TempDir()

	// Override home directory for testing
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	adapter, err := NewGeminiAdapter(&GeminiConfig{
		APIKey:       "test-key",
		SessionStore: newTestMockStore(),
	})
	require.NoError(t, err)

	ctx := SessionContext{
		Name:             "test-session",
		WorkingDirectory: "/tmp",
		Project:          "test-project",
	}

	sessionID, err := adapter.CreateSession(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, sessionID)

	// Verify session directory was created
	geminiAdapter := adapter.(*GeminiAdapter)
	sessionDir, err := geminiAdapter.getSessionDir(sessionID)
	require.NoError(t, err)

	stat, err := os.Stat(sessionDir)
	require.NoError(t, err)
	assert.True(t, stat.IsDir())

	// Verify session metadata was stored
	metadata, err := geminiAdapter.sessionStore.Get(sessionID)
	require.NoError(t, err)
	assert.Equal(t, "/tmp", metadata.WorkingDir)
	assert.Equal(t, "test-project", metadata.Project)
	assert.False(t, metadata.CreatedAt.IsZero())
}

func TestGeminiAdapter_GetHistory(t *testing.T) {
	// Create temp directory for session storage
	tmpDir := t.TempDir()

	// Override home directory for testing
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	adapter, err := NewGeminiAdapter(&GeminiConfig{
		APIKey:       "test-key",
		SessionStore: newTestMockStore(),
	})
	require.NoError(t, err)

	// Create session
	sessionID, err := adapter.CreateSession(SessionContext{
		Name:             "test-session",
		WorkingDirectory: "/tmp",
	})
	require.NoError(t, err)

	// Initially, history should be empty
	history, err := adapter.GetHistory(sessionID)
	require.NoError(t, err)
	assert.Empty(t, history)

	// Write some messages to history
	geminiAdapter := adapter.(*GeminiAdapter)
	msg1 := Message{
		ID:        "msg-1",
		Role:      RoleUser,
		Content:   "Hello",
		Timestamp: time.Now(),
	}
	msg2 := Message{
		ID:        "msg-2",
		Role:      RoleAssistant,
		Content:   "Hi there!",
		Timestamp: time.Now(),
	}

	err = geminiAdapter.appendToHistory(sessionID, msg1)
	require.NoError(t, err)
	err = geminiAdapter.appendToHistory(sessionID, msg2)
	require.NoError(t, err)

	// Retrieve history
	history, err = adapter.GetHistory(sessionID)
	require.NoError(t, err)
	assert.Len(t, history, 2)
	assert.Equal(t, "Hello", history[0].Content)
	assert.Equal(t, "Hi there!", history[1].Content)
}

func TestGeminiAdapter_ExportConversation(t *testing.T) {
	// Create temp directory for session storage
	tmpDir := t.TempDir()

	// Override home directory for testing
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	adapter, err := NewGeminiAdapter(&GeminiConfig{
		APIKey:       "test-key",
		SessionStore: newTestMockStore(),
	})
	require.NoError(t, err)

	// Create session with history
	sessionID, err := adapter.CreateSession(SessionContext{
		Name:             "test-session",
		WorkingDirectory: "/tmp",
	})
	require.NoError(t, err)

	geminiAdapter := adapter.(*GeminiAdapter)
	msg1 := Message{
		ID:        "msg-1",
		Role:      RoleUser,
		Content:   "Test question",
		Timestamp: time.Now(),
	}
	err = geminiAdapter.appendToHistory(sessionID, msg1)
	require.NoError(t, err)

	tests := []struct {
		name        string
		format      ConversationFormat
		wantErr     bool
		checkOutput func(t *testing.T, data []byte)
	}{
		{
			name:    "JSONL export",
			format:  FormatJSONL,
			wantErr: false,
			checkOutput: func(t *testing.T, data []byte) {
				lines := splitLines(string(data))
				assert.Len(t, lines, 2) // 1 message + empty line at end

				var msg Message
				err := json.Unmarshal([]byte(lines[0]), &msg)
				require.NoError(t, err)
				assert.Equal(t, "Test question", msg.Content)
			},
		},
		{
			name:    "Markdown export",
			format:  FormatMarkdown,
			wantErr: false,
			checkOutput: func(t *testing.T, data []byte) {
				content := string(data)
				assert.Contains(t, content, "# Gemini Conversation")
				assert.Contains(t, content, "Test question")
				assert.Contains(t, content, "## User")
			},
		},
		{
			name:    "HTML export not supported",
			format:  FormatHTML,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := adapter.ExportConversation(sessionID, tt.format)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, data)

			if tt.checkOutput != nil {
				tt.checkOutput(t, data)
			}
		})
	}
}

func TestGeminiAdapter_ImportConversation(t *testing.T) {
	// Create temp directory for session storage
	tmpDir := t.TempDir()

	// Override home directory for testing
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	adapter, err := NewGeminiAdapter(&GeminiConfig{
		APIKey:       "test-key",
		SessionStore: newTestMockStore(),
	})
	require.NoError(t, err)

	// Create sample JSONL data
	msg1 := Message{
		ID:        "msg-1",
		Role:      RoleUser,
		Content:   "Imported question",
		Timestamp: time.Now(),
	}
	msg2 := Message{
		ID:        "msg-2",
		Role:      RoleAssistant,
		Content:   "Imported response",
		Timestamp: time.Now(),
	}

	data1, _ := json.Marshal(msg1)
	data2, _ := json.Marshal(msg2)
	jsonlData := append(data1, '\n')
	jsonlData = append(jsonlData, data2...)
	jsonlData = append(jsonlData, '\n')

	// Import conversation
	sessionID, err := adapter.ImportConversation(jsonlData, FormatJSONL)
	require.NoError(t, err)
	assert.NotEmpty(t, sessionID)

	// Verify history was imported
	history, err := adapter.GetHistory(sessionID)
	require.NoError(t, err)
	assert.Len(t, history, 2)
	assert.Equal(t, "Imported question", history[0].Content)
	assert.Equal(t, "Imported response", history[1].Content)
}

func TestGeminiAdapter_GetSessionStatus(t *testing.T) {
	adapter, err := NewGeminiAdapter(&GeminiConfig{
		APIKey:       "test-key",
		SessionStore: newTestMockStore(),
	})
	require.NoError(t, err)

	// Non-existent session should be terminated
	status, err := adapter.GetSessionStatus("non-existent")
	require.NoError(t, err)
	assert.Equal(t, StatusTerminated, status)

	// Create a session
	geminiAdapter := adapter.(*GeminiAdapter)
	sessionID := SessionID("test-session")
	err = geminiAdapter.sessionStore.Set(sessionID, &SessionMetadata{
		TmuxName:   "test",
		CreatedAt:  time.Now(),
		WorkingDir: "/tmp",
	})
	require.NoError(t, err)

	// Existing session should be active
	status, err = adapter.GetSessionStatus(sessionID)
	require.NoError(t, err)
	assert.Equal(t, StatusActive, status)
}

func TestGeminiAdapter_TerminateSession(t *testing.T) {
	// Create temp directory for session storage
	tmpDir := t.TempDir()

	// Override home directory for testing
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	adapter, err := NewGeminiAdapter(&GeminiConfig{
		APIKey:       "test-key",
		SessionStore: newTestMockStore(),
	})
	require.NoError(t, err)

	// Create session
	sessionID, err := adapter.CreateSession(SessionContext{
		Name:             "test-session",
		WorkingDirectory: "/tmp",
	})
	require.NoError(t, err)

	// Verify session exists
	status, err := adapter.GetSessionStatus(sessionID)
	require.NoError(t, err)
	assert.Equal(t, StatusActive, status)

	// Terminate session
	err = adapter.TerminateSession(sessionID)
	require.NoError(t, err)

	// Verify session is terminated
	status, err = adapter.GetSessionStatus(sessionID)
	require.NoError(t, err)
	assert.Equal(t, StatusTerminated, status)

	// Verify session directory still exists (preserved for historical purposes)
	geminiAdapter := adapter.(*GeminiAdapter)
	sessionDir, err := geminiAdapter.getSessionDir(sessionID)
	require.NoError(t, err)
	_, err = os.Stat(sessionDir)
	assert.NoError(t, err, "session directory should still exist after termination")
}

func TestGeminiAdapter_Capabilities(t *testing.T) {
	adapter, err := NewGeminiAdapter(&GeminiConfig{
		ModelName:    "gemini-1.5-pro",
		APIKey:       "test-key",
		SessionStore: newTestMockStore(),
	})
	require.NoError(t, err)

	caps := adapter.Capabilities()

	assert.False(t, caps.SupportsSlashCommands, "API agents don't support slash commands")
	assert.False(t, caps.SupportsHooks)
	assert.True(t, caps.SupportsTools, "Gemini supports function calling")
	assert.True(t, caps.SupportsVision, "Gemini 2.0 supports vision")
	assert.True(t, caps.SupportsMultimodal, "Gemini 2.0 supports multimodal")
	assert.Equal(t, 1000000, caps.MaxContextWindow)
	assert.Equal(t, "gemini-1.5-pro", caps.ModelName)
}

func TestGeminiAdapter_ExecuteCommand(t *testing.T) {
	adapter, err := NewGeminiAdapter(&GeminiConfig{
		APIKey:       "test-key",
		SessionStore: newTestMockStore(),
	})
	require.NoError(t, err)

	tests := []struct {
		name    string
		cmd     Command
		wantErr bool
	}{
		{
			name: "rename command (no-op)",
			cmd: Command{
				Type:   CommandRename,
				Params: map[string]interface{}{"name": "new-name"},
			},
			wantErr: false,
		},
		{
			name: "set directory (no-op)",
			cmd: Command{
				Type:   CommandSetDir,
				Params: map[string]interface{}{"path": "/new/path"},
			},
			wantErr: false,
		},
		{
			name: "authorize directory (no-op)",
			cmd: Command{
				Type:   CommandAuthorize,
				Params: map[string]interface{}{"path": "/authorized/path"},
			},
			wantErr: false,
		},
		{
			name: "run hook (not supported)",
			cmd: Command{
				Type:   CommandRunHook,
				Params: map[string]interface{}{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := adapter.ExecuteCommand(tt.cmd)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGeminiAdapter_splitLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "empty string",
			input: "",
			want:  nil,
		},
		{
			name:  "single line",
			input: "line1",
			want:  []string{"line1"},
		},
		{
			name:  "multiple lines",
			input: "line1\nline2\nline3",
			want:  []string{"line1", "line2", "line3"},
		},
		{
			name:  "trailing newline",
			input: "line1\nline2\n",
			want:  []string{"line1", "line2", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitLines(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGeminiAdapter_getSessionDir(t *testing.T) {
	adapter, err := NewGeminiAdapter(&GeminiConfig{
		APIKey:       "test-key",
		SessionStore: newTestMockStore(),
	})
	require.NoError(t, err)

	geminiAdapter := adapter.(*GeminiAdapter)
	sessionID := SessionID("test-session-123")

	dir, err := geminiAdapter.getSessionDir(sessionID)
	require.NoError(t, err)

	homeDir, _ := os.UserHomeDir()
	expected := filepath.Join(homeDir, ".agm", "gemini", "test-session-123")
	assert.Equal(t, expected, dir)
}

func TestGeminiAdapter_getHistoryPath(t *testing.T) {
	adapter, err := NewGeminiAdapter(&GeminiConfig{
		APIKey:       "test-key",
		SessionStore: newTestMockStore(),
	})
	require.NoError(t, err)

	geminiAdapter := adapter.(*GeminiAdapter)
	sessionID := SessionID("test-session-123")

	historyPath, err := geminiAdapter.getHistoryPath(sessionID)
	require.NoError(t, err)

	homeDir, _ := os.UserHomeDir()
	expected := filepath.Join(homeDir, ".agm", "gemini", "test-session-123", "history.jsonl")
	assert.Equal(t, expected, historyPath)
}
