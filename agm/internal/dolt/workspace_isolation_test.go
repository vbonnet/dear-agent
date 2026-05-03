package dolt

import (
	"os"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// TestWorkspaceIsolation verifies zero cross-contamination between workspaces
// This is a critical security/privacy requirement for multi-workspace AGM
func TestWorkspaceIsolation(t *testing.T) {
	if os.Getenv("DOLT_TEST_INTEGRATION") == "" {
		t.Skip("Skipping integration test (set DOLT_TEST_INTEGRATION=1 to enable)")
	}

	// Initialize lookupEnv for testing
	lookupEnv = os.LookupEnv

	// Test Setup: Create two separate workspace adapters
	// In production, these would connect to separate Dolt instances
	// OSS workspace: port 3307, Acme Corp workspace: port 3308

	// OSS Workspace Adapter
	t.Setenv("WORKSPACE", "testoss")
	t.Setenv("DOLT_PORT", "3307")
	os.Unsetenv("DOLT_DATABASE") // Let it default to workspace name

	ossConfig, err := DefaultConfig()
	if err != nil {
		t.Fatalf("Failed to create OSS config: %v", err)
	}

	ossAdapter, err := New(ossConfig)
	if err != nil {
		t.Fatalf("Failed to create OSS adapter: %v", err)
	}
	defer ossAdapter.Close()

	if err := ossAdapter.ApplyMigrations(); err != nil {
		t.Fatalf("Failed to apply OSS migrations: %v", err)
	}

	// Acme Corp Workspace Adapter
	// Note: For testing, we use the same Dolt server (3307) but different database name
	// In production, workspaces would use separate Dolt instances
	t.Setenv("WORKSPACE", "testacme")
	t.Setenv("DOLT_PORT", "3307") // Same server, different workspace database
	os.Unsetenv("DOLT_DATABASE")   // Let it default to workspace name

	acmeConfig, err := DefaultConfig()
	if err != nil {
		t.Fatalf("Failed to create Acme Corp config: %v", err)
	}

	acmeAdapter, err := New(acmeConfig)
	if err != nil {
		t.Fatalf("Failed to create Acme Corp adapter: %v", err)
	}
	defer acmeAdapter.Close()

	if err := acmeAdapter.ApplyMigrations(); err != nil {
		t.Fatalf("Failed to apply Acme Corp migrations: %v", err)
	}

	// Test 1: Verify workspace names are correctly set
	t.Run("WorkspaceNames", func(t *testing.T) {
		if ossAdapter.Workspace() != "testoss" {
			t.Errorf("Expected OSS workspace name 'testoss', got '%s'", ossAdapter.Workspace())
		}
		if acmeAdapter.Workspace() != "testacme" {
			t.Errorf("Expected Acme Corp workspace name 'testacme', got '%s'", acmeAdapter.Workspace())
		}
	})

	// Test 2: Create sessions in both workspaces with overlapping session IDs
	// This tests that session IDs can be reused across workspaces without conflict
	sessionID := "test-isolation-session-" + time.Now().Format("20060102-150405")

	ossSession := &manifest.Manifest{
		SessionID:     sessionID,
		Name:          "OSS Session",
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Harness:       "claude-code",
		Context: manifest.Context{
			Project: "/testoss/project",
			Purpose: "OSS Development",
			Tags:    []string{"oss", "public"},
		},
		Claude: manifest.Claude{
			UUID: "oss-uuid-123",
		},
		Tmux: manifest.Tmux{
			SessionName: "oss-tmux",
		},
	}

	acmeSession := &manifest.Manifest{
		SessionID:     sessionID, // Same ID as OSS session
		Name:          "Acme Session",
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Harness:       "claude-code",
		Context: manifest.Context{
			Project: "/testacme/confidential",
			Purpose: "Acme Confidential Work",
			Tags:    []string{"acme", "confidential"},
		},
		Claude: manifest.Claude{
			UUID: "acme-uuid-456",
		},
		Tmux: manifest.Tmux{
			SessionName: "acme-tmux",
		},
	}

	t.Run("CreateSessions", func(t *testing.T) {
		if err := ossAdapter.CreateSession(ossSession); err != nil {
			t.Fatalf("Failed to create OSS session: %v", err)
		}

		if err := acmeAdapter.CreateSession(acmeSession); err != nil {
			t.Fatalf("Failed to create Acme Corp session: %v", err)
		}
	})

	// Cleanup sessions after tests
	defer func() {
		ossAdapter.DeleteSession(sessionID)
		acmeAdapter.DeleteSession(sessionID)
	}()

	// Test 3: Verify session retrieval respects workspace boundaries
	t.Run("SessionIsolation", func(t *testing.T) {
		// Get OSS session
		ossRetrieved, err := ossAdapter.GetSession(sessionID)
		if err != nil {
			t.Fatalf("Failed to retrieve OSS session: %v", err)
		}

		// Verify OSS session data
		if ossRetrieved.Name != "OSS Session" {
			t.Errorf("Expected OSS session name 'OSS Session', got '%s'", ossRetrieved.Name)
		}
		if ossRetrieved.Context.Project != "/testoss/project" {
			t.Errorf("Expected OSS project '/oss/project', got '%s'", ossRetrieved.Context.Project)
		}
		if ossRetrieved.Claude.UUID != "oss-uuid-123" {
			t.Errorf("Expected OSS UUID 'oss-uuid-123', got '%s'", ossRetrieved.Claude.UUID)
		}

		// Get Acme Corp session
		acmeRetrieved, err := acmeAdapter.GetSession(sessionID)
		if err != nil {
			t.Fatalf("Failed to retrieve Acme Corp session: %v", err)
		}

		// Verify Acme Corp session data
		if acmeRetrieved.Name != "Acme Session" {
			t.Errorf("Expected Acme Corp session name 'Acme Session', got '%s'", acmeRetrieved.Name)
		}
		if acmeRetrieved.Context.Project != "/testacme/confidential" {
			t.Errorf("Expected Acme Corp project '/acme/confidential', got '%s'", acmeRetrieved.Context.Project)
		}
		if acmeRetrieved.Claude.UUID != "acme-uuid-456" {
			t.Errorf("Expected Acme Corp UUID 'acme-uuid-456', got '%s'", acmeRetrieved.Claude.UUID)
		}

		// CRITICAL: Verify no cross-contamination
		if ossRetrieved.Context.Project == acmeRetrieved.Context.Project {
			t.Error("SECURITY VIOLATION: OSS and Acme Corp sessions have same project path")
		}
		if ossRetrieved.Claude.UUID == acmeRetrieved.Claude.UUID {
			t.Error("SECURITY VIOLATION: OSS and Acme Corp sessions have same UUID")
		}
	})

	// Test 4: Create messages in both workspaces
	t.Run("MessageIsolation", func(t *testing.T) {
		// OSS message
		ossMsg := &Message{
			SessionID:      sessionID,
			Role:           "user",
			Content:        `[{"type":"text","text":"OSS public message"}]`,
			SequenceNumber: 0,
			Harness:        "claude-code",
		}

		if err := ossAdapter.CreateMessage(ossMsg); err != nil {
			t.Fatalf("Failed to create OSS message: %v", err)
		}

		// Acme Corp message
		acmeMsg := &Message{
			SessionID:      sessionID,
			Role:           "user",
			Content:        `[{"type":"text","text":"Acme confidential message"}]`,
			SequenceNumber: 0,
			Harness:        "claude-code",
		}

		if err := acmeAdapter.CreateMessage(acmeMsg); err != nil {
			t.Fatalf("Failed to create Acme Corp message: %v", err)
		}

		// Retrieve messages from each workspace
		ossMessages, err := ossAdapter.GetSessionMessages(sessionID)
		if err != nil {
			t.Fatalf("Failed to get OSS messages: %v", err)
		}

		acmeMessages, err := acmeAdapter.GetSessionMessages(sessionID)
		if err != nil {
			t.Fatalf("Failed to get Acme Corp messages: %v", err)
		}

		// Verify message counts
		if len(ossMessages) != 1 {
			t.Errorf("Expected 1 OSS message, got %d", len(ossMessages))
		}
		if len(acmeMessages) != 1 {
			t.Errorf("Expected 1 Acme Corp message, got %d", len(acmeMessages))
		}

		// Verify message content isolation
		if len(ossMessages) > 0 && ossMessages[0].Content != `[{"type":"text","text":"OSS public message"}]` {
			t.Error("SECURITY VIOLATION: OSS message content corrupted or leaked")
		}
		if len(acmeMessages) > 0 && acmeMessages[0].Content != `[{"type":"text","text":"Acme confidential message"}]` {
			t.Error("SECURITY VIOLATION: Acme Corp message content corrupted or leaked")
		}
	})

	// Test 5: List sessions - verify each workspace only sees its own sessions
	t.Run("ListSessionsIsolation", func(t *testing.T) {
		// List OSS sessions
		ossSessions, err := ossAdapter.ListSessions(&SessionFilter{})
		if err != nil {
			t.Fatalf("Failed to list OSS sessions: %v", err)
		}

		// List Acme Corp sessions
		acmeSessions, err := acmeAdapter.ListSessions(&SessionFilter{})
		if err != nil {
			t.Fatalf("Failed to list Acme Corp sessions: %v", err)
		}

		// Verify OSS workspace only sees OSS sessions
		for _, session := range ossSessions {
			if session.Workspace != "testoss" {
				t.Errorf("SECURITY VIOLATION: OSS workspace sees non-OSS session: %s (workspace: %s)",
					session.SessionID, session.Workspace)
			}
			if session.Context.Project == "/testacme/confidential" {
				t.Error("SECURITY VIOLATION: OSS workspace sees Acme Corp confidential data")
			}
		}

		// Verify Acme Corp workspace only sees Acme Corp sessions
		for _, session := range acmeSessions {
			if session.Workspace != "testacme" {
				t.Errorf("SECURITY VIOLATION: Acme Corp workspace sees non-Acme session: %s (workspace: %s)",
					session.SessionID, session.Workspace)
			}
			if session.Context.Project == "/testoss/project" {
				t.Error("SECURITY VIOLATION: Acme Corp workspace sees OSS data")
			}
		}

		// Log session counts for verification
		t.Logf("OSS workspace has %d sessions", len(ossSessions))
		t.Logf("Acme workspace has %d sessions", len(acmeSessions))
	})

	// Test 6: Tool call isolation
	t.Run("ToolCallIsolation", func(t *testing.T) {
		// Create messages to attach tool calls
		ossToolMsg := &Message{
			SessionID:      sessionID,
			Role:           "assistant",
			Content:        `[{"type":"text","text":"Using OSS tools"}]`,
			SequenceNumber: 1,
		}
		if err := ossAdapter.CreateMessage(ossToolMsg); err != nil {
			t.Fatalf("Failed to create OSS tool message: %v", err)
		}

		acmeToolMsg := &Message{
			SessionID:      sessionID,
			Role:           "assistant",
			Content:        `[{"type":"text","text":"Using Acme Corp tools"}]`,
			SequenceNumber: 1,
		}
		if err := acmeAdapter.CreateMessage(acmeToolMsg); err != nil {
			t.Fatalf("Failed to create Acme Corp tool message: %v", err)
		}

		// Create tool calls
		ossToolCall := &ToolCall{
			MessageID:       ossToolMsg.ID,
			SessionID:       sessionID,
			ToolName:        "read_file",
			Arguments:       map[string]any{"path": "/oss/public/file.txt"},
			Result:          map[string]any{"content": "public content"},
			ExecutionTimeMs: 100,
		}
		if err := ossAdapter.CreateToolCall(ossToolCall); err != nil {
			t.Fatalf("Failed to create OSS tool call: %v", err)
		}

		acmeToolCall := &ToolCall{
			MessageID:       acmeToolMsg.ID,
			SessionID:       sessionID,
			ToolName:        "read_file",
			Arguments:       map[string]any{"path": "/acme/confidential/secret.txt"},
			Result:          map[string]any{"content": "confidential content"},
			ExecutionTimeMs: 100,
		}
		if err := acmeAdapter.CreateToolCall(acmeToolCall); err != nil {
			t.Fatalf("Failed to create Acme Corp tool call: %v", err)
		}

		// Retrieve tool calls
		ossToolCalls, err := ossAdapter.GetSessionToolCalls(sessionID)
		if err != nil {
			t.Fatalf("Failed to get OSS tool calls: %v", err)
		}

		acmeToolCalls, err := acmeAdapter.GetSessionToolCalls(sessionID)
		if err != nil {
			t.Fatalf("Failed to get Acme Corp tool calls: %v", err)
		}

		// Verify tool call isolation
		if len(ossToolCalls) != 1 {
			t.Errorf("Expected 1 OSS tool call, got %d", len(ossToolCalls))
		}
		if len(acmeToolCalls) != 1 {
			t.Errorf("Expected 1 Acme Corp tool call, got %d", len(acmeToolCalls))
		}

		// Verify tool call content
		if len(ossToolCalls) > 0 {
			if path, ok := ossToolCalls[0].Arguments["path"].(string); ok {
				if path != "/oss/public/file.txt" {
					t.Error("SECURITY VIOLATION: OSS tool call path corrupted")
				}
			}
		}

		if len(acmeToolCalls) > 0 {
			if path, ok := acmeToolCalls[0].Arguments["path"].(string); ok {
				if path != "/acme/confidential/secret.txt" {
					t.Error("SECURITY VIOLATION: Acme Corp tool call path corrupted")
				}
			}
		}
	})

	// Test 7: Update operations respect workspace boundaries
	t.Run("UpdateIsolation", func(t *testing.T) {
		// Attempt to update OSS session via OSS adapter
		ossSession.Name = "OSS Session Updated"
		if err := ossAdapter.UpdateSession(ossSession); err != nil {
			t.Errorf("Failed to update OSS session via OSS adapter: %v", err)
		}

		// Verify update succeeded
		updated, err := ossAdapter.GetSession(sessionID)
		if err != nil {
			t.Fatalf("Failed to retrieve updated OSS session: %v", err)
		}
		if updated.Name != "OSS Session Updated" {
			t.Error("OSS session update failed")
		}

		// Verify Acme Corp session remains unchanged
		acmeCheck, err := acmeAdapter.GetSession(sessionID)
		if err != nil {
			t.Fatalf("Failed to retrieve Acme Corp session: %v", err)
		}
		if acmeCheck.Name != "Acme Session" {
			t.Error("SECURITY VIOLATION: Acme Corp session was modified by OSS update")
		}
	})

	// Test 8: Delete operations respect workspace boundaries
	t.Run("DeleteIsolation", func(t *testing.T) {
		// Create additional session for delete test
		deleteTestID := "test-delete-isolation-" + time.Now().Format("20060102-150405")

		ossDeleteSession := &manifest.Manifest{
			SessionID:     deleteTestID,
			Name:          "OSS Delete Test",
			SchemaVersion: "2.0",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Harness:       "claude-code",
			Context: manifest.Context{
				Project: "/oss/delete-test",
			},
			Claude: manifest.Claude{
				UUID: "oss-delete-uuid",
			},
			Tmux: manifest.Tmux{
				SessionName: "oss-delete-tmux",
			},
		}

		acmeDeleteSession := &manifest.Manifest{
			SessionID:     deleteTestID,
			Name:          "Acme Delete Test",
			SchemaVersion: "2.0",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Harness:       "claude-code",
			Context: manifest.Context{
				Project: "/acme/delete-test",
			},
			Claude: manifest.Claude{
				UUID: "acme-delete-uuid",
			},
			Tmux: manifest.Tmux{
				SessionName: "acme-delete-tmux",
			},
		}

		// Create sessions
		if err := ossAdapter.CreateSession(ossDeleteSession); err != nil {
			t.Fatalf("Failed to create OSS delete test session: %v", err)
		}
		if err := acmeAdapter.CreateSession(acmeDeleteSession); err != nil {
			t.Fatalf("Failed to create Acme Corp delete test session: %v", err)
		}

		// Delete OSS session
		if err := ossAdapter.DeleteSession(deleteTestID); err != nil {
			t.Errorf("Failed to delete OSS session: %v", err)
		}

		// Verify OSS session is deleted
		_, err := ossAdapter.GetSession(deleteTestID)
		if err == nil {
			t.Error("OSS session still exists after deletion")
		}

		// Verify Acme Corp session still exists
		acmeStillExists, err := acmeAdapter.GetSession(deleteTestID)
		if err != nil {
			t.Error("SECURITY VIOLATION: Acme Corp session was deleted by OSS delete operation")
		}
		if acmeStillExists == nil {
			t.Error("SECURITY VIOLATION: Acme Corp session is nil after OSS delete")
		} else if acmeStillExists.Name != "Acme Delete Test" {
			t.Error("SECURITY VIOLATION: Acme Corp session corrupted after OSS delete")
		}

		// Cleanup
		acmeAdapter.DeleteSession(deleteTestID)
	})
}

// BenchmarkWorkspaceQueries measures query performance for workspace-isolated operations
// Target: <10ms per operation (acceptable vs SQLite ~1ms)
func BenchmarkWorkspaceQueries(b *testing.B) {
	if os.Getenv("DOLT_TEST_INTEGRATION") == "" {
		b.Skip("Skipping benchmark (set DOLT_TEST_INTEGRATION=1 to enable)")
	}

	// Setup
	lookupEnv = os.LookupEnv
	b.Setenv("WORKSPACE", "benchmark")
	b.Setenv("DOLT_PORT", "3307")

	config, err := DefaultConfig()
	if err != nil {
		b.Fatalf("Failed to create config: %v", err)
	}

	adapter, err := New(config)
	if err != nil {
		b.Fatalf("Failed to create adapter: %v", err)
	}
	defer adapter.Close()

	if err := adapter.ApplyMigrations(); err != nil {
		b.Fatalf("Failed to apply migrations: %v", err)
	}

	// Create test session
	session := &manifest.Manifest{
		SessionID:     "benchmark-session",
		Name:          "Benchmark Session",
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Harness:       "claude-code",
		Context: manifest.Context{
			Project: "/benchmark/project",
		},
		Claude: manifest.Claude{
			UUID: "benchmark-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: "benchmark-tmux",
		},
	}

	if err := adapter.CreateSession(session); err != nil {
		b.Fatalf("Failed to create benchmark session: %v", err)
	}
	defer adapter.DeleteSession("benchmark-session")

	// Benchmark GetSession with workspace filter
	b.Run("GetSession", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := adapter.GetSession("benchmark-session")
			if err != nil {
				b.Fatalf("GetSession failed: %v", err)
			}
		}
	})

	// Benchmark ListSessions with workspace filter
	b.Run("ListSessions", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := adapter.ListSessions(&SessionFilter{Limit: 10})
			if err != nil {
				b.Fatalf("ListSessions failed: %v", err)
			}
		}
	})

	// Benchmark CreateMessage
	b.Run("CreateMessage", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			msg := &Message{
				SessionID:      "benchmark-session",
				Role:           "user",
				Content:        `[{"type":"text","text":"Benchmark message"}]`,
				SequenceNumber: i,
			}
			if err := adapter.CreateMessage(msg); err != nil {
				b.Fatalf("CreateMessage failed: %v", err)
			}
		}
	})

	// Benchmark GetSessionMessages
	b.Run("GetSessionMessages", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := adapter.GetSessionMessages("benchmark-session")
			if err != nil {
				b.Fatalf("GetSessionMessages failed: %v", err)
			}
		}
	})
}

// TestWorkspaceFilterEdgeCases tests edge cases and error conditions
func TestWorkspaceFilterEdgeCases(t *testing.T) {
	if os.Getenv("DOLT_TEST_INTEGRATION") == "" {
		t.Skip("Skipping integration test (set DOLT_TEST_INTEGRATION=1 to enable)")
	}

	lookupEnv = os.LookupEnv
	t.Setenv("WORKSPACE", "testedgecase")
	t.Setenv("DOLT_PORT", "3307")
	os.Unsetenv("DOLT_DATABASE") // Let it default to workspace name

	config, err := DefaultConfig()
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	adapter, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}
	defer adapter.Close()

	if err := adapter.ApplyMigrations(); err != nil {
		t.Fatalf("Failed to apply migrations: %v", err)
	}

	t.Run("NonExistentSession", func(t *testing.T) {
		_, err := adapter.GetSession("nonexistent-session-id")
		if err == nil {
			t.Error("Expected error for non-existent session")
		}
	})

	t.Run("EmptySessionID", func(t *testing.T) {
		_, err := adapter.GetSession("")
		if err == nil {
			t.Error("Expected error for empty session ID")
		}
	})

	t.Run("ListWithFilters", func(t *testing.T) {
		sessions, err := adapter.ListSessions(&SessionFilter{
			Harness: "claude-code",
			Limit:   5,
			Offset:  0,
		})
		if err != nil {
			t.Errorf("Failed to list sessions with filters: %v", err)
		}
		// Verify all returned sessions are from this workspace
		for _, session := range sessions {
			if session.Workspace != "edgecase" {
				t.Errorf("Filter returned session from wrong workspace: %s", session.Workspace)
			}
		}
	})
}
