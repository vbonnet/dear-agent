package gpt

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/agent"
)

// TestGPTAdapter_InterfaceCompliance verifies that Adapter satisfies the Agent interface.
func TestGPTAdapter_InterfaceCompliance(t *testing.T) {
	var _ agent.Agent = (*Adapter)(nil)
}

// TestNewAdapter_MissingAPIKey tests that constructor fails without API key.
func TestNewAdapter_MissingAPIKey(t *testing.T) {
	// Save and unset API key
	originalKey := os.Getenv("OPENAI_API_KEY")
	os.Unsetenv("OPENAI_API_KEY")
	defer os.Setenv("OPENAI_API_KEY", originalKey)

	_, err := NewAdapter()
	assert.ErrorIs(t, err, ErrAPIKeyNotSet)
}

// TestNewAdapter_WithAPIKey tests that constructor succeeds with API key.
func TestNewAdapter_WithAPIKey(t *testing.T) {
	// Set fake API key for unit test
	os.Setenv("OPENAI_API_KEY", "sk-test-key")

	adapter, err := NewAdapter()
	require.NoError(t, err)
	require.NotNil(t, adapter)

	assert.Equal(t, "gpt", adapter.Name())
	assert.Equal(t, "gpt-4-turbo", adapter.Version())
}

// TestAdapter_Metadata tests Name, Version, and Capabilities methods.
func TestAdapter_Metadata(t *testing.T) {
	os.Setenv("OPENAI_API_KEY", "sk-test-key")
	adapter, _ := NewAdapter()

	t.Run("Name", func(t *testing.T) {
		assert.Equal(t, "gpt", adapter.Name())
	})

	t.Run("Version", func(t *testing.T) {
		assert.Equal(t, "gpt-4-turbo", adapter.Version())
	})

	t.Run("Capabilities", func(t *testing.T) {
		caps := adapter.Capabilities()
		assert.False(t, caps.SupportsSlashCommands)
		assert.True(t, caps.SupportsTools)
		assert.True(t, caps.SupportsVision)
		assert.Equal(t, 128000, caps.MaxContextWindow)
		assert.Equal(t, "gpt-4-turbo", caps.ModelName)
	})
}

// TestAdapter_CreateSession tests session creation.
func TestAdapter_CreateSession(t *testing.T) {
	os.Setenv("OPENAI_API_KEY", "sk-test-key")
	adapter, _ := NewAdapter()
	gptAdapter := adapter.(*Adapter)

	t.Run("ValidContext", func(t *testing.T) {
		ctx := agent.SessionContext{
			Name:             "test-session",
			WorkingDirectory: "/tmp",
		}

		sessionID, err := gptAdapter.CreateSession(ctx)
		require.NoError(t, err)
		assert.NotEmpty(t, sessionID)
		assert.Regexp(t, `^[0-9a-f-]{36}$`, sessionID) // UUID format

		// Verify session stored
		gptAdapter.mu.RLock()
		session, exists := gptAdapter.sessions[sessionID]
		gptAdapter.mu.RUnlock()

		assert.True(t, exists)
		assert.Equal(t, sessionID, session.ID)
		assert.Equal(t, "test-session", session.Context.Name)
		assert.Equal(t, agent.StatusActive, session.Status)
	})

	t.Run("MissingName", func(t *testing.T) {
		ctx := agent.SessionContext{
			WorkingDirectory: "/tmp",
		}

		_, err := gptAdapter.CreateSession(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "session name required")
	})

	t.Run("MissingWorkingDirectory", func(t *testing.T) {
		ctx := agent.SessionContext{
			Name: "test",
		}

		_, err := gptAdapter.CreateSession(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "working directory required")
	})
}

// TestAdapter_SessionManagement tests session lifecycle.
func TestAdapter_SessionManagement(t *testing.T) {
	os.Setenv("OPENAI_API_KEY", "sk-test-key")
	adapter, _ := NewAdapter()
	gptAdapter := adapter.(*Adapter)

	ctx := agent.SessionContext{
		Name:             "test-session",
		WorkingDirectory: "/tmp",
	}
	sessionID, _ := gptAdapter.CreateSession(ctx)

	t.Run("GetSessionStatus_Active", func(t *testing.T) {
		status, err := gptAdapter.GetSessionStatus(sessionID)
		require.NoError(t, err)
		assert.Equal(t, agent.StatusActive, status)
	})

	t.Run("ResumeSession", func(t *testing.T) {
		err := gptAdapter.ResumeSession(sessionID)
		assert.NoError(t, err) // No-op for in-memory sessions
	})

	t.Run("TerminateSession", func(t *testing.T) {
		err := gptAdapter.TerminateSession(sessionID)
		require.NoError(t, err)

		// Verify session removed
		gptAdapter.mu.RLock()
		_, exists := gptAdapter.sessions[sessionID]
		gptAdapter.mu.RUnlock()

		assert.False(t, exists)
	})

	t.Run("GetSessionStatus_Terminated", func(t *testing.T) {
		status, err := gptAdapter.GetSessionStatus(sessionID)
		require.NoError(t, err)
		assert.Equal(t, agent.StatusTerminated, status)
	})

	t.Run("TerminateSession_NotFound", func(t *testing.T) {
		err := gptAdapter.TerminateSession("non-existent-id")
		assert.ErrorIs(t, err, ErrSessionNotFound)
	})
}

// TestAdapter_GetHistory tests conversation history retrieval.
func TestAdapter_GetHistory(t *testing.T) {
	os.Setenv("OPENAI_API_KEY", "sk-test-key")
	adapter, _ := NewAdapter()
	gptAdapter := adapter.(*Adapter)

	ctx := agent.SessionContext{
		Name:             "test-session",
		WorkingDirectory: "/tmp",
	}
	sessionID, _ := gptAdapter.CreateSession(ctx)

	t.Run("EmptyHistory", func(t *testing.T) {
		history, err := gptAdapter.GetHistory(sessionID)
		require.NoError(t, err)
		assert.Empty(t, history)
	})

	t.Run("WithMessages", func(t *testing.T) {
		// Manually add messages to session
		gptAdapter.mu.Lock()
		gptAdapter.sessions[sessionID].Messages = []agent.Message{
			{
				ID:        "msg-1",
				Role:      agent.RoleUser,
				Content:   "Hello",
				Timestamp: time.Now(),
			},
			{
				ID:        "msg-2",
				Role:      agent.RoleAssistant,
				Content:   "Hi!",
				Timestamp: time.Now(),
			},
		}
		gptAdapter.mu.Unlock()

		history, err := gptAdapter.GetHistory(sessionID)
		require.NoError(t, err)
		assert.Len(t, history, 2)
		assert.Equal(t, "Hello", history[0].Content)
		assert.Equal(t, "Hi!", history[1].Content)
	})

	t.Run("SessionNotFound", func(t *testing.T) {
		_, err := gptAdapter.GetHistory("non-existent-id")
		assert.ErrorIs(t, err, ErrSessionNotFound)
	})
}

// TestAdapter_ExportImport tests conversation export and import.
func TestAdapter_ExportImport(t *testing.T) {
	os.Setenv("OPENAI_API_KEY", "sk-test-key")
	adapter, _ := NewAdapter()
	gptAdapter := adapter.(*Adapter)

	// Create session with messages
	ctx := agent.SessionContext{
		Name:             "test-session",
		WorkingDirectory: "/tmp",
	}
	sessionID, _ := gptAdapter.CreateSession(ctx)

	gptAdapter.mu.Lock()
	gptAdapter.sessions[sessionID].Messages = []agent.Message{
		{
			ID:        "msg-1",
			Role:      agent.RoleUser,
			Content:   "Hello",
			Timestamp: time.Now(),
		},
	}
	gptAdapter.mu.Unlock()

	t.Run("ExportJSONL", func(t *testing.T) {
		data, err := gptAdapter.ExportConversation(sessionID, agent.FormatJSONL)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"Content":"Hello"`)
		assert.Contains(t, string(data), `"Role":"user"`)
	})

	t.Run("ExportMarkdown", func(t *testing.T) {
		data, err := gptAdapter.ExportConversation(sessionID, agent.FormatMarkdown)
		require.NoError(t, err)
		assert.Contains(t, string(data), "## user")
		assert.Contains(t, string(data), "Hello")
	})

	t.Run("ExportInvalidFormat", func(t *testing.T) {
		_, err := gptAdapter.ExportConversation(sessionID, agent.ConversationFormat("invalid"))
		assert.ErrorIs(t, err, ErrInvalidFormat)
	})

	t.Run("ImportJSONL", func(t *testing.T) {
		jsonlData := `{"ID":"msg-1","Role":"user","Content":"Test","Timestamp":"2026-01-16T00:00:00Z","Metadata":null}` + "\n"

		newSessionID, err := gptAdapter.ImportConversation([]byte(jsonlData), agent.FormatJSONL)
		require.NoError(t, err)
		assert.NotEmpty(t, newSessionID)

		history, _ := gptAdapter.GetHistory(newSessionID)
		assert.Len(t, history, 1)
		assert.Equal(t, "Test", history[0].Content)
	})

	t.Run("ImportUnsupportedFormat", func(t *testing.T) {
		_, err := gptAdapter.ImportConversation([]byte("data"), agent.FormatMarkdown)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "only JSONL import supported")
	})
}

// TestAdapter_ExecuteCommand tests command execution.
func TestAdapter_ExecuteCommand(t *testing.T) {
	os.Setenv("OPENAI_API_KEY", "sk-test-key")
	adapter, _ := NewAdapter()
	gptAdapter := adapter.(*Adapter)

	ctx := agent.SessionContext{
		Name:             "test-session",
		WorkingDirectory: "/tmp",
	}
	sessionID, _ := gptAdapter.CreateSession(ctx)

	t.Run("CommandRename", func(t *testing.T) {
		cmd := agent.Command{
			Type: agent.CommandRename,
			Params: map[string]interface{}{
				"session_id": sessionID,
				"name":       "new-name",
			},
		}

		err := gptAdapter.ExecuteCommand(cmd)
		require.NoError(t, err)

		gptAdapter.mu.RLock()
		newName := gptAdapter.sessions[sessionID].Context.Name
		gptAdapter.mu.RUnlock()

		assert.Equal(t, "new-name", newName)
	})

	t.Run("CommandSetDir", func(t *testing.T) {
		cmd := agent.Command{
			Type: agent.CommandSetDir,
			Params: map[string]interface{}{
				"session_id": sessionID,
				"path":       "/home/user",
			},
		}

		err := gptAdapter.ExecuteCommand(cmd)
		require.NoError(t, err)

		gptAdapter.mu.RLock()
		newPath := gptAdapter.sessions[sessionID].Context.WorkingDirectory
		gptAdapter.mu.RUnlock()

		assert.Equal(t, "/home/user", newPath)
	})

	t.Run("CommandAuthorize", func(t *testing.T) {
		cmd := agent.Command{
			Type: agent.CommandAuthorize,
			Params: map[string]interface{}{
				"path": "/some/path",
			},
		}

		err := gptAdapter.ExecuteCommand(cmd)
		assert.NoError(t, err) // No-op
	})

	t.Run("CommandRunHook", func(t *testing.T) {
		cmd := agent.Command{
			Type: agent.CommandRunHook,
			Params: map[string]interface{}{
				"hook_name": "test",
			},
		}

		err := gptAdapter.ExecuteCommand(cmd)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "hooks not supported")
	})

	t.Run("UnsupportedCommand", func(t *testing.T) {
		cmd := agent.Command{
			Type:   agent.CommandType("unknown"),
			Params: map[string]interface{}{},
		}

		err := gptAdapter.ExecuteCommand(cmd)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported command")
	})
}

// TestAdapter_ConcurrentAccess tests thread safety.
func TestAdapter_ConcurrentAccess(t *testing.T) {
	os.Setenv("OPENAI_API_KEY", "sk-test-key")
	adapter, _ := NewAdapter()
	gptAdapter := adapter.(*Adapter)

	// Create 100 sessions concurrently
	sessionIDs := make(chan agent.SessionID, 100)

	for i := 0; i < 100; i++ {
		go func(index int) {
			ctx := agent.SessionContext{
				Name:             "test-session",
				WorkingDirectory: "/tmp",
			}
			sessionID, err := gptAdapter.CreateSession(ctx)
			if err == nil {
				sessionIDs <- sessionID
			}
		}(i)
	}

	// Collect session IDs
	var ids []agent.SessionID
	for i := 0; i < 100; i++ {
		ids = append(ids, <-sessionIDs)
	}

	assert.Len(t, ids, 100)
	assert.Len(t, gptAdapter.sessions, 100)
}

// TestTranslator_Conversion tests message translation.
func TestTranslator_Conversion(t *testing.T) {
	t.Run("toOpenAIMessage_User", func(t *testing.T) {
		msg := agent.Message{
			Role:    agent.RoleUser,
			Content: "Hello",
		}

		openaiMsg := toOpenAIMessage(msg)
		assert.NotNil(t, openaiMsg)
	})

	t.Run("toOpenAIMessage_Assistant", func(t *testing.T) {
		msg := agent.Message{
			Role:    agent.RoleAssistant,
			Content: "Hi!",
		}

		openaiMsg := toOpenAIMessage(msg)
		assert.NotNil(t, openaiMsg)
	})
}
