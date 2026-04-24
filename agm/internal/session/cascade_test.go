package session

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/testutil"
)

// TestPromptCascadeTermination_NoChildren tests prompt with no children
func TestPromptCascadeTermination_NoChildren(t *testing.T) {
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	parentID := "cascade-test-" + uuid.New().String()[:8]
	defer testutil.CleanupTestSession(t, adapter, parentID)

	parent := newTestManifest(parentID, "test-parent")
	if err := adapter.CreateSession(parent); err != nil {
		t.Fatalf("failed to create parent session: %v", err)
	}

	action, err := PromptCascadeTermination(adapter, parentID)
	if err != nil {
		t.Fatalf("PromptCascadeTermination failed: %v", err)
	}

	if action != CascadeSkip {
		t.Errorf("Expected CascadeSkip for no children, got %s", action)
	}
}

// TestPromptCascadeTermination_NilAdapter tests error handling for nil adapter
func TestPromptCascadeTermination_NilAdapter(t *testing.T) {
	_, err := PromptCascadeTermination(nil, "parent-1")
	if err == nil {
		t.Error("Expected error for nil adapter, got nil")
	}
	if !strings.Contains(err.Error(), "adapter cannot be nil") {
		t.Errorf("Expected 'adapter cannot be nil' error, got: %v", err)
	}
}

// TestPromptCascadeTermination_EmptyParentID tests error handling for empty parent ID
func TestPromptCascadeTermination_EmptyParentID(t *testing.T) {
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	_, err := PromptCascadeTermination(adapter, "")
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
			adapter := testutil.GetTestDoltAdapter(t)
			defer adapter.Close()

			parentID := "cascade-input-" + uuid.New().String()[:8]
			childID := "cascade-input-c-" + uuid.New().String()[:8]
			defer testutil.CleanupTestSession(t, adapter, parentID)
			defer testutil.CleanupTestSession(t, adapter, childID)

			parent := newTestManifest(parentID, "test-parent")
			if err := adapter.CreateSession(parent); err != nil {
				t.Fatalf("failed to create parent session: %v", err)
			}

			createTestChild(adapter, t, childID, parentID, "test-child-1")

			reader := strings.NewReader(tt.input)
			action, err := promptCascadeTerminationWithReader(adapter, parentID, reader)

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
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	parentID := "cascade-term-" + uuid.New().String()[:8]
	childIDs := []string{
		"cascade-term-c1-" + uuid.New().String()[:8],
		"cascade-term-c2-" + uuid.New().String()[:8],
		"cascade-term-c3-" + uuid.New().String()[:8],
	}
	defer testutil.CleanupTestSession(t, adapter, parentID)
	for _, id := range childIDs {
		defer testutil.CleanupTestSession(t, adapter, id)
	}

	parent := newTestManifest(parentID, "test-parent")
	if err := adapter.CreateSession(parent); err != nil {
		t.Fatalf("failed to create parent session: %v", err)
	}

	for i, childID := range childIDs {
		createTestChild(adapter, t, childID, parentID, fmt.Sprintf("test-child-%d", i+1))
	}

	// Verify children are active
	for _, childID := range childIDs {
		verifyChildLifecycle(adapter, t, childID, "")
	}

	// Execute cascade terminate
	err := ExecuteCascadeTermination(adapter, parentID, CascadeTerminate)
	if err != nil {
		t.Fatalf("ExecuteCascadeTermination failed: %v", err)
	}

	// Verify all children are archived
	for _, childID := range childIDs {
		verifyChildLifecycle(adapter, t, childID, "archived")
	}
}

// TestExecuteCascadeTermination_Skip tests leaving children running
func TestExecuteCascadeTermination_Skip(t *testing.T) {
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	parentID := "cascade-skip-" + uuid.New().String()[:8]
	childIDs := []string{
		"cascade-skip-c1-" + uuid.New().String()[:8],
		"cascade-skip-c2-" + uuid.New().String()[:8],
	}
	defer testutil.CleanupTestSession(t, adapter, parentID)
	for _, id := range childIDs {
		defer testutil.CleanupTestSession(t, adapter, id)
	}

	parent := newTestManifest(parentID, "test-parent")
	if err := adapter.CreateSession(parent); err != nil {
		t.Fatalf("failed to create parent session: %v", err)
	}

	for i, childID := range childIDs {
		createTestChild(adapter, t, childID, parentID, fmt.Sprintf("test-child-%d", i+1))
	}

	// Execute cascade skip
	err := ExecuteCascadeTermination(adapter, parentID, CascadeSkip)
	if err != nil {
		t.Fatalf("ExecuteCascadeTermination failed: %v", err)
	}

	// Verify children are still active
	for _, childID := range childIDs {
		verifyChildLifecycle(adapter, t, childID, "")
	}

	// Verify children still have parent
	for _, childID := range childIDs {
		verifyChildParent(adapter, t, childID, parentID)
	}
}

// TestExecuteCascadeTermination_Detach tests detaching children from parent
func TestExecuteCascadeTermination_Detach(t *testing.T) {
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	parentID := "cascade-detach-" + uuid.New().String()[:8]
	childIDs := []string{
		"cascade-detach-c1-" + uuid.New().String()[:8],
		"cascade-detach-c2-" + uuid.New().String()[:8],
	}
	defer testutil.CleanupTestSession(t, adapter, parentID)
	for _, id := range childIDs {
		defer testutil.CleanupTestSession(t, adapter, id)
	}

	parent := newTestManifest(parentID, "test-parent")
	if err := adapter.CreateSession(parent); err != nil {
		t.Fatalf("failed to create parent session: %v", err)
	}

	for i, childID := range childIDs {
		createTestChild(adapter, t, childID, parentID, fmt.Sprintf("test-child-%d", i+1))
	}

	// Verify children have parent
	for _, childID := range childIDs {
		verifyChildParent(adapter, t, childID, parentID)
	}

	// Execute cascade detach
	err := ExecuteCascadeTermination(adapter, parentID, CascadeDetach)
	if err != nil {
		t.Fatalf("ExecuteCascadeTermination failed: %v", err)
	}

	// Verify children are detached (parent_session_id = NULL)
	for _, childID := range childIDs {
		verifyChildParent(adapter, t, childID, "")
	}

	// Verify children are still active
	for _, childID := range childIDs {
		verifyChildLifecycle(adapter, t, childID, "")
	}
}

// TestExecuteCascadeTermination_NoChildren tests with no children
func TestExecuteCascadeTermination_NoChildren(t *testing.T) {
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	parentID := "cascade-nochild-" + uuid.New().String()[:8]
	defer testutil.CleanupTestSession(t, adapter, parentID)

	parent := newTestManifest(parentID, "test-parent")
	if err := adapter.CreateSession(parent); err != nil {
		t.Fatalf("failed to create parent session: %v", err)
	}

	// All actions should succeed with no children
	for _, action := range []CascadeAction{CascadeTerminate, CascadeSkip, CascadeDetach} {
		err := ExecuteCascadeTermination(adapter, parentID, action)
		if err != nil {
			t.Fatalf("ExecuteCascadeTermination(%s) failed: %v", action, err)
		}
	}
}

// TestExecuteCascadeTermination_NilAdapter tests error handling for nil adapter
func TestExecuteCascadeTermination_NilAdapter(t *testing.T) {
	err := ExecuteCascadeTermination(nil, "parent-1", CascadeTerminate)
	if err == nil {
		t.Error("Expected error for nil adapter, got nil")
	}
	if !strings.Contains(err.Error(), "adapter cannot be nil") {
		t.Errorf("Expected 'adapter cannot be nil' error, got: %v", err)
	}
}

// TestExecuteCascadeTermination_EmptyParentID tests error handling for empty parent ID
func TestExecuteCascadeTermination_EmptyParentID(t *testing.T) {
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	err := ExecuteCascadeTermination(adapter, "", CascadeTerminate)
	if err == nil {
		t.Error("Expected error for empty parentID, got nil")
	}
	if !strings.Contains(err.Error(), "parentID cannot be empty") {
		t.Errorf("Expected 'parentID cannot be empty' error, got: %v", err)
	}
}

// TestExecuteCascadeTermination_InvalidAction tests error handling for invalid action
func TestExecuteCascadeTermination_InvalidAction(t *testing.T) {
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	parentID := "cascade-invalid-" + uuid.New().String()[:8]
	childID := "cascade-invalid-c-" + uuid.New().String()[:8]
	defer testutil.CleanupTestSession(t, adapter, parentID)
	defer testutil.CleanupTestSession(t, adapter, childID)

	parent := newTestManifest(parentID, "test-parent")
	if err := adapter.CreateSession(parent); err != nil {
		t.Fatalf("failed to create parent session: %v", err)
	}

	createTestChild(adapter, t, childID, parentID, "test-child-1")

	err := ExecuteCascadeTermination(adapter, parentID, CascadeAction("invalid"))
	if err == nil {
		t.Error("Expected error for invalid action, got nil")
	}
	if !strings.Contains(err.Error(), "invalid cascade action") {
		t.Errorf("Expected 'invalid cascade action' error, got: %v", err)
	}
}

// TestExecuteCascadeTermination_MultipleChildren tests with multiple children
func TestExecuteCascadeTermination_MultipleChildren(t *testing.T) {
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	parentID := "cascade-multi-" + uuid.New().String()[:8]
	defer testutil.CleanupTestSession(t, adapter, parentID)

	parent := newTestManifest(parentID, "test-parent")
	if err := adapter.CreateSession(parent); err != nil {
		t.Fatalf("failed to create parent session: %v", err)
	}

	// Create 5 child sessions
	childIDs := make([]string, 5)
	for i := 0; i < 5; i++ {
		childIDs[i] = fmt.Sprintf("cascade-multi-c%d-%s", i+1, uuid.New().String()[:8])
		defer testutil.CleanupTestSession(t, adapter, childIDs[i])
		createTestChild(adapter, t, childIDs[i], parentID, fmt.Sprintf("test-child-%d", i+1))
	}

	// Test terminate
	err := ExecuteCascadeTermination(adapter, parentID, CascadeTerminate)
	if err != nil {
		t.Fatalf("ExecuteCascadeTermination(terminate) failed: %v", err)
	}

	// Verify all children are archived
	for _, childID := range childIDs {
		verifyChildLifecycle(adapter, t, childID, "archived")
	}

	// Reset children for next test
	for _, childID := range childIDs {
		child, err := adapter.GetSession(childID)
		if err != nil {
			t.Fatalf("failed to get child: %v", err)
		}
		child.Lifecycle = ""
		if err := adapter.UpdateSession(child); err != nil {
			t.Fatalf("failed to reset child: %v", err)
		}
	}

	// Test skip
	err = ExecuteCascadeTermination(adapter, parentID, CascadeSkip)
	if err != nil {
		t.Fatalf("ExecuteCascadeTermination(skip) failed: %v", err)
	}

	// Verify all children are still active
	for _, childID := range childIDs {
		verifyChildLifecycle(adapter, t, childID, "")
	}

	// Test detach
	err = ExecuteCascadeTermination(adapter, parentID, CascadeDetach)
	if err != nil {
		t.Fatalf("ExecuteCascadeTermination(detach) failed: %v", err)
	}

	// Verify all children are detached
	for _, childID := range childIDs {
		verifyChildParent(adapter, t, childID, "")
	}
}

// Helper functions

func newTestManifest(sessionID, tmuxName string) *manifest.Manifest {
	now := time.Now()
	return &manifest.Manifest{
		SessionID:     sessionID,
		Name:          sessionID,
		SchemaVersion: "2.0",
		CreatedAt:     now,
		UpdatedAt:     now,
		Lifecycle:     "",
		IsTest:        true,
		Context: manifest.Context{
			Project: "/tmp/test",
		},
		Tmux: manifest.Tmux{
			SessionName: tmuxName,
		},
	}
}

func createTestChild(adapter *dolt.Adapter, t *testing.T, childID, parentID, tmuxName string) *manifest.Manifest {
	t.Helper()

	child := newTestManifest(childID, tmuxName)
	child.ParentSessionID = &parentID

	if err := adapter.CreateSession(child); err != nil {
		t.Fatalf("failed to create child session: %v", err)
	}

	return child
}

func verifyChildLifecycle(adapter *dolt.Adapter, t *testing.T, childID string, expectedLifecycle string) {
	t.Helper()

	child, err := adapter.GetSession(childID)
	if err != nil {
		t.Fatalf("failed to get child session %s: %v", childID, err)
	}

	if child.Lifecycle != expectedLifecycle {
		t.Errorf("Expected child %s lifecycle to be '%s', got '%s'", childID, expectedLifecycle, child.Lifecycle)
	}
}

func verifyChildParent(adapter *dolt.Adapter, t *testing.T, childID string, expectedParentID string) {
	t.Helper()

	conn := adapter.Conn()
	var parentID sql.NullString
	query := `SELECT parent_session_id FROM agm_sessions WHERE id = ?`
	err := conn.QueryRow(query, childID).Scan(&parentID)
	if err != nil {
		t.Fatalf("failed to get parent_session_id for child %s: %v", childID, err)
	}

	actualParentID := ""
	if parentID.Valid {
		actualParentID = parentID.String
	}

	if actualParentID != expectedParentID {
		t.Errorf("Expected child %s parent to be '%s', got '%s'", childID, expectedParentID, actualParentID)
	}
}
