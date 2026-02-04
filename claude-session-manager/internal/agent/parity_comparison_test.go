package agent

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAgentParity_InterfaceCompliance verifies all agents implement the same interface
func TestAgentParity_InterfaceCompliance(t *testing.T) {
	tests := []struct {
		name    string
		agent   Agent
		wantErr bool
	}{
		{
			name: "GeminiAdapter implements Agent",
			agent: func() Agent {
				a, _ := NewGeminiAdapter(&GeminiConfig{
					APIKey:       "test-key",
					SessionStore: newTestMockStore(),
				})
				return a
			}(),
			wantErr: false,
		},
		{
			name: "ClaudeAdapter implements Agent",
			agent: func() Agent {
				a, _ := NewClaudeAdapter(newTestMockStore())
				return a
			}(),
			wantErr: false,
		},
		{
			name: "MockAgent implements Agent",
			agent: &MockAgent{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotNil(t, tt.agent)

			// Verify all methods exist
			assert.NotEmpty(t, tt.agent.Name())
			assert.NotEmpty(t, tt.agent.Version())

			caps := tt.agent.Capabilities()
			assert.NotEmpty(t, caps.ModelName)
		})
	}
}

// TestAgentParity_Capabilities compares capabilities across agents
func TestAgentParity_Capabilities(t *testing.T) {
	geminiAdapter, err := NewGeminiAdapter(&GeminiConfig{
		APIKey:       "test-key",
		SessionStore: newTestMockStore(),
	})
	require.NoError(t, err)

	claudeAdapter, err := NewClaudeAdapter(newTestMockStore())
	require.NoError(t, err)

	geminCaps := geminiAdapter.Capabilities()
	claudeCaps := claudeAdapter.Capabilities()

	tests := []struct {
		name          string
		geminiValue   interface{}
		claudeValue   interface{}
		shouldMatch   bool
		description   string
	}{
		{
			name:          "SupportsTools",
			geminiValue:   geminCaps.SupportsTools,
			claudeValue:   claudeCaps.SupportsTools,
			shouldMatch:   true,
			description:   "Both should support tools/functions",
		},
		{
			name:          "SupportsVision",
			geminiValue:   geminCaps.SupportsVision,
			claudeValue:   claudeCaps.SupportsVision,
			shouldMatch:   true,
			description:   "Both should support vision",
		},
		{
			name:          "SupportsStreaming",
			geminiValue:   geminCaps.SupportsStreaming,
			claudeValue:   claudeCaps.SupportsStreaming,
			shouldMatch:   true,
			description:   "Both should support streaming",
		},
		{
			name:          "SupportsSystemPrompts",
			geminiValue:   geminCaps.SupportsSystemPrompts,
			claudeValue:   claudeCaps.SupportsSystemPrompts,
			shouldMatch:   true,
			description:   "Both should support system prompts",
		},
		{
			name:          "SupportsSlashCommands",
			geminiValue:   geminCaps.SupportsSlashCommands,
			claudeValue:   claudeCaps.SupportsSlashCommands,
			shouldMatch:   false,
			description:   "Only Claude CLI supports slash commands",
		},
		{
			name:          "SupportsMultimodal",
			geminiValue:   geminCaps.SupportsMultimodal,
			claudeValue:   claudeCaps.SupportsMultimodal,
			shouldMatch:   false,
			description:   "Gemini 2.0 has audio/video, Claude doesn't yet",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shouldMatch {
				assert.Equal(t, tt.claudeValue, tt.geminiValue, tt.description)
			} else {
				assert.NotEqual(t, tt.claudeValue, tt.geminiValue, tt.description)
			}
		})
	}

	// Verify context windows are positive
	assert.Greater(t, geminCaps.MaxContextWindow, 0)
	assert.Greater(t, claudeCaps.MaxContextWindow, 0)

	// Document context window differences
	t.Logf("Gemini context window: %d tokens", geminCaps.MaxContextWindow)
	t.Logf("Claude context window: %d tokens", claudeCaps.MaxContextWindow)
	t.Logf("Context window ratio: %.2fx", float64(geminCaps.MaxContextWindow)/float64(claudeCaps.MaxContextWindow))
}

// TestAgentParity_SessionLifecycle compares session lifecycle behavior
func TestAgentParity_SessionLifecycle(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "parity-lifecycle-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	tests := []struct {
		name  string
		agent Agent
	}{
		{
			name: "Gemini",
			agent: func() Agent {
				a, _ := NewGeminiAdapter(&GeminiConfig{
					APIKey:       "test-key",
					SessionStore: newTestMockStore(),
				})
				return a
			}(),
		},
		// Note: Claude adapter requires tmux, so we skip it in this test
		// It's tested separately in claude_adapter_test.go
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create session
			ctx := SessionContext{
				Name:             "parity-test",
				WorkingDirectory: tmpDir,
				Project:          "test-project",
			}
			sessionID, err := tt.agent.CreateSession(ctx)
			require.NoError(t, err)
			assert.NotEmpty(t, sessionID)

			// Check status
			status, err := tt.agent.GetSessionStatus(sessionID)
			require.NoError(t, err)
			assert.Equal(t, StatusActive, status)

			// Get empty history
			history, err := tt.agent.GetHistory(sessionID)
			require.NoError(t, err)
			assert.Empty(t, history)

			// Terminate
			err = tt.agent.TerminateSession(sessionID)
			require.NoError(t, err)

			// Verify terminated
			status, err = tt.agent.GetSessionStatus(sessionID)
			require.NoError(t, err)
			assert.Equal(t, StatusTerminated, status)
		})
	}
}

// TestAgentParity_ExportFormats compares export format support
func TestAgentParity_ExportFormats(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "parity-export-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	tests := []struct {
		agentName      string
		agent          Agent
		supportedFormats []ConversationFormat
		unsupportedFormats []ConversationFormat
	}{
		{
			agentName: "Gemini",
			agent: func() Agent {
				a, _ := NewGeminiAdapter(&GeminiConfig{
					APIKey:       "test-key",
					SessionStore: newTestMockStore(),
				})
				return a
			}(),
			supportedFormats: []ConversationFormat{
				FormatJSONL,
				FormatMarkdown,
			},
			unsupportedFormats: []ConversationFormat{
				FormatHTML,
			},
		},
		{
			agentName: "Claude",
			agent: func() Agent {
				a, _ := NewClaudeAdapter(newTestMockStore())
				return a
			}(),
			supportedFormats: []ConversationFormat{
				FormatJSONL,
				FormatMarkdown,
			},
			unsupportedFormats: []ConversationFormat{
				FormatHTML, // Not yet implemented
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.agentName, func(t *testing.T) {
			// Create session
			sessionID, err := tt.agent.CreateSession(SessionContext{
				Name:             "export-test",
				WorkingDirectory: tmpDir,
			})
			require.NoError(t, err)

			// Test supported formats
			for _, format := range tt.supportedFormats {
				t.Run("supported_"+string(format), func(t *testing.T) {
					data, err := tt.agent.ExportConversation(sessionID, format)
					require.NoError(t, err, "agent %s should support %s export", tt.agentName, format)
					// Data can be empty (nil or empty slice) for empty conversations
					_ = data
				})
			}

			// Test unsupported formats
			for _, format := range tt.unsupportedFormats {
				t.Run("unsupported_"+string(format), func(t *testing.T) {
					data, err := tt.agent.ExportConversation(sessionID, format)
					require.Error(t, err, "agent %s should not support %s export", tt.agentName, format)
					assert.Nil(t, data)
				})
			}
		})
	}
}

// TestAgentParity_CommandSupport compares command execution support
func TestAgentParity_CommandSupport(t *testing.T) {
	tests := []struct {
		agentName        string
		agent            Agent
		supportedCommands []CommandType
		unsupportedCommands []CommandType
	}{
		{
			agentName: "Gemini",
			agent: func() Agent {
				a, _ := NewGeminiAdapter(&GeminiConfig{
					APIKey:       "test-key",
					SessionStore: newTestMockStore(),
				})
				return a
			}(),
			supportedCommands: []CommandType{
				CommandRename,
				CommandSetDir,
				CommandAuthorize,
			},
			unsupportedCommands: []CommandType{
				CommandRunHook, // API agents don't support hooks
				CommandClearHistory,
				CommandSetSystemPrompt,
			},
		},
		{
			agentName: "Claude",
			agent: func() Agent {
				a, _ := NewClaudeAdapter(newTestMockStore())
				return a
			}(),
			supportedCommands: []CommandType{
				CommandRename,
				CommandSetDir,
			},
			unsupportedCommands: []CommandType{
				CommandAuthorize, // Not yet implemented
				CommandRunHook,   // Not yet implemented
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.agentName, func(t *testing.T) {
			// Test supported commands (should not error or return specific behavior)
			for _, cmdType := range tt.supportedCommands {
				t.Run("supported_"+string(cmdType), func(t *testing.T) {
					_ = Command{
						Type: cmdType,
						Params: map[string]interface{}{
							"session_id": "test-session",
							"name":       "test-name",
							"path":       "/test/path",
						},
					}
					// Note: We can't fully test commands without valid sessions
					// This just documents which commands each agent claims to support
					t.Logf("%s claims to support %s", tt.agentName, cmdType)
				})
			}

			// Test unsupported commands
			for _, cmdType := range tt.unsupportedCommands {
				t.Run("unsupported_"+string(cmdType), func(t *testing.T) {
					cmd := Command{
						Type: cmdType,
						Params: map[string]interface{}{
							"session_id": "test-session",
						},
					}
					err := tt.agent.ExecuteCommand(cmd)
					assert.Error(t, err, "%s should not support %s", tt.agentName, cmdType)
				})
			}
		})
	}
}

// TestAgentParity_NameAndVersion verifies agent identification
func TestAgentParity_NameAndVersion(t *testing.T) {
	tests := []struct {
		name          string
		agent         Agent
		expectedName  string
		versionPrefix string
	}{
		{
			name: "Gemini",
			agent: func() Agent {
				a, _ := NewGeminiAdapter(&GeminiConfig{
					APIKey:       "test-key",
					SessionStore: newTestMockStore(),
				})
				return a
			}(),
			expectedName:  "gemini",
			versionPrefix: "gemini-",
		},
		{
			name: "Claude",
			agent: func() Agent {
				a, _ := NewClaudeAdapter(newTestMockStore())
				return a
			}(),
			expectedName:  "claude",
			versionPrefix: "claude-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedName, tt.agent.Name())
			assert.Contains(t, tt.agent.Version(), tt.versionPrefix)
		})
	}
}

// TestAgentParity_SessionMetadata compares how agents store metadata
func TestAgentParity_SessionMetadata(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "parity-metadata-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	ctx := SessionContext{
		Name:             "metadata-test",
		WorkingDirectory: tmpDir,
		Project:          "test-project",
		AuthorizedDirs: []string{
			tmpDir + "/dir1",
			tmpDir + "/dir2",
		},
	}

	tests := []struct {
		name  string
		agent Agent
		store SessionStore
	}{
		{
			name: "Gemini",
			agent: func() Agent {
				store := newTestMockStore()
				a, _ := NewGeminiAdapter(&GeminiConfig{
					APIKey:       "test-key",
					SessionStore: store,
				})
				return a
			}(),
			store: newTestMockStore(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionID, err := tt.agent.CreateSession(ctx)
			require.NoError(t, err)

			// Extract store from agent
			var store SessionStore
			switch a := tt.agent.(type) {
			case *GeminiAdapter:
				store = a.sessionStore
			case *ClaudeAdapter:
				store = a.sessionStore
			}

			require.NotNil(t, store)

			// Verify metadata was stored
			metadata, err := store.Get(sessionID)
			require.NoError(t, err)
			assert.Equal(t, ctx.WorkingDirectory, metadata.WorkingDir)
			assert.Equal(t, ctx.Project, metadata.Project)
			assert.False(t, metadata.CreatedAt.IsZero())
		})
	}
}
