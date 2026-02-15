package session

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/db"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
)

// TestPromptCascadeTermination_NoChildren tests prompt with no children
func TestPromptCascadeTermination_NoChildren(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Create parent session with no children
	parent := &manifest.Manifest{
		SessionID:     "parent-1",
		Name:          "parent",
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     "",
		Context: manifest.Context{
			Project: "/tmp/test",
		},
		Tmux: manifest.Tmux{
			SessionName: "test-parent",
		},
	}

	if err := database.CreateSession(parent); err != nil {
		t.Fatalf("failed to create parent session: %v", err)
	}

	// Test prompt with no children - should return CascadeSkip immediately
	action, err := PromptCascadeTermination(database, "parent-1")
	if err != nil {
		t.Fatalf("PromptCascadeTermination failed: %v", err)
	}

	if action != CascadeSkip {
		t.Errorf("Expected CascadeSkip for no children, got %s", action)
	}
}

// TestPromptCascadeTermination_NilDatabase tests error handling for nil database
func TestPromptCascadeTermination_NilDatabase(t *testing.T) {
	_, err := PromptCascadeTermination(nil, "parent-1")
	if err == nil {
		t.Error("Expected error for nil database, got nil")
	}
	if !strings.Contains(err.Error(), "database cannot be nil") {
		t.Errorf("Expected 'database cannot be nil' error, got: %v", err)
	}
}

// TestPromptCascadeTermination_EmptyParentID tests error handling for empty parent ID
func TestPromptCascadeTermination_EmptyParentID(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	_, err = PromptCascadeTermination(database, "")
	if err == nil {
		t.Error("Expected error for empty parentID, got nil")
	}
	if !strings.Contains(err.Error(), "parentID cannot be empty") {
		t.Errorf("Expected 'parentID cannot be empty' error, got: %v", err)
	}
}

// TestPromptCascadeTermination_WithInput tests user input variations
func TestPromptCascadeTermination_WithInput(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedAction CascadeAction
		expectError    bool
	}{
		{"empty input (yes)", "\n", CascadeTerminate, false},
		{"y", "y\n", CascadeTerminate, false},
		{"yes", "yes\n", CascadeTerminate, false},
		{"Y (uppercase)", "Y\n", CascadeTerminate, false},
		{"YES (uppercase)", "YES\n", CascadeTerminate, false},
		{"n", "n\n", CascadeSkip, false},
		{"no", "no\n", CascadeSkip, false},
		{"N (uppercase)", "N\n", CascadeSkip, false},
		{"NO (uppercase)", "NO\n", CascadeSkip, false},
		{"keep", "keep\n", CascadeDetach, false},
		{"KEEP (uppercase)", "KEEP\n", CascadeDetach, false},
		{"invalid input", "invalid\n", "", true},
		{"whitespace y", "  y  \n", CascadeTerminate, false},
		{"whitespace keep", "  keep  \n", CascadeDetach, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			database, err := db.Open(":memory:")
			if err != nil {
				t.Fatalf("failed to open database: %v", err)
			}
			defer database.Close()

			// Create parent and child
			parent := &manifest.Manifest{
				SessionID:     "parent-1",
				Name:          "parent",
				SchemaVersion: "2.0",
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
				Lifecycle:     "",
				Context: manifest.Context{
					Project: "/tmp/test",
				},
				Tmux: manifest.Tmux{
					SessionName: "test-parent",
				},
			}

			if err := database.CreateSession(parent); err != nil {
				t.Fatalf("failed to create parent session: %v", err)
			}

			createTestChild(database, t, "child-1", "parent-1", "test-child-1")

			// Create mock input reader
			reader := strings.NewReader(tt.input)

			// Call prompt with mocked input
			action, err := promptCascadeTerminationWithReader(database, "parent-1", reader)

			// Check error expectation
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for input '%s', got nil", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for input '%s', got: %v", tt.input, err)
				}
				if action != tt.expectedAction {
					t.Errorf("Expected action %s for input '%s', got %s", tt.expectedAction, tt.input, action)
				}
			}
		})
	}
}

// TestParseCascadeInput tests the input parsing function directly
func TestParseCascadeInput(t *testing.T) {
	tests := []struct {
		input          string
		expectedAction CascadeAction
		expectError    bool
	}{
		{"", CascadeTerminate, false},
		{"y", CascadeTerminate, false},
		{"yes", CascadeTerminate, false},
		{"n", CascadeSkip, false},
		{"no", CascadeSkip, false},
		{"keep", CascadeDetach, false},
		{"invalid", "", true},
		{"maybe", "", true},
		{"terminate", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			action, err := parseCascadeInput(tt.input)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for input '%s', got nil", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for input '%s', got: %v", tt.input, err)
				}
				if action != tt.expectedAction {
					t.Errorf("Expected action %s for input '%s', got %s", tt.expectedAction, tt.input, action)
				}
			}
		})
	}
}

// TestExecuteCascadeTermination_Terminate tests terminating all children
func TestExecuteCascadeTermination_Terminate(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Create parent session
	parent := &manifest.Manifest{
		SessionID:     "parent-1",
		Name:          "parent",
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     "",
		Context: manifest.Context{
			Project: "/tmp/test",
		},
		Tmux: manifest.Tmux{
			SessionName: "test-parent",
		},
	}

	if err := database.CreateSession(parent); err != nil {
		t.Fatalf("failed to create parent session: %v", err)
	}

	// Create child sessions
	child1 := createTestChild(database, t, "child-1", "parent-1", "test-child-1")
	child2 := createTestChild(database, t, "child-2", "parent-1", "test-child-2")
	child3 := createTestChild(database, t, "child-3", "parent-1", "test-child-3")

	// Verify children are active
	verifyChildLifecycle(database, t, child1.SessionID, "")
	verifyChildLifecycle(database, t, child2.SessionID, "")
	verifyChildLifecycle(database, t, child3.SessionID, "")

	// Execute cascade terminate
	err = ExecuteCascadeTermination(database, "parent-1", CascadeTerminate)
	if err != nil {
		t.Fatalf("ExecuteCascadeTermination failed: %v", err)
	}

	// Verify all children are archived
	verifyChildLifecycle(database, t, child1.SessionID, "archived")
	verifyChildLifecycle(database, t, child2.SessionID, "archived")
	verifyChildLifecycle(database, t, child3.SessionID, "archived")
}

// TestExecuteCascadeTermination_Skip tests leaving children running
func TestExecuteCascadeTermination_Skip(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Create parent session
	parent := &manifest.Manifest{
		SessionID:     "parent-1",
		Name:          "parent",
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     "",
		Context: manifest.Context{
			Project: "/tmp/test",
		},
		Tmux: manifest.Tmux{
			SessionName: "test-parent",
		},
	}

	if err := database.CreateSession(parent); err != nil {
		t.Fatalf("failed to create parent session: %v", err)
	}

	// Create child sessions
	child1 := createTestChild(database, t, "child-1", "parent-1", "test-child-1")
	child2 := createTestChild(database, t, "child-2", "parent-1", "test-child-2")

	// Execute cascade skip
	err = ExecuteCascadeTermination(database, "parent-1", CascadeSkip)
	if err != nil {
		t.Fatalf("ExecuteCascadeTermination failed: %v", err)
	}

	// Verify children are still active
	verifyChildLifecycle(database, t, child1.SessionID, "")
	verifyChildLifecycle(database, t, child2.SessionID, "")

	// Verify children still have parent
	verifyChildParent(database, t, child1.SessionID, "parent-1")
	verifyChildParent(database, t, child2.SessionID, "parent-1")
}

// TestExecuteCascadeTermination_Detach tests detaching children from parent
func TestExecuteCascadeTermination_Detach(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Create parent session
	parent := &manifest.Manifest{
		SessionID:     "parent-1",
		Name:          "parent",
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     "",
		Context: manifest.Context{
			Project: "/tmp/test",
		},
		Tmux: manifest.Tmux{
			SessionName: "test-parent",
		},
	}

	if err := database.CreateSession(parent); err != nil {
		t.Fatalf("failed to create parent session: %v", err)
	}

	// Create child sessions
	child1 := createTestChild(database, t, "child-1", "parent-1", "test-child-1")
	child2 := createTestChild(database, t, "child-2", "parent-1", "test-child-2")

	// Verify children have parent
	verifyChildParent(database, t, child1.SessionID, "parent-1")
	verifyChildParent(database, t, child2.SessionID, "parent-1")

	// Execute cascade detach
	err = ExecuteCascadeTermination(database, "parent-1", CascadeDetach)
	if err != nil {
		t.Fatalf("ExecuteCascadeTermination failed: %v", err)
	}

	// Verify children are detached (parent_session_id = NULL)
	verifyChildParent(database, t, child1.SessionID, "")
	verifyChildParent(database, t, child2.SessionID, "")

	// Verify children are still active
	verifyChildLifecycle(database, t, child1.SessionID, "")
	verifyChildLifecycle(database, t, child2.SessionID, "")
}

// TestExecuteCascadeTermination_NoChildren tests with no children
func TestExecuteCascadeTermination_NoChildren(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Create parent session with no children
	parent := &manifest.Manifest{
		SessionID:     "parent-1",
		Name:          "parent",
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     "",
		Context: manifest.Context{
			Project: "/tmp/test",
		},
		Tmux: manifest.Tmux{
			SessionName: "test-parent",
		},
	}

	if err := database.CreateSession(parent); err != nil {
		t.Fatalf("failed to create parent session: %v", err)
	}

	// Execute cascade terminate with no children - should succeed
	err = ExecuteCascadeTermination(database, "parent-1", CascadeTerminate)
	if err != nil {
		t.Fatalf("ExecuteCascadeTermination failed: %v", err)
	}

	// Execute cascade skip with no children - should succeed
	err = ExecuteCascadeTermination(database, "parent-1", CascadeSkip)
	if err != nil {
		t.Fatalf("ExecuteCascadeTermination failed: %v", err)
	}

	// Execute cascade detach with no children - should succeed
	err = ExecuteCascadeTermination(database, "parent-1", CascadeDetach)
	if err != nil {
		t.Fatalf("ExecuteCascadeTermination failed: %v", err)
	}
}

// TestExecuteCascadeTermination_NilDatabase tests error handling for nil database
func TestExecuteCascadeTermination_NilDatabase(t *testing.T) {
	err := ExecuteCascadeTermination(nil, "parent-1", CascadeTerminate)
	if err == nil {
		t.Error("Expected error for nil database, got nil")
	}
	if !strings.Contains(err.Error(), "database cannot be nil") {
		t.Errorf("Expected 'database cannot be nil' error, got: %v", err)
	}
}

// TestExecuteCascadeTermination_EmptyParentID tests error handling for empty parent ID
func TestExecuteCascadeTermination_EmptyParentID(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	err = ExecuteCascadeTermination(database, "", CascadeTerminate)
	if err == nil {
		t.Error("Expected error for empty parentID, got nil")
	}
	if !strings.Contains(err.Error(), "parentID cannot be empty") {
		t.Errorf("Expected 'parentID cannot be empty' error, got: %v", err)
	}
}

// TestExecuteCascadeTermination_InvalidAction tests error handling for invalid action
func TestExecuteCascadeTermination_InvalidAction(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Create parent session
	parent := &manifest.Manifest{
		SessionID:     "parent-1",
		Name:          "parent",
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     "",
		Context: manifest.Context{
			Project: "/tmp/test",
		},
		Tmux: manifest.Tmux{
			SessionName: "test-parent",
		},
	}

	if err := database.CreateSession(parent); err != nil {
		t.Fatalf("failed to create parent session: %v", err)
	}

	// Create a child
	createTestChild(database, t, "child-1", "parent-1", "test-child-1")

	// Execute with invalid action
	err = ExecuteCascadeTermination(database, "parent-1", CascadeAction("invalid"))
	if err == nil {
		t.Error("Expected error for invalid action, got nil")
	}
	if !strings.Contains(err.Error(), "invalid cascade action") {
		t.Errorf("Expected 'invalid cascade action' error, got: %v", err)
	}
}

// TestExecuteCascadeTermination_MultipleChildren tests with multiple children
func TestExecuteCascadeTermination_MultipleChildren(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Create parent session
	parent := &manifest.Manifest{
		SessionID:     "parent-1",
		Name:          "parent",
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     "",
		Context: manifest.Context{
			Project: "/tmp/test",
		},
		Tmux: manifest.Tmux{
			SessionName: "test-parent",
		},
	}

	if err := database.CreateSession(parent); err != nil {
		t.Fatalf("failed to create parent session: %v", err)
	}

	// Create 5 child sessions
	children := make([]*manifest.Manifest, 5)
	for i := 0; i < 5; i++ {
		childID := fmt.Sprintf("child-%d", i+1)
		tmuxName := fmt.Sprintf("test-child-%d", i+1)
		children[i] = createTestChild(database, t, childID, "parent-1", tmuxName)
	}

	// Test terminate
	err = ExecuteCascadeTermination(database, "parent-1", CascadeTerminate)
	if err != nil {
		t.Fatalf("ExecuteCascadeTermination failed: %v", err)
	}

	// Verify all children are archived
	for _, child := range children {
		verifyChildLifecycle(database, t, child.SessionID, "archived")
	}

	// Reset children for next test
	for _, child := range children {
		child.Lifecycle = ""
		if err := database.UpdateSession(child); err != nil {
			t.Fatalf("failed to reset child: %v", err)
		}
	}

	// Test skip
	err = ExecuteCascadeTermination(database, "parent-1", CascadeSkip)
	if err != nil {
		t.Fatalf("ExecuteCascadeTermination failed: %v", err)
	}

	// Verify all children are still active
	for _, child := range children {
		verifyChildLifecycle(database, t, child.SessionID, "")
	}

	// Test detach
	err = ExecuteCascadeTermination(database, "parent-1", CascadeDetach)
	if err != nil {
		t.Fatalf("ExecuteCascadeTermination failed: %v", err)
	}

	// Verify all children are detached
	for _, child := range children {
		verifyChildParent(database, t, child.SessionID, "")
	}
}

// Helper functions

func createTestChild(database *db.DB, t *testing.T, childID, parentID, tmuxName string) *manifest.Manifest {
	t.Helper()

	child := &manifest.Manifest{
		SessionID:     childID,
		Name:          childID,
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     "",
		Context: manifest.Context{
			Project: "/tmp/test",
		},
		Tmux: manifest.Tmux{
			SessionName: tmuxName,
		},
	}

	if err := database.CreateSession(child); err != nil {
		t.Fatalf("failed to create child session: %v", err)
	}

	// Set parent_session_id using raw SQL
	conn := database.Conn()
	query := `UPDATE sessions SET parent_session_id = ? WHERE session_id = ?`
	_, err := conn.Exec(query, parentID, childID)
	if err != nil {
		t.Fatalf("failed to set parent_session_id: %v", err)
	}

	return child
}

func verifyChildLifecycle(database *db.DB, t *testing.T, childID string, expectedLifecycle string) {
	t.Helper()

	child, err := database.GetSession(childID)
	if err != nil {
		t.Fatalf("failed to get child session %s: %v", childID, err)
	}

	if child.Lifecycle != expectedLifecycle {
		t.Errorf("Expected child %s lifecycle to be '%s', got '%s'", childID, expectedLifecycle, child.Lifecycle)
	}
}

func verifyChildParent(database *db.DB, t *testing.T, childID string, expectedParentID string) {
	t.Helper()

	conn := database.Conn()
	var parentID *string
	query := `SELECT parent_session_id FROM sessions WHERE session_id = ?`
	err := conn.QueryRow(query, childID).Scan(&parentID)
	if err != nil {
		t.Fatalf("failed to get parent_session_id for child %s: %v", childID, err)
	}

	actualParentID := ""
	if parentID != nil {
		actualParentID = *parentID
	}

	if actualParentID != expectedParentID {
		t.Errorf("Expected child %s parent to be '%s', got '%s'", childID, expectedParentID, actualParentID)
	}
}
