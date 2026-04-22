package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/agm/internal/conversation"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// setupTestDB creates an in-memory test database
func setupTestDB(t *testing.T) *DB {
	db, err := Open(":memory:")
	require.NoError(t, err, "failed to create test database")
	return db
}

// TestDatabaseOpen tests database initialization
func TestDatabaseOpen(t *testing.T) {
	t.Run("successful open", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()

		assert.NotNil(t, db)
		assert.NotNil(t, db.conn)
	})

	t.Run("schema applied", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()

		// Verify tables exist by querying them
		_, err := db.conn.Exec("SELECT 1 FROM sessions LIMIT 1")
		assert.NoError(t, err)

		_, err = db.conn.Exec("SELECT 1 FROM messages LIMIT 1")
		assert.NoError(t, err)

		_, err = db.conn.Exec("SELECT 1 FROM escalations LIMIT 1")
		assert.NoError(t, err)
	})
}

// TestDatabaseClose tests database closure
func TestDatabaseClose(t *testing.T) {
	db := setupTestDB(t)
	err := db.Close()
	assert.NoError(t, err)
}

// TestDatabaseTransaction tests transaction support
func TestDatabaseTransaction(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	tx, err := db.BeginTx()
	require.NoError(t, err)

	// Try to insert a session within transaction
	_, err = tx.Exec(`
		INSERT INTO sessions (session_id, name, schema_version, created_at, updated_at, lifecycle, harness)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "test-session", "Test", "2.0", time.Now(), time.Now(), "", "claude-code")
	require.NoError(t, err)

	// Rollback should discard the insert
	err = tx.Rollback()
	require.NoError(t, err)

	// Verify session was not persisted
	session, err := db.GetSession("test-session")
	assert.Error(t, err)
	assert.Nil(t, session)
}

// createTestSession creates a test session for use in tests
func createTestSession(sessionID string) *manifest.Manifest {
	now := time.Now()
	return &manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     sessionID,
		Name:          "Test Session",
		CreatedAt:     now,
		UpdatedAt:     now,
		Lifecycle:     "",
		Harness:       "claude-code",
		Context: manifest.Context{
			Project: "test-project",
			Purpose: "testing",
			Tags:    []string{"test", "db"},
			Notes:   "Test notes",
		},
		Claude: manifest.Claude{
			UUID: "claude-uuid-123",
		},
		Tmux: manifest.Tmux{
			SessionName: "tmux-test",
		},
		EngramMetadata: &manifest.EngramMetadata{
			Enabled:   true,
			Query:     "test query",
			EngramIDs: []string{"engram-1", "engram-2"},
			LoadedAt:  now,
			Count:     2,
		},
	}
}

// TestSessionCRUD tests session CRUD operations
func TestSessionCRUD(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	session := createTestSession("test-session-1")

	t.Run("create session", func(t *testing.T) {
		err := db.CreateSession(session)
		assert.NoError(t, err)
	})

	t.Run("create duplicate session fails", func(t *testing.T) {
		err := db.CreateSession(session)
		assert.Error(t, err)
	})

	t.Run("get session", func(t *testing.T) {
		retrieved, err := db.GetSession("test-session-1")
		require.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, session.SessionID, retrieved.SessionID)
		assert.Equal(t, session.Name, retrieved.Name)
		assert.Equal(t, session.Harness, retrieved.Harness)
		assert.Equal(t, session.Context.Project, retrieved.Context.Project)
		assert.Equal(t, session.Context.Tags, retrieved.Context.Tags)
		assert.Equal(t, session.Claude.UUID, retrieved.Claude.UUID)
		assert.Equal(t, session.Tmux.SessionName, retrieved.Tmux.SessionName)
		assert.NotNil(t, retrieved.EngramMetadata)
		assert.True(t, retrieved.EngramMetadata.Enabled)
		assert.Equal(t, session.EngramMetadata.Query, retrieved.EngramMetadata.Query)
		assert.Equal(t, session.EngramMetadata.EngramIDs, retrieved.EngramMetadata.EngramIDs)
		assert.Equal(t, session.EngramMetadata.Count, retrieved.EngramMetadata.Count)
	})

	t.Run("get nonexistent session", func(t *testing.T) {
		retrieved, err := db.GetSession("nonexistent")
		assert.Error(t, err)
		assert.Nil(t, retrieved)
	})

	t.Run("update session", func(t *testing.T) {
		session.Name = "Updated Name"
		session.Lifecycle = "archived"
		session.UpdatedAt = time.Now()

		err := db.UpdateSession(session)
		assert.NoError(t, err)

		retrieved, err := db.GetSession("test-session-1")
		require.NoError(t, err)
		assert.Equal(t, "Updated Name", retrieved.Name)
		assert.Equal(t, "archived", retrieved.Lifecycle)
	})

	t.Run("update nonexistent session", func(t *testing.T) {
		nonexistent := createTestSession("nonexistent")
		err := db.UpdateSession(nonexistent)
		assert.Error(t, err)
	})

	t.Run("list sessions", func(t *testing.T) {
		// Create a few more sessions
		session2 := createTestSession("test-session-2")
		session2.Harness = "gemini-cli"
		err := db.CreateSession(session2)
		require.NoError(t, err)

		session3 := createTestSession("test-session-3")
		session3.Lifecycle = "archived"
		err = db.CreateSession(session3)
		require.NoError(t, err)

		// List all sessions
		sessions, err := db.ListSessions(nil)
		require.NoError(t, err)
		assert.Len(t, sessions, 3)

		// Filter by lifecycle
		filter := &SessionFilter{Lifecycle: "archived"}
		sessions, err = db.ListSessions(filter)
		require.NoError(t, err)
		assert.Len(t, sessions, 2) // session-1 and session-3

		// Filter by agent
		filter = &SessionFilter{Harness: "gemini-cli"}
		sessions, err = db.ListSessions(filter)
		require.NoError(t, err)
		assert.Len(t, sessions, 1)
		assert.Equal(t, "gemini-cli", sessions[0].Harness)

		// Test limit
		filter = &SessionFilter{Limit: 2}
		sessions, err = db.ListSessions(filter)
		require.NoError(t, err)
		assert.Len(t, sessions, 2)
	})

	t.Run("delete session", func(t *testing.T) {
		err := db.DeleteSession("test-session-1")
		assert.NoError(t, err)

		// Verify deletion
		retrieved, err := db.GetSession("test-session-1")
		assert.Error(t, err)
		assert.Nil(t, retrieved)
	})

	t.Run("delete nonexistent session", func(t *testing.T) {
		err := db.DeleteSession("nonexistent")
		assert.Error(t, err)
	})
}

// TestSessionWithoutEngramMetadata tests sessions without engram metadata
func TestSessionWithoutEngramMetadata(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	session := createTestSession("no-engram-session")
	session.EngramMetadata = nil

	err := db.CreateSession(session)
	require.NoError(t, err)

	retrieved, err := db.GetSession("no-engram-session")
	require.NoError(t, err)
	assert.Nil(t, retrieved.EngramMetadata)
}

// createTestMessage creates a test message for use in tests
func createTestMessage(role, harness string) *conversation.Message {
	return &conversation.Message{
		Timestamp: time.Now(),
		Role:      role,
		Harness:   harness,
		Content: []conversation.ContentBlock{
			conversation.TextBlock{
				Type: "text",
				Text: "This is a test message",
			},
		},
		Usage: &conversation.TokenUsage{
			InputTokens:  100,
			OutputTokens: 200,
		},
	}
}

// TestMessageCRUD tests message CRUD operations
func TestMessageCRUD(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create a test session first
	session := createTestSession("msg-test-session")
	err := db.CreateSession(session)
	require.NoError(t, err)

	t.Run("create message", func(t *testing.T) {
		msg := createTestMessage("user", "claude-code")
		err := db.CreateMessage("msg-test-session", msg)
		assert.NoError(t, err)
	})

	t.Run("create message with empty session fails", func(t *testing.T) {
		msg := createTestMessage("user", "claude-code")
		err := db.CreateMessage("", msg)
		assert.Error(t, err)
	})

	t.Run("create message with nil message fails", func(t *testing.T) {
		err := db.CreateMessage("msg-test-session", nil)
		assert.Error(t, err)
	})

	t.Run("get messages", func(t *testing.T) {
		// Add a few more messages
		msg2 := createTestMessage("assistant", "claude-code")
		err := db.CreateMessage("msg-test-session", msg2)
		require.NoError(t, err)

		msg3 := createTestMessage("user", "gemini-cli")
		err = db.CreateMessage("msg-test-session", msg3)
		require.NoError(t, err)

		// Get all messages
		messages, err := db.GetMessages("msg-test-session", nil)
		require.NoError(t, err)
		assert.Len(t, messages, 3)

		// Verify content unmarshaling
		assert.NotNil(t, messages[0].Content)
		assert.Len(t, messages[0].Content, 1)
		textBlock, ok := messages[0].Content[0].(conversation.TextBlock)
		assert.True(t, ok)
		assert.Equal(t, "This is a test message", textBlock.Text)

		// Verify token usage
		assert.NotNil(t, messages[0].Usage)
		assert.Equal(t, 100, messages[0].Usage.InputTokens)
		assert.Equal(t, 200, messages[0].Usage.OutputTokens)
	})

	t.Run("filter messages by role", func(t *testing.T) {
		opts := &MessageOptions{Role: "user"}
		messages, err := db.GetMessages("msg-test-session", opts)
		require.NoError(t, err)
		assert.Len(t, messages, 2)
		for _, msg := range messages {
			assert.Equal(t, "user", msg.Role)
		}
	})

	t.Run("filter messages by agent", func(t *testing.T) {
		opts := &MessageOptions{Harness: "gemini-cli"}
		messages, err := db.GetMessages("msg-test-session", opts)
		require.NoError(t, err)
		assert.Len(t, messages, 1)
		assert.Equal(t, "gemini-cli", messages[0].Harness)
	})

	t.Run("filter messages by time", func(t *testing.T) {
		now := time.Now()
		opts := &MessageOptions{
			After: now.Add(-1 * time.Hour),
		}
		messages, err := db.GetMessages("msg-test-session", opts)
		require.NoError(t, err)
		assert.Len(t, messages, 3)
	})

	t.Run("limit messages", func(t *testing.T) {
		opts := &MessageOptions{Limit: 2}
		messages, err := db.GetMessages("msg-test-session", opts)
		require.NoError(t, err)
		assert.Len(t, messages, 2)
	})

	t.Run("delete messages", func(t *testing.T) {
		err := db.DeleteMessages("msg-test-session")
		assert.NoError(t, err)

		// Verify deletion
		messages, err := db.GetMessages("msg-test-session", nil)
		require.NoError(t, err)
		assert.Len(t, messages, 0)
	})

	t.Run("delete messages for empty session fails", func(t *testing.T) {
		err := db.DeleteMessages("")
		assert.Error(t, err)
	})
}

// TestMessageWithComplexContent tests messages with various content types
func TestMessageWithComplexContent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	session := createTestSession("complex-msg-session")
	err := db.CreateSession(session)
	require.NoError(t, err)

	t.Run("message with multiple content blocks", func(t *testing.T) {
		msg := &conversation.Message{
			Timestamp: time.Now(),
			Role:      "assistant",
			Harness:   "claude-code",
			Content: []conversation.ContentBlock{
				conversation.TextBlock{
					Type: "text",
					Text: "Here's an image:",
				},
				conversation.ImageBlock{
					Type: "image",
					Source: struct {
						Type      string `json:"type"`
						MediaType string `json:"media_type"`
						Data      string `json:"data,omitempty"`
						URL       string `json:"url,omitempty"`
					}{
						Type:      "url",
						MediaType: "image/png",
						URL:       "https://example.com/image.png",
					},
				},
			},
		}

		err := db.CreateMessage("complex-msg-session", msg)
		require.NoError(t, err)

		messages, err := db.GetMessages("complex-msg-session", nil)
		require.NoError(t, err)
		assert.Len(t, messages, 1)
		assert.Len(t, messages[0].Content, 2)

		// Verify text block
		textBlock, ok := messages[0].Content[0].(conversation.TextBlock)
		assert.True(t, ok)
		assert.Equal(t, "Here's an image:", textBlock.Text)

		// Verify image block
		imgBlock, ok := messages[0].Content[1].(conversation.ImageBlock)
		assert.True(t, ok)
		assert.Equal(t, "image", imgBlock.Type)
		assert.Equal(t, "url", imgBlock.Source.Type)
	})
}

// createTestEscalation creates a test escalation event
func createTestEscalation(eventType string) *EscalationEvent {
	return &EscalationEvent{
		Type:        eventType,
		Pattern:     `(?i)error:`,
		Line:        "Error: something went wrong",
		LineNumber:  42,
		DetectedAt:  time.Now(),
		Description: "Generic error message",
	}
}

// TestEscalationCRUD tests escalation CRUD operations
func TestEscalationCRUD(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create a test session first
	session := createTestSession("esc-test-session")
	err := db.CreateSession(session)
	require.NoError(t, err)

	t.Run("create escalation", func(t *testing.T) {
		escalation := createTestEscalation("error")
		id, err := db.CreateEscalation("esc-test-session", escalation)
		assert.NoError(t, err)
		assert.Greater(t, id, int64(0))
	})

	t.Run("create escalation with empty session fails", func(t *testing.T) {
		escalation := createTestEscalation("error")
		_, err := db.CreateEscalation("", escalation)
		assert.Error(t, err)
	})

	t.Run("create escalation with nil event fails", func(t *testing.T) {
		_, err := db.CreateEscalation("esc-test-session", nil)
		assert.Error(t, err)
	})

	t.Run("get escalations", func(t *testing.T) {
		// Add a few more escalations
		escalation2 := createTestEscalation("warning")
		_, err := db.CreateEscalation("esc-test-session", escalation2)
		require.NoError(t, err)

		escalation3 := createTestEscalation("prompt")
		_, err = db.CreateEscalation("esc-test-session", escalation3)
		require.NoError(t, err)

		// Get all escalations
		escalations, err := db.GetEscalations("esc-test-session")
		require.NoError(t, err)
		assert.Len(t, escalations, 3)

		// Verify fields
		assert.Equal(t, "prompt", escalations[0].Type) // Most recent first
		assert.Equal(t, `(?i)error:`, escalations[0].Pattern)
		assert.Equal(t, "Error: something went wrong", escalations[0].Line)
		assert.Equal(t, 42, escalations[0].LineNumber)
		assert.Equal(t, "Generic error message", escalations[0].Description)
	})

	t.Run("get unresolved escalations", func(t *testing.T) {
		escalations, err := db.GetUnresolvedEscalations("esc-test-session")
		require.NoError(t, err)
		assert.Len(t, escalations, 3) // All unresolved initially
	})

	t.Run("resolve escalation", func(t *testing.T) {
		// Get the first escalation
		escalations, err := db.GetEscalations("esc-test-session")
		require.NoError(t, err)
		require.NotEmpty(t, escalations)

		// Resolve it (we need to get the ID from the database)
		// For this test, we'll use ID 1 since it's the first inserted
		err = db.ResolveEscalation(1, "Fixed the issue")
		assert.NoError(t, err)

		// Verify unresolved count decreased
		unresolved, err := db.GetUnresolvedEscalations("esc-test-session")
		require.NoError(t, err)
		assert.Len(t, unresolved, 2)
	})

	t.Run("resolve nonexistent escalation", func(t *testing.T) {
		err := db.ResolveEscalation(99999, "note")
		assert.Error(t, err)
	})

	t.Run("delete escalations", func(t *testing.T) {
		err := db.DeleteEscalations("esc-test-session")
		assert.NoError(t, err)

		// Verify deletion
		escalations, err := db.GetEscalations("esc-test-session")
		require.NoError(t, err)
		assert.Len(t, escalations, 0)
	})
}

// TestCascadeDelete tests that deleting a session cascades to messages and escalations
func TestCascadeDelete(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create session with messages and escalations
	session := createTestSession("cascade-test")
	err := db.CreateSession(session)
	require.NoError(t, err)

	msg := createTestMessage("user", "claude-code")
	err = db.CreateMessage("cascade-test", msg)
	require.NoError(t, err)

	escalation := createTestEscalation("error")
	_, err = db.CreateEscalation("cascade-test", escalation)
	require.NoError(t, err)

	// Delete the session
	err = db.DeleteSession("cascade-test")
	require.NoError(t, err)

	// Verify messages were deleted
	messages, err := db.GetMessages("cascade-test", nil)
	require.NoError(t, err)
	assert.Len(t, messages, 0)

	// Verify escalations were deleted
	escalations, err := db.GetEscalations("cascade-test")
	require.NoError(t, err)
	assert.Len(t, escalations, 0)
}

// TestErrorCases tests various error conditions
func TestErrorCases(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	t.Run("create session with empty ID fails", func(t *testing.T) {
		session := createTestSession("")
		err := db.CreateSession(session)
		assert.Error(t, err)
	})

	t.Run("create session with nil fails", func(t *testing.T) {
		err := db.CreateSession(nil)
		assert.Error(t, err)
	})

	t.Run("get session with empty ID fails", func(t *testing.T) {
		_, err := db.GetSession("")
		assert.Error(t, err)
	})

	t.Run("update session with empty ID fails", func(t *testing.T) {
		session := createTestSession("")
		err := db.UpdateSession(session)
		assert.Error(t, err)
	})

	t.Run("delete session with empty ID fails", func(t *testing.T) {
		err := db.DeleteSession("")
		assert.Error(t, err)
	})

	t.Run("create message with empty role fails", func(t *testing.T) {
		session := createTestSession("err-test")
		err := db.CreateSession(session)
		require.NoError(t, err)

		msg := createTestMessage("", "claude-code")
		err = db.CreateMessage("err-test", msg)
		assert.Error(t, err)
	})

	t.Run("create message with empty agent fails", func(t *testing.T) {
		msg := createTestMessage("user", "")
		err := db.CreateMessage("err-test", msg)
		assert.Error(t, err)
	})

	t.Run("get messages with empty session fails", func(t *testing.T) {
		_, err := db.GetMessages("", nil)
		assert.Error(t, err)
	})

	t.Run("create escalation with empty type fails", func(t *testing.T) {
		escalation := createTestEscalation("")
		_, err := db.CreateEscalation("err-test", escalation)
		assert.Error(t, err)
	})

	t.Run("create escalation with empty pattern fails", func(t *testing.T) {
		escalation := createTestEscalation("error")
		escalation.Pattern = ""
		_, err := db.CreateEscalation("err-test", escalation)
		assert.Error(t, err)
	})

	t.Run("get escalations with empty session fails", func(t *testing.T) {
		_, err := db.GetEscalations("")
		assert.Error(t, err)
	})

	t.Run("resolve escalation with invalid ID fails", func(t *testing.T) {
		err := db.ResolveEscalation(0, "note")
		assert.Error(t, err)
	})
}
