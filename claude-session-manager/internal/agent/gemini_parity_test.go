package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGeminiAdapter_FeatureParity_AgentInterface verifies Gemini implements all Agent methods
func TestGeminiAdapter_FeatureParity_AgentInterface(t *testing.T) {
	var _ Agent = (*GeminiAdapter)(nil)

	adapter, err := NewGeminiAdapter(&GeminiConfig{
		APIKey:       "test-key",
		SessionStore: newTestMockStore(),
	})
	require.NoError(t, err)

	// Verify all interface methods exist and return expected types
	tests := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Name returns string",
			testFunc: func(t *testing.T) {
				name := adapter.Name()
				assert.Equal(t, "gemini", name)
			},
		},
		{
			name: "Version returns string",
			testFunc: func(t *testing.T) {
				version := adapter.Version()
				assert.NotEmpty(t, version)
			},
		},
		{
			name: "Capabilities returns struct",
			testFunc: func(t *testing.T) {
				caps := adapter.Capabilities()
				assert.NotEmpty(t, caps.ModelName)
				assert.Greater(t, caps.MaxContextWindow, 0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

// TestGeminiAdapter_SessionLifecycle tests complete session lifecycle
func TestGeminiAdapter_SessionLifecycle(t *testing.T) {
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	adapter, err := NewGeminiAdapter(&GeminiConfig{
		APIKey:       "test-key",
		SessionStore: newTestMockStore(),
	})
	require.NoError(t, err)

	// Create session
	ctx := SessionContext{
		Name:             "lifecycle-test",
		WorkingDirectory: tmpDir,
		Project:          "test-project",
	}
	sessionID, err := adapter.CreateSession(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, sessionID)

	// Verify session is active
	status, err := adapter.GetSessionStatus(sessionID)
	require.NoError(t, err)
	assert.Equal(t, StatusActive, status)

	// Resume session (should not error)
	err = adapter.ResumeSession(sessionID)
	require.NoError(t, err)

	// Terminate session
	err = adapter.TerminateSession(sessionID)
	require.NoError(t, err)

	// Verify session is terminated
	status, err = adapter.GetSessionStatus(sessionID)
	require.NoError(t, err)
	assert.Equal(t, StatusTerminated, status)
}

// TestGeminiAdapter_ResumeSession_EdgeCases tests resume session error conditions
func TestGeminiAdapter_ResumeSession_EdgeCases(t *testing.T) {
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	tests := []struct {
		name        string
		setup       func(t *testing.T, adapter Agent) SessionID
		wantErr     bool
		errContains string
	}{
		{
			name: "resume non-existent session",
			setup: func(t *testing.T, adapter Agent) SessionID {
				return SessionID("non-existent-session")
			},
			wantErr:     true,
			errContains: "session not found",
		},
		{
			name: "resume valid session",
			setup: func(t *testing.T, adapter Agent) SessionID {
				sessionID, err := adapter.CreateSession(SessionContext{
					Name:             "valid-session",
					WorkingDirectory: tmpDir,
				})
				require.NoError(t, err)
				return sessionID
			},
			wantErr: false,
		},
		{
			name: "resume after directory deleted",
			setup: func(t *testing.T, adapter Agent) SessionID {
				sessionID, err := adapter.CreateSession(SessionContext{
					Name:             "deleted-dir-session",
					WorkingDirectory: tmpDir,
				})
				require.NoError(t, err)

				// Delete session directory
				geminiAdapter := adapter.(*GeminiAdapter)
				sessionDir, err := geminiAdapter.getSessionDir(sessionID)
				require.NoError(t, err)
				os.RemoveAll(sessionDir)

				return sessionID
			},
			wantErr:     true,
			errContains: "session directory not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := NewGeminiAdapter(&GeminiConfig{
				APIKey:       "test-key",
				SessionStore: newTestMockStore(),
			})
			require.NoError(t, err)

			sessionID := tt.setup(t, adapter)
			err = adapter.ResumeSession(sessionID)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestGeminiAdapter_CreateSession_ErrorHandling tests session creation failures
func TestGeminiAdapter_CreateSession_ErrorHandling(t *testing.T) {
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	tests := []struct {
		name        string
		setup       func(t *testing.T) *GeminiAdapter
		ctx         SessionContext
		wantErr     bool
		errContains string
	}{
		{
			name: "invalid working directory path",
			setup: func(t *testing.T) *GeminiAdapter {
				adapter, err := NewGeminiAdapter(&GeminiConfig{
					APIKey:       "test-key",
					SessionStore: newTestMockStore(),
				})
				require.NoError(t, err)
				return adapter.(*GeminiAdapter)
			},
			ctx: SessionContext{
				Name:             "test",
				WorkingDirectory: "", // Empty working directory
			},
			wantErr: false, // Should still create session
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := tt.setup(t)
			sessionID, err := adapter.CreateSession(tt.ctx)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Empty(t, sessionID)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, sessionID)
			}
		})
	}
}

// TestGeminiAdapter_HistoryPersistence tests history file persistence
func TestGeminiAdapter_HistoryPersistence(t *testing.T) {
	tmpDir := t.TempDir()

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
		Name:             "history-test",
		WorkingDirectory: tmpDir,
	})
	require.NoError(t, err)

	// Add messages to history
	geminiAdapter := adapter.(*GeminiAdapter)
	messages := []Message{
		{
			ID:        "msg-1",
			Role:      RoleUser,
			Content:   "First message",
			Timestamp: time.Now(),
		},
		{
			ID:        "msg-2",
			Role:      RoleAssistant,
			Content:   "First response",
			Timestamp: time.Now(),
		},
		{
			ID:        "msg-3",
			Role:      RoleUser,
			Content:   "Second message",
			Timestamp: time.Now(),
		},
	}

	for _, msg := range messages {
		err = geminiAdapter.appendToHistory(sessionID, msg)
		require.NoError(t, err)
	}

	// Verify history file exists
	historyPath, err := geminiAdapter.getHistoryPath(sessionID)
	require.NoError(t, err)

	stat, err := os.Stat(historyPath)
	require.NoError(t, err)
	assert.False(t, stat.IsDir())

	// Read and verify history
	history, err := adapter.GetHistory(sessionID)
	require.NoError(t, err)
	assert.Len(t, history, 3)
	assert.Equal(t, "First message", history[0].Content)
	assert.Equal(t, "First response", history[1].Content)
	assert.Equal(t, "Second message", history[2].Content)

	// Verify JSONL format
	data, err := os.ReadFile(historyPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	assert.Len(t, lines, 3)

	// Each line should be valid JSON
	for i, line := range lines {
		var msg Message
		err := json.Unmarshal([]byte(line), &msg)
		require.NoError(t, err, "line %d should be valid JSON", i)
	}
}

// TestGeminiAdapter_GetHistory_MalformedData tests robustness against corrupted history
func TestGeminiAdapter_GetHistory_MalformedData(t *testing.T) {
	tmpDir := t.TempDir()

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
		Name:             "malformed-test",
		WorkingDirectory: tmpDir,
	})
	require.NoError(t, err)

	// Write malformed data to history file
	geminiAdapter := adapter.(*GeminiAdapter)
	historyPath, err := geminiAdapter.getHistoryPath(sessionID)
	require.NoError(t, err)

	malformedData := `{"id":"msg-1","role":"user","content":"Valid message","timestamp":"2024-01-01T00:00:00Z"}
invalid json line
{"id":"msg-2","role":"assistant","content":"Another valid message","timestamp":"2024-01-01T00:00:01Z"}
`
	err = os.WriteFile(historyPath, []byte(malformedData), 0644)
	require.NoError(t, err)

	// GetHistory should skip malformed lines and return valid messages
	history, err := adapter.GetHistory(sessionID)
	require.NoError(t, err)
	assert.Len(t, history, 2, "should skip malformed line and return 2 valid messages")
	assert.Equal(t, "Valid message", history[0].Content)
	assert.Equal(t, "Another valid message", history[1].Content)
}

// TestGeminiAdapter_ExportImport_RoundTrip tests export/import round-trip
func TestGeminiAdapter_ExportImport_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()

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
		Name:             "roundtrip-test",
		WorkingDirectory: tmpDir,
	})
	require.NoError(t, err)

	geminiAdapter := adapter.(*GeminiAdapter)
	originalMessages := []Message{
		{
			ID:        "msg-1",
			Role:      RoleUser,
			Content:   "Test question with special chars: \n\t\"quotes\"",
			Timestamp: time.Now(),
		},
		{
			ID:        "msg-2",
			Role:      RoleAssistant,
			Content:   "Response with emoji 🚀 and unicode ñ",
			Timestamp: time.Now(),
		},
	}

	for _, msg := range originalMessages {
		err = geminiAdapter.appendToHistory(sessionID, msg)
		require.NoError(t, err)
	}

	// Export conversation
	exportedData, err := adapter.ExportConversation(sessionID, FormatJSONL)
	require.NoError(t, err)
	assert.NotEmpty(t, exportedData)

	// Import conversation into new session
	importedSessionID, err := adapter.ImportConversation(exportedData, FormatJSONL)
	require.NoError(t, err)
	assert.NotEmpty(t, importedSessionID)
	assert.NotEqual(t, sessionID, importedSessionID, "imported session should have different ID")

	// Verify imported history matches original
	importedHistory, err := adapter.GetHistory(importedSessionID)
	require.NoError(t, err)
	assert.Len(t, importedHistory, len(originalMessages))

	for i, original := range originalMessages {
		assert.Equal(t, original.ID, importedHistory[i].ID)
		assert.Equal(t, original.Role, importedHistory[i].Role)
		assert.Equal(t, original.Content, importedHistory[i].Content)
	}
}

// TestGeminiAdapter_ExportMarkdown_Format tests markdown export formatting
func TestGeminiAdapter_ExportMarkdown_Format(t *testing.T) {
	tmpDir := t.TempDir()

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
		Name:             "markdown-test",
		WorkingDirectory: tmpDir,
	})
	require.NoError(t, err)

	geminiAdapter := adapter.(*GeminiAdapter)
	testTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	messages := []Message{
		{
			ID:        "msg-1",
			Role:      RoleUser,
			Content:   "What is the capital of France?",
			Timestamp: testTime,
		},
		{
			ID:        "msg-2",
			Role:      RoleAssistant,
			Content:   "The capital of France is Paris.",
			Timestamp: testTime.Add(1 * time.Second),
		},
	}

	for _, msg := range messages {
		err = geminiAdapter.appendToHistory(sessionID, msg)
		require.NoError(t, err)
	}

	// Export as markdown
	markdown, err := adapter.ExportConversation(sessionID, FormatMarkdown)
	require.NoError(t, err)
	assert.NotEmpty(t, markdown)

	markdownStr := string(markdown)

	// Verify markdown structure
	assert.Contains(t, markdownStr, "# Gemini Conversation")
	assert.Contains(t, markdownStr, "Session ID: "+string(sessionID))
	assert.Contains(t, markdownStr, "## User")
	assert.Contains(t, markdownStr, "## Assistant")
	assert.Contains(t, markdownStr, "What is the capital of France?")
	assert.Contains(t, markdownStr, "The capital of France is Paris.")
	assert.Contains(t, markdownStr, testTime.Format(time.RFC3339))
}

// TestGeminiAdapter_Capabilities_Parity tests capabilities match expectations
func TestGeminiAdapter_Capabilities_Parity(t *testing.T) {
	adapter, err := NewGeminiAdapter(&GeminiConfig{
		ModelName:    "gemini-2.0-flash-exp",
		APIKey:       "test-key",
		SessionStore: newTestMockStore(),
	})
	require.NoError(t, err)

	caps := adapter.Capabilities()

	// Verify expected capabilities
	tests := []struct {
		name     string
		expected bool
		actual   bool
		reason   string
	}{
		{
			name:     "SupportsSlashCommands",
			expected: false,
			actual:   caps.SupportsSlashCommands,
			reason:   "API agents don't have CLI slash commands",
		},
		{
			name:     "SupportsHooks",
			expected: false,
			actual:   caps.SupportsHooks,
			reason:   "AGM-level feature, not agent-specific",
		},
		{
			name:     "SupportsTools",
			expected: true,
			actual:   caps.SupportsTools,
			reason:   "Gemini supports function calling",
		},
		{
			name:     "SupportsVision",
			expected: true,
			actual:   caps.SupportsVision,
			reason:   "Gemini 2.0 supports vision",
		},
		{
			name:     "SupportsMultimodal",
			expected: true,
			actual:   caps.SupportsMultimodal,
			reason:   "Gemini 2.0 supports audio/video",
		},
		{
			name:     "SupportsStreaming",
			expected: true,
			actual:   caps.SupportsStreaming,
			reason:   "Gemini API supports streaming",
		},
		{
			name:     "SupportsSystemPrompts",
			expected: true,
			actual:   caps.SupportsSystemPrompts,
			reason:   "Gemini supports system instructions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.actual, tt.reason)
		})
	}

	// Verify numeric capabilities
	assert.Equal(t, 1000000, caps.MaxContextWindow, "Gemini 2.0 has 1M token context")
	assert.Equal(t, "gemini-2.0-flash-exp", caps.ModelName)
}

// TestGeminiAdapter_ExecuteCommand_Coverage tests all command types
func TestGeminiAdapter_ExecuteCommand_Coverage(t *testing.T) {
	adapter, err := NewGeminiAdapter(&GeminiConfig{
		APIKey:       "test-key",
		SessionStore: newTestMockStore(),
	})
	require.NoError(t, err)

	tests := []struct {
		name        string
		cmd         Command
		wantErr     bool
		errContains string
	}{
		{
			name: "rename command",
			cmd: Command{
				Type:   CommandRename,
				Params: map[string]interface{}{"name": "new-name"},
			},
			wantErr: false,
		},
		{
			name: "set directory command",
			cmd: Command{
				Type:   CommandSetDir,
				Params: map[string]interface{}{"path": "/new/path"},
			},
			wantErr: false,
		},
		{
			name: "authorize directory command",
			cmd: Command{
				Type:   CommandAuthorize,
				Params: map[string]interface{}{"path": "/authorized/path"},
			},
			wantErr: false,
		},
		{
			name: "run hook command (unsupported)",
			cmd: Command{
				Type:   CommandRunHook,
				Params: map[string]interface{}{},
			},
			wantErr:     true,
			errContains: "hooks not supported",
		},
		{
			name: "clear history command (unsupported)",
			cmd: Command{
				Type:   CommandClearHistory,
				Params: map[string]interface{}{},
			},
			wantErr:     true,
			errContains: "unsupported command type",
		},
		{
			name: "set system prompt command (unsupported)",
			cmd: Command{
				Type:   CommandSetSystemPrompt,
				Params: map[string]interface{}{},
			},
			wantErr:     true,
			errContains: "unsupported command type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := adapter.ExecuteCommand(tt.cmd)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestGeminiAdapter_SessionDirectory_Structure tests directory structure
func TestGeminiAdapter_SessionDirectory_Structure(t *testing.T) {
	tmpDir := t.TempDir()

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
		Name:             "structure-test",
		WorkingDirectory: tmpDir,
	})
	require.NoError(t, err)

	geminiAdapter := adapter.(*GeminiAdapter)

	// Verify session directory exists
	sessionDir, err := geminiAdapter.getSessionDir(sessionID)
	require.NoError(t, err)

	expectedPath := filepath.Join(tmpDir, ".agm", "gemini", string(sessionID))
	assert.Equal(t, expectedPath, sessionDir)

	stat, err := os.Stat(sessionDir)
	require.NoError(t, err)
	assert.True(t, stat.IsDir())
	assert.Equal(t, os.FileMode(0755), stat.Mode().Perm())

	// Verify history file path
	historyPath, err := geminiAdapter.getHistoryPath(sessionID)
	require.NoError(t, err)

	expectedHistoryPath := filepath.Join(sessionDir, "history.jsonl")
	assert.Equal(t, expectedHistoryPath, historyPath)
}

// TestGeminiAdapter_ConcurrentSessions tests multiple simultaneous sessions
func TestGeminiAdapter_ConcurrentSessions(t *testing.T) {
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	adapter, err := NewGeminiAdapter(&GeminiConfig{
		APIKey:       "test-key",
		SessionStore: newTestMockStore(),
	})
	require.NoError(t, err)

	// Create multiple sessions
	numSessions := 5
	sessionIDs := make([]SessionID, numSessions)

	for i := 0; i < numSessions; i++ {
		sessionID, err := adapter.CreateSession(SessionContext{
			Name:             "concurrent-test-" + string(rune('0'+i)),
			WorkingDirectory: tmpDir,
		})
		require.NoError(t, err)
		sessionIDs[i] = sessionID
	}

	// Verify all sessions are independent
	for i, sessionID := range sessionIDs {
		// Check status
		status, err := adapter.GetSessionStatus(sessionID)
		require.NoError(t, err)
		assert.Equal(t, StatusActive, status)

		// Add unique message to each session
		geminiAdapter := adapter.(*GeminiAdapter)
		msg := Message{
			ID:        "msg-" + string(rune('0'+i)),
			Role:      RoleUser,
			Content:   "Message for session " + string(rune('0'+i)),
			Timestamp: time.Now(),
		}
		err = geminiAdapter.appendToHistory(sessionID, msg)
		require.NoError(t, err)
	}

	// Verify each session has only its own message
	for i, sessionID := range sessionIDs {
		history, err := adapter.GetHistory(sessionID)
		require.NoError(t, err)
		assert.Len(t, history, 1)
		assert.Equal(t, "Message for session "+string(rune('0'+i)), history[0].Content)
	}
}

// TestGeminiAdapter_TerminateSession_Preservation tests history preservation after termination
func TestGeminiAdapter_TerminateSession_Preservation(t *testing.T) {
	tmpDir := t.TempDir()

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
		Name:             "preserve-test",
		WorkingDirectory: tmpDir,
	})
	require.NoError(t, err)

	geminiAdapter := adapter.(*GeminiAdapter)
	msg := Message{
		ID:        "msg-1",
		Role:      RoleUser,
		Content:   "Important message",
		Timestamp: time.Now(),
	}
	err = geminiAdapter.appendToHistory(sessionID, msg)
	require.NoError(t, err)

	// Get session directory path before termination
	sessionDir, err := geminiAdapter.getSessionDir(sessionID)
	require.NoError(t, err)

	// Terminate session
	err = adapter.TerminateSession(sessionID)
	require.NoError(t, err)

	// Verify session directory still exists (preserved for historical purposes)
	stat, err := os.Stat(sessionDir)
	require.NoError(t, err, "session directory should still exist after termination")
	assert.True(t, stat.IsDir())

	// Verify history file still exists and is readable
	historyPath, err := geminiAdapter.getHistoryPath(sessionID)
	require.NoError(t, err)

	data, err := os.ReadFile(historyPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "Important message")
}

// TestGeminiAdapter_ImportConversation_InvalidFormat tests import error handling
func TestGeminiAdapter_ImportConversation_InvalidFormat(t *testing.T) {
	tmpDir := t.TempDir()

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	adapter, err := NewGeminiAdapter(&GeminiConfig{
		APIKey:       "test-key",
		SessionStore: newTestMockStore(),
	})
	require.NoError(t, err)

	tests := []struct {
		name        string
		data        []byte
		format      ConversationFormat
		wantErr     bool
		errContains string
	}{
		{
			name:        "unsupported format (HTML)",
			data:        []byte("<html>test</html>"),
			format:      FormatHTML,
			wantErr:     true,
			errContains: "unsupported import format",
		},
		{
			name:        "unsupported format (Markdown)",
			data:        []byte("# Test"),
			format:      FormatMarkdown,
			wantErr:     true,
			errContains: "unsupported import format",
		},
		{
			name:        "invalid JSONL data",
			data:        []byte("not json"),
			format:      FormatJSONL,
			wantErr:     true,
			errContains: "failed to parse message",
		},
		{
			name:   "empty JSONL data",
			data:   []byte(""),
			format: FormatJSONL,
			// Should create session with no history
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionID, err := adapter.ImportConversation(tt.data, tt.format)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Empty(t, sessionID)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, sessionID)
			}
		})
	}
}
