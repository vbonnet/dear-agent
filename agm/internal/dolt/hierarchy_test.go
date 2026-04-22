package dolt

import (
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// Integration tests for session hierarchy methods
// Skip if DOLT_TEST_INTEGRATION is not set

func TestGetParent(t *testing.T) {
	adapter := getTestAdapter(t)
	defer adapter.Close()

	timestamp := time.Now().Format("20060102-150405")

	// Test 1: Session with no parent (root session)
	rootSession := &manifest.Manifest{
		SessionID:     "test-root-" + timestamp,
		Name:          "Root Session",
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Harness:       "claude-code",
		Context: manifest.Context{
			Project: "/test/root",
		},
		Claude: manifest.Claude{
			UUID: "root-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: "root-tmux",
		},
	}

	if err := adapter.CreateSession(rootSession); err != nil {
		t.Fatalf("Failed to create root session: %v", err)
	}
	defer adapter.DeleteSession(rootSession.SessionID)

	parent, err := adapter.GetParent(rootSession.SessionID)
	if err != nil {
		t.Fatalf("GetParent failed for root session: %v", err)
	}
	if parent != nil {
		t.Error("Expected nil parent for root session")
	}

	// Test 2: Session with parent
	childSession := &manifest.Manifest{
		SessionID:       "test-child-" + timestamp,
		Name:            "Child Session",
		ParentSessionID: &rootSession.SessionID,
		SchemaVersion:   "2.0",
		CreatedAt:       time.Now().Add(1 * time.Second),
		UpdatedAt:       time.Now().Add(1 * time.Second),
		Harness:         "claude-code",
		Context: manifest.Context{
			Project: "/test/child",
		},
		Claude: manifest.Claude{
			UUID: "child-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: "child-tmux",
		},
	}

	if err := adapter.CreateSession(childSession); err != nil {
		t.Fatalf("Failed to create child session: %v", err)
	}
	defer adapter.DeleteSession(childSession.SessionID)

	parent, err = adapter.GetParent(childSession.SessionID)
	if err != nil {
		t.Fatalf("GetParent failed for child session: %v", err)
	}
	if parent == nil {
		t.Fatal("Expected parent to be returned")
	}
	if parent.SessionID != rootSession.SessionID {
		t.Errorf("Expected parent ID '%s', got '%s'", rootSession.SessionID, parent.SessionID)
	}
	if parent.Name != rootSession.Name {
		t.Errorf("Expected parent name '%s', got '%s'", rootSession.Name, parent.Name)
	}

	// Test 3: Non-existent session
	_, err = adapter.GetParent("non-existent-session")
	if err == nil {
		t.Error("Expected error for non-existent session")
	}

	// Test 4: Empty session ID
	_, err = adapter.GetParent("")
	if err == nil {
		t.Error("Expected error for empty session ID")
	}
}

func TestGetChildren(t *testing.T) {
	adapter := getTestAdapter(t)
	defer adapter.Close()

	timestamp := time.Now().Format("20060102-150405")

	// Create parent session
	parentSession := &manifest.Manifest{
		SessionID:     "test-parent-" + timestamp,
		Name:          "Parent Session",
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Harness:       "claude-code",
		Context: manifest.Context{
			Project: "/test/parent",
		},
		Claude: manifest.Claude{
			UUID: "parent-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: "parent-tmux",
		},
	}

	if err := adapter.CreateSession(parentSession); err != nil {
		t.Fatalf("Failed to create parent session: %v", err)
	}
	defer adapter.DeleteSession(parentSession.SessionID)

	// Test 1: Parent with no children
	children, err := adapter.GetChildren(parentSession.SessionID)
	if err != nil {
		t.Fatalf("GetChildren failed: %v", err)
	}
	if len(children) != 0 {
		t.Errorf("Expected 0 children, got %d", len(children))
	}

	// Test 2: Parent with multiple children
	// Create children with different timestamps to test ordering
	child1 := &manifest.Manifest{
		SessionID:       "test-child1-" + timestamp,
		Name:            "Child 1",
		ParentSessionID: &parentSession.SessionID,
		SchemaVersion:   "2.0",
		CreatedAt:       time.Now().Add(1 * time.Second),
		UpdatedAt:       time.Now().Add(1 * time.Second),
		Harness:         "claude-code",
		Context: manifest.Context{
			Project: "/test/child1",
		},
		Claude: manifest.Claude{
			UUID: "child1-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: "child1-tmux",
		},
	}

	child2 := &manifest.Manifest{
		SessionID:       "test-child2-" + timestamp,
		Name:            "Child 2",
		ParentSessionID: &parentSession.SessionID,
		SchemaVersion:   "2.0",
		CreatedAt:       time.Now().Add(2 * time.Second),
		UpdatedAt:       time.Now().Add(2 * time.Second),
		Harness:         "claude-code",
		Context: manifest.Context{
			Project: "/test/child2",
		},
		Claude: manifest.Claude{
			UUID: "child2-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: "child2-tmux",
		},
	}

	child3 := &manifest.Manifest{
		SessionID:       "test-child3-" + timestamp,
		Name:            "Child 3",
		ParentSessionID: &parentSession.SessionID,
		SchemaVersion:   "2.0",
		CreatedAt:       time.Now().Add(3 * time.Second),
		UpdatedAt:       time.Now().Add(3 * time.Second),
		Harness:         "claude-code",
		Context: manifest.Context{
			Project: "/test/child3",
		},
		Claude: manifest.Claude{
			UUID: "child3-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: "child3-tmux",
		},
	}

	if err := adapter.CreateSession(child1); err != nil {
		t.Fatalf("Failed to create child1: %v", err)
	}
	defer adapter.DeleteSession(child1.SessionID)

	if err := adapter.CreateSession(child2); err != nil {
		t.Fatalf("Failed to create child2: %v", err)
	}
	defer adapter.DeleteSession(child2.SessionID)

	if err := adapter.CreateSession(child3); err != nil {
		t.Fatalf("Failed to create child3: %v", err)
	}
	defer adapter.DeleteSession(child3.SessionID)

	children, err = adapter.GetChildren(parentSession.SessionID)
	if err != nil {
		t.Fatalf("GetChildren failed: %v", err)
	}

	if len(children) != 3 {
		t.Fatalf("Expected 3 children, got %d", len(children))
	}

	// Verify ordering by created_at ASC
	if children[0].Name != "Child 1" {
		t.Errorf("Expected first child to be 'Child 1', got '%s'", children[0].Name)
	}
	if children[1].Name != "Child 2" {
		t.Errorf("Expected second child to be 'Child 2', got '%s'", children[1].Name)
	}
	if children[2].Name != "Child 3" {
		t.Errorf("Expected third child to be 'Child 3', got '%s'", children[2].Name)
	}

	// Test 3: Empty session ID
	_, err = adapter.GetChildren("")
	if err == nil {
		t.Error("Expected error for empty session ID")
	}
}

func TestGetSessionTree(t *testing.T) {
	adapter := getTestAdapter(t)
	defer adapter.Close()

	timestamp := time.Now().Format("20060102-150405")

	// Create a 3-level hierarchy: root -> child -> grandchild
	rootSession := &manifest.Manifest{
		SessionID:     "test-tree-root-" + timestamp,
		Name:          "Tree Root",
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Harness:       "claude-code",
		Context: manifest.Context{
			Project: "/test/tree/root",
		},
		Claude: manifest.Claude{
			UUID: "tree-root-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: "tree-root-tmux",
		},
	}

	if err := adapter.CreateSession(rootSession); err != nil {
		t.Fatalf("Failed to create root session: %v", err)
	}
	defer adapter.DeleteSession(rootSession.SessionID)

	childSession := &manifest.Manifest{
		SessionID:       "test-tree-child-" + timestamp,
		Name:            "Tree Child",
		ParentSessionID: &rootSession.SessionID,
		SchemaVersion:   "2.0",
		CreatedAt:       time.Now().Add(1 * time.Second),
		UpdatedAt:       time.Now().Add(1 * time.Second),
		Harness:         "claude-code",
		Context: manifest.Context{
			Project: "/test/tree/child",
		},
		Claude: manifest.Claude{
			UUID: "tree-child-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: "tree-child-tmux",
		},
	}

	if err := adapter.CreateSession(childSession); err != nil {
		t.Fatalf("Failed to create child session: %v", err)
	}
	defer adapter.DeleteSession(childSession.SessionID)

	grandchildSession := &manifest.Manifest{
		SessionID:       "test-tree-grandchild-" + timestamp,
		Name:            "Tree Grandchild",
		ParentSessionID: &childSession.SessionID,
		SchemaVersion:   "2.0",
		CreatedAt:       time.Now().Add(2 * time.Second),
		UpdatedAt:       time.Now().Add(2 * time.Second),
		Harness:         "claude-code",
		Context: manifest.Context{
			Project: "/test/tree/grandchild",
		},
		Claude: manifest.Claude{
			UUID: "tree-grandchild-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: "tree-grandchild-tmux",
		},
	}

	if err := adapter.CreateSession(grandchildSession); err != nil {
		t.Fatalf("Failed to create grandchild session: %v", err)
	}
	defer adapter.DeleteSession(grandchildSession.SessionID)

	// Test 1: Get tree for root session (depth 0)
	tree, err := adapter.GetSessionTree(rootSession.SessionID)
	if err != nil {
		t.Fatalf("GetSessionTree failed for root: %v", err)
	}
	if tree.Root.SessionID != rootSession.SessionID {
		t.Errorf("Expected root session ID '%s', got '%s'", rootSession.SessionID, tree.Root.SessionID)
	}
	if tree.Parent != nil {
		t.Error("Expected nil parent for root session")
	}
	if len(tree.Children) != 1 {
		t.Errorf("Expected 1 child, got %d", len(tree.Children))
	}
	if tree.Depth != 0 {
		t.Errorf("Expected depth 0 for root, got %d", tree.Depth)
	}

	// Test 2: Get tree for child session (depth 1)
	tree, err = adapter.GetSessionTree(childSession.SessionID)
	if err != nil {
		t.Fatalf("GetSessionTree failed for child: %v", err)
	}
	if tree.Depth != 1 {
		t.Errorf("Expected depth 1 for child, got %d", tree.Depth)
	}
	if tree.Parent == nil {
		t.Fatal("Expected parent for child session")
	}
	if tree.Parent.SessionID != rootSession.SessionID {
		t.Errorf("Expected parent ID '%s', got '%s'", rootSession.SessionID, tree.Parent.SessionID)
	}
	if len(tree.Children) != 1 {
		t.Errorf("Expected 1 child (grandchild), got %d", len(tree.Children))
	}

	// Test 3: Get tree for grandchild session (depth 2)
	tree, err = adapter.GetSessionTree(grandchildSession.SessionID)
	if err != nil {
		t.Fatalf("GetSessionTree failed for grandchild: %v", err)
	}
	if tree.Depth != 2 {
		t.Errorf("Expected depth 2 for grandchild, got %d", tree.Depth)
	}
	if tree.Parent == nil {
		t.Fatal("Expected parent for grandchild session")
	}
	if tree.Parent.SessionID != childSession.SessionID {
		t.Errorf("Expected parent ID '%s', got '%s'", childSession.SessionID, tree.Parent.SessionID)
	}
	if len(tree.Children) != 0 {
		t.Errorf("Expected 0 children for grandchild, got %d", len(tree.Children))
	}

	// Test 4: Non-existent session
	_, err = adapter.GetSessionTree("non-existent-session")
	if err == nil {
		t.Error("Expected error for non-existent session")
	}

	// Test 5: Empty session ID
	_, err = adapter.GetSessionTree("")
	if err == nil {
		t.Error("Expected error for empty session ID")
	}
}

func TestCircularReferenceDetection(t *testing.T) {
	adapter := getTestAdapter(t)
	defer adapter.Close()

	timestamp := time.Now().Format("20060102-150405")

	// Create session1
	session1 := &manifest.Manifest{
		SessionID:     "test-circular1-" + timestamp,
		Name:          "Circular 1",
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Harness:       "claude-code",
		Context: manifest.Context{
			Project: "/test/circular1",
		},
		Claude: manifest.Claude{
			UUID: "circular1-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: "circular1-tmux",
		},
	}

	if err := adapter.CreateSession(session1); err != nil {
		t.Fatalf("Failed to create session1: %v", err)
	}
	defer adapter.DeleteSession(session1.SessionID)

	// Create session2 with session1 as parent
	session2 := &manifest.Manifest{
		SessionID:       "test-circular2-" + timestamp,
		Name:            "Circular 2",
		ParentSessionID: &session1.SessionID,
		SchemaVersion:   "2.0",
		CreatedAt:       time.Now().Add(1 * time.Second),
		UpdatedAt:       time.Now().Add(1 * time.Second),
		Harness:         "claude-code",
		Context: manifest.Context{
			Project: "/test/circular2",
		},
		Claude: manifest.Claude{
			UUID: "circular2-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: "circular2-tmux",
		},
	}

	if err := adapter.CreateSession(session2); err != nil {
		t.Fatalf("Failed to create session2: %v", err)
	}
	defer adapter.DeleteSession(session2.SessionID)

	// Manually create a circular reference by updating session1 to have session2 as parent
	// This requires direct SQL UPDATE since CreateSession/UpdateSession should prevent this
	query := `UPDATE agm_sessions SET parent_session_id = ? WHERE id = ? AND workspace = ?`
	_, err := adapter.conn.Exec(query, session2.SessionID, session1.SessionID, adapter.workspace)
	if err != nil {
		t.Fatalf("Failed to create circular reference: %v", err)
	}

	// GetSessionTree should detect the circular reference
	_, err = adapter.GetSessionTree(session1.SessionID)
	if err == nil {
		t.Error("Expected error for circular reference")
	}
	if err != nil && err.Error() != "circular reference detected in session hierarchy: "+session1.SessionID {
		t.Errorf("Expected circular reference error, got: %v", err)
	}

	// Also test from session2's perspective
	_, err = adapter.GetSessionTree(session2.SessionID)
	if err == nil {
		t.Error("Expected error for circular reference from session2")
	}
}

func TestOrphanedChildSession(t *testing.T) {
	adapter := getTestAdapter(t)
	defer adapter.Close()

	timestamp := time.Now().Format("20060102-150405")

	// Create parent session
	parentSession := &manifest.Manifest{
		SessionID:     "test-orphan-parent-" + timestamp,
		Name:          "Orphan Parent",
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Harness:       "claude-code",
		Context: manifest.Context{
			Project: "/test/orphan/parent",
		},
		Claude: manifest.Claude{
			UUID: "orphan-parent-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: "orphan-parent-tmux",
		},
	}

	if err := adapter.CreateSession(parentSession); err != nil {
		t.Fatalf("Failed to create parent session: %v", err)
	}

	// Create child session
	childSession := &manifest.Manifest{
		SessionID:       "test-orphan-child-" + timestamp,
		Name:            "Orphan Child",
		ParentSessionID: &parentSession.SessionID,
		SchemaVersion:   "2.0",
		CreatedAt:       time.Now().Add(1 * time.Second),
		UpdatedAt:       time.Now().Add(1 * time.Second),
		Harness:         "claude-code",
		Context: manifest.Context{
			Project: "/test/orphan/child",
		},
		Claude: manifest.Claude{
			UUID: "orphan-child-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: "orphan-child-tmux",
		},
	}

	if err := adapter.CreateSession(childSession); err != nil {
		t.Fatalf("Failed to create child session: %v", err)
	}
	defer adapter.DeleteSession(childSession.SessionID)

	// Delete parent session (orphans the child)
	if err := adapter.DeleteSession(parentSession.SessionID); err != nil {
		t.Fatalf("Failed to delete parent session: %v", err)
	}

	// GetParent should return nil for orphaned child (parent was deleted)
	parent, err := adapter.GetParent(childSession.SessionID)
	if err != nil {
		t.Fatalf("GetParent failed for orphaned child: %v", err)
	}
	if parent != nil {
		t.Error("Expected nil parent for orphaned child")
	}

	// GetSessionTree should still work for orphaned child
	tree, err := adapter.GetSessionTree(childSession.SessionID)
	if err != nil {
		t.Fatalf("GetSessionTree failed for orphaned child: %v", err)
	}
	if tree.Parent != nil {
		t.Error("Expected nil parent in tree for orphaned child")
	}
	if tree.Depth != 0 {
		t.Errorf("Expected depth 0 for orphaned child, got %d", tree.Depth)
	}
}
